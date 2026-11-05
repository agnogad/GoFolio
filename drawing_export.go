package main

import (
	"bytes"
	"image"
	"image/draw"
	"image/png"
	"math"
	"os"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"gofolio/internal/doc"
	"gofolio/internal/editor"
	"gofolio/internal/geo"
)

// RenderPNG draws what export draws (the selection, else the page) at a
// scale, and encodes it.
func (d *drawSurface) RenderPNG(scale float64) ([]byte, bool) {
	e := d.ed
	box, ids, ok := e.ExportBounds()
	if !ok {
		return nil, false
	}
	box = box.Expand(32)
	// Very large pages come out at a size a file can hold.
	const maxSide = 8192.0
	if k := maxSide / (math.Max(box.W, box.H) * scale); k < 1 {
		scale *= k
	}
	pal := d.paletteNoUI()
	// In tiles, each a frame with a mask atlas of its own: a whole page of
	// strokes wouldn't fit one. Tiles are whole device pixels apart, so that
	// nothing shows where they meet.
	const tile = 1024
	w, h := int(math.Ceil(box.W*scale)), int(math.Ceil(box.H*scale))
	out := image.NewRGBA(image.Rect(0, 0, w, h))
	side := int(math.Ceil(tile / scale))
	for ty := 0; ty < h; ty += tile {
		for tx := 0; tx < w; tx += tile {
			at := geo.V(box.X+float64(tx)/scale, box.Y+float64(ty)/scale)
			img := ui.Render(func(c *ui.Context) {
				ui.Box(c).Fill().Draw(func(p *ui.Painter, r ui.Rect) {
					s := &scene{a: d, e: e, p: p, pal: pal, export: true}
					s.cam = editor.Camera{X: -at.X, Y: -at.Y, Z: 1}
					s.view = geo.Box{X: at.X, Y: at.Y, W: float64(side), H: float64(side)}
					p.Fill(r, toUI(pal.Background), 0)
					for _, id := range ids {
						sh := e.Doc.Shapes[id]
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
			}, side, side, float32(scale))
			draw.Draw(out, image.Rect(tx, ty, tx+tile, ty+tile), img, image.Point{}, draw.Src)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return nil, false
	}
	return buf.Bytes(), true
}

func (d *drawSurface) exportName(ext string) string {
	name := d.ed.Doc.Name
	if name == "" {
		name = "drawing"
	}
	return name + ext
}

// ExportPNG asks where, and writes a PNG there.
func (d *drawSurface) ExportPNG() {
	data, ok := d.RenderPNG(2)
	if !ok {
		return
	}
	d.saveAs(data, d.exportName(".png"), "PNG image", "png")
}

// ExportSVG asks where, and writes an SVG there.
func (d *drawSurface) ExportSVG() {
	data, ok := d.ed.SVG(d.ed.DarkMode)
	if !ok {
		return
	}
	d.saveAs(data, d.exportName(".svg"), "SVG image", "svg")
}

// CopyPNG copies what export draws as an image.
func (d *drawSurface) CopyPNG() {
	data, ok := d.RenderPNG(2)
	if !ok || !d.hasSvc {
		return
	}
	d.services.WriteClipboardData(transfer.New(transfer.NewItem(transfer.Bytes(transfer.PNG, data))))
}

func (d *drawSurface) saveAs(data []byte, name, kind, ext string) {
	go func() {
		var parent *mygo.Window
		if d.win != nil {
			parent = d.win
		}
		path, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent: parent, Title: "Export", DefaultPath: name,
			Filters: []mygo.FileFilter{{Name: kind, Extensions: []string{ext}}},
		})
		if err != nil || path == "" {
			return
		}
		os.WriteFile(path, data, 0o644)
	}()
}

// --- files ---

// loadDrawing reads a .tldr file into an editor.
func loadDrawing(path string) (*editor.Editor, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d, err := doc.Read(f)
	if err != nil {
		return nil, err
	}
	m := newDrawMeasurer()
	return editor.New(d, m.measure), nil
}

// saveDrawing writes an editor's drawing to a .tldr file, atomically.
func saveDrawing(e *editor.Editor, path string) error {
	var buf bytes.Buffer
	if err := doc.Write(&buf, e.Doc); err != nil {
		return err
	}
	return writeFileAtomic(path, buf.Bytes())
}

// renderDrawingBitmap draws a .tldr file to a bitmap, for a note that embeds
// it with ![[sketch.tldr]]. The surface is built without a window, as the
// render needs none.
func renderDrawingBitmap(path string) (*ui.Bitmap, error) {
	d, err := openDrawing(nil, path)
	if err != nil {
		return nil, err
	}
	data, ok := d.RenderPNG(2)
	if !ok {
		return nil, errEmptyDrawing
	}
	return ui.DecodeBitmap(data)
}
