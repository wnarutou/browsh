# TTY CJK display width

## Data model and cause

`webext/src/dom/text_builder.js` advances `_tty_tracker.x` by one in
`_stepToNextCharacter`, independently of display width. `TTYGrid._calculateIndex`
stores each character at `y * frame.width + x`; `SerialiseMixin.__serialiseFrame`
sends one string for each grid position. The official 1.8.2 embedded XPI contains
the same stepping logic. No extension code or XPI was changed for this fix.

Reproduce the data-model check with:

```sh
node scripts/cjk_frame_fixture.mjs
```

This executes the actual positioning, grid and serialization methods with a
controlled DOM rectangle and mocked DOM/pixel services, without npm dependencies.
It writes `interfacer/src/browsh/testdata/cjk_frame.json`. This is a controlled
source-method test, not a capture from the user's live Firefox page.

`TestCJKWebExtensionFrameCoordinates` parses that generated JSON through the Go
frame builder and checks the actual stored cells:

| Character | Source x / cell index (row 0) | Terminal x after fix |
|---|---:|---:|
| 一 | 0 | 0 |
| 二 | 1 | 2 |
| 三 | 2 | 4 |
| 四 | 3 | 6 |
| 五 | 4 | 8 |
| 六 | 5 | 10 |

`frame.populateFrameText` and `frame.buildCell` preserve these source indexes.
No continuation cells are inserted. Thus this is model A for continuous text.

Previously `renderCurrentTabWindow` called SetCell at each source x. tcell 1.4
stores all those runes but its physical draw loop advances by the rune's display
width. A wide rune at x=0 makes it skip x=1, then draw x=2 and skip x=3. A real
tcell SimulationScreen reproduces the odd-character-only output. This is more
precise than saying every subsequent SetCell necessarily erases the wide glyph:
the following source character occupies what tcell treats as its continuation.

## Rendering change

The TTY page renderer paints the original half-block pixel
background first, then overlays text using separate source and terminal columns.
`github.com/mattn/go-runewidth` v0.0.15 is promoted from indirect to direct use;
its version and go.sum are unchanged. `RuneWidth` gives 1 for ASCII, 2 for the
tested CJK/fullwidth characters and 0 for combining marks. No byte count or rune
count is used as a display width.

Contiguous text consumes its display width. Combining marks are attached through
tcell.SetContent to the preceding visible base rune, including marks supplied
in a separate source cell. Standalone marks and controls do not consume output
columns. Explicit spaces retain their width. Empty source cells break text runs;
the next run starts at the later of its source x and the prior text's output end.
Consequently pre-spaced wide text at source x=0,2,4 is not expanded twice, and
sufficient gaps preserve subsequent ASCII anchors. Background painting finishes
before text, so no later graphics write covers a wide continuation column.

Graphics remain at their original coordinates; text takes foreground colour
from its source and background colour from its destination. ASCII rows retain
their coordinates. A double-width glyph necessarily occupies two physical
columns and can hide graphics under those columns; remaining graphics are not
shifted. Full preservation of separate DOM boxes that overlap after expansion
would require layout/run information absent from this protocol, and is outside
this renderer-only fix. Input hit-testing, input cursor calculation, URL/status
bar rendering and full emoji grapheme shaping are not redesigned here.
Input-overlay cells retain their existing editor coordinates; expanded page
text is prevented from covering those cells. Existing editor-specific CJK
limitations are not claimed fixed by this page-rendering change.

The original text and pixel foreground are carried in the existing locked cell
store, so rendering never reads the receiver's mutable raw text/pixel maps.

Horizontal scrolling uses the same mapping from the source row origin. Glyphs
cut by either viewport boundary are omitted instead of writing a partial wide
glyph. Background repaint clears old wide text when a later frame is narrower.

## Verification

```sh
cd interfacer
go test -mod=readonly ./src/browsh -count=1 -v
go vet ./src/browsh
```

Tests cover ABCDEF (6), 中文测试 (8), A中B文 (6), all ten characters of
一二三四五六七八九十 (20), ABC中文123测试XYZ, CJK punctuation, Japanese/Korean,
combining marks in one or multiple source cells, explicit spaces, pre-spaced
wide cells, anchored graphics/colours, repaint, monochrome, horizontal scroll
and clipped wide characters at either edge. Assertions inspect tcell's physical
simulated output, not just the source buffer.

Linux ARM64 build uses the existing official XPI and entry point:

```sh
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build -mod=readonly -trimpath \
  -ldflags '-s -w' -o ../dist/browsh-1.8.2-cjk-arm64/browsh ./cmd/browsh
```

Local validation is on Windows amd64. Real SSH terminal/font and Kylin/Firefox79
page rendering still require a target-machine check; the package includes a
small UTF-8 test page for that purpose.
