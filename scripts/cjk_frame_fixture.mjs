// Run from the repository root: node scripts/cjk_frame_fixture.mjs
// Exercise the actual grid positioning and serialization methods with a
// controlled DOM rectangle. No browser, npm dependencies or XPI edits needed.
import fs from "node:fs";
import vm from "node:vm";
import assert from "node:assert/strict";

const scope = vm.createContext({
  TEST: true,
  _: { clone: (value) => ({ ...value }) },
  utils: {
    snap: Math.round,
    mixins: (...mixins) => mixins.reduce((base, mixin) => mixin(base), class {}),
  },
  CommonMixin: (base) => base,
  window: { scrollX: 0, scrollY: 0 },
});
for (const [name, file] of [
  ["TTYCell", "tty_cell.js"],
  ["TTYGrid", "tty_grid.js"],
  ["SerialiseMixin", "serialise_mixin.js"],
  ["TextBuilder", "text_builder.js"],
]) {
  const code = fs.readFileSync(`webext/src/dom/${file}`, "utf8")
    .replace(/^import .*;\r?\n/gm, "")
    .replace("export default ", "").trim().replace(/;$/, "");
  scope[name] = vm.runInContext(`(${code}\n)`, scope);
}
const width = 16;
const dimensions = {
  char: { width: 8 },
  scale_factor: { width: 1 / 8, height: 1 / 8 },
  frame: { width, sub: { top: 0, left: 0, width, height: 2 } },
  getFrameMeta: () => ({ sub_left: 0, sub_top: 0, sub_width: width,
    sub_height: 2, total_width: width, total_height: 2 }),
};
const grid = new scope.TTYGrid(dimensions, {
  getUnscaledFGPixelAt: () => [255, 255, 255],
  getUnscaledBGPixelAt: () => [0, 0, 0],
}, { browsh: { use_experimental_text_visibility: false } });
grid.cells = [];
const builder = Object.create(scope.TextBuilder.prototype);
Object.assign(builder, {
  dimensions, tty_grid: grid, channel: { name: "1" },
  _text: "一二三四五六", _character_index: 0,
  _node: { parentElement: {} }, _previous_dom_box: {},
  _dom_box: { left: 0, top: 0, width: 12 * 8 },
  _getAllInputBoxes: () => ({}),
});
builder._handleSingleDOMBox();
builder.__serialiseFrame();
const text = Array.from(builder.frame.text);
assert.deepEqual(text.slice(0, 6), Array.from("一二三四五六"));
assert.ok(text.slice(6).every((value) => value === ""));
for (let x = 0; x < 6; x++) {
  assert.equal(grid.cells[x].tty_coords.x, x);
  console.log(`index=${x} x=${x} text=${text[x]}`);
}
const output = "interfacer/src/browsh/testdata/cjk_frame.json";
fs.mkdirSync("interfacer/src/browsh/testdata", { recursive: true });
fs.writeFileSync(output, JSON.stringify(builder.frame, null, 2) + "\n");
console.log(`Wrote ${output}: model A, no CJK continuation cells`);
