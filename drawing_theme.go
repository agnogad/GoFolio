package main

import (
	"github.com/egoist/mygo/ui"

	"gofolio/internal/editor"
)

// monoDrawPalette maps godraw's tldraw palette onto GoFolio's monochrome
// look. The Swatches keep their KEYS (so shapes load and style with the same
// color names), but every value is a neutral gray, white or black: no hue
// anywhere. The chrome colors (selection, snaps, grid, brushes) are muted
// grays from the theme.
func monoDrawPalette(t *ui.Theme, dark bool) *editor.Palette {
	p := *editor.Light
	if dark {
		p = *editor.Dark
	}
	p.Dark = dark

	// Chrome, from GoFolio's theme so the canvas sits in the same light as
	// the rest of the app.
	p.Background = toED(t.Background)
	p.Text = toED(t.Text)
	p.Cursor = toED(t.Text)
	p.Solid = toED(t.Background)
	p.Selection = toED(t.Text)
	p.SelFill = toED(t.Selection)
	p.Snap = toED(t.TextMuted)
	p.Grid = toED(t.Border)
	p.BrushFill = toED(t.Text.Alpha(0.10))
	p.BrushLine = toED(t.Text.Alpha(0.25))
	p.Laser = toED(t.TextMuted)

	// Grays the swatches climb along: black-ish for the darkest names,
	// white-ish for the lightest. Each swatch's Solid is what a stroke
	// shows; Semi and Fill are its faint forms.
	solid := func(lum float64) editor.Color {
		return grayOf(lum, dark, 1)
	}
	semi := func(lum float64) editor.Color { return grayOf(lum, dark, 0.14) }
	pattern := func(lum float64) editor.Color { return grayOf(lum, dark, 0.45) }
	fill := func(lum float64) editor.Color { return grayOf(lum, dark, 0.20) }
	lined := func(lum float64) editor.Color { return grayOf(lum, dark, 0.35) }

	// The named colors, dark names first, so the panel reads as a ramp even
	// though the hues are gone. Values are lightness in the light theme;
	// dark theme mirrors them so dark shapes read on a dark canvas.
	swatches := map[string]editor.Swatch{
		"black":        monoSwatch(t, dark, 0.05, solid, semi, pattern, fill, lined),
		"grey":         monoSwatch(t, dark, 0.55, solid, semi, pattern, fill, lined),
		"light-violet": monoSwatch(t, dark, 0.72, solid, semi, pattern, fill, lined),
		"violet":       monoSwatch(t, dark, 0.36, solid, semi, pattern, fill, lined),
		"blue":         monoSwatch(t, dark, 0.40, solid, semi, pattern, fill, lined),
		"light-blue":   monoSwatch(t, dark, 0.70, solid, semi, pattern, fill, lined),
		"yellow":       monoSwatch(t, dark, 0.82, solid, semi, pattern, fill, lined),
		"orange":       monoSwatch(t, dark, 0.62, solid, semi, pattern, fill, lined),
		"green":        monoSwatch(t, dark, 0.42, solid, semi, pattern, fill, lined),
		"light-green":  monoSwatch(t, dark, 0.74, solid, semi, pattern, fill, lined),
		"light-red":    monoSwatch(t, dark, 0.68, solid, semi, pattern, fill, lined),
		"red":          monoSwatch(t, dark, 0.33, solid, semi, pattern, fill, lined),
		"white":        monoSwatch(t, dark, 0.98, solid, semi, pattern, fill, lined),
	}
	p.Swatches = swatches
	return &p
}

func monoSwatch(t *ui.Theme, dark bool, lum float64, solid, semi, pattern, fill, lined func(float64) editor.Color) editor.Swatch {
	// In a dark theme strokes sit on a dark canvas, so the ramp inverts:
	// a name that is near-black on paper is near-white here, and vice versa.
	l := lum
	if dark {
		l = 1 - lum
	}
	hl := l
	return editor.Swatch{
		Solid:       solid(l),
		Semi:        semi(l),
		Pattern:     pattern(l),
		Fill:        fill(l),
		Lined:       lined(l),
		NoteFill:    solid(l),
		NoteText:    noteTextOf(t, l),
		FrameStroke: semi(l),
		FrameFill:   grayOf(l, dark, 0.10),
		FrameText:   toED(t.TextMuted),
		Highlight:   grayOf(hl, dark, 0.30),
	}
}

// grayOf returns a neutral gray of the given lightness (0 black, 1 white),
// with alpha a. It is a pure gray: R = G = B, no hue at all.
func grayOf(lum float64, dark bool, a float64) editor.Color {
	if lum < 0 {
		lum = 0
	}
	if lum > 1 {
		lum = 1
	}
	v := uint8(lum*255 + 0.5)
	// Solid strokes need to read against a same-ish canvas in either
	// appearance, so lift near-invisible values a touch toward the theme's
	// own text/background contrast.
	if a >= 1 {
		if dark && v < 24 {
			v = 24
		}
		if !dark && v > 231 {
			v = 231
		}
	}
	return editor.Color{R: v, G: v, B: v, A: a}
}

// noteTextOf picks label text for a note whose fill is the gray of l, so a
// near-black note gets light text and a near-white note gets dark text.
func noteTextOf(t *ui.Theme, l float64) editor.Color {
	if l < 0.5 {
		return toED(t.Background)
	}
	return toED(t.Text)
}

// toED converts a theme color to the editor's Color.
func toED(c ui.Color) editor.Color {
	return editor.Color{R: c.R, G: c.G, B: c.B, A: float64(c.A) / 255}
}
