package browsh

import (
	"log/slog"
	"time"
)

var firefoxConsoleReady = make(chan struct{}, 1)

// Register before Addon:Install: startup errors otherwise happen before the
// extension can forward its own logs over the WebSocket. Output uses Firefox's
// existing stdout pipe, not the Marionette response reader's small buffer.
func setupFirefoxDebugLogging() {
	if !*isDebug {
		return
	}
	sendFirefoxCommand("Marionette:SetContext", map[string]interface{}{"value": "chrome"})
	sendFirefoxCommand("WebDriver:ExecuteScript", map[string]interface{}{
		"script": firefoxDebugScript,
		"args":   []interface{}{},
	})
	// Wait for the stdout marker before changing context or installing the addon.
	// Marionette command writes alone do not confirm script execution.
	select {
	case <-firefoxConsoleReady:
		slog.Info("Firefox console diagnostics ready")
	case <-time.After(5 * time.Second):
		slog.Warn("Firefox console diagnostics not confirmed; check FF-MRNT errors")
	}
	sendFirefoxCommand("Marionette:SetContext", map[string]interface{}{"value": "content"})
}

const firefoxDebugScript = `
var Services = ChromeUtils.import("resource://gre/modules/Services.jsm").Services;
var win = Services.appShell.hiddenDOMWindow;
Services.prefs.setBoolPref("browser.dom.window.dump.enabled", true);
function emit(kind, text) {
  // Keep lines within the Go stdout scanner limit without discarding content.
  var value = JSON.stringify(String(text));
  for (var i = 0; i < value.length; i += 2000) {
    win.dump("BROWSH-FIREFOX-" + kind + " " + value.slice(i, i + 2000) + "\n");
  }
}
var listener = {
  observe: function(message) {
    var text = message.message;
    if (/moz-extension:|WebExtension|ExtensionError|SyntaxError|ReferenceError|TypeError|WebSocket/i.test(text)) {
      emit("CONSOLE", text);
    }
  }
};
Services.console.registerListener(listener);
win.addEventListener("unload", function() {
  Services.console.unregisterListener(listener);
}, {once: true});
Services.console.getMessageArray().forEach(function(message) {
  listener.observe(message);
});
emit("DIAGNOSTICS", "Console listener ready before addon installation");
win.setTimeout(function() {
  var AddonManager = ChromeUtils.import("resource://gre/modules/AddonManager.jsm").AddonManager;
  AddonManager.getAddonByID("{8ff2d753-2dc8-46de-a837-fa28331d9fcf}").then(function(addon) {
    emit("ADDON-STATE", JSON.stringify(addon ? {
      id: addon.id, version: addon.version, isActive: addon.isActive,
      appDisabled: addon.appDisabled, userDisabled: addon.userDisabled,
      isCompatible: addon.isCompatible, temporarilyInstalled: addon.temporarilyInstalled
    } : {found: false}));
  }).catch(function(error) { emit("DIAGNOSTICS", String(error)); });
}, 10000);
return "Browsh Firefox console diagnostics enabled";
`
