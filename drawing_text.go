package main

import (
	"strings"
	"sync"
	"unicode/utf16"

	"github.com/egoist/mygo/ui"

	"gofolio/internal/assets"
	"gofolio/internal/editor"
)

// drawFamily is the hand-drawn font godraw's "draw" style uses.
const drawFamily = "Shantell Sans"

var registerDrawFonts = sync.OnceFunc(func() {
	if err := ui.RegisterFont(assets.DrawFont, drawFamily); err != nil {
		panic(err)
	}
})

// drawFamilyOf returns the font family of a font style.
func drawFamilyOf(font string) string {
	switch font {
	case "draw":
		return drawFamily
	case "serif":
		return "Georgia, serif"
	case "mono":
		return "monospace"
	}
	return "system-ui"
}

type drawMeasureKey struct {
	text, font  string
	size, width float32
}

// drawMeasurer lays out the text of shapes, keeping what it laid out: a
// shape's text is measured for its geometry and its drawing alike.
type drawMeasurer struct {
	cache map[drawMeasureKey]editor.TextBlock
}

func newDrawMeasurer() *drawMeasurer {
	registerDrawFonts()
	return &drawMeasurer{cache: map[drawMeasureKey]editor.TextBlock{}}
}

func (m *drawMeasurer) measure(text, font string, size, width float64) editor.TextBlock {
	k := drawMeasureKey{text, font, float32(size), float32(width)}
	if b, ok := m.cache[k]; ok {
		return b
	}
	if len(m.cache) > 4096 {
		clear(m.cache)
	}
	b := layoutDrawBlock(text, font, size, width)
	m.cache[k] = b
	return b
}

func layoutDrawBlock(text, font string, size, width float64) editor.TextBlock {
	f := ui.Font{Family: drawFamilyOf(font), Size: float32(size), LineHeight: editor.LineHeight}
	l := ui.ShapeText(text, f, float32(width))
	line := size * editor.LineHeight
	units := utf16.Encode([]rune(text))
	lines := l.Lines()
	out := editor.TextBlock{Line: line}
	for i, ln := range lines {
		end := min(ln.End, len(units))
		start := min(ln.Start, end)
		s := string(utf16.Decode(units[start:end]))
		trimmed := strings.TrimRight(s, " \t\r\n")
		w := float64(ln.Rect.W)
		if trimmed != s {
			// Whitespace that ends a line takes no room when it aligns.
			c := l.Caret(start + len(utf16.Encode([]rune(trimmed))))
			w = float64(c.X - ln.Rect.X)
		}
		out.Lines = append(out.Lines, editor.TextLine{Text: trimmed, Y: float64(i) * line, W: w})
		if float64(ln.Rect.W) > out.W {
			out.W = float64(ln.Rect.W)
		}
	}
	if len(lines) == 0 {
		out.Lines = []editor.TextLine{{}}
	}
	out.H = float64(len(out.Lines)) * line
	return out
}

// naturalHeights caches the height a font's line takes, in its size.
var naturalHeights sync.Map

// naturalHeight returns the height a font's line takes, in its size, without
// a set line height: text draws in it, centered in its line.
func naturalHeight(font string) float32 {
	if v, ok := naturalHeights.Load(font); ok {
		return v.(float32)
	}
	registerDrawFonts()
	const probe = 100
	l := ui.ShapeText("Hg", ui.Font{Family: drawFamilyOf(font), Size: probe}, 0)
	_, h := l.Size()
	v := h / probe
	naturalHeights.Store(font, v)
	return v
}
