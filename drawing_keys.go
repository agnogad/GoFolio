package main

import (
	"github.com/egoist/mygo/ui"

	"gofolio/internal/doc"
	"gofolio/internal/editor"
	"gofolio/internal/geo"
)

// drawShortcuts takes the keyboard. Keys that type stand down while
// something takes text.
func (d *drawSurface) drawShortcuts(c *ui.Context) {
	e := d.ed
	if e.Editing != "" {
		if c.Shortcut(0, ui.KeyEscape) || c.Shortcut(ui.Cmd, ui.KeyEnter) {
			e.StopEditing()
		}
		// Enter in a frame's name ends it, as it has but one line.
		if s := e.Doc.Shapes[e.Editing]; s != nil && s.Type == doc.TypeFrame && c.Shortcut(0, ui.KeyEnter) {
			e.StopEditing()
		}
		return
	}
	if d.renamingPage != "" || d.helpOpen {
		return
	}
	cmd := ui.Cmd
	shift := ui.Shift
	alt := ui.Alt

	// Tools.
	for _, k := range []struct {
		key  ui.Key
		tool editor.Tool
	}{
		{ui.KeyV, editor.ToolSelect}, {ui.KeyH, editor.ToolHand}, {ui.KeyD, editor.ToolDraw}, {ui.KeyB, editor.ToolDraw},
		{ui.KeyE, editor.ToolEraser}, {ui.KeyA, editor.ToolArrow}, {ui.KeyT, editor.ToolText}, {ui.KeyN, editor.ToolNote},
		{ui.KeyL, editor.ToolLine}, {ui.KeyF, editor.ToolFrame}, {ui.KeyK, editor.ToolLaser},
	} {
		if c.Shortcut(0, k.key) {
			e.SetTool(k.tool)
		}
	}
	if c.Shortcut(shift, ui.KeyD) {
		e.SetTool(editor.ToolHighlight)
	}
	if c.Shortcut(0, ui.KeyR) {
		e.SetGeoTool("rectangle")
	}
	if c.Shortcut(0, ui.KeyO) {
		e.SetGeoTool("ellipse")
	}
	if c.Shortcut(0, ui.KeyQ) {
		e.ToolLocked = !e.ToolLocked
	}
	if c.Shortcut(cmd, ui.KeyU) {
		d.InsertImageFromDialog()
	}

	// Editing.
	if c.Shortcut(cmd, ui.KeyZ) {
		e.Undo()
	}
	if c.Shortcut(cmd|shift, ui.KeyZ) || c.Shortcut(cmd, ui.KeyY) {
		e.Redo()
	}
	if c.Shortcut(0, ui.KeyBackspace) || c.Shortcut(0, ui.KeyDelete) {
		e.Delete()
	}
	if c.Shortcut(cmd, ui.KeyA) {
		e.SelectAll()
	}
	if c.Shortcut(cmd, ui.KeyD) {
		e.Duplicate()
	}
	if c.Shortcut(cmd, ui.KeyC) {
		d.copySelection()
	}
	if c.Shortcut(cmd, ui.KeyX) {
		if d.copySelection() {
			e.Delete()
		}
	}
	if c.Shortcut(cmd, ui.KeyV) {
		d.tryPaste()
	}
	if c.Shortcut(cmd, ui.KeyG) {
		e.Group()
	}
	if c.Shortcut(cmd|shift, ui.KeyG) {
		e.Ungroup()
	}
	if c.Shortcut(shift, ui.KeyL) {
		e.ToggleLock()
	}
	if c.Shortcut(0, ui.KeyEscape) {
		e.Cancel()
	}
	if c.Shortcut(0, ui.KeyEnter) {
		if ids := e.Selection(); len(ids) == 1 {
			if s := e.Doc.Shapes[ids[0]]; s != nil && s.Type != doc.TypeGroup && s.Type != doc.TypeDraw && s.Type != doc.TypeHighlight && s.Type != doc.TypeLine && s.Type != doc.TypeImage {
				e.StartEditing(ids[0])
			}
		}
	}

	// Order. Brackets go by what the keys type on the layout.
	if c.Shortcut(0, ui.KeyBracketRight) {
		e.Reorder("forward")
	}
	if c.Shortcut(0, ui.KeyBracketLeft) {
		e.Reorder("backward")
	}
	if c.Shortcut(cmd, ui.KeyBracketRight) {
		e.Reorder("front")
	}
	if c.Shortcut(cmd, ui.KeyBracketLeft) {
		e.Reorder("back")
	}

	// Align and flip.
	if c.Shortcut(alt, ui.KeyA) {
		e.Align("left")
	}
	if c.Shortcut(alt, ui.KeyH) {
		e.Align("center-h")
	}
	if c.Shortcut(alt, ui.KeyD) {
		e.Align("right")
	}
	if c.Shortcut(alt, ui.KeyW) {
		e.Align("top")
	}
	if c.Shortcut(alt, ui.KeyV) {
		e.Align("center-v")
	}
	if c.Shortcut(alt, ui.KeyS) {
		e.Align("bottom")
	}
	if c.Shortcut(shift, ui.KeyH) {
		e.Flip(true)
	}
	if c.Shortcut(shift, ui.KeyV) {
		e.Flip(false)
	}

	// Nudge. A held arrow repeats.
	step := 1.0
	for _, k := range []struct {
		key ui.Key
		d   geo.Vec
	}{{ui.KeyLeft, geo.V(-1, 0)}, {ui.KeyRight, geo.V(1, 0)}, {ui.KeyUp, geo.V(0, -1)}, {ui.KeyDown, geo.V(0, 1)}} {
		if c.Shortcut(0, k.key) {
			e.Nudge(k.d.Mul(step))
		}
		if c.Shortcut(shift, k.key) {
			e.Nudge(k.d.Mul(10))
		}
	}

	// View.
	if c.Shortcut(cmd, ui.KeyEqual) || c.Shortcut(cmd|shift, ui.KeyEqual) {
		e.ZoomIn()
	}
	if c.Shortcut(cmd, ui.KeyMinus) {
		e.ZoomOut()
	}
	if c.Shortcut(cmd, ui.Key0) || c.Shortcut(shift, ui.Key0) {
		e.ResetZoom()
	}
	if c.Shortcut(shift, ui.Key1) {
		e.ZoomToFit()
	}
	if c.Shortcut(shift, ui.Key2) {
		e.ZoomToSelection()
	}
	if c.Shortcut(cmd, ui.KeySlash) {
		d.ToggleDark()
	}
	if c.Shortcut(shift, ui.KeySlash) {
		d.helpOpen = true
	}
	if c.Shortcut(cmd, ui.KeyPeriod) {
		e.FocusMode = !e.FocusMode
	}
	if c.Shortcut(cmd, ui.KeyQuote) {
		e.Grid = !e.Grid
	}
}

// ToggleDark goes between the light and dark themes.
func (d *drawSurface) ToggleDark() {
	if d.ed.DarkMode {
		d.theme = "light"
	} else {
		d.theme = "dark"
	}
}

// SetTheme sets the theme: "light", "dark", or "" as the system has it.
func (d *drawSurface) SetTheme(t string) { d.theme = t }

// ThemeName returns the theme setting.
func (d *drawSurface) ThemeName() string { return d.theme }

// contextMenu builds the canvas's right-click menu.
func (d *drawSurface) contextMenu(m *ui.Menu) {
	e := d.ed
	has := len(e.Selection()) > 0
	if m.Item("Cut").Disabled(!has).Shortcut(ui.Cmd, ui.KeyX).Chosen() {
		if d.copySelection() {
			e.Delete()
		}
	}
	if m.Item("Copy").Disabled(!has).Shortcut(ui.Cmd, ui.KeyC).Chosen() {
		d.copySelection()
	}
	if m.Item("Paste").Shortcut(ui.Cmd, ui.KeyV).Chosen() {
		d.tryPaste()
	}
	if m.Item("Duplicate").Disabled(!has).Shortcut(ui.Cmd, ui.KeyD).Chosen() {
		e.Duplicate()
	}
	m.Separator()
	if m.Item("Group").Disabled(!e.CanGroup()).Shortcut(ui.Cmd, ui.KeyG).Chosen() {
		e.Group()
	}
	if m.Item("Ungroup").Disabled(!e.CanUngroup()).Shortcut(ui.Cmd|ui.Shift, ui.KeyG).Chosen() {
		e.Ungroup()
	}
	m.Submenu("Arrange", func(m *ui.Menu) {
		if m.Item("Bring to Front").Disabled(!has).Shortcut(ui.Cmd, ui.KeyBracketRight).Chosen() {
			e.Reorder("front")
		}
		if m.Item("Bring Forward").Disabled(!has).Shortcut(0, ui.KeyBracketRight).Chosen() {
			e.Reorder("forward")
		}
		if m.Item("Send Backward").Disabled(!has).Shortcut(0, ui.KeyBracketLeft).Chosen() {
			e.Reorder("backward")
		}
		if m.Item("Send to Back").Disabled(!has).Shortcut(ui.Cmd, ui.KeyBracketLeft).Chosen() {
			e.Reorder("back")
		}
	})
	many := len(e.Selection()) > 1
	m.Submenu("Align", func(m *ui.Menu) {
		for _, it := range []struct{ label, how string }{
			{"Left", "left"}, {"Center Horizontally", "center-h"}, {"Right", "right"},
			{"Top", "top"}, {"Center Vertically", "center-v"}, {"Bottom", "bottom"},
		} {
			if m.Item(it.label).Disabled(!many).Chosen() {
				e.Align(it.how)
			}
		}
		m.Separator()
		if m.Item("Distribute Horizontally").Disabled(len(e.Selection()) < 3).Chosen() {
			e.Distribute(true)
		}
		if m.Item("Distribute Vertically").Disabled(len(e.Selection()) < 3).Chosen() {
			e.Distribute(false)
		}
	})
	if m.Item("Flip Horizontally").Disabled(!has).Shortcut(ui.Shift, ui.KeyH).Chosen() {
		e.Flip(true)
	}
	if m.Item("Flip Vertically").Disabled(!has).Shortcut(ui.Shift, ui.KeyV).Chosen() {
		e.Flip(false)
	}
	if m.Item("Lock / Unlock").Disabled(!has).Shortcut(ui.Shift, ui.KeyL).Chosen() {
		e.ToggleLock()
	}
	if pages := e.Pages(); len(pages) > 1 {
		m.Submenu("Move to Page", func(m *ui.Menu) {
			for _, p := range pages {
				if m.Item(p.Name).Disabled(!has || p.ID == e.Page).Chosen() {
					e.MoveToPage(p.ID)
				}
			}
		})
	}
	m.Separator()
	if m.Item("Export as PNG…").Chosen() {
		d.ExportPNG()
	}
	if m.Item("Export as SVG…").Chosen() {
		d.ExportSVG()
	}
	m.Separator()
	if m.Item("Delete").Disabled(!has).Shortcut(0, ui.KeyBackspace).Chosen() {
		e.Delete()
	}
}
