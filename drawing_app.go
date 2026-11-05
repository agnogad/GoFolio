package main

import (
	"bytes"
	"encoding/base64"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/transfer"
	"github.com/egoist/mygo/ui"

	"gofolio/internal/doc"
	"gofolio/internal/editor"
	"gofolio/internal/geo"
)

// newDrawSurface returns the state of a .tldr drawing open in a GoFolio tab.
// win may be nil, as in tests; calls that would touch it are guarded.
func newDrawSurface(win *mygo.Window, d *doc.Drawing) *drawSurface {
	m := newDrawMeasurer()
	s := &drawSurface{
		meas:    m,
		win:     win,
		t0:      time.Now(),
		bitmaps: map[doc.ID]*ui.Bitmap{}, decoded: map[doc.ID]string{}, pending: map[doc.ID]bool{},
		minimap: false, sizeBadge: true,
	}
	s.ed = editor.New(d, m.measure)
	s.ed.OnChange = func(*doc.Drawing) {
		// GoFolio's tab layer saves through saveDrawing; nothing to do here
		// beyond asking for a frame when there is a window.
		if win != nil {
			win.Update(func() {})
		}
	}
	return s
}

// openDrawing loads a .tldr file and returns the surface to show it, sharing
// one measurer between the editor and the painting.
func openDrawing(win *mygo.Window, path string) (*drawSurface, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	d, err := doc.Read(f)
	if err != nil {
		return nil, err
	}
	return newDrawSurface(win, d), nil
}

// Editor returns the surface's editor, so the tab layer can save it.
func (d *drawSurface) Editor() *editor.Editor { return d.ed }

func (d *drawSurface) showSizeBadge() bool { return d.sizeBadge }

func (d *drawSurface) seconds(t time.Time) float64 { return t.Sub(d.t0).Seconds() }

// bitmapOf returns the decoded bitmap of an asset, nil until it is ready.
func (d *drawSurface) bitmapOf(id doc.ID) *ui.Bitmap {
	return d.bitmaps[id]
}

// decodeAssets decodes on the side what shapes are about to show.
func (d *drawSurface) decodeAssets() {
	for id, asset := range d.ed.Doc.Assets {
		if d.decoded[id] == asset.Src || d.pending[id] {
			continue
		}
		if !strings.HasPrefix(asset.Src, "data:") {
			d.decoded[id] = asset.Src
			continue
		}
		d.pending[id] = true
		src := asset.Src
		go func() {
			b := decodeDataURL(src)
			run := func() {
				delete(d.pending, id)
				d.decoded[id] = src
				if b != nil {
					d.bitmaps[id] = b
				}
			}
			if d.win != nil {
				d.win.Update(run)
			} else {
				run()
			}
		}()
	}
}

func decodeDataURL(src string) *ui.Bitmap {
	i := strings.IndexByte(src, ',')
	if i < 0 {
		return nil
	}
	data, err := base64.StdEncoding.DecodeString(src[i+1:])
	if err != nil {
		return nil
	}
	b, err := ui.DecodeBitmap(data)
	if err != nil {
		return nil
	}
	return b
}

// view renders the whole drawing surface inside the current context. active
// tells whether this surface's tab is the one showing: panels and the
// keyboard stand down when it is not.
func (d *drawSurface) view(c *ui.Context, active bool) {
	e := d.ed
	d.services = c.Services()
	d.hasSvc = true
	d.applyTheme(c)
	// The camera works in the size the canvas had last.
	if b := d.canv.Bounds(c); b.W > 0 {
		e.VW, e.VH = float64(b.W), float64(b.H)
		e.FitNewPage()
	}
	d.decodeAssets()
	e.ModsChanged(modsOf(c.Modifiers()))
	if len(e.Trails) > 0 {
		if e.AgeTrails(d.seconds(c.Now())); len(e.Trails) > 0 {
			c.AnimationFrame()
		}
	}
	if !active {
		// Still paint the canvas; only take the keyboard when showing.
		ui.Box(c).Fill().Children(func() { d.canvas(c) })
		return
	}
	d.drawShortcuts(c)
	ui.Box(c).Fill().Children(func() {
		d.canvas(c)
		d.textEditor(c)
		if !e.FocusMode {
			d.topLeft(c)
			d.stylePanel(c)
			d.toolbar(c)
			d.zoomPanel(c)
		}
	})
	d.helpDialog(c)
}

func (d *drawSurface) applyTheme(c *ui.Context) {
	var t *ui.Theme
	switch d.theme {
	case "light":
		t = ui.LightTheme()
	case "dark":
		t = ui.DarkTheme()
	default:
		// As the system has it.
		d.ed.DarkMode = c.Theme().Dark
		return
	}
	c.SetTheme(t)
	d.ed.DarkMode = t.Dark
}

func modsOf(m ui.Modifiers) editor.Mods {
	return editor.Mods{Shift: m&ui.Shift != 0, Alt: m&ui.Alt != 0, Cmd: m&ui.Cmd != 0, Ctrl: m&ui.Ctrl != 0}
}

// canvas builds the element the drawing is painted on and takes input on.
func (d *drawSurface) canvas(c *ui.Context) {
	e := d.ed
	cv := ui.Box(c.Key("canvas")).Absolute().Top(0).Left(0).Right(0).Bottom(0).
		Focusable().FocusRing(false).Bind(&d.canv).
		Cursor(cursorOf(e.CursorNow()))
	cv.Label("Canvas")
	cv.Draw(d.paintCanvas)
	cv.HandleInput(func(ev ui.InputEvent) bool { return d.input(c, ev) })
	cv.ContextMenu(d.contextMenu)
}

func cursorOf(k editor.Cursor) ui.Cursor {
	switch k {
	case editor.CursorCross:
		return ui.CursorCrosshair
	case editor.CursorText:
		return ui.CursorText
	case editor.CursorGrab:
		return ui.CursorGrab
	case editor.CursorGrabbing:
		return ui.CursorGrabbing
	case editor.CursorEW:
		return ui.CursorResizeEW
	case editor.CursorNS:
		return ui.CursorResizeNS
	case editor.CursorNESW:
		return ui.CursorResizeNESW
	case editor.CursorNWSE:
		return ui.CursorResizeNWSE
	case editor.CursorRotate:
		return ui.CursorMove
	case editor.CursorPointer:
		return ui.CursorPointer
	case editor.CursorMove:
		return ui.CursorMove
	}
	return ui.CursorDefault
}

// input takes the canvas's input as it comes.
func (d *drawSurface) input(c *ui.Context, ev ui.InputEvent) bool {
	e := d.ed
	t := d.seconds(ev.Time)
	x, y := float64(ev.X), float64(ev.Y)
	switch ev.Kind {
	case ui.InputPointerDown:
		d.canv.Focus(c)
		e.PointerDown(x, y, ev.Button, ev.Clicks, modsOf(ev.Mods), t)
		// The right button's menu opens as the framework's: only its press
		// is the editor's, for what it selects.
		return ev.Button != 1
	case ui.InputPointerMove:
		e.PointerMove(x, y, modsOf(ev.Mods), t)
		return true
	case ui.InputPointerUp:
		e.PointerUp(x, y, modsOf(ev.Mods), t)
		return true
	case ui.InputScroll:
		if ev.Mods&(ui.Cmd|ui.Ctrl) != 0 {
			e.ZoomAt(geo.V(x, y), math.Exp(-float64(ev.DY)*0.01))
		} else {
			e.Pan(float64(ev.DX), float64(ev.DY))
		}
		e.PointerMove(x, y, modsOf(ev.Mods), t)
		return true
	case ui.InputZoom:
		e.ZoomAt(geo.V(x, y), float64(ev.Scale))
		return true
	case ui.InputKeyDown:
		if ev.Key == ui.KeySpace && e.Editing == "" {
			e.SetSpace(true)
			return true
		}
	case ui.InputKeyUp:
		if ev.Key == ui.KeySpace {
			e.SetSpace(false)
			return true
		}
	}
	return false
}

// textEditor builds the input over the shape whose text is being edited.
func (d *drawSurface) textEditor(c *ui.Context) {
	e := d.ed
	if e.Editing == "" {
		d.lastEditing = ""
		return
	}
	if d.lastEditing != e.Editing {
		d.lastEditing = e.Editing
		d.editKey++
	}
	box, size, font, align, col := e.EditBox()
	if size == 0 {
		return
	}
	cam := e.Cam()
	m := e.PageMat(e.Editing)
	// Text that sizes itself gets a little room, on the side it grows to, to
	// keep what is typed from wrapping before the shape has grown to hold it.
	// Text that wraps wraps as it will show.
	if s := e.Doc.Shapes[e.Editing]; s != nil && s.Type == doc.TypeText && s.AutoSize {
		switch align {
		case "middle":
			box.X -= size / 2
		case "end":
			box.X -= size
		}
		box.W += size
	}
	// Room below for rounding, without moving the lines.
	box.H += 2 / cam.Z
	center := e.ToScreen(m.Apply(box.Center()))
	sw, sh := box.W*cam.Z, box.H*cam.Z
	input := ui.TextAreaBase(c.Key(d.editKey), &e.EditText).
		Absolute().Left(float32(center.X-sw/2)).Top(float32(center.Y-sh/2)).Size(float32(sw), float32(sh)).
		Font(drawFamilyOf(font)).FontSize(float32(size * cam.Z)).LineHeight(editor.LineHeight).
		TextColor(toUI(col)).Label("Text").AutoFocus().FocusRing(false)
	switch align {
	case "middle":
		input.TextAlign(ui.Center)
	case "end":
		input.TextAlign(ui.End)
	default:
		input.TextAlign(ui.Start)
	}
	if deg := m.Rotation() * 180 / math.Pi; math.Abs(deg) > 0.01 {
		input.Rotate(float32(deg))
	}
	input.OnChange(e.EditChanged)
}

// --- clipboard ---

func (d *drawSurface) copySelection() bool {
	data, text := d.ed.CopyData()
	if data == nil {
		return false
	}
	item := []transfer.Representation{transfer.Bytes(transfer.Format(editor.ClipboardFormat), data)}
	if text != "" {
		item = append(item, transfer.Bytes(transfer.Text, []byte(text)))
	}
	d.services.WriteClipboardData(transfer.New(transfer.NewItem(item...)))
	return true
}

func (d *drawSurface) tryPaste() {
	e := d.ed
	formats := d.services.ClipboardFormats()
	has := func(f transfer.Format) bool {
		for _, g := range formats {
			if g == f {
				return true
			}
		}
		return false
	}
	// As in tldraw, what is pasted comes where it was if that shows, not at
	// the pointer.
	var at *geo.Vec
	switch {
	case has(transfer.Format(editor.ClipboardFormat)):
		data := d.services.ReadClipboardData(transfer.Format(editor.ClipboardFormat))
		if b, err := data.Read(transfer.Format(editor.ClipboardFormat)); err == nil {
			e.Paste(b, at)
		}
	case has(transfer.PNG):
		data := d.services.ReadClipboardData(transfer.PNG)
		if b, err := data.Read(transfer.PNG); err == nil {
			d.insertImageData("image.png", "image/png", b, at)
		}
	default:
		e.PasteText(d.services.ReadClipboard())
	}
}

// insertImageData adds an image from its bytes.
func (d *drawSurface) insertImageData(name, mime string, data []byte, at *geo.Vec) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return
	}
	d.ed.InsertImage(name, mime, data, float64(cfg.Width), float64(cfg.Height), at)
}

// --- files ---

func mimeOf(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	}
	return ""
}

// DropFiles adds the images among files dropped at a point of the window, in
// DIPs.
func (d *drawSurface) DropFiles(files []string, x, y float64) {
	d.dropFiles(files, d.ed.ToPage(geo.V(x, y)), d.win)
}

// dropFiles adds the images among files, around a point of the page. Reading
// them happens on the side.
func (d *drawSurface) dropFiles(files []string, at geo.Vec, win *mygo.Window) {
	go func() {
		offset := 0.0
		for _, path := range files {
			mime := mimeOf(path)
			if mime == "" {
				continue
			}
			data, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			p := at.Add(geo.V(offset, offset))
			offset += 24
			name := filepath.Base(path)
			run := func() { d.insertImageData(name, mime, data, &p) }
			if d.win != nil {
				d.win.Update(run)
			} else {
				run()
			}
		}
	}()
}

// InsertImageFromDialog asks for images, and adds them.
func (d *drawSurface) InsertImageFromDialog() {
	go func() {
		var parent *mygo.Window
		if d.win != nil {
			parent = d.win
		}
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent: parent, Title: "Insert Image", Filters: []mygo.FileFilter{{Name: "Images", Extensions: []string{"png", "jpg", "jpeg", "gif"}}},
		})
		if err != nil || len(paths) == 0 {
			return
		}
		d.dropFiles(paths, d.ed.ViewBox().Center(), d.win)
	}()
}
