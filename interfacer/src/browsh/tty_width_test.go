package browsh

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gdamore/tcell"
	"github.com/mattn/go-runewidth"
	"github.com/spf13/viper"
)

func widthTestFrame(text []string) frame {
	var f frame
	meta := jsonFrameBase{SubWidth: len(text), SubHeight: 2, TotalWidth: len(text), TotalHeight: 2}
	f.buildFrameText(incomingFrameText{Meta: meta, Text: text, Colours: make([]int32, len(text)*3)})
	pixels := make([]int32, len(text)*6)
	for x := range text {
		pixels[x*3] = int32(x + 1)
		pixels[len(text)*3+x*3+2] = int32(x + 1)
	}
	f.buildFramePixels(incomingFramePixels{Meta: meta, Colours: pixels})
	return f
}

func widthTestScreen(t *testing.T, f frame, width int) tcell.SimulationScreen {
	t.Helper()
	oldScreen, oldTab, oldInput, oldMono := screen, CurrentTab, activeInputBox, IsMonochromeMode
	oldSupport := viper.Get("browsh_supporter")
	s := tcell.NewSimulationScreen("UTF-8")
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	s.SetSize(width, 6)
	screen, CurrentTab, activeInputBox, IsMonochromeMode = s, &tab{frame: f}, nil, false
	viper.Set("browsh_supporter", "I have shown my support for Browsh")
	t.Cleanup(func() {
		s.Fini()
		screen, CurrentTab, activeInputBox, IsMonochromeMode = oldScreen, oldTab, oldInput, oldMono
		viper.Set("browsh_supporter", oldSupport)
	})
	return s
}

func TestCJKWebExtensionFrameCoordinates(t *testing.T) {
	data, err := os.ReadFile("testdata/cjk_frame.json")
	if err != nil {
		t.Fatal(err)
	}
	var incoming incomingFrameText
	if err := json.Unmarshal(data, &incoming); err != nil {
		t.Fatal(err)
	}
	var f frame
	f.buildFrameText(incoming)
	for x, want := range []rune("一二三四五六") {
		c, ok := f.cells.load(x)
		if !ok || len(c.character) != 1 || c.character[0] != want {
			t.Fatalf("source index=%d x=%d: got %q, want %c", x, x, string(c.character), want)
		}
		t.Logf("source index=%d x=%d character=%c", x, x, want)
	}
}

func TestTTYDisplayWidths(t *testing.T) {
	for _, tc := range []struct {
		text  string
		width int
	}{
		{"ABCDEF", 6}, {"中文测试", 8}, {"A中B文", 6},
		{"一二三四五六七八九十", 20}, {"ABC中文123测试XYZ", 17},
		{"中文，测试。", 12}, {"《中文测试》", 12}, {"（测试）", 8},
		{"あいう한글", 10}, {"e\u0301B", 2}, {"中 文", 5},
	} {
		t.Run(tc.text, func(t *testing.T) {
			if got := runewidth.StringWidth(tc.text); got != tc.width {
				t.Fatalf("width=%d want %d", got, tc.width)
			}
			text := make([]string, 40)
			for i, r := range []rune(tc.text) {
				text[i] = string(r)
			}
			s := widthTestScreen(t, widthTestFrame(text), 40)
			renderCurrentTabWindow()
			front, w, _ := s.GetContents()
			x, previous := 0, -1
			for _, r := range []rune(tc.text) {
				rw := runewidth.RuneWidth(r)
				if rw == 0 {
					_, combining, _, _ := s.GetContent(previous, uiHeight)
					if len(combining) == 0 || combining[len(combining)-1] != r {
						t.Errorf("lost combining rune %U", r)
					}
					continue
				}
				if r != ' ' {
					got := front[uiHeight*w+x].Runes
					if len(got) == 0 || got[0] != r {
						t.Errorf("physical column %d: got %q want %c", x, string(got), r)
					}
				}
				previous, x = x, x+rw
			}
			// Graphics beyond the text stay anchored, with both half-block colours.
			glyph, _, style, _ := s.GetContent(30, uiHeight)
			fg, bg, _ := style.Decompose()
			if glyph != '▄' || fg != tcell.NewRGBColor(0, 0, 31) || bg != tcell.NewRGBColor(31, 0, 0) {
				t.Error("background graphics moved or changed")
			}
		})
	}
}

func TestTTYSpacedWideCellsAndRepaint(t *testing.T) {
	s := widthTestScreen(t, widthTestFrame([]string{"中", "", "文", "", "", "", "A", ""}), 8)
	renderCurrentTabWindow()
	for x, want := range map[int]rune{0: '中', 2: '文', 6: 'A'} {
		got, _, _, _ := s.GetContent(x, uiHeight)
		if got != want {
			t.Errorf("column %d=%c want %c (do not expand pre-spaced cells twice)", x, got, want)
		}
	}
	CurrentTab.frame = widthTestFrame([]string{"A", "B", "", "", "", "", "", ""})
	renderCurrentTabWindow()
	for x, want := range map[int]rune{0: 'A', 1: 'B', 2: '▄', 3: '▄'} {
		got, _, _, _ := s.GetContent(x, uiHeight)
		if got != want {
			t.Errorf("stale wide glyph at %d: %c", x, got)
		}
	}
}

func TestTTYWideRightEdge(t *testing.T) {
	s := widthTestScreen(t, widthTestFrame([]string{"A", "中"}), 2)
	renderCurrentTabWindow()
	_, _, _, width := s.GetContent(1, uiHeight)
	if width != 1 {
		t.Error("wide glyph written across right edge")
	}
}

func TestTTYWideHorizontalScroll(t *testing.T) {
	f := widthTestFrame([]string{"中", "文", "A", "", "", ""})
	f.xScroll = 2
	s := widthTestScreen(t, f, 4)
	renderCurrentTabWindow()
	for x, want := range map[int]rune{0: '文', 2: 'A'} {
		got, _, _, _ := s.GetContent(x, uiHeight)
		if got != want {
			t.Errorf("scrolled column %d=%c want %c", x, got, want)
		}
	}
}

func TestTTYCombiningClusterAndLeftClip(t *testing.T) {
	f := widthTestFrame([]string{"中", "e\u0301", "文", "", "", ""})
	f.xScroll = 1
	s := widthTestScreen(t, f, 5)
	renderCurrentTabWindow()
	main, combining, _, _ := s.GetContent(1, uiHeight)
	if main != 'e' || string(combining) != "\u0301" {
		t.Errorf("cluster lost: %c %q", main, string(combining))
	}
	main, _, _, width := s.GetContent(0, uiHeight)
	if main != '▄' || width != 1 {
		t.Error("partially visible wide glyph was not clipped")
	}
}

func TestTTYMonochromeDoesNotMutateFrame(t *testing.T) {
	f := widthTestFrame([]string{"中", "文", "", "", ""})
	s := widthTestScreen(t, f, 5)
	IsMonochromeMode = true
	renderCurrentTabWindow()
	main, _, style, _ := s.GetContent(2, uiHeight)
	fg, bg, _ := style.Decompose()
	if main != '文' || fg != tcell.ColorWhite || bg != tcell.ColorBlack {
		t.Error("monochrome text failed")
	}
	c, _ := CurrentTab.frame.cells.load(4)
	if string(c.character) != "▄" {
		t.Error("monochrome mutated source graphics")
	}
	IsMonochromeMode = false
	renderCurrentTabWindow()
	main, _, style, _ = s.GetContent(4, uiHeight)
	fg, bg, _ = style.Decompose()
	if main != '▄' || fg != tcell.NewRGBColor(0, 0, 5) || bg != tcell.NewRGBColor(5, 0, 0) {
		t.Error("colour graphics not restored")
	}
}

func TestTTYInputOverlayKeepsEditorCoordinates(t *testing.T) {
	f := widthTestFrame([]string{"中", "文", "测", "试", "", "", "", ""})
	s := widthTestScreen(t, f, 8)
	input := &inputBox{X: 4, Y: 0, Width: 2, FgColour: [3]int32{255, 255, 255}}
	input.addCharacterToFrame(4, 0, 'A')
	input.addCharacterToFrame(5, 0, 'B')
	renderCurrentTabWindow()
	front, width, _ := s.GetContents()
	for x, want := range map[int]rune{4: 'A', 5: 'B'} {
		got := front[uiHeight*width+x].Runes
		if len(got) == 0 || got[0] != want {
			t.Errorf("input overlay moved/covered at %d: %q", x, string(got))
		}
	}
	x, y := input.getCoordsOfIndex(1)
	if x != 5 || y != uiHeight {
		t.Errorf("input cursor no longer aligned: %d,%d", x, y)
	}
}
