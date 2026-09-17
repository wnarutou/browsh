package browsh

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type addonLogWriter chan string

func (w addonLogWriter) Write(p []byte) (int, error) {
	w <- string(p)
	return len(p), nil
}

// Exercise the real extraction and Marionette wire request without Firefox.
func TestInstallWebextension(t *testing.T) {
	oldConn, oldCount, oldLogger := marionette, ffCommandCount, slog.Default()
	defer func() {
		marionette, ffCommandCount = oldConn, oldCount
		slog.SetDefault(oldLogger)
		viper.Reset()
	}()
	t.Setenv("TMPDIR", t.TempDir())
	t.Setenv("TMP", t.TempDir())
	t.Setenv("TEMP", t.TempDir())
	for _, tc := range []struct {
		name, config string
		temporary    bool
	}{
		{"default", "[firefox]\n", false},
		{"enabled", "[firefox]\ntemporary-addon = true\n", true},
		{"disabled", "[firefox]\ntemporary-addon = false\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			viper.Reset()
			viper.SetConfigType("toml")
			if err := viper.ReadConfig(strings.NewReader(tc.config)); err != nil {
				t.Fatal(err)
			}
			client, server := net.Pipe()
			defer client.Close()
			defer server.Close()
			client.SetDeadline(time.Now().Add(5 * time.Second))
			server.SetDeadline(time.Now().Add(5 * time.Second))
			marionette = client
			logs := make(addonLogWriter, 16)
			slog.SetDefault(slog.New(slog.NewTextHandler(logs, nil)))
			done := make(chan struct{})
			go func() { installWebextension(); close(done) }()
			r := bufio.NewReader(server)
			prefix, err := r.ReadString(':')
			if err != nil {
				t.Fatal(err)
			}
			n, err := strconv.Atoi(strings.TrimSuffix(prefix, ":"))
			if err != nil {
				t.Fatal(err)
			}
			payload := make([]byte, n)
			if _, err := io.ReadFull(r, payload); err != nil {
				t.Fatal(err)
			}
			<-done
			var command []json.RawMessage
			if err := json.Unmarshal(payload, &command); err != nil {
				t.Fatal(err)
			}
			if len(command) != 4 || string(command[2]) != `"Addon:Install"` {
				t.Fatalf("unexpected request: %s", payload)
			}
			var args map[string]interface{}
			if err := json.Unmarshal(command[3], &args); err != nil {
				t.Fatal(err)
			}
			if tc.temporary {
				if args["temporary"] != true {
					t.Errorf("temporary=true missing: %s", payload)
				}
			} else if _, exists := args["temporary"]; exists {
				t.Errorf("default request changed: %s", payload)
			}
			data, err := os.ReadFile(args["path"].(string))
			if err != nil {
				t.Fatal(err)
			}
			embedded, err := browshXpi.ReadFile("browsh.xpi")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(data, embedded) {
				t.Error("extracted XPI differs from embedded XPI")
			}
			// The original response must reach debug logging without filtering.
			response := fmt.Sprintf(`[1,%s,{"error":"unknown error","message":"ERROR_CORRUPT_FILE UNKNOWN_ERROR AddonInstallException","stacktrace":"original stack"},null]`, command[1])
			wire := fmt.Sprintf("%d:%s", len(response), response)
			if _, err := io.WriteString(server, wire); err != nil {
				t.Fatal(err)
			}
			for {
				select {
				case line := <-logs:
					if strings.Contains(line, "FF-MRNT") {
						for _, text := range []string{"ERROR_CORRUPT_FILE", "UNKNOWN_ERROR", "AddonInstallException", "original stack"} {
							if !strings.Contains(line, text) {
								t.Errorf("lost Firefox error: %s", line)
							}
						}
						return
					}
				case <-time.After(5 * time.Second):
					t.Fatal("Marionette response not logged")
				}
			}
		})
	}
}

func TestTemporaryAddonFlag(t *testing.T) {
	flag := pflag.Lookup("firefox.temporary-addon")
	if flag == nil {
		t.Fatal("missing --firefox.temporary-addon")
	}
	if flag.DefValue != "false" {
		t.Fatalf("default = %s, want false", flag.DefValue)
	}
	oldValue, oldChanged := flag.Value.String(), flag.Changed
	defer func() {
		flag.Value.Set(oldValue)
		flag.Changed = oldChanged
		viper.Reset()
	}()
	for _, value := range []string{"true", "false"} {
		viper.Reset()
		viper.SetConfigType("toml")
		if err := viper.ReadConfig(strings.NewReader("[firefox]\ntemporary-addon = true\n")); err != nil {
			t.Fatal(err)
		}
		if err := pflag.CommandLine.Set(flag.Name, value); err != nil {
			t.Fatal(err)
		}
		if err := viper.BindPFlags(pflag.CommandLine); err != nil {
			t.Fatal(err)
		}
		if got := viper.GetBool(flag.Name); got != (value == "true") {
			t.Fatalf("CLI %s did not override config: %v", value, got)
		}
	}
}
