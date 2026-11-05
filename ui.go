package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"
)

func (a *app) view(c *ui.Context) {
	t := monoTheme(c.Theme())
	c.SetTheme(t)
	if a.notice != "" {
		c.Toast(a.notice)
		a.notice = ""
	}
	a.shortcuts(c)
	if a.v == nil {
		tb := c.TitleBar()
		ui.Column(c).Fill().Children(func() {
			a.titlebar(c, tb)
			a.emptyView(c)
		})
	} else {
		a.workspace(c)
	}
	a.dragGhost(c)
	a.modals(c)
}

// dragGhost draws the floating label that follows the pointer while a tree
// row is dragged, so the move reads as a real drag.
func (a *app) dragGhost(c *ui.Context) {
	if a.dragRel == "" {
		return
	}
	t := c.Theme()
	names := a.selectedRels(a.dragRel)
	title := trimExt(pathBase(a.dragRel))
	if len(names) > 1 {
		title = fmt.Sprintf("%d items", len(names))
	}
	x, y := a.dragX+14, a.dragY+6
	ui.Overlay(c, func() {
		ui.Row(c).
			Absolute().
			Left(x).
			Top(y).
			Padding(5, 10).
			Gap(7).
			Radius(7).
			AlignItems(ui.Center).
			Background(t.Background).
			Border(1, t.Border).
			Shadow(0, 8, 22, 0, ui.RGBA(0, 0, 0, 0.20)).
			Children(func() {
				ui.Icon(c, iconFile).FontSize(13).TextColor(t.TextMuted)
				ui.Text(c, title).FontSize(12.5).TextColor(t.Text)
			})
	})
}

func (a *app) shortcuts(c *ui.Context) {
	if a.paletteOpen || a.promptOpen || a.deleteOpen {
		return
	}
	if a.v == nil {
		if c.Shortcut(ui.Cmd, ui.KeyO) {
			a.chooseVault()
		}
		return
	}
	n := a.activeNote()
	if c.Shortcut(ui.Cmd, ui.KeyS) {
		if n != nil {
			a.saveNote(n, true)
		}
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyN) {
		a.newFolder("")
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyD) {
		a.newDrawing("")
	}
	if c.Shortcut(ui.Cmd, ui.KeyN) {
		a.newNote("")
	}
	if c.Shortcut(ui.Cmd, ui.KeyP) {
		a.openPalette()
	}
	if c.Shortcut(ui.Cmd|ui.Shift, ui.KeyF) {
		a.fullText = true
		a.sidebar = true
		a.searchH.Focus(c)
		a.saveSoon()
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.chooseVault()
	}
	if c.Shortcut(ui.Cmd, ui.KeyR) {
		a.rescan()
		a.flash("Vault reloaded")
	}
	if c.Shortcut(ui.Cmd, ui.KeyG) {
		a.toggleGraph()
	}
	if c.Shortcut(ui.Cmd, ui.KeyE) && n != nil && n.draw == nil {
		a.setMode(n, viewMode((int(n.mode)+1)%3))
	}
	if c.Shortcut(ui.Cmd, ui.Key1) && n != nil && n.draw == nil {
		a.setMode(n, modeSource)
	}
	if c.Shortcut(ui.Cmd, ui.Key2) && n != nil && n.draw == nil {
		a.setMode(n, modeLive)
	}
	if c.Shortcut(ui.Cmd, ui.Key3) && n != nil && n.draw == nil {
		a.setMode(n, modeRead)
	}
	if c.Shortcut(ui.Cmd, ui.KeyW) && n != nil {
		a.closeTab(a.active)
	}
	if c.Shortcut(ui.Cmd, ui.KeyBackslash) {
		a.sidebar = !a.sidebar
		a.saveSoon()
	}
	if c.Shortcut(ui.Cmd, ui.KeyT) {
		a.newTab()
	}
	if c.Shortcut(ui.Cmd, ui.KeyO) {
		a.chooseVault()
	}
	if c.Shortcut(0, ui.KeyF2) {
		a.renameActive()
	}
	if c.Shortcut(ui.Cmd, ui.KeyBackspace) {
		if len(a.selectedRels("")) > 0 {
			a.deleteTarget = ""
			a.deleteOpen = true
		} else if n != nil {
			a.deleteTarget = n.rel
			a.deleteOpen = true
		}
	}
	if c.Shortcut(ui.Cmd, ui.KeyA) && a.sidebar {
		a.selectAllVisible()
	}
	if c.Shortcut(ui.Cmd, ui.KeyComma) {
		a.settingsOpen = true
		a.setTab = 0
	}
	if c.Shortcut(0, ui.KeyEscape) && a.fullText {
		a.fullText = false
		a.query = ""
		a.hits = nil
	}
}

func (a *app) workspace(c *ui.Context) {
	tb := c.TitleBar()
	ui.Column(c).Fill().Children(func() {
		a.titlebar(c, tb)
		ui.Row(c).Grow(1).FillWidth().AlignItems(ui.Stretch).Children(func() {
			if a.ribbon {
				a.ribbonPane(c)
			}
			if a.sidebar && !a.collapsed {
				split := ui.Split(c, &a.width, func() {
					a.sidebarPane(c)
				}, func() {
					a.mainPane(c)
				}).Grow(1)
				if split.Changed() {
					a.saveSoon()
				}
			} else {
				a.mainPane(c)
			}
		})
		a.statusbar(c)
	})
}

// ribbonPane is the vertical icon rail on the far left, as in Obsidian: the
// app menu, search, graph, and the settings/vaults entries.
func (a *app) ribbonPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).
		Width(44).
		FillHeight().
		Padding(6, 0, 6, 0).
		Gap(2).
		AlignItems(ui.Center).
		Background(t.Surface).
		BorderWidth(0, 1, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			popoverIconButton(c, iconMore, "Menu", a.appMenu)
			ui.Box(c).Height(4)
			if a.v != nil {
				iconButton(c, iconSearch, "Search", a.sidebar && a.fullText, func() {
					a.sidebar = true
					a.collapsed = false
					a.fullText = !a.fullText
					if a.fullText {
						a.searchH.Focus(c)
					}
					a.saveSoon()
				})
				iconButton(c, iconGraph, "Graph view", a.graph, func() {
					a.toggleGraph()
				})
			}
			ui.Row(c).Grow(1)
			popoverIconButton(c, iconSettings, "Settings", a.settingsMenu)
			iconButton(c, iconVaults, "Vaults", a.vaultsOpen, func() {
				a.vaultsOpen = true
			})
		})
}

func (a *app) titlebar(c *ui.Context, tb ui.TitleBar) {
	t := c.Theme()
	height := max(tb.Height, 46)
	left := tb.Left + 14
	if tb.Left == 0 {
		left = 14
	}
	right := tb.Right + 10
	ui.Row(c).Height(height).
		Padding(0, right, 0, left).
		Gap(2).
		AlignItems(ui.Center).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		DragWindow().
		Children(func() {
			ui.Text(c, "GoFolio").FontSize(13.5).FontWeight(650).LetterSpacing(-0.2).TextColor(t.Text).PaddingEnd(8)
			name := "Open a folder…"
			if a.vaultPath != "" {
				name = filepath.Base(a.vaultPath)
			}
			if r := []rune(name); len(r) > 22 {
				name = string(r[:21]) + "…"
			}
			vault := ui.MenuButton(c, name, func(m *ui.Menu) {
				if m.Item("Open folder…").Chosen() {
					a.chooseVault()
				}
				if m.Item("Vaults…").Chosen() {
					a.vaultsOpen = true
				}
				a.recentsMenu(m)
				if a.vaultPath != "" {
					m.Separator()
					if m.Item("Reload vault").Shortcut(ui.Cmd, ui.KeyR).Chosen() {
						a.rescan()
						a.flash("Vault reloaded")
					}
				}
			}).FontSize(12.5)
			vault.Background(ui.Transparent).Border(0, ui.Transparent)
			if a.v != nil {
				collapse := iconChevronDouble
				if a.collapsed {
					collapse = iconChevronDoubleRight
				}
				iconButton(c, collapse, "Collapse sidebar", false, func() {
					a.collapsed = !a.collapsed
					if !a.collapsed {
						a.sidebar = true
					}
					a.saveSoon()
				})
			}
			ui.Row(c).Grow(1)
			if a.v != nil {
				iconButton(c, iconSearch, "Search", a.sidebar && a.fullText, func() {
					a.sidebar = true
					a.collapsed = false
					a.searchH.Focus(c)
					a.saveSoon()
				})
				iconButton(c, iconGraph, "Graph view", a.graph, func() {
					a.toggleGraph()
				})
				iconButton(c, iconSidebar, "Toggle sidebar", a.sidebar, func() {
					a.sidebar = !a.sidebar
					a.saveSoon()
				})
			}
			popoverIconButton(c, iconMore, "Menu", a.appMenu)
			if runtime.GOOS != "darwin" {
				ui.Row(c).Width(6)
				windowButton(c, iconMinus, "Minimize", func() {
					if a.win != nil {
						a.win.Minimize()
					}
				})
				windowButton(c, iconMax, "Maximize", func() {
					if a.win != nil {
						a.win.ToggleMaximize()
					}
				})
				windowButton(c, iconX, "Close window", func() {
					if a.win != nil {
						a.win.Close()
					}
				})
			}
		})
}

func (a *app) recentsMenu(m *ui.Menu) {
	if len(a.recents) == 0 {
		return
	}
	m.Separator()
	for _, recent := range a.recents {
		recent := recent
		label := filepath.Base(recent)
		if recent == a.vaultPath {
			label += "  ✓"
		}
		if m.Item(label).Chosen() {
			if err := a.setVault(recent, nil); err != nil {
				a.flash(err.Error())
			}
		}
	}
}

// newMenu is the sidebar "+" menu: create a note, drawing or folder.
func (a *app) newMenu(m *ui.Menu) {
	if m.Item("New note").Shortcut(ui.Cmd, ui.KeyN).Chosen() {
		a.newNote("")
	}
	if m.Item("New drawing").Shortcut(ui.Cmd|ui.Shift, ui.KeyD).Chosen() {
		a.newDrawing("")
	}
	if m.Item("New folder").Shortcut(ui.Cmd|ui.Shift, ui.KeyN).Chosen() {
		a.newFolder("")
	}
}

// sidebarMenu is the sidebar "…" menu: view options.
func (a *app) sidebarMenu(m *ui.Menu) {
	if m.Item("Sort by " + a.nextSortLabel()).Chosen() {
		a.toggleSort()
	}
	if m.Item("Collapse all folders").Chosen() {
		a.collapseAll()
	}
	if m.Item("Reveal active note").Chosen() {
		a.revealActive()
	}
	m.Separator()
	if m.Item("Search in notes").Shortcut(ui.Cmd|ui.Shift, ui.KeyF).Chosen() {
		a.fullText = true
		a.sidebar = true
		a.saveSoon()
	}
	if m.Item("Quick switcher…").Shortcut(ui.Cmd, ui.KeyP).Chosen() {
		a.openPalette()
	}
}

// nextSortLabel names what the Sort item would switch to.
func (a *app) nextSortLabel() string {
	switch a.sortKey {
	case 0:
		return "modified time"
	case 1:
		return "name (Z–A)"
	default:
		return "name (A–Z)"
	}
}

// settingsMenu is the ribbon gear's menu.
func (a *app) settingsMenu(m *ui.Menu) {
	if m.Item("Settings…").Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
		a.settingsOpen = true
		a.setTab = 0
	}
	if m.Item("Vaults…").Chosen() {
		a.vaultsOpen = true
	}
	m.Separator()
	if m.Item("Open folder…").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
		a.chooseVault()
	}
	if a.v != nil && m.Item("Reload vault").Shortcut(ui.Cmd, ui.KeyR).Chosen() {
		a.rescan()
		a.flash("Vault reloaded")
	}
}

func (a *app) appMenu(m *ui.Menu) {
	if a.v == nil {
		if m.Item("Open folder…").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
			a.chooseVault()
		}
		a.recentsMenu(m)
		return
	}
	if m.Item("New note").Shortcut(ui.Cmd, ui.KeyN).Chosen() {
		a.newNote("")
	}
	if m.Item("New folder").Shortcut(ui.Cmd|ui.Shift, ui.KeyN).Chosen() {
		a.newFolder("")
	}
	if m.Item("New drawing").Shortcut(ui.Cmd|ui.Shift, ui.KeyD).Chosen() {
		a.newDrawing("")
	}
	m.Separator()
	if m.Item("Quick switcher…").Shortcut(ui.Cmd, ui.KeyP).Chosen() {
		a.openPalette()
	}
	if m.Item("Search in notes").Shortcut(ui.Cmd|ui.Shift, ui.KeyF).Chosen() {
		a.fullText = true
		a.sidebar = true
		a.saveSoon()
	}
	label := "Graph view"
	if a.graph {
		label = "Close graph view"
	}
	if m.Item(label).Shortcut(ui.Cmd, ui.KeyG).Chosen() {
		a.toggleGraph()
	}
	m.Separator()
	sidebar := "Hide sidebar"
	if !a.sidebar {
		sidebar = "Show sidebar"
	}
	if m.Item(sidebar).Shortcut(ui.Cmd, ui.KeyBackslash).Chosen() {
		a.sidebar = !a.sidebar
		a.saveSoon()
	}
	if m.Item("Reload vault").Shortcut(ui.Cmd, ui.KeyR).Chosen() {
		a.rescan()
		a.flash("Vault reloaded")
	}
	if n := a.activeNote(); n != nil {
		m.Separator()
		if m.Item("Rename…").Shortcut(0, ui.KeyF2).Chosen() {
			a.renameEntry(n.rel)
		}
		if m.Item("Duplicate").Chosen() {
			a.duplicateEntry(n.rel)
		}
		if m.Item("Copy path").Chosen() {
			a.copyPathToClipboard(n.rel)
		}
		if m.Item("Reveal in sidebar").Chosen() {
			a.revealActive()
		}
		m.Separator()
		if m.Item("Move to Trash…").Shortcut(ui.Cmd, ui.KeyBackspace).Chosen() {
			a.deleteTarget = n.rel
			a.deleteOpen = true
		}
		if m.Item("Close tab").Shortcut(ui.Cmd, ui.KeyW).Chosen() {
			a.closeTab(a.active)
		}
	}
	m.Separator()
	if m.Item("Settings…").Shortcut(ui.Cmd, ui.KeyComma).Chosen() {
		a.settingsOpen = true
		a.setTab = 0
	}
	if m.Item("Vaults…").Chosen() {
		a.vaultsOpen = true
	}
	m.Separator()
	if m.Item("Open folder…").Shortcut(ui.Cmd, ui.KeyO).Chosen() {
		a.chooseVault()
	}
}

func (a *app) sidebarPane(c *ui.Context) {
	t := c.Theme()
	section := "Files"
	if a.fullText {
		section = "Search"
	}
	col := ui.Column(c).
		Fill().
		Background(t.Surface).
		BorderWidth(0, 1, 0, 0).
		BorderColor(t.Border)
	col.Children(func() {
		ui.Row(c).Padding(14, 10, 6, 12).Gap(2).AlignItems(ui.Center).Children(func() {
			selCount := 0
			if !a.fullText {
				selCount = len(a.selectedRels(""))
			}
			if selCount > 1 {
				ui.Textf(c, "%d SELECTED", selCount).
					FontSize(10).
					FontWeight(650).
					LetterSpacing(1.2).
					TextColor(t.Text).
					Grow(1)
				iconButton(c, iconCopy, "Duplicate selected", false, func() {
					a.duplicateSelection()
				})
				iconButton(c, iconTrash, "Move selected to Trash", false, func() {
					a.deleteTarget = ""
					a.deleteOpen = true
				})
				iconButton(c, iconX, "Clear selection", false, func() {
					a.clearSel()
				})
				return
			}
			ui.Text(c, strings.ToUpper(section)).
				FontSize(10).
				FontWeight(650).
				LetterSpacing(1.2).
				TextColor(t.TextMuted).
				Grow(1)
			if !a.fullText {
				popoverIconButton(c, iconPlus, "New…", a.newMenu)
				popoverIconButton(c, iconMore, "Sidebar options", a.sidebarMenu)
			} else {
				iconButton(c, iconX, "Close search", false, func() {
					a.fullText = false
					a.query = ""
					a.hits = nil
				})
			}
		})
		ui.Row(c).Padding(0, 12, 10, 12).Children(func() {
			field := ui.SearchField(c, &a.query).
				Bind(&a.searchH).
				Placeholder("Search notes").
				Label("Search notes").
				Grow(1)
			if field.Changed() {
				if a.fullText {
					a.hitsFor = ""
					a.refreshSearch()
				}
			}
		})
		if a.fullText {
			ui.Row(c).Padding(0, 12, 8, 12).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Inside note text").FontSize(11).TextColor(t.TextMuted).Grow(1)
			})
		}
		sc := ui.Scroll(c).Grow(1).FillWidth()
		sc.Children(func() {
			inner := ui.Column(c).Gap(1).Padding(4, 6).FillHeight().Background(t.Surface)
			if a.width > 40 {
				inner.Width(a.width)
			} else {
				inner.FillWidth()
			}
			inner.Children(func() {
				switch {
				case a.fullText && strings.TrimSpace(a.query) != "":
					a.searchResults(c)
				case strings.TrimSpace(a.query) != "":
					a.filterResults(c)
				default:
					ui.Tree(c, func() { a.tree(c, a.v.nodes) }).FillWidth()
				}
			})
		})
		// A click in the list's empty space, not on a row, clears the
		// selection the way clicking blank space does.
		if sc.Clicked() {
			b := sc.Bounds()
			px, py, _ := sc.PointerPosition()
			if !a.hitRow(b.X+px, b.Y+py) {
				a.clearSel()
			}
		}
		if n := a.activeNote(); n != nil {
			a.backlinksPane(c, n)
		}
	})
}

func (a *app) tree(c *ui.Context, nodes []*fileNode) {
	t := c.Theme()
	// Only the outermost call resets per-frame state and commits a finished
	// drag; subtree calls (depth > 0) just add rows.
	outer := a.treeDepth == 0
	a.treeDepth++
	defer func() { a.treeDepth-- }()
	if outer && a.rowBoxes == nil {
		a.rowBoxes = make(map[string]ui.Rect)
	}
	if outer && a.dragRel == "" {
		a.rowBoxes = make(map[string]ui.Rect)
		a.dragOver = ""
		a.dragInto = false
	}
	active := ""
	if n := a.activeNote(); n != nil {
		active = n.rel
	}
	if outer {
		a.visOrder = a.visOrder[:0]
	}
	for _, node := range a.order(nodes) {
		node := node
		var kids func()
		var open *bool
		if node.dir {
			open = a.dirOpen(node.rel)
			kids = func() { a.tree(c, node.kids) }
		}
		item := ui.TreeItem(c, node.name, open, kids).Radius(5).FillWidth()
		// Draggable with a pinned, rigid zero-size bound enables drag
		// detection without moving the row; the drop target is tracked by
		// pointer hit-testing instead.
		dragBound := ui.Rect{}
		item.Draggable(nil, ui.DraggableOptions{Rigid: true, Bounds: &dragBound})
		a.rowBoxes[node.rel] = item.Bounds()
		a.visOrder = append(a.visOrder, node.rel)
		isActive := !node.dir && node.rel == active
		// Selection takes priority over the active highlight, as in Obsidian.
		sel := a.selected(node.rel)
		drop := a.dragOver == node.rel
		if sel {
			item.Background(t.SurfacePressed)
			item.TextColor(t.Text)
		}
		if drop && a.dragInto {
			item.Background(t.SurfaceHover)
			item.TextColor(t.Text)
		}
		if isActive {
			item.Background(t.SurfaceHover)
			item.TextColor(t.Text)
			item.FontWeight(600)
			item.DrawOver(func(p *ui.Painter, r ui.Rect) {
				p.Fill(ui.Rect{X: 0, Y: r.Y + 3, W: 2.5, H: r.H - 6}, t.Text, 2)
			})
		}
		// Drag-to-move: the row under the pointer becomes the drop target
		// while a row is held and dragged.
		if item.Dragging() {
			if a.dragRel == "" {
				a.dragRel = node.rel
				// Dragging a row that is not selected drags just that row.
				if !a.selected(node.rel) {
					a.clearSel()
					a.selectOne(node.rel)
				}
			}
			a.dragLive = true
			// The pointer is usually outside the source row, so use its
			// position whether or not it still hovers the row.
			x, y, _ := item.PointerPosition()
			b := item.Bounds()
			if b.W > 0 {
				a.dragX, a.dragY = x+b.X, y+b.Y
				a.updateDrop(a.dragX, a.dragY)
			}
		}
		item.ContextMenu(func(m *ui.Menu) {
			if !a.selected(node.rel) {
				a.clearSel()
				a.selectOne(node.rel)
			}
			a.entryMenu(m, node)
		})
		if item.DoubleClicked() {
			a.renameEntry(node.rel)
		}
		if item.Clicked() {
			a.clickRow(item.ClickModifiers(), node)
		}
	}
	if !outer {
		return
	}
	// A drag whose row no longer reports the pointer down has finished.
	if a.dragRel != "" {
		if a.dragLive {
			a.dragLive = false
		} else {
			src, dest := a.dragRel, a.dragOver
			into := a.dragInto
			a.dragRel, a.dragOver, a.dragInto = "", "", false
			if dest != "" && dest != src {
				target := dest
				if !into {
					target = pathDir(dest)
				}
				a.moveSelection(src, target)
			}
		}
	}
}

// moveSelection moves the dragged row and every other selected row into
// target, skipping rows already there or nested under the target.
func (a *app) moveSelection(src, target string) {
	rels := a.selectedRels(src)
	if len(rels) == 0 {
		a.moveEntry(src, target)
		return
	}
	moved := 0
	first := ""
	for _, rel := range rels {
		if rel == target || strings.HasPrefix(target+"/", rel+"/") {
			continue
		}
		if pathDir(rel) == target {
			continue
		}
		next, err := a.v.move(rel, target)
		if err != nil {
			a.flash("Could not move " + pathBase(rel) + ": " + err.Error())
			continue
		}
		a.retargetTabs(rel, next)
		if first == "" {
			first = next
		}
		moved++
	}
	if target != "" {
		*a.dirOpen(target) = true
	}
	a.rescan()
	a.lastActive = ""
	a.blinksFor = ""
	a.afterActive()
	if moved == 1 {
		a.flash("Moved " + pathBase(first))
	} else if moved > 1 {
		a.flash(fmt.Sprintf("Moved %d items to %s", moved, dropName(target)))
	}
	a.saveSoon()
}

// selectedRels lists the selection, making sure the dragged row is in it.
func (a *app) selectedRels(extra string) []string {
	seen := map[string]bool{}
	var out []string
	for _, rel := range a.visOrder {
		if a.selected(rel) {
			out = append(out, rel)
			seen[rel] = true
		}
	}
	if extra != "" && !seen[extra] {
		out = append([]string{extra}, out...)
	}
	return out
}

// order returns the visible order of tree nodes under the current sort.
func (a *app) order(nodes []*fileNode) []*fileNode {
	if a.sortKey == 0 {
		return nodes
	}
	out := append([]*fileNode(nil), nodes...)
	sort.SliceStable(out, func(i, j int) bool {
		x, y := out[i], out[j]
		if x.dir != y.dir {
			return x.dir
		}
		if a.sortKey == 1 {
			return x.mod.After(y.mod)
		}
		return strings.ToLower(x.name) > strings.ToLower(y.name)
	})
	return out
}

// entryMenu fills the context menu for one tree row.
func (a *app) entryMenu(m *ui.Menu, node *fileNode) {
	sel := a.selectedRels("")
	if len(sel) > 1 {
		m.Item(fmt.Sprintf("%d items selected", len(sel))).Disabled(true)
		m.Separator()
		if m.Item("Move to Trash…").Shortcut(ui.Cmd, ui.KeyBackspace).Chosen() {
			a.deleteTarget = ""
			a.deleteOpen = true
		}
		return
	}
	if node.dir {
		if m.Item("New note here").Chosen() {
			a.promptKind = promptNewNote
			a.promptTitle = "New note in “" + node.name + "”"
			a.promptValue = "Untitled"
			a.promptParent = node.rel
			a.promptOpen = true
		}
		if m.Item("New drawing here").Chosen() {
			a.newDrawing(node.rel)
		}
		if m.Item("New folder here").Chosen() {
			a.promptKind = promptNewFolder
			a.promptTitle = "New folder in “" + node.name + "”"
			a.promptValue = "New folder"
			a.promptParent = node.rel
			a.promptOpen = true
		}
		m.Separator()
	}
	if m.Item("Open").Shortcut(0, ui.KeyEnter).Chosen() && !node.dir {
		a.openPath(node.rel)
	}
	if m.Item("Rename…").Shortcut(0, ui.KeyF2).Chosen() {
		a.renameEntry(node.rel)
	}
	if m.Item("Duplicate").Chosen() {
		a.duplicateEntry(node.rel)
	}
	if m.Item("Copy path").Chosen() {
		a.copyPathToClipboard(node.rel)
	}
	m.Separator()
	if m.Item("Move to Trash…").Shortcut(ui.Cmd, ui.KeyBackspace).Chosen() {
		a.deleteTarget = node.rel
		a.deleteOpen = true
	}
}

// hitRow reports whether a window point lands on any tree row.
func (a *app) hitRow(x, y float32) bool {
	for _, r := range a.rowBoxes {
		if x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H {
			return true
		}
	}
	return false
}

// updateDrop finds the tree row under a window point and records it as the
// drop target. Dropping on a folder moves inside it; on a file, beside it.
func (a *app) updateDrop(x, y float32) {
	a.dragOver, a.dragInto = "", false
	for rel, r := range a.rowBoxes {
		if rel == a.dragRel {
			continue
		}
		if y >= r.Y && y < r.Y+r.H && x >= r.X-4 && x < r.X+r.W+40 {
			a.dragOver = rel
			if info := a.nodeInfo(rel); info != nil && info.dir {
				a.dragInto = true
			}
			return
		}
	}
}

// nodeInfo finds a tree node by path.
func (a *app) nodeInfo(rel string) *fileNode {
	var find func(nodes []*fileNode) *fileNode
	find = func(nodes []*fileNode) *fileNode {
		for _, n := range nodes {
			if n.rel == rel {
				return n
			}
			if n.dir {
				if got := find(n.kids); got != nil {
					return got
				}
			}
		}
		return nil
	}
	if a.v == nil {
		return nil
	}
	return find(a.v.nodes)
}

func (a *app) filterResults(c *ui.Context) {
	t := c.Theme()
	matches := a.filterNotes(a.query)
	if len(matches) == 0 {
		ui.Text(c, "No notes match").FontSize(12).TextColor(t.TextMuted).Padding(8, 8)
		return
	}
	for _, m := range matches {
		m := m
		row := ui.Column(c).Padding(6, 8).Gap(1).Radius(5).FillWidth()
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			ui.Text(c, trimExt(m.name)).FontSize(12.5).SingleLine().TextColor(t.Text)
			if dir := pathDir(m.rel); dir != "" {
				ui.Text(c, dir).FontSize(10.5).SingleLine().TextColor(t.TextMuted)
			}
		})
		if row.Clicked() {
			a.openPath(m.rel)
		}
	}
}

func (a *app) searchResults(c *ui.Context) {
	t := c.Theme()
	if len(a.hits) == 0 {
		ui.Text(c, "No matches").FontSize(12).TextColor(t.TextMuted).Padding(8, 8)
		return
	}
	for _, hit := range a.hits {
		hit := hit
		row := ui.Column(c).Padding(6, 8).Gap(1).Radius(5).FillWidth()
		if row.Hovered() {
			row.Background(t.SurfaceHover)
		}
		row.Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, trimExt(pathBase(hit.rel))).FontSize(12.5).FontWeight(600).SingleLine().Grow(1).TextColor(t.Text)
				ui.Textf(c, "L%d", hit.line).FontSize(10).TextColor(t.TextMuted)
			})
			if hit.text != "" {
				ui.Text(c, hit.text).FontSize(11).SingleLine().TextColor(t.TextMuted)
			}
		})
		if row.Clicked() {
			a.openAt(hit.rel, hit.start, hit.length)
		}
	}
}

func (a *app) backlinksPane(c *ui.Context, n *note) {
	t := c.Theme()
	ui.Column(c).BorderWidth(1, 0, 0, 0).BorderColor(t.Border).Children(func() {
		head := ui.Row(c).Padding(9, 10).Gap(6).AlignItems(ui.Center)
		if head.Hovered() {
			head.Background(t.SurfaceHover)
		}
		head.Children(func() {
			ui.Icon(c, iconLink).FontSize(12).TextColor(t.TextMuted)
			ui.Text(c, "Backlinks").FontSize(11).FontWeight(600).LetterSpacing(0.2).TextColor(t.TextMuted).Grow(1)
			ui.Textf(c, "%d", len(a.blinks)).FontSize(10.5).TextColor(t.TextMuted)
		})
		if head.Clicked() {
			a.backlinks = !a.backlinks
			a.saveSoon()
		}
		if !a.backlinks {
			return
		}
		if len(a.blinks) == 0 {
			ui.Text(c, "No notes link here").FontSize(11.5).TextColor(t.TextMuted).Padding(0, 12, 10, 12)
			return
		}
		for i, b := range a.blinks {
			if i == 6 {
				ui.Textf(c, "+%d more", len(a.blinks)-6).FontSize(10.5).TextColor(t.TextMuted).Padding(0, 12, 8, 12)
				break
			}
			b := b
			row := ui.Column(c).Padding(5, 12).Gap(1).FillWidth()
			if row.Hovered() {
				row.Background(t.SurfaceHover)
			}
			row.Children(func() {
				ui.Text(c, trimExt(pathBase(b.rel))).FontSize(11.5).FontWeight(600).SingleLine().TextColor(t.Text)
				ui.Text(c, b.line).FontSize(10.5).SingleLine().TextColor(t.TextMuted)
			})
			if row.Clicked() {
				a.openPath(b.rel)
			}
		}
	})
}

func (a *app) mainPane(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillHeight().Background(t.Background).Children(func() {
		a.tabStrip(c)
		if a.graph {
			a.graphPane(c)
			return
		}
		n := a.activeNote()
		if n == nil {
			a.noNoteView(c)
			return
		}
		if n.draw != nil {
			n.draw.view(c, true)
			return
		}
		switch n.mode {
		case modeSource:
			a.sourcePane(c, n)
		case modeLive:
			a.livePane(c, n)
		default:
			a.readPane(c, n)
		}
	})
}

func (a *app) tabStrip(c *ui.Context) {
	t := c.Theme()
	prev := a.active
	closeIdx := -1
	ui.Row(c).
		Height(42).
		Gap(0).
		PaddingStart(10).
		PaddingEnd(10).
		Background(t.Background).
		BorderWidth(0, 0, 1, 0).
		BorderColor(t.Border).
		Children(func() {
			if len(a.open) > 0 {
				tabs := ui.TabsBase(c, &a.active, len(a.open))
				tabs.List.Gap(4)
				tabs.List.Children(func() {
					for i, n := range a.open {
						i, n := i, n
						on := i == a.active
						tab := tabs.Tab(i).
							Height(30).
							Padding(0, 9).
							Gap(7).
							Radius(6).
							AlignItems(ui.Center)
						if on {
							tab.Background(t.SurfaceHover)
							tab.BorderWidth(0, 0, 2, 0).BorderColor(t.Text)
						} else if tab.Hovered() {
							tab.Background(t.Surface)
						}
						tab.Children(func() {
							title := ui.Text(c, trimExt(pathBase(n.rel))).
								FontSize(12.5).
								SingleLine().
								MaxWidth(170)
							if on {
								title.TextColor(t.Text).FontWeight(550)
							} else {
								title.TextColor(t.TextMuted)
							}
							if n.dirty() {
								ui.Box(c).Size(5, 5).Radius(3).Background(t.TextMuted)
							}
							// The close button keeps its slot so tabs never
							// shift; it only shows on the active or hovered tab.
							cl := ui.Box(c).Size(17, 17).Radius(4).Center()
							if on || tab.Hovered() {
								if cl.Hovered() {
									cl.Background(t.SurfacePressed)
								}
								cl.Children(func() {
									ui.Icon(c, iconX).FontSize(10).TextColor(t.TextMuted)
								})
							}
							if cl.Clicked() {
								closeIdx = i
							}
						})
					}
				})
				if a.active != prev {
					a.afterActive()
				}
			}
			ui.Row(c).PaddingStart(4).Children(func() {
				iconButton(c, iconPlus, "New note", false, func() { a.newNote("") })
			})
			ui.Row(c).Grow(1)
		})
	if closeIdx >= 0 {
		a.closeTab(closeIdx)
	}
}

func (a *app) sourcePane(c *ui.Context, n *note) {
	a.syncNote(n)
	n.st.Theme = monoEditorTheme(c.Theme())
	n.st.NoActiveLine = false
	ui.Column(c).Grow(1).FillHeight().Padding(14, 34, 4, 34).Children(func() {
		editor.View(c, n.st).Grow(1).FillWidth()
	})
}

func (a *app) livePane(c *ui.Context, n *note) {
	t := c.Theme()
	if n.doc == nil {
		a.ensureLive(n)
	}
	ui.Row(c).Grow(1).Justify(ui.Center).Children(func() {
		ui.Column(c).MaxWidth(790).Grow(1).FillHeight().Padding(20, 18, 0, 18).Children(func() {
			if n.front != "" {
				front := ui.Column(c).Border(1, t.Border).Radius(6).Padding(6, 10).Gap(3)
				if front.Hovered() {
					front.Background(t.SurfaceHover)
				}
				front.Children(func() {
					ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
						ui.Icon(c, iconFile).FontSize(11).TextColor(t.TextMuted)
						ui.Text(c, "YAML frontmatter").FontSize(10.5).FontWeight(600).LetterSpacing(0.3).TextColor(t.TextMuted).Grow(1)
						label := "Show"
						if a.frontOpen {
							label = "Hide"
						}
						ui.Text(c, label).FontSize(10.5).TextColor(t.TextMuted)
					})
					if a.frontOpen {
						ui.Text(c, strings.TrimSpace(n.front)).FontSize(11).TextColor(t.TextMuted)
					}
				})
				if front.Clicked() {
					a.frontOpen = !a.frontOpen
				}
			}
			body := richtext.EditorBase(c, n.doc, richtext.EditorOptions{
				Label:       n.rel,
				Placeholder: "Start writing…",
				Font:        ui.Font{Size: 16, LineHeight: 1.65},
				Theme:       monoRichtextTheme(t),
				OnLink:      func(url string) { a.openLink(n.rel, url) },
			}).Grow(1)
			if body.Changed() {
				a.liveChanged(n)
			}
		})
	})
}

func (a *app) readPane(c *ui.Context, n *note) {
	_, body := splitFrontmatter(n.src)
	ui.Scroll(c).Grow(1).Children(func() {
		ui.Row(c).Justify(ui.Center).Padding(24, 20, 56, 20).Children(func() {
			ui.Column(c).MaxWidth(720).WidthPercent(100).Children(func() {
				if strings.TrimSpace(body) == "" {
					ui.Text(c, "This note is empty.").FontSize(14).TextColor(c.Theme().TextMuted)
					return
				}
				markdown.View(c, linkify(body), a.readOptions(c, n))
			})
		})
	})
}

// readOptions is a note's Read-pane Markdown options, kept between frames so
// the view's image cache persists: an embedded drawing loads on a goroutine
// and would otherwise be forgotten before it could show.
func (a *app) readOptions(c *ui.Context, n *note) *markdown.Options {
	dark := c.Theme().Dark
	if n.mdRead == nil || n.mdReadDark != dark {
		n.mdRead = &markdown.Options{
			Theme:     monoMarkdownTheme(c.Theme()),
			LoadImage: a.markdownImage(n.rel, n.abs),
			OnLink:    func(url string) { a.openLink(n.rel, url) },
		}
		n.mdReadDark = dark
	}
	return n.mdRead
}

func (a *app) noNoteView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).Center().Gap(10).Padding(24).Children(func() {
		ui.Text(c, "No note open").FontSize(18).FontWeight(650).TextColor(t.Text)
		msg := "Choose a note from the sidebar, or start a new one."
		if a.v != nil && len(a.v.notes) == 0 {
			msg = "This vault is empty. Start with a note."
		}
		ui.Text(c, msg).FontSize(13).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Padding(4, 0, 0, 0).Children(func() {
			if ui.PrimaryButton(c, "New note").Clicked() {
				a.newNote("")
			}
			if ui.Button(c, "Quick switcher").Clicked() {
				a.openPalette()
			}
		})
	})
}

func (a *app) statusbar(c *ui.Context) {
	t := c.Theme()
	n := a.activeNote()
	left := "No note open"
	if n != nil && n.draw != nil {
		left = statusDrawCount(n.draw)
	} else if n != nil {
		words := len(strings.Fields(n.src))
		left = fmt.Sprintf("%d words · %d characters", words, utf8.RuneCountInString(n.src))
	}
	ui.Row(c).
		Height(30).
		Padding(0, 14).
		Gap(10).
		AlignItems(ui.Center).
		Background(t.Surface).
		BorderWidth(1, 0, 0, 0).
		BorderColor(t.Border).
		Children(func() {
			ui.Text(c, left).FontSize(11).TextColor(t.TextMuted)
			if n != nil {
				if dir := pathDir(n.rel); dir != "" {
					ui.Box(c).Size(1, 11).Background(t.Border)
					ui.Text(c, dir).FontSize(11).SingleLine().MaxWidth(260).TextColor(t.TextMuted)
				}
			}
			ui.Row(c).Grow(1)
			if n != nil && n.draw != nil {
				ui.Text(c, "Drawing").FontSize(11).TextColor(t.TextMuted)
				return
			}
			if n != nil {
				state := "Edited"
				if !n.dirty() {
					state = "Saved"
				}
				ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
					dot := ui.Box(c).Size(5, 5).Radius(3)
					if n.dirty() {
						dot.Background(t.Text)
					} else {
						dot.Background(faint(t))
					}
				})
				ui.Text(c, state).FontSize(11).TextColor(t.TextMuted)
				ui.Box(c).Size(1, 11).Background(t.Border)
				m := int(n.mode)
				seg := ui.Segmented(c, &m, "Source", "Live", "Read").Height(24).FontSize(11)
				if m != int(n.mode) {
					a.setMode(n, viewMode(m))
				}
				_ = seg
			}
		})
}

func (a *app) emptyView(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Grow(1).FillWidth().Center().Gap(12).Padding(24).Children(func() {
		ui.Text(c, "GoFolio").FontSize(32).FontWeight(700).LetterSpacing(-0.8).TextColor(t.Text)
		ui.Text(c, "A quiet place for Markdown notes.").FontSize(14).TextColor(t.TextMuted)
		ui.Row(c).Gap(8).Padding(8, 0, 10, 0).Children(func() {
			if ui.PrimaryButton(c, "Open a folder").Width(150).Clicked() {
				a.chooseVault()
			}
		})
		ui.Text(c, "Point it at an Obsidian vault, or any folder of Markdown files.").
			FontSize(11.5).
			TextColor(t.TextMuted)
		if len(a.recents) > 0 {
			ui.Column(c).Width(380).Gap(2).Padding(16, 0, 0, 0).Children(func() {
				ui.Text(c, "RECENT").FontSize(9.5).FontWeight(650).LetterSpacing(1.1).TextColor(t.TextMuted).Padding(0, 0, 4, 4)
				for _, recent := range a.recents {
					recent := recent
					row := ui.Row(c).Padding(7, 10).Gap(8).Radius(5).AlignItems(ui.Center)
					if row.Hovered() {
						row.Background(t.SurfaceHover)
					}
					row.Children(func() {
						ui.Icon(c, iconFolder).FontSize(13).TextColor(t.TextMuted)
						ui.Text(c, filepath.Base(recent)).FontSize(12.5).TextColor(t.Text)
						ui.Row(c).Grow(1)
						ui.Text(c, recent).FontSize(10.5).SingleLine().MaxWidth(220).TextColor(t.TextMuted)
					})
					if row.Clicked() {
						if err := a.setVault(recent, nil); err != nil {
							a.flash(err.Error())
						}
					}
				}
			})
		}
	})
}

func (a *app) modals(c *ui.Context) {
	if a.paletteOpen {
		a.paletteModal(c)
	}
	if a.settingsOpen {
		a.settingsModal(c)
	}
	if a.vaultsOpen {
		a.vaultsModal(c)
	}
	if a.promptOpen {
		a.promptModal(c)
	}
	if a.deleteOpen {
		msg := "It moves to the vault's .trash folder; nothing is deleted for good."
		if n := len(a.selectedRels("")); n > 1 && a.deleteTarget == "" {
			msg = fmt.Sprintf("%d items move to the vault's .trash folder; nothing is deleted for good.", n)
		}
		switch ui.AlertDialog(c, &a.deleteOpen, "Move to Trash?", msg, "Cancel", "Move to Trash") {
		case 1:
			if a.deleteTarget == "" {
				a.trashSelection()
			} else {
				a.deleteNow()
			}
		}
	}
}

// settingsModal is a centered settings window with a category rail, as in
// Obsidian. The stored settings apply on the next launch and to new notes.
func (a *app) settingsModal(c *ui.Context) {
	t := c.Theme()
	cats := []string{"Editor", "Files & Links", "Appearance", "About"}
	ui.DialogBase(c, &a.settingsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.42))
		panel.Width(720).Height(500).Padding(0).Radius(12).Background(t.Background).
			Border(1, t.Border).Shadow(0, 18, 56, 0, ui.RGBA(0, 0, 0, 0.22))
		ui.Row(c).Fill().Children(func() {
			// Category rail.
			ui.Column(c).Width(190).FillHeight().Padding(14, 8, 14, 8).Gap(2).
				Background(t.Surface).BorderWidth(0, 1, 0, 0).BorderColor(t.Border).
				Children(func() {
					ui.Text(c, "SETTINGS").FontSize(10).FontWeight(650).LetterSpacing(1.2).
						TextColor(t.TextMuted).Padding(0, 8, 8, 8)
					for i, name := range cats {
						i, name := i, name
						row := ui.Row(c).Padding(8, 10).Gap(8).Radius(6).AlignItems(ui.Center).FillWidth()
						if i == a.setTab {
							row.Background(t.SurfaceHover)
						} else if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							col := t.TextMuted
							if i == a.setTab {
								col = t.Text
							}
							ui.Text(c, name).FontSize(12.5).TextColor(col)
						})
						if row.Clicked() {
							a.setTab = i
						}
					}
				})
			// Detail.
			ui.Column(c).Grow(1).FillHeight().Padding(20, 24, 20, 24).Gap(16).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Children(func() {
					ui.Text(c, cats[a.setTab]).FontSize(17).FontWeight(650).TextColor(t.Text).Grow(1)
					iconButton(c, iconX, "Close settings", false, func() { a.settingsOpen = false })
				})
				ui.Box(c).Height(1).FillWidth().Background(t.Border)
				switch a.setTab {
				case 0:
					a.settingsEditor(c)
				case 1:
					a.settingsFiles(c)
				case 2:
					a.settingsAppearance(c)
				default:
					a.settingsAbout(c)
				}
			})
		})
	})
}

// settingRow lays out a labelled setting with a control below it.
func settingRow(c *ui.Context, title, desc string, control func()) {
	t := c.Theme()
	ui.Column(c).FillWidth().Gap(3).Padding(0, 0, 14, 0).Children(func() {
		ui.Text(c, title).FontSize(13).FontWeight(600).TextColor(t.Text)
		if desc != "" {
			ui.Text(c, desc).FontSize(11.5).TextColor(t.TextMuted)
		}
		ui.Row(c).Padding(4, 0, 0, 0).Children(control)
	})
}

func (a *app) settingsEditor(c *ui.Context) {
	m := int(a.defaultMode)
	settingRow(c, "Default view", "Which mode a note opens in when you first create or open it.", func() {
		seg := ui.Segmented(c, &m, "Source", "Live", "Read").Height(28).FontSize(12)
		_ = seg
	})
	if m != int(a.defaultMode) {
		a.defaultMode = viewMode(m)
		a.modeSet = true
		a.saveSoon()
	}
	settingRow(c, "Spellcheck", "Underline misspelled words as you type in Live mode.", func() {
		on := a.spellcheck
		sw := ui.Switch(c, &on)
		_ = sw
	})
}

func (a *app) settingsFiles(c *ui.Context) {
	settingRow(c, "Vault folder", "Everything lives in plain files here.", func() {
		path := "No vault open"
		if a.vaultPath != "" {
			path = a.vaultPath
		}
		ui.Text(c, path).FontSize(12).TextColor(c.Theme().TextMuted).SingleLine().MaxWidth(460)
	})
	settingRow(c, "Deleted files", "Trash keeps removed notes in .trash inside the vault.", func() {
		if ui.Button(c, "Open .trash").Clicked() && a.vaultPath != "" {
			_ = mygo.Shell.OpenPath(filepath.Join(a.vaultPath, ".trash"))
		}
	})
}

func (a *app) settingsAppearance(c *ui.Context) {
	settingRow(c, "Theme", "GoFolio follows your system's light or dark setting.", func() {
		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, iconSun).FontSize(14).TextColor(c.Theme().TextMuted)
			ui.Text(c, "Automatic (system)").FontSize(12).TextColor(c.Theme().Text)
			ui.Icon(c, iconMoon).FontSize(14).TextColor(c.Theme().TextMuted)
		})
	})
	settingRow(c, "Sidebar", "The file list is monochrome and uses no accent colour.", func() {
		ui.Text(c, "Monochrome").FontSize(12).TextColor(c.Theme().TextMuted)
	})
}

func (a *app) settingsAbout(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Gap(6).Children(func() {
		ui.Text(c, "GoFolio "+appVersion).FontSize(14).FontWeight(600).TextColor(t.Text)
		ui.Text(c, "A quiet, native place for Markdown notes and drawings.").FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, "Built on MyGo's ui toolkit.").FontSize(12).TextColor(t.TextMuted)
	})
}

// vaultsModal is the Obsidian-like vault switcher: the open vault on top,
// then every other vault the user has opened or pinned.
func (a *app) vaultsModal(c *ui.Context) {
	t := c.Theme()
	current := a.vaultPath
	seen := map[string]bool{}
	var list []string
	if current != "" {
		list = append(list, current)
		seen[current] = true
	}
	for _, p := range a.pinned {
		if !seen[p] {
			list = append(list, p)
			seen[p] = true
		}
	}
	for _, p := range a.recents {
		if !seen[p] {
			list = append(list, p)
			seen[p] = true
		}
	}
	ui.DialogBase(c, &a.vaultsOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.42))
		panel.Width(600).Height(460).Padding(0).Radius(12).Background(t.Background).
			Border(1, t.Border).Shadow(0, 18, 56, 0, ui.RGBA(0, 0, 0, 0.22))
		ui.Column(c).Fill().Padding(0).Children(func() {
			ui.Row(c).Padding(16, 16, 10, 16).AlignItems(ui.Center).Children(func() {
				ui.Text(c, "Vaults").FontSize(17).FontWeight(650).TextColor(t.Text).Grow(1)
				iconButton(c, iconX, "Close vaults", false, func() { a.vaultsOpen = false })
			})
			ui.Row(c).Padding(0, 16, 12, 16).Gap(8).Children(func() {
				if ui.PrimaryButton(c, "Open folder…").Clicked() {
					a.vaultsOpen = false
					a.chooseVault()
				}
				if ui.Button(c, "Create new vault…").Clicked() {
					a.vaultsOpen = false
					a.createVault()
				}
			})
			ui.Scroll(c).Grow(1).Children(func() {
				ui.Column(c).FillWidth().Padding(0, 12, 12, 12).Gap(2).Children(func() {
					if len(list) == 0 {
						ui.Text(c, "No vaults yet.").FontSize(12.5).TextColor(t.TextMuted).Padding(8, 12)
						return
					}
					for _, p := range list {
						p := p
						isCur := p == current
						row := ui.Row(c).Padding(9, 12).Gap(10).Radius(7).AlignItems(ui.Center).FillWidth()
						if isCur {
							row.Background(t.SurfaceHover)
						} else if row.Hovered() {
							row.Background(t.SurfaceHover)
						}
						row.Children(func() {
							ui.Icon(c, iconVaults).FontSize(15).TextColor(t.TextMuted)
							ui.Column(c).Grow(1).Gap(0).Children(func() {
								ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
									ui.Text(c, filepath.Base(p)).FontSize(13).FontWeight(600).TextColor(t.Text)
									if isCur {
										ui.Text(c, "current").FontSize(10).TextColor(t.TextMuted)
									}
								})
								ui.Text(c, p).FontSize(10.5).SingleLine().TextColor(t.TextMuted)
							})
							if row.Hovered() {
								if ui.Button(c, "Open").Clicked() && !isCur {
									if err := a.setVault(p, nil); err != nil {
										a.flash(err.Error())
									}
									a.vaultsOpen = false
								}
							}
						})
						if row.Clicked() && !isCur {
							if err := a.setVault(p, nil); err != nil {
								a.flash(err.Error())
							}
							a.vaultsOpen = false
						}
					}
				})
			})
		})
	})
}

func (a *app) paletteModal(c *ui.Context) {
	t := c.Theme()
	a.buildPalette()
	ui.DialogBase(c, &a.paletteOpen, func(backdrop, panel ui.Element) {
		backdrop.Background(ui.RGBA(0, 0, 0, 0.40))
		panel.Width(590).Padding(0).Radius(10).Background(t.Background).Border(1, t.Border).
			Shadow(0, 14, 48, 0, ui.RGBA(0, 0, 0, 0.18))
		ui.Column(c).Padding(7).Gap(3).Children(func() {
			input := ui.TextInput(c, &a.paletteQuery).
				AutoFocus().
				Placeholder("Search notes, or > for commands").
				Label("Quick switcher").
				Height(38)
			input.OnTextKey(func(k ui.TextKey) bool {
				switch {
				case k.Key == ui.KeyDown:
					if len(a.paletteItems) > 0 {
						a.paletteIndex = (a.paletteIndex + 1) % len(a.paletteItems)
					}
				case k.Key == ui.KeyUp:
					if len(a.paletteItems) > 0 {
						a.paletteIndex = (a.paletteIndex - 1 + len(a.paletteItems)) % len(a.paletteItems)
					}
				case k.Key == ui.KeyEnter:
					a.runPalette()
				case k.Key == ui.KeyEscape:
					a.paletteOpen = false
				default:
					return false
				}
				return true
			})
			if input.Changed() {
				a.paletteIndex = 0
			}
			chosen := -1
			if len(a.paletteItems) == 0 {
				ui.Text(c, "No match").FontSize(12).TextColor(t.TextMuted).Padding(10, 10)
				return
			}
			for i, it := range a.paletteItems {
				i, it := i, it
				on := i == a.paletteIndex
				row := ui.Row(c).Padding(7, 9).Gap(9).Radius(6).AlignItems(ui.Center).FillWidth()
				if on {
					row.Background(t.SurfaceHover)
				}
				row.Children(func() {
					ui.Column(c).Grow(1).Gap(0).Children(func() {
						ui.Text(c, it.label).FontSize(12.5).SingleLine().TextColor(t.Text)
						if it.detail != "" {
							ui.Text(c, it.detail).FontSize(10.5).SingleLine().TextColor(t.TextMuted)
						}
					})
					if it.hint != "" {
						ui.Text(c, it.hint).FontSize(10.5).TextColor(t.TextMuted)
					}
				})
				if row.Clicked() {
					chosen = i
				}
			}
			if chosen >= 0 {
				a.paletteIndex = chosen
				a.runPalette()
			}
		})
	})
}

func (a *app) promptModal(c *ui.Context) {
	label := "Create"
	if a.promptKind == promptRename {
		label = "Rename"
	}
	ui.Modal(c, &a.promptOpen, func() {
		ui.Column(c).Width(380).Gap(12).Children(func() {
			ui.Text(c, a.promptTitle).FontSize(15).FontWeight(650)
			input := ui.TextInput(c, &a.promptValue).AutoFocus().Label(a.promptTitle)
			if input.Submitted() {
				a.submitPrompt()
			}
			ui.Row(c).Justify(ui.End).Gap(8).Children(func() {
				if ui.Button(c, "Cancel").Clicked() {
					a.promptOpen = false
				}
				if ui.PrimaryButton(c, label).Clicked() {
					a.submitPrompt()
				}
			})
		})
	})
}

// --- helpers -------------------------------------------------------------

func (a *app) dirOpen(rel string) *bool {
	if a.openDirs == nil {
		a.openDirs = make(map[string]*bool)
	}
	p, ok := a.openDirs[rel]
	if !ok {
		open := true
		p = &open
		a.openDirs[rel] = p
	}
	return p
}

func (a *app) openLink(fromRel, dest string) {
	if strings.HasPrefix(dest, "http://") || strings.HasPrefix(dest, "https://") || strings.HasPrefix(dest, "mailto:") {
		_ = mygo.Shell.OpenExternal(dest)
		return
	}
	if a.v == nil {
		return
	}
	if abs, ok := strings.CutPrefix(dest, "draw:"); ok {
		if rel := a.resolveDraw(fromRel, pathUnescape(abs)); rel != "" {
			a.openPath(rel)
			return
		}
		a.flash("No drawing named " + pathUnescape(abs))
		return
	}
	if rel, ok := a.v.resolveLink(fromRel, dest); ok {
		a.openPath(rel)
		return
	}
	if strings.TrimSpace(dest) != "" {
		a.flash("No note named " + pathUnescape(dest))
	}
}

// markdownImage is the Read pane's image loader: a draw: URL renders the
// .tldr it names, any other URL is a file beside the note.
func (a *app) markdownImage(fromRel, noteAbs string) func(url string) (ui.ImageSource, error) {
	file := markdown.FileImages(filepath.Dir(noteAbs))
	return func(url string) (ui.ImageSource, error) {
		if abs, ok := strings.CutPrefix(url, "draw:"); ok {
			rel := a.resolveDraw(fromRel, pathUnescape(abs))
			if b := a.embeddedDrawing(rel); b != nil {
				return b, nil
			}
			return nil, errEmptyDrawing
		}
		return file(url)
	}
}

func iconButton(c *ui.Context, ic *ui.SVG, label string, active bool, fn func()) {
	t := c.Theme()
	b := ui.Button(c, "").
		Label(label).
		Tooltip(label).
		Width(27).
		Height(27).
		Padding(0).
		Radius(5).
		Center().
		Background(ui.Transparent).
		Border(0, ui.Transparent)
	if active || b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, ic).FontSize(14.5).TextColor(t.TextMuted)
	})
	if b.Clicked() {
		fn()
	}
}

func windowButton(c *ui.Context, ic *ui.SVG, label string, fn func()) {
	t := c.Theme()
	b := ui.Button(c, "").
		Label(label).
		Tooltip(label).
		Width(33).
		Height(27).
		Padding(0).
		Radius(4).
		Center().
		Background(ui.Transparent).
		Border(0, ui.Transparent)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, ic).FontSize(12).TextColor(t.TextMuted)
	})
	if b.Clicked() {
		fn()
	}
}

func popoverIconButton(c *ui.Context, ic *ui.SVG, label string, build func(m *ui.Menu)) {
	t := c.Theme()
	b := ui.ButtonBase(c).
		Label(label).
		Tooltip(label).
		Width(27).
		Height(27).
		Padding(0).
		Radius(5).
		Center().
		Background(ui.Transparent)
	if b.Hovered() {
		b.Background(t.SurfaceHover)
	}
	b.Children(func() {
		ui.Icon(c, ic).FontSize(14.5).TextColor(t.TextMuted)
	})
	b.PopoverMenuButton(build)
}
