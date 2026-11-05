package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"

	"gofolio/internal/doc"
	"gofolio/internal/editor"
)

// addRectangle draws a rectangle with the geo tool, through the editor's
// public pointer API (what a user's drag does).
func addRectangle(t *testing.T, e *editor.Editor, x0, y0, x1, y1 float64) {
	t.Helper()
	e.SetGeoTool("rectangle")
	e.PointerDown(x0, y0, 0, 1, editor.Mods{}, 0)
	e.PointerMove(x1, y1, editor.Mods{}, 0.01)
	e.PointerUp(x1, y1, editor.Mods{}, 0.02)
}

func TestDrawingSaveLoadRoundTrip(t *testing.T) {
	e := editor.New(doc.New(), newDrawMeasurer().measure)
	addRectangle(t, e, 100, 100, 200, 180)
	e.PasteText("Hello world")

	shapes := len(e.Doc.Shapes)
	if shapes < 2 {
		t.Fatalf("shapes = %d, want at least a rectangle and a text", shapes)
	}

	path := filepath.Join(t.TempDir(), "drawing.tldr")
	if err := saveDrawing(e, path); err != nil {
		t.Fatalf("saveDrawing: %v", err)
	}
	re, err := loadDrawing(path)
	if err != nil {
		t.Fatalf("loadDrawing: %v", err)
	}
	if got := len(re.Doc.Shapes); got != shapes {
		t.Fatalf("reloaded shapes = %d, want %d", got, shapes)
	}
	var text string
	var rects int
	for _, s := range re.Doc.Shapes {
		switch s.Type {
		case doc.TypeGeo:
			if s.Geo == "rectangle" {
				rects++
			}
		case doc.TypeText:
			text = s.Text
		}
	}
	if rects != 1 {
		t.Fatalf("reloaded rectangles = %d, want 1", rects)
	}
	if text != "Hello world" {
		t.Fatalf("reloaded text = %q, want %q", text, "Hello world")
	}
}

func TestDrawSurfaceViewSmoke(t *testing.T) {
	e := editor.New(doc.New(), newDrawMeasurer().measure)
	addRectangle(t, e, 100, 100, 220, 180)
	e.PasteText("Note text")

	d := newDrawSurface(nil, e.Doc)
	// Use the same editor the test built, so the shapes are there.
	d.ed = e
	d.ed.OnChange = func(*doc.Drawing) {}

	tt := ui.NewTester(func(c *ui.Context) { d.view(c, true) }, 1000, 700)
	tt.Frame()
	tt.Advance(0)
	// A second frame exercises panel state built on the first.
	tt.Frame()
	_ = tt.Image()

	// The inactive path paints the canvas but takes no panels or keyboard.
	it := ui.NewTester(func(c *ui.Context) { d.view(c, false) }, 1000, 700)
	it.Frame()
	_ = it.Image()
}

func TestMonoPaletteHasNoHue(t *testing.T) {
	for _, dark := range []bool{false, true} {
		p := monoDrawPalette(ui.LightTheme(), dark)
		for _, name := range editor.Colors {
			s := p.Swatch(name)
			for _, c := range []editor.Color{s.Solid, s.Semi, s.Pattern, s.Fill, s.Lined, s.NoteFill, s.Highlight} {
				if c.R != c.G || c.G != c.B {
					t.Fatalf("dark=%v %s: %v is not gray", dark, name, c)
				}
			}
		}
	}
}

func TestDrawingTabLifecycle(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Note.md"), "# A note\n")
	a := &app{sidebar: true, width: 260}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}

	// Create, and open it as a tab.
	a.newDrawing("")
	n := a.activeNote()
	if n == nil || n.draw == nil {
		t.Fatalf("no drawing tab after newDrawing: %+v", n)
	}
	if !isDraw(n.rel) {
		t.Fatalf("drawing rel = %q, want .tldr", n.rel)
	}
	if len(a.v.drawings) != 1 || a.v.drawings[0] != n.rel {
		t.Fatalf("vault drawings = %v, want [%s]", a.v.drawings, n.rel)
	}

	// Draw on it and let the debounced save run.
	addRectangle(t, n.draw.Editor(), 40, 40, 160, 120)
	n.draw.Editor().PasteText("Sketch")

	// Saving through the tab writes the file.
	a.saveNote(n, false)

	// Reopen from disk, as a new session would.
	c := &app{sidebar: true, width: 260}
	if err := c.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	c.openPath(n.rel)
	rn := c.activeNote()
	if rn == nil || rn.draw == nil {
		t.Fatalf("reopened drawing = %+v", rn)
	}
	var text string
	var rects int
	for _, s := range rn.draw.Editor().Doc.Shapes {
		switch s.Type {
		case doc.TypeGeo:
			if s.Geo == "rectangle" {
				rects++
			}
		case doc.TypeText:
			text = s.Text
		}
	}
	if rects != 1 || text != "Sketch" {
		t.Fatalf("reopened drawing has %d rects and text %q", rects, text)
	}
}

func TestDrawingEmbedInNote(t *testing.T) {
	dir := t.TempDir()
	a := &app{sidebar: true, width: 260}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	// A drawing and a note that embeds it.
	a.newDrawing("")
	drawRel := a.activeNote().rel
	addRectangle(t, a.activeNote().draw.Editor(), 40, 40, 200, 140)
	a.saveNote(a.activeNote(), false)

	noteRel := "Note.md"
	write(t, filepath.Join(dir, noteRel), "# Sketch\n\n![[Drawing.tldr]]\n")
	a.rescan()
	a.openPath(noteRel)

	// linkify turns the embed into an image with a draw: URL.
	_, body := splitFrontmatter(a.activeNote().src)
	linked := linkify(body)
	if !strings.Contains(linked, "](draw:Drawing.tldr)") {
		t.Fatalf("linkified = %q, want a draw: image", linked)
	}
	// The draw: URL resolves back to the drawing, and it renders.
	if rel := a.resolveDraw(noteRel, "Drawing.tldr"); rel != drawRel {
		t.Fatalf("resolveDraw = %q, want %q", rel, drawRel)
	}
	if b := a.embeddedDrawing(drawRel); b == nil {
		t.Fatal("embedded drawing did not render")
	}
	// The embed does not block Live mode, since a .tldr survives as an image.
	if why := liveUnsafe(a.activeNote().src); why != "" {
		t.Fatalf("liveUnsafe = %q, want empty for a .tldr embed", why)
	}
	// A non-drawing embed is still refused.
	if why := liveUnsafe("see ![[Other]], a transclusion\n"); why == "" {
		t.Fatal("liveUnsafe allowed a note transclusion")
	}
}

func TestDrawingTabSmoke(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Note.md"), "# A note\n")
	a := &app{sidebar: true, width: 260}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.newDrawing("")
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()
	if !tt.HasText("Drawing") {
		t.Fatalf("status bar missing Drawing: %q", tt.Texts())
	}
}
