package main

import (
	"math"

	"github.com/egoist/mygo/ui"

	"gofolio/internal/doc"
	"gofolio/internal/editor"
	"gofolio/internal/geo"
)

// drawPreview draws a drawing as a picture w by h DIPs of its page fitted in
// it, at scale device pixels each: what a list shows of a drawing before it
// is open. It returns nil for a drawing with nothing on it.
func drawPreview(d *doc.Drawing, w, h int, scale float32, dark bool) *ui.Bitmap {
	if d == nil || w <= 0 || h <= 0 {
		return nil
	}
	e := editor.New(d, newDrawMeasurer().measure)
	e.DarkMode = dark
	box, ids, ok := e.ExportBounds()
	if !ok || box.W <= 0 || box.H <= 0 {
		return nil
	}
	z := math.Min(float64(w)/box.W, float64(h)/box.H)
	if z <= 0 || math.IsInf(z, 0) || math.IsNaN(z) {
		return nil
	}
	// The page's middle in the picture's middle.
	cam := editor.Camera{
		Z: z,
		X: float64(w)/(2*z) - (box.X + box.W/2),
		Y: float64(h)/(2*z) - (box.Y + box.H/2),
	}
	pal := monoDrawPalette(ui.LightTheme(), dark)
	if dark {
		pal = monoDrawPalette(ui.DarkTheme(), true)
	}
	a := &drawSurface{ed: e, bitmaps: map[doc.ID]*ui.Bitmap{}, decoded: map[doc.ID]string{}}
	img := ui.Render(func(c *ui.Context) {
		ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
			s := &scene{a: a, e: e, p: p, pal: pal, cam: cam, export: true}
			s.view = geo.Box{X: -cam.X, Y: -cam.Y, W: float64(w) / z, H: float64(h) / z}
			p.Fill(r, toUI(pal.Background), 0)
			for _, id := range ids {
				sh := e.Doc.Shapes[id]
				if sh == nil {
					continue
				}
				switch sh.Type {
				case doc.TypeGroup:
					s.layer(id, true)
					s.layer(id, false)
				case doc.TypeFrame:
					s.frame(sh)
				default:
					s.shape(sh)
				}
			}
		})
	}, w, h, scale)
	return ui.NewBitmap(img)
}
