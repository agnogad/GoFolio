package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/editor"
	"github.com/egoist/mygo/ui/markdown"
	"github.com/egoist/mygo/ui/richtext"

	"gofolio/internal/doc"
)

const appVersion = "0.1.0"

type viewMode int

const (
	modeSource viewMode = iota
	modeLive
	modeRead
)

const (
	promptNewNote = iota
	promptNewFolder
	promptRename
)

// note is an open document: the source text is the truth, and the live
// editor's document is built from it when that mode shows.
type note struct {
	rel, abs string
	src      string // canonical, \n line endings, frontmatter included
	saved    string
	eol      string
	front    string
	body     string
	mode     viewMode

	buf *editor.Buffer
	st  *editor.State
	doc *richtext.Document
	ver uint64
	// docFor is the source text the live document was built from, so
	// switching modes keeps its undo history when nothing changed.
	docFor string

	// draw is non-nil for a .tldr drawing, which replaces the note's text
	// panes with the drawing surface.
	draw *drawSurface

	// mdRead is the Read pane's Markdown options, kept between frames so the
	// view's image cache survives and embedded drawings show once loaded.
	mdRead     *markdown.Options
	mdReadDark bool

	timer *time.Timer
}

func (n *note) dirty() bool { return n.src != n.saved }

type app struct {
	win *mygo.Window

	v         *vault
	vaultPath string
	recents   []string

	open   []*note
	active int

	width      float32
	sidebar    bool
	backlinks  bool
	openDirs   map[string]*bool
	query      string
	fullText   bool
	hits       []searchHit
	hitsFor    string
	blinks     []backlink
	blinksFor  string
	lastActive string
	searchH    ui.Handle
	frontOpen  bool

	paletteOpen  bool
	paletteQuery string
	paletteIndex int
	paletteItems []paletteItem

	promptOpen   bool
	promptKind   int
	promptTitle  string
	promptValue  string
	promptParent string
	promptTarget string

	deleteOpen   bool
	deleteTarget string

	// sidebar drag-to-move: dragRel is the row being dragged, dropRel the
	// folder under the pointer, dropInto true when it would move inside it.
	dragRel  string
	dragOver string
	dragInto bool
	// dragLive is set while any row reports an active drag this frame; when a
	// pending dragRel no longer is live, the pointer was released.
	dragLive bool
	// dragX/dragY track the pointer in window DIPs so a floating ghost of the
	// dragged row can follow it, as in Obsidian.
	dragX, dragY float32

	// selection is the set of tree rows chosen with Cmd/Ctrl or Shift click;
	// drag and delete act on all of them.
	sel       map[string]bool
	selAnchor string
	lastClick time.Time

	// sidebar sort: sortKey and sortFolders control the tree order.
	sortKey     int
	sortReverse bool

	// tree row boxes for the current frame, for drag hit-testing.
	rowBoxes map[string]ui.Rect
	// treeDepth tracks nested tree() calls so only the outermost commits drags.
	treeDepth int
	// visOrder is the flat, visible row order this frame, for Shift ranges.
	visOrder []string

	// focusPath is a tree row to scroll into view once (Reveal in sidebar).
	focusPath string

	// graph view
	graph       bool
	graphH      ui.Handle
	graphState  *graphView
	graphPrefs  *graphSettings // remembered between launches
	graphSaveOn bool

	// settings and the Vaults window.
	settingsOpen bool
	vaultsOpen   bool
	defaultMode  viewMode
	modeSet      bool
	spellcheck   bool
	pinned       []string
	setTab       int

	// ribbon: the vertical icon rail on the far left.
	ribbon bool
	// collapsed hides the sidebar contents, leaving only the ribbon.
	collapsed bool

	// drawCache holds bitmaps of .tldr files embedded in notes, keyed by
	// vault-relative path, so a Read pane does not redraw them per frame.
	drawCache map[string]*ui.Bitmap

	notice    string
	saveTimer *time.Timer
}

type paletteItem struct {
	label  string
	detail string
	hint   string
	run    func()
}

func (a *app) activeNote() *note {
	if a.active >= 0 && a.active < len(a.open) {
		return a.open[a.active]
	}
	return nil
}

func (a *app) restore() {
	s := loadSession()
	a.recents = s.Recents
	a.width = s.Sidebar
	a.sidebar = s.ShowSide
	a.collapsed = s.Collapsed
	a.backlinks = s.Backlinks
	a.fullText = s.FullText
	a.defaultMode = viewMode(s.DefaultMode)
	if !s.ModeSet {
		a.defaultMode = modeLive
	}
	a.spellcheck = s.Spellcheck
	a.pinned = s.Pinned
	a.graphPrefs = s.Graph
	a.ribbon = true
	a.openDirs = make(map[string]*bool)
	if s.Vault != "" {
		if err := a.setVault(s.Vault, &s); err != nil {
			a.notice = err.Error()
		}
	}
}

func (a *app) setVault(path string, s *session) error {
	path = filepath.Clean(path)
	v, err := scanVault(path)
	if err != nil {
		return fmt.Errorf("cannot open %s: %v", filepath.Base(path), err)
	}
	a.flush()
	a.v = v
	a.vaultPath = path
	a.recents = remember(a.recents, path)
	a.open = nil
	a.active = -1
	a.openDirs = make(map[string]*bool)
	a.query = ""
	a.hits = nil
	a.fullText = false
	a.blinks = nil
	a.blinksFor = ""
	if s != nil {
		for _, t := range s.Open {
			if !v.has(t.Path) {
				continue
			}
			n := a.loadNote(t.Path)
			if n == nil {
				continue
			}
			mode := viewMode(t.Mode)
			if mode < modeSource || mode > modeRead {
				mode = modeSource
			}
			if mode == modeLive && liveUnsafe(n.src) != "" {
				mode = modeSource
			}
			n.mode = mode
			a.open = append(a.open, n)
		}
		if s.Active >= 0 && s.Active < len(a.open) {
			a.active = s.Active
		}
	}
	if a.active < 0 && len(a.open) > 0 {
		a.active = 0
	}
	a.afterActive()
	a.saveSoon()
	return nil
}

func (a *app) loadNote(rel string) *note {
	abs, err := a.v.abs(rel)
	if err != nil {
		return nil
	}
	if isDraw(rel) {
		d, err := openDrawing(a.win, abs)
		if err != nil {
			a.notice = "Could not open " + pathBase(rel) + ": " + err.Error()
			return nil
		}
		n := &note{rel: rel, abs: abs, draw: d}
		// Autosave the drawing, debounced like a note.
		d.Editor().OnChange = func(*doc.Drawing) {
			a.touch(n)
			if a.win != nil {
				a.win.Update(func() {})
			}
		}
		return n
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		a.notice = "Could not read " + pathBase(rel)
		return nil
	}
	raw := string(b)
	src := strings.ReplaceAll(raw, "\r\n", "\n")
	mode := a.defaultMode
	if mode < modeSource || mode > modeRead {
		mode = modeSource
	}
	if mode == modeLive && liveUnsafe(src) != "" {
		mode = modeSource
	}
	n := &note{
		rel:   rel,
		abs:   abs,
		src:   src,
		saved: src,
		eol:   eolOf(raw),
		mode:  mode,
	}
	n.buf = editor.NewBuffer(src)
	n.buf.SetLanguage(editor.LanguageFor(rel))
	n.ver = n.buf.Version()
	n.st = &editor.State{
		Buffer:         n.buf,
		Font:           ui.Font{Size: 14.5},
		LineHeight:     1.7,
		SoftWrap:       true,
		LineNumbers:    editor.LineNumbersOff,
		NoIndentGuides: true,
		NoFolding:      true,
		Placeholder:    "Write something…",
		Label:          rel,
	}
	n.front, n.body = splitFrontmatter(src)
	return n
}

func (a *app) openPath(rel string) {
	for i, n := range a.open {
		if n.rel == rel {
			a.active = i
			a.afterActive()
			return
		}
	}
	n := a.loadNote(rel)
	if n == nil {
		return
	}
	a.open = append(a.open, n)
	a.active = len(a.open) - 1
	a.afterActive()
}

// openAt opens rel in Source mode and selects start..start+length.
func (a *app) openAt(rel string, start, length int) {
	a.openPath(rel)
	n := a.activeNote()
	if n == nil {
		return
	}
	a.setMode(n, modeSource)
	end := start + length
	if end > len(n.src) {
		end = len(n.src)
	}
	n.st.Select(start, end)
	a.afterActive()
}

func (a *app) afterActive() {
	n := a.activeNote()
	if n == nil {
		a.lastActive = ""
		a.blinks = nil
		a.blinksFor = ""
		if a.win != nil {
			a.win.SetTitle("GoFolio")
			a.win.SetDocumentEdited(false)
		}
		a.saveSoon()
		return
	}
	if n.rel != a.lastActive {
		a.lastActive = n.rel
		a.refreshBacklinks()
	}
	if a.win != nil {
		a.win.SetTitle("GoFolio — " + pathBase(n.rel))
		a.win.SetRepresentedFile(n.abs)
		a.win.SetDocumentEdited(n.dirty())
	}
	a.saveSoon()
}

func (a *app) refreshBacklinks() {
	n := a.activeNote()
	if n == nil || a.v == nil {
		a.blinks = nil
		a.blinksFor = ""
		return
	}
	if a.blinksFor == n.rel {
		return
	}
	a.blinks = backlinksFor(a.v, n.rel)
	a.blinksFor = n.rel
}

func (a *app) syncNote(n *note) {
	if n.mode != modeSource || n.buf == nil {
		return
	}
	if v := n.buf.Version(); v != n.ver {
		n.ver = v
		n.src = n.buf.String()
		if n.src != n.saved {
			a.touch(n)
		}
	}
}

func (a *app) ensureLive(n *note) {
	if n.doc != nil && n.docFor == n.src {
		return
	}
	front, body := splitFrontmatter(n.src)
	n.front = front
	n.body = body
	n.doc = richtext.NewDocument(richtext.ParseMarkdown(linkify(body)))
	n.doc.SetSelection(0, 0)
	n.docFor = n.src
}

// liveUnsafe reports the first construct in src that the rich text editor
// cannot write back without losing it. Live mode is refused for those
// notes; Source mode always edits the file itself.
func liveUnsafe(src string) string {
	_, body := splitFrontmatter(src)
	lines := strings.Split(body, "\n")
	inFence := false
	for i, line := range lines {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~") {
			if !inFence {
				inFence = true
				if info := strings.TrimSpace(strings.TrimLeft(trim, "`~")); info != "" {
					return "a code block with a language tag"
				}
			} else {
				inFence = false
			}
			continue
		}
		if inFence {
			continue
		}
		if strings.Contains(line, "|") && i+1 < len(lines) && isTableDelimiter(lines[i+1]) {
			return "a table"
		}
		if isThematicBreak(trim) {
			return "a horizontal rule"
		}
		if strings.HasPrefix(trim, "<") && looksLikeHTML(trim) {
			return "HTML"
		}
		if strings.Contains(line, "![[") {
			// A drawing referenced with ![[sketch.tldr]] survives as a
			// plain image link, so Live mode may keep it; any other embed
			// (a note transclusion) would be lost.
			if !strings.Contains(line, ".tldr]]") {
				return "an embed"
			}
		}
		if strings.HasPrefix(trim, "$$") || strings.HasPrefix(trim, "[^") {
			return "math or a footnote"
		}
	}
	return ""
}

func isThematicBreak(s string) bool {
	if len(s) < 3 {
		return false
	}
	var mark rune
	for _, r := range s {
		if r == ' ' || r == '\t' {
			continue
		}
		if r != '-' && r != '*' && r != '_' {
			return false
		}
		if mark == 0 {
			mark = r
		} else if r != mark {
			return false
		}
	}
	return mark != 0
}

func isTableDelimiter(line string) bool {
	cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
	if len(cells) == 0 {
		return false
	}
	for _, cell := range cells {
		c := strings.TrimSpace(cell)
		c = strings.TrimPrefix(c, ":")
		c = strings.TrimSuffix(c, ":")
		if len(c) < 3 || strings.Trim(c, "-") != "" {
			return false
		}
	}
	return true
}

func looksLikeHTML(s string) bool {
	if strings.HasPrefix(s, "<!--") {
		return true
	}
	if len(s) < 2 || s[0] != '<' {
		return false
	}
	s = s[1:]
	if strings.HasPrefix(s, "/") {
		s = s[1:]
	}
	if s == "" {
		return false
	}
	c := s[0]
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '!'
}

func (a *app) setMode(n *note, m viewMode) {
	if n.draw != nil || n.mode == m {
		return
	}
	a.syncNote(n)
	if m == modeLive {
		if reason := liveUnsafe(n.src); reason != "" {
			a.flash("Live mode can't safely edit this note: it has " + reason + ". Use Source.")
			return
		}
		a.ensureLive(n)
	}
	if m == modeSource && n.buf.String() != n.src {
		n.buf.Set(n.src)
		n.ver = n.buf.Version()
	}
	n.mode = m
	if a.win != nil {
		a.win.SetDocumentEdited(n.dirty())
	}
	a.saveSoon()
}

func (a *app) liveChanged(n *note) {
	body := unlinkify(n.doc.Text().Markdown())
	if n.front != "" && strings.HasPrefix(n.body, "\n") && !strings.HasPrefix(body, "\n") {
		body = "\n" + body
	}
	if strings.HasSuffix(n.body, "\n") && !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	n.src = n.front + body
	n.docFor = n.src
	if a.win != nil {
		a.win.SetDocumentEdited(n.dirty())
	}
	if n.dirty() {
		a.touch(n)
	}
}

func (a *app) touch(n *note) {
	if n.timer != nil {
		n.timer.Stop()
	}
	n.timer = time.AfterFunc(700*time.Millisecond, func() {
		if a.win != nil {
			a.win.Update(func() { a.saveNote(n, false) })
		}
	})
}

func (a *app) saveNote(n *note, report bool) {
	if n.timer != nil {
		n.timer.Stop()
	}
	if n.draw != nil {
		if err := saveDrawing(n.draw.Editor(), n.abs); err != nil {
			a.flash("Save failed: " + err.Error())
			return
		}
		if report {
			a.flash("Saved " + pathBase(n.rel))
		}
		return
	}
	a.syncNote(n)
	if !n.dirty() {
		if report {
			a.flash("Already saved")
		}
		return
	}
	data := withEOL(n.src, n.eol)
	if err := writeFileAtomic(n.abs, []byte(data)); err != nil {
		a.flash("Save failed: " + err.Error())
		return
	}
	n.saved = n.src
	if n.mode == modeSource {
		n.buf.MarkSaved()
	}
	if a.win != nil {
		a.win.SetDocumentEdited(false)
	}
	if n.rel == a.lastActive {
		a.refreshBacklinks()
	}
	if report {
		a.flash("Saved " + pathBase(n.rel))
	}
}

func (a *app) flush() {
	for _, n := range a.open {
		if n.timer != nil {
			n.timer.Stop()
		}
		a.saveNote(n, false)
	}
}

func (a *app) saveSoon() {
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	a.saveTimer = time.AfterFunc(500*time.Millisecond, func() {
		if a.win != nil {
			a.win.Update(func() { a.saveState() })
		}
	})
}

func (a *app) currentSession() session {
	s := session{
		Vault:       a.vaultPath,
		Recents:     a.recents,
		Active:      a.active,
		Sidebar:     a.width,
		ShowSide:    a.sidebar,
		Collapsed:   a.collapsed,
		Backlinks:   a.backlinks,
		FullText:    a.fullText,
		DefaultMode: int(a.defaultMode),
		ModeSet:     a.modeSet,
		Spellcheck:  a.spellcheck,
		Pinned:      a.pinned,
		Graph:       a.graphState.settings(),
	}
	for _, n := range a.open {
		s.Open = append(s.Open, sessionTab{Path: n.rel, Mode: int(n.mode)})
	}
	return s
}

func (a *app) saveState() {
	if a.saveTimer != nil {
		a.saveTimer.Stop()
	}
	if err := saveSession(a.currentSession()); err != nil {
		log.Printf("saving state: %v", err)
	}
}

// statusDrawCount is the status bar's left text for a drawing.
func statusDrawCount(d *drawSurface) string {
	var shapes, strokes int
	for _, s := range d.ed.Doc.Shapes {
		if s.Type == doc.TypeDraw {
			strokes++
			continue
		}
		shapes++
	}
	if strokes == 0 && shapes == 0 {
		return "Empty drawing"
	}
	return fmt.Sprintf("%d shapes · %d strokes", shapes, strokes)
}

func (a *app) flash(msg string) {
	a.notice = msg
	if a.win != nil {
		a.win.Update(func() {})
	}
}

// --- sidebar selection ---------------------------------------------------

// selected reports whether a tree row is in the current selection.
func (a *app) selected(rel string) bool {
	return a.sel != nil && a.sel[rel]
}

// selectOne replaces the selection with a single row.
func (a *app) selectOne(rel string) {
	a.sel = map[string]bool{rel: true}
	a.selAnchor = rel
}

// clearSel empties the selection.
func (a *app) clearSel() {
	a.sel = nil
	a.selAnchor = ""
}

// selectAllVisible selects every row currently shown in the tree.
func (a *app) selectAllVisible() {
	if len(a.visOrder) == 0 {
		return
	}
	a.sel = map[string]bool{}
	for _, rel := range a.visOrder {
		a.sel[rel] = true
	}
	a.selAnchor = a.visOrder[0]
}

// toggleSel adds or removes one row, as Cmd/Ctrl-click does.
func (a *app) toggleSel(rel string) {
	if a.sel == nil {
		a.sel = map[string]bool{}
	}
	if a.sel[rel] {
		delete(a.sel, rel)
	} else {
		a.sel[rel] = true
	}
	a.selAnchor = rel
}

// rangeSel selects every visible row between the anchor and rel, as
// Shift-click does.
func (a *app) rangeSel(rel string) {
	if a.selAnchor == "" || a.selAnchor == rel {
		a.selectOne(rel)
		return
	}
	from, to := -1, -1
	for i, r := range a.visOrder {
		if r == a.selAnchor {
			from = i
		}
		if r == rel {
			to = i
		}
	}
	if from < 0 || to < 0 {
		a.selectOne(rel)
		return
	}
	if from > to {
		from, to = to, from
	}
	if a.sel == nil {
		a.sel = map[string]bool{}
	}
	for i := from; i <= to; i++ {
		a.sel[a.visOrder[i]] = true
	}
}

// clickRow handles a click on a tree row: plain opens it, modifier-click
// extends the selection, and a folder row toggles open on a plain click.
func (a *app) clickRow(mods ui.Modifiers, node *fileNode) {
	switch {
	case mods&ui.Cmd != 0:
		a.toggleSel(node.rel)
		return
	case mods&ui.Shift != 0:
		a.rangeSel(node.rel)
		return
	}
	if node.dir {
		o := a.dirOpen(node.rel)
		*o = !*o
		a.selectOne(node.rel)
		return
	}
	a.selectOne(node.rel)
	a.openPath(node.rel)
}

// trashSelection moves every selected row to the vault's trash.
func (a *app) trashSelection() {
	rels := a.selectedRels("")
	if len(rels) == 0 {
		if n := a.activeNote(); n != nil {
			a.deleteTarget = n.rel
			a.deleteOpen = true
		}
		return
	}
	a.flush()
	count := 0
	for _, rel := range rels {
		a.removeNoteTabs(rel)
		if err := a.v.trash(rel); err != nil {
			a.flash("Could not move " + pathBase(rel) + " to Trash")
			continue
		}
		count++
	}
	a.clearSel()
	a.rescan()
	a.lastActive = ""
	a.blinksFor = ""
	a.afterActive()
	if count == 1 {
		a.flash("Moved to .trash")
	} else {
		a.flash(fmt.Sprintf("Moved %d items to .trash", count))
	}
	a.saveSoon()
}

// applyDefaultMode puts a freshly made note into the default mode, keeping
// Source when Live cannot round-trip it.
func (a *app) applyDefaultMode(n *note) {
	if n == nil || n.draw != nil {
		return
	}
	m := a.defaultMode
	if m < modeSource || m > modeRead {
		m = modeLive
	}
	if m == modeLive && liveUnsafe(n.src) != "" {
		m = modeSource
	}
	n.mode = m
	if m == modeLive {
		a.ensureLive(n)
	}
}

func (a *app) newNote(parent string) {
	if a.v == nil {
		return
	}
	rel, err := a.v.createNote(parent, "Untitled")
	if err != nil {
		a.flash("Could not create note: " + err.Error())
		return
	}
	a.rescan()
	a.openPath(rel)
	a.applyDefaultMode(a.activeNote())
	a.saveSoon()
}

// newDrawing creates an empty drawing next to the notes and opens it.
func (a *app) newDrawing(parent string) {
	if a.v == nil {
		return
	}
	rel, err := a.v.createDrawing(parent, "Drawing")
	if err != nil {
		a.flash("Could not create drawing: " + err.Error())
		return
	}
	a.rescan()
	a.openPath(rel)
	a.flash("Drawing: drag to pan, scroll to zoom, P to draw")
}

func (a *app) newFolder(parent string) {
	if a.v == nil {
		return
	}
	name, err := a.v.createFolder(parent, "New folder")
	if err != nil {
		a.flash("Could not create folder: " + err.Error())
		return
	}
	a.rescan()
	*a.dirOpen(name) = true
	a.flash("Created " + name)
}

func (a *app) rescan() {
	if a.vaultPath == "" {
		return
	}
	if v, err := scanVault(a.vaultPath); err == nil {
		a.v = v
		a.blinksFor = ""
		a.graphState = nil
		a.drawCache = nil
	}
}

// embeddedDrawing returns a bitmap of a .tldr a note embeds, cached by path.
// It returns nil for a path the vault does not hold or that will not render.
func (a *app) embeddedDrawing(rel string) *ui.Bitmap {
	if rel == "" {
		return nil
	}
	if b, ok := a.drawCache[rel]; ok {
		return b
	}
	if a.drawCache == nil {
		a.drawCache = map[string]*ui.Bitmap{}
	}
	abs, err := a.v.abs(rel)
	if err != nil {
		a.drawCache[rel] = nil
		return nil
	}
	b, err := renderDrawingBitmap(abs)
	if err != nil {
		b = nil
	}
	a.drawCache[rel] = b
	return b
}

// resolveDraw resolves a draw: link target from a note to a vault path.
func (a *app) resolveDraw(fromRel, target string) string {
	if a.v == nil {
		return ""
	}
	if dest, ok := a.v.resolveLink(fromRel, target); ok {
		return dest
	}
	if isDraw(target) && a.v.has(target) {
		return target
	}
	return ""
}

// toggleGraph opens the vault's link graph as the main pane, or closes it.
func (a *app) toggleGraph() {
	if a.v == nil {
		return
	}
	a.graph = !a.graph
	if a.graph {
		// Leaving note tabs open keeps their state; the graph just takes
		// the pane until it is closed.
		a.graphState = nil
		a.flash("Graph: drag to pan, scroll to zoom, click a node to open it")
	}
	if a.win != nil {
		a.win.Update(func() {})
	}
	a.saveSoon()
}

func (a *app) closeTab(i int) {
	if i < 0 || i >= len(a.open) {
		return
	}
	n := a.open[i]
	if n.timer != nil {
		n.timer.Stop()
	}
	a.saveNote(n, false)
	a.open = append(a.open[:i], a.open[i+1:]...)
	if i < a.active {
		a.active--
	} else if i == a.active && a.active >= len(a.open) {
		a.active = len(a.open) - 1
	}
	if a.active < 0 && len(a.open) > 0 {
		a.active = 0
	}
	a.lastActive = ""
	a.afterActive()
}

func (a *app) removeNoteTabs(rel string) {
	var kept []*note
	for _, n := range a.open {
		if n.rel == rel || strings.HasPrefix(n.rel, rel+"/") {
			if n.timer != nil {
				n.timer.Stop()
			}
			continue
		}
		kept = append(kept, n)
	}
	a.open = kept
	if a.active >= len(a.open) {
		a.active = len(a.open) - 1
	}
	a.lastActive = ""
	a.afterActive()
}

func (a *app) submitPrompt() {
	kind := a.promptKind
	value := a.promptValue
	parent := a.promptParent
	target := a.promptTarget
	a.promptOpen = false
	a.promptValue = ""
	if a.v == nil || strings.TrimSpace(value) == "" {
		return
	}
	switch kind {
	case promptNewNote:
		rel, err := a.v.createNote(parent, value)
		if err != nil {
			a.flash(err.Error())
			return
		}
		a.rescan()
		a.openPath(rel)
		a.applyDefaultMode(a.activeNote())
	case promptNewFolder:
		rel, err := a.v.createFolder(parent, value)
		if err != nil {
			a.flash(err.Error())
			return
		}
		a.rescan()
		*a.dirOpen(rel) = true
	case promptRename:
		next, err := a.v.rename(target, value)
		if err != nil {
			a.flash(err.Error())
			return
		}
		for _, n := range a.open {
			if n.rel == target || strings.HasPrefix(n.rel, target+"/") {
				n.rel = next + n.rel[len(target):]
				if abs, err := a.v.abs(n.rel); err == nil {
					n.abs = abs
				}
			}
		}
		a.rescan()
		a.lastActive = ""
		a.blinksFor = ""
		a.afterActive()
	}
	a.saveSoon()
}

func (a *app) deleteNow() {
	rel := a.deleteTarget
	a.deleteOpen = false
	a.deleteTarget = ""
	if a.v == nil || rel == "" {
		return
	}
	a.flush()
	a.removeNoteTabs(rel)
	if err := a.v.trash(rel); err != nil {
		a.flash("Could not move to Trash: " + err.Error())
		return
	}
	a.rescan()
	a.lastActive = ""
	a.afterActive()
	a.flash("Moved to .trash")
	a.saveSoon()
}

// moveEntry moves a tree entry into destDir and follows it if it carried the
// active note.
func (a *app) moveEntry(rel, destDir string) {
	if a.v == nil || rel == "" || rel == destDir {
		return
	}
	if destDir != "" && (destDir == rel || strings.HasPrefix(destDir+"/", rel+"/")) {
		a.flash("A folder cannot move inside itself")
		return
	}
	if pathDir(rel) == destDir {
		return
	}
	next, err := a.v.move(rel, destDir)
	if err != nil {
		a.flash("Could not move: " + err.Error())
		return
	}
	a.retargetTabs(rel, next)
	if destDir != "" {
		*a.dirOpen(destDir) = true
	}
	a.rescan()
	a.lastActive = ""
	a.blinksFor = ""
	a.afterActive()
	a.flash("Moved to " + dropName(destDir))
	a.saveSoon()
}

// retargetTabs updates open tabs after rel moved to next.
func (a *app) retargetTabs(rel, next string) {
	if rel == next {
		return
	}
	for _, n := range a.open {
		if n.rel == rel || strings.HasPrefix(n.rel, rel+"/") {
			n.rel = next + n.rel[len(rel):]
			if abs, err := a.v.abs(n.rel); err == nil {
				n.abs = abs
			}
		}
	}
}

// dropName names a destination folder for a message.
func dropName(destDir string) string {
	if destDir == "" {
		return "the vault root"
	}
	return pathBase(destDir)
}

// duplicateSelection duplicates every selected row.
func (a *app) duplicateSelection() {
	rels := a.selectedRels("")
	if len(rels) == 0 {
		return
	}
	for _, rel := range rels {
		a.duplicateEntry(rel)
	}
	a.clearSel()
}

// duplicateEntry copies a file or folder beside itself with a "copy" name.
func (a *app) duplicateEntry(rel string) {
	if a.v == nil || rel == "" {
		return
	}
	src, err := a.v.abs(rel)
	if err != nil {
		a.flash(err.Error())
		return
	}
	parent := pathDir(rel)
	name := pathBase(rel)
	base, ext := name, ""
	if isNote(rel) || isDraw(rel) {
		ext = filepath.Ext(name)
		base = strings.TrimSuffix(name, ext)
	}
	dest := a.v.uniqueRel(parent, base+" copy", ext)
	dp, err := a.v.abs(dest)
	if err != nil {
		a.flash(err.Error())
		return
	}
	if err := copyPath(src, dp); err != nil {
		a.flash("Could not duplicate: " + err.Error())
		return
	}
	a.rescan()
	a.flash("Duplicated " + name)
	a.saveSoon()
}

// copyPath copies a file, or a folder tree, from src to dest.
func copyPath(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		b, err := os.ReadFile(src)
		if err != nil {
			return err
		}
		return os.WriteFile(dest, b, info.Mode().Perm())
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyPath(filepath.Join(src, e.Name()), filepath.Join(dest, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// copyPathToClipboard puts a vault path on the clipboard as text.
func (a *app) copyPathToClipboard(rel string) {
	if a.v == nil {
		return
	}
	abs, err := a.v.abs(rel)
	if err != nil {
		return
	}
	mygo.Clipboard.WriteText(abs)
	a.flash("Path copied")
}

// renameEntry opens the rename prompt for a vault path.
func (a *app) renameEntry(rel string) {
	if a.v == nil || rel == "" {
		return
	}
	a.promptKind = promptRename
	a.promptTitle = "Rename “" + trimExt(pathBase(rel)) + "”"
	a.promptValue = trimExt(pathBase(rel))
	a.promptTarget = rel
	a.promptOpen = true
}

// renameActive renames the active tab's entry, if any.
func (a *app) renameActive() {
	if n := a.activeNote(); n != nil {
		a.renameEntry(n.rel)
	}
}

// newTab opens a fresh untitled note at the vault root.
func (a *app) newTab() {
	a.newNote("")
}

// revealActive opens the folders leading to the active note in the tree.
func (a *app) revealActive() {
	n := a.activeNote()
	if n == nil {
		return
	}
	parts := strings.Split(n.rel, "/")
	acc := ""
	for i := 0; i < len(parts)-1; i++ {
		if acc == "" {
			acc = parts[i]
		} else {
			acc += "/" + parts[i]
		}
		*a.dirOpen(acc) = true
	}
	a.focusPath = n.rel
	a.saveSoon()
}

// toggleSort cycles the tree sort key: name, modified, then reversed name.
func (a *app) toggleSort() {
	a.sortKey = (a.sortKey + 1) % 3
	a.flash("Sorted by " + a.sortLabel())
	a.saveSoon()
}

func (a *app) sortLabel() string {
	switch a.sortKey {
	case 1:
		return "modified time"
	case 2:
		return "name (Z–A)"
	default:
		return "name (A–Z)"
	}
}

// collapseAll folds every open folder in the tree.
func (a *app) collapseAll() {
	for k := range a.openDirs {
		*a.openDirs[k] = false
	}
	a.saveSoon()
}

// createVault makes a new empty folder and opens it as a vault.
func (a *app) createVault() {
	go func() {
		paths, err := mygo.Dialog.Save(mygo.SaveDialogOptions{
			Parent:      a.win,
			Title:       "Create a new vault folder",
			ButtonLabel: "Create",
			DefaultPath: "My Vault",
		})
		if err != nil || paths == "" {
			return
		}
		if err := os.MkdirAll(paths, 0o755); err != nil {
			if a.win != nil {
				a.win.Update(func() { a.flash("Could not create vault: " + err.Error()) })
			}
			return
		}
		if a.win != nil {
			a.win.Update(func() {
				if err := a.setVault(paths, nil); err != nil {
					a.flash(err.Error())
				} else {
					a.flash("Created " + filepath.Base(paths))
				}
			})
		}
	}()
}

func (a *app) chooseVault() {
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      a.win,
			Title:       "Open a folder of Markdown notes",
			ButtonLabel: "Open Vault",
			Directory:   true,
		})
		if err != nil || len(paths) == 0 {
			return
		}
		if a.win != nil {
			a.win.Update(func() {
				if err := a.setVault(paths[0], nil); err != nil {
					a.flash(err.Error())
				} else {
					a.flash("Opened " + filepath.Base(paths[0]))
				}
			})
		}
	}()
}

func (a *app) refreshSearch() {
	if a.v == nil {
		a.hits = nil
		return
	}
	if !a.fullText || strings.TrimSpace(a.query) == "" {
		a.hits = nil
		a.hitsFor = ""
		return
	}
	if a.hitsFor == a.query {
		return
	}
	a.hits = searchVault(a.v, a.query, 150)
	a.hitsFor = a.query
}

func (a *app) filterNotes(q string) []*fileNode {
	type scored struct {
		node  *fileNode
		score int
	}
	var ranked []scored
	for _, rel := range a.v.notes {
		score, ok := fuzzyScore(q, rel)
		if !ok {
			continue
		}
		ranked = append(ranked, scored{node: &fileNode{name: pathBase(rel), rel: rel}, score: score})
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].score != ranked[j].score {
			return ranked[i].score > ranked[j].score
		}
		return strings.ToLower(ranked[i].node.rel) < strings.ToLower(ranked[j].node.rel)
	})
	if len(ranked) > 200 {
		ranked = ranked[:200]
	}
	out := make([]*fileNode, len(ranked))
	for i, r := range ranked {
		out[i] = r.node
	}
	return out
}

// --- palette -------------------------------------------------------------

func (a *app) commands() []paletteItem {
	items := []paletteItem{
		{label: "New note", detail: "Create an empty note", hint: "⌘N", run: func() { a.newNote("") }},
		{label: "New folder", detail: "Create a folder in the vault", hint: "⇧⌘N", run: func() { a.newFolder("") }},
		{label: "New drawing", detail: "Create a blank drawing", hint: "⇧⌘D", run: func() { a.newDrawing("") }},
		{label: "Open vault…", detail: "Choose a folder to open", hint: "⌘O", run: a.chooseVault},
		{label: "Toggle sidebar", detail: "Show or hide the file sidebar", hint: "⌘\\", run: func() { a.sidebar = !a.sidebar; a.saveSoon() }},
		{label: "Toggle full-text search", detail: "Search inside every note", hint: "⇧⌘F", run: func() { a.fullText = !a.fullText; a.query = ""; a.hits = nil }},
		{label: "Graph view", detail: "Show the vault's link graph", hint: "⌘G", run: a.toggleGraph},
		{label: "Reload vault", detail: "Scan the folder again", hint: "⌘R", run: func() { a.rescan(); a.flash("Vault reloaded") }},
		{label: "Sort tree", detail: "Cycle name, modified, name reversed", hint: "", run: a.toggleSort},
		{label: "Collapse all folders", detail: "Fold every open folder", hint: "", run: a.collapseAll},
		{label: "Reveal active note", detail: "Expand folders to the active note", hint: "", run: a.revealActive},
		{label: "Select all in sidebar", detail: "Select every row in the file list", hint: "⌘A", run: a.selectAllVisible},
		{label: "Settings…", detail: "Editor, files, appearance", hint: "⌘,", run: func() { a.settingsOpen = true; a.setTab = 0 }},
		{label: "Vaults…", detail: "Switch, open or create a vault", hint: "", run: func() { a.vaultsOpen = true }},
	}
	if n := a.activeNote(); n != nil {
		if n.draw == nil {
			items = append(items,
				paletteItem{label: "Switch to Source", detail: n.rel, hint: "⌘1", run: func() { a.setMode(n, modeSource) }},
				paletteItem{label: "Switch to Live", detail: n.rel, hint: "⌘2", run: func() { a.setMode(n, modeLive) }},
				paletteItem{label: "Switch to Read", detail: n.rel, hint: "⌘3", run: func() { a.setMode(n, modeRead) }},
			)
		}
		items = append(items,
			paletteItem{label: "Rename note…", detail: n.rel, hint: "F2", run: func() { a.renameEntry(n.rel) }},
			paletteItem{label: "Duplicate note", detail: n.rel, hint: "", run: func() { a.duplicateEntry(n.rel) }},
			paletteItem{label: "Copy path", detail: n.rel, hint: "", run: func() { a.copyPathToClipboard(n.rel) }},
			paletteItem{label: "Move note to Trash…", detail: n.rel, hint: "", run: func() {
				a.deleteOpen = true
				a.deleteTarget = n.rel
			}},
			paletteItem{label: "Close tab", detail: n.rel, hint: "⌘W", run: func() { a.closeTab(a.active) }},
		)
	}
	return items
}

func (a *app) buildPalette() {
	q := strings.TrimSpace(a.paletteQuery)
	var items []paletteItem
	if strings.HasPrefix(q, ">") {
		cmdq := strings.TrimSpace(q[1:])
		for _, it := range a.commands() {
			if score, ok := fuzzyScore(cmdq, it.label); ok {
				items = append(items, it)
				_ = score
			}
		}
	} else {
		if a.v != nil {
			type ranked struct {
				item  paletteItem
				score int
			}
			var rankedNotes []ranked
			for _, rel := range a.v.notes {
				score, ok := fuzzyScore(q, rel)
				if !ok {
					continue
				}
				rel := rel
				rankedNotes = append(rankedNotes, ranked{
					item: paletteItem{
						label:  pathBase(rel),
						detail: pathDir(rel),
						run:    func() { a.openPath(rel) },
					},
					score: score,
				})
			}
			sort.SliceStable(rankedNotes, func(i, j int) bool {
				if rankedNotes[i].score != rankedNotes[j].score {
					return rankedNotes[i].score > rankedNotes[j].score
				}
				return strings.ToLower(rankedNotes[i].item.label) < strings.ToLower(rankedNotes[j].item.label)
			})
			for _, r := range rankedNotes {
				items = append(items, r.item)
				if len(items) == 40 {
					break
				}
			}
		}
		if q == "" {
			for _, it := range a.commands() {
				it.detail = "Command · " + it.detail
				items = append(items, it)
			}
		}
	}
	if len(items) > 10 {
		items = items[:10]
	}
	a.paletteItems = items
	if a.paletteIndex >= len(items) {
		a.paletteIndex = max(0, len(items)-1)
	}
	if a.paletteIndex < 0 {
		a.paletteIndex = 0
	}
}

func (a *app) runPalette() {
	if a.paletteIndex >= 0 && a.paletteIndex < len(a.paletteItems) {
		it := a.paletteItems[a.paletteIndex]
		a.paletteOpen = false
		a.paletteQuery = ""
		a.paletteIndex = 0
		a.paletteItems = nil
		it.run()
	}
}

func fuzzyScore(query, s string) (int, bool) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return 0, true
	}
	s = strings.ToLower(s)
	if i := strings.Index(s, q); i >= 0 {
		return 1000 - i*3 - len(s), true
	}
	// Subsequence match: every rune of the query appears in order.
	score, last := 0, -1
	for _, r := range q {
		at := strings.IndexRune(s[last+1:], r)
		if at < 0 {
			return 0, false
		}
		at += last + 1
		if last >= 0 && at == last+1 {
			score += 8
		}
		score += 4
		last = at
	}
	return score - len(s), true
}

// --- menu ----------------------------------------------------------------

func (a *app) setupMenu() {
	mygo.App.SetMenu(mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu},
		{Label: "File", Submenu: []*mygo.MenuItem{
			{Label: "New Note", Accelerator: "CmdOrCtrl+N", Click: func(*mygo.MenuItem, *mygo.Window) { a.newNote("") }},
			{Label: "New Folder", Accelerator: "CmdOrCtrl+Shift+N", Click: func(*mygo.MenuItem, *mygo.Window) { a.newFolder("") }},
			{Label: "New Drawing", Accelerator: "CmdOrCtrl+Shift+D", Click: func(*mygo.MenuItem, *mygo.Window) { a.newDrawing("") }},
			{Type: mygo.MenuItemSeparator},
			{Label: "Open Vault…", Accelerator: "CmdOrCtrl+O", Click: func(*mygo.MenuItem, *mygo.Window) { a.chooseVault() }},
			{Type: mygo.MenuItemSeparator},
			{Role: mygo.RoleClose},
		}},
		{Role: mygo.RoleEditMenu},
		{Label: "View", Submenu: []*mygo.MenuItem{
			{Label: "Quick Switcher…", Accelerator: "CmdOrCtrl+P", Click: func(*mygo.MenuItem, *mygo.Window) { a.openPalette() }},
			{Label: "Search in Notes", Accelerator: "CmdOrCtrl+Shift+F", Click: func(*mygo.MenuItem, *mygo.Window) { a.fullTextSearch() }},
			{Label: "Graph View", Accelerator: "CmdOrCtrl+G", Click: func(*mygo.MenuItem, *mygo.Window) { a.toggleGraph() }},
			{Type: mygo.MenuItemSeparator},
			{Label: "Source", Accelerator: "CmdOrCtrl+1", Click: func(*mygo.MenuItem, *mygo.Window) { a.modeCommand(modeSource) }},
			{Label: "Live", Accelerator: "CmdOrCtrl+2", Click: func(*mygo.MenuItem, *mygo.Window) { a.modeCommand(modeLive) }},
			{Label: "Read", Accelerator: "CmdOrCtrl+3", Click: func(*mygo.MenuItem, *mygo.Window) { a.modeCommand(modeRead) }},
			{Type: mygo.MenuItemSeparator},
			{Label: "Toggle Sidebar", Accelerator: "CmdOrCtrl+\\", Click: func(*mygo.MenuItem, *mygo.Window) {
				a.sidebar = !a.sidebar
				a.saveSoon()
			}},
		}},
		{Role: mygo.RoleWindowMenu},
	}))
}

func (a *app) openPalette() {
	a.paletteOpen = true
	a.paletteQuery = ""
	a.paletteIndex = 0
}

func (a *app) fullTextSearch() {
	a.fullText = true
	a.sidebar = true
	a.searchH.Focus()
}

func (a *app) modeCommand(m viewMode) {
	if n := a.activeNote(); n != nil {
		a.setMode(n, m)
	}
}

func main() {
	a := &app{}
	a.restore()
	mygo.App.WhenReady(func() {
		a.setupMenu()
		a.win = mygo.NewWindow(mygo.WindowOptions{
			Title:             "GoFolio",
			Width:             1180,
			Height:            780,
			MinWidth:          720,
			MinHeight:         480,
			StateKey:          "gofolio",
			TitleBarStyle:     mygo.TitleBarHidden,
			HideWindowButtons: runtime.GOOS != "darwin",
			BackgroundColor:   "light-dark(#fcfcfb, #1a1a19)",
			Content:           ui.View(a.view),
		})
		a.win.OnClose(func(*mygo.CloseEvent) {
			a.flush()
			a.saveState()
		})
		a.afterActive()
	})
	mygo.App.OnQuit(func() {
		a.flush()
		a.saveState()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
