package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egoist/mygo/ui"
)

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestVaultMoveAndCopy(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n")
	write(t, filepath.Join(dir, "sub", "Beta.md"), "# Beta\n")
	v, err := scanVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Move Alpha into sub/.
	next, err := v.move("Alpha.md", "sub")
	if err != nil {
		t.Fatal(err)
	}
	if next != "sub/Alpha.md" {
		t.Fatalf("move = %q, want sub/Alpha.md", next)
	}
	if _, err := os.Stat(filepath.Join(dir, "Alpha.md")); !os.IsNotExist(err) {
		t.Fatalf("Alpha.md still at root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub", "Alpha.md")); err != nil {
		t.Fatal(err)
	}
	// A name clash is respelled, not overwritten.
	clash, err := v.move("sub/Beta.md", "sub")
	if err != nil {
		t.Fatal(err)
	}
	if clash != "sub/Beta.md" {
		t.Fatalf("same-folder move = %q, want unchanged", clash)
	}
	// A folder cannot move inside itself.
	if _, err := v.move("sub", "sub/deeper"); err == nil {
		t.Fatal("moving a folder into its own subtree did not fail")
	}
	// Copy a file, then a folder tree.
	src, _ := v.abs("sub/Alpha.md")
	dst, _ := v.abs("sub/Alpha copy.md")
	if err := copyPath(src, dst); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "sub", "Alpha copy.md")); string(got) != "# Alpha\n" {
		t.Fatalf("copy content = %q", got)
	}
}

func TestVaultScanAndFileOps(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n")
	write(t, filepath.Join(dir, "folder", "Beta.markdown"), "# Beta\n")
	write(t, filepath.Join(dir, "notes.txt"), "not a note")
	write(t, filepath.Join(dir, ".git", "config"), "skip")
	write(t, filepath.Join(dir, ".trash", "Old.md"), "skip")

	v, err := scanVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(v.notes); got != 2 {
		t.Fatalf("notes = %v, want 2", v.notes)
	}
	if len(v.nodes) != 2 || !v.nodes[0].dir {
		t.Fatalf("nodes = %+v, want a folder then a note", v.nodes)
	}

	first, err := v.createNote("folder", "Untitled")
	if err != nil {
		t.Fatal(err)
	}
	if first != "folder/Untitled.md" {
		t.Fatalf("first = %q", first)
	}
	second, err := v.createNote("folder", "Untitled")
	if err != nil {
		t.Fatal(err)
	}
	if second != "folder/Untitled 2.md" {
		t.Fatalf("second = %q", second)
	}

	renamed, err := v.rename(first, "Hello")
	if err != nil {
		t.Fatal(err)
	}
	if renamed != "folder/Hello.md" {
		t.Fatalf("renamed = %q", renamed)
	}
	if _, err := os.Stat(filepath.Join(dir, "folder", "Hello.md")); err != nil {
		t.Fatal(err)
	}
	if err := v.trash(renamed); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "folder", "Hello.md")); !os.IsNotExist(err) {
		t.Fatalf("trashed note still exists: %v", err)
	}
	entries, _ := os.ReadDir(filepath.Join(dir, ".trash"))
	if len(entries) != 2 {
		t.Fatalf(".trash holds %d entries, want 2", len(entries))
	}
}

func TestFrontmatter(t *testing.T) {
	src := "---\ntitle: Hello\ntags: [a]\n---\n\n# Body\n"
	front, body := splitFrontmatter(src)
	if front != "---\ntitle: Hello\ntags: [a]\n---\n" {
		t.Fatalf("front = %q", front)
	}
	if body != "\n# Body\n" {
		t.Fatalf("body = %q", body)
	}
	if f, b := splitFrontmatter("# No front\n"); f != "" || b != "# No front\n" {
		t.Fatalf("split without frontmatter = %q, %q", f, b)
	}
	if got := withEOL("a\nb\n", "\r\n"); got != "a\r\nb\r\n" {
		t.Fatalf("withEOL = %q", got)
	}
}

func TestWikilinks(t *testing.T) {
	body := "See [[Alpha]], [[Alpha|the first]] and [[folder/Beta#Intro|Beta]]."
	md := linkify(body)
	for _, want := range []string{"[Alpha](note:Alpha)", "[the first](note:Alpha)", "[Beta](note:folder/Beta#Intro)"} {
		if !strings.Contains(md, want) {
			t.Fatalf("%q missing from %q", want, md)
		}
	}
	back := unlinkify(md)
	if back != body {
		t.Fatalf("round trip = %q, want %q", back, body)
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "")
	write(t, filepath.Join(dir, "folder", "Beta.md"), "")
	v, _ := scanVault(dir)
	if rel, ok := v.resolveLink("folder/Beta.md", "note:Alpha"); !ok || rel != "Alpha.md" {
		t.Fatalf("resolve Alpha = %q, %v", rel, ok)
	}
	if rel, ok := v.resolveLink("Alpha.md", "Beta.md"); !ok || rel != "folder/Beta.md" {
		t.Fatalf("resolve Beta = %q, %v", rel, ok)
	}
	// A path-qualified link without its extension is how wikilinks usually
	// name a note in a subfolder: [[folder/Beta]] must find folder/Beta.md.
	if rel, ok := v.resolveLink("Alpha.md", "folder/Beta"); !ok || rel != "folder/Beta.md" {
		t.Fatalf("resolve folder/Beta = %q, %v", rel, ok)
	}
	if rel, ok := v.resolveLink("other/Note.md", "folder/Beta"); !ok || rel != "folder/Beta.md" {
		t.Fatalf("resolve folder/Beta from elsewhere = %q, %v", rel, ok)
	}
}

func TestSearchAndBacklinks(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n\nA needle hides here.\n")
	write(t, filepath.Join(dir, "folder", "Beta.md"), "Links to [[Alpha]] and [[Alpha|first]].\n")
	write(t, filepath.Join(dir, "folder", "Gamma.md"), "Nothing.\n")
	v, _ := scanVault(dir)

	hits := searchVault(v, "needle", 10)
	if len(hits) != 1 || hits[0].rel != "Alpha.md" || hits[0].line != 3 {
		t.Fatalf("hits = %+v", hits)
	}
	if got := hits[0]; got.length != len("needle") {
		t.Fatalf("hit range = %+v", got)
	}
	src, _ := os.ReadFile(filepath.Join(dir, "Alpha.md"))
	if got := string(src)[hits[0].start : hits[0].start+hits[0].length]; got != "needle" {
		t.Fatalf("hit text = %q", got)
	}

	links := backlinksFor(v, "Alpha.md")
	if len(links) != 1 || links[0].rel != "folder/Beta.md" {
		t.Fatalf("backlinks = %+v", links)
	}
}

func TestCreateNoteExtension(t *testing.T) {
	dir := t.TempDir()
	v, err := scanVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	rel, err := v.createNote("", "Ideas.md")
	if err != nil {
		t.Fatal(err)
	}
	if rel != "Ideas.md" {
		t.Fatalf("rel = %q, want Ideas.md", rel)
	}
}

func TestCRLFNote(t *testing.T) {
	dir := t.TempDir()
	raw := "---\r\ntitle: x\r\n---\r\n\r\nBody text\r\n"
	write(t, filepath.Join(dir, "Win.md"), raw)
	a := &app{sidebar: true}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Win.md")
	n := a.activeNote()
	if n.eol != "\r\n" {
		t.Fatalf("eol = %q", n.eol)
	}
	if n.src != "---\ntitle: x\n---\n\nBody text\n" {
		t.Fatalf("src = %q", n.src)
	}
	if n.front != "---\ntitle: x\n---\n" || n.body != "\nBody text\n" {
		t.Fatalf("front = %q body = %q", n.front, n.body)
	}
	hits := searchVault(a.v, "Body", 5)
	if len(hits) != 1 {
		t.Fatalf("hits = %+v", hits)
	}
	if got := n.src[hits[0].start : hits[0].start+hits[0].length]; got != "Body" {
		t.Fatalf("offset %d points at %q", hits[0].start, got)
	}
	n.buf.Set("---\ntitle: x\n---\n\nChanged\n")
	a.saveNote(n, false)
	b, _ := os.ReadFile(filepath.Join(dir, "Win.md"))
	if string(b) != "---\r\ntitle: x\r\n---\r\n\r\nChanged\r\n" {
		t.Fatalf("saved = %q", b)
	}
}

func TestLiveSafety(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		unsafe bool
	}{
		{"prose", "# Title\n\nA paragraph with **bold** and a [link](https://example.com).\n", false},
		{"lists", "- one\n- [ ] two\n", false},
		{"quote", "> quoted\n", false},
		{"plain fence", "```\n| a | b |\n| --- | --- |\n```\n", false},
		{"table", "| a | b |\n| --- | --- |\n| 1 | 2 |\n", true},
		{"rule", "before\n\n---\n\nafter\n", true},
		{"html", "<div>raw</div>\n", true},
		{"embed", "![[Other]]\n", true},
		{"language fence", "```go\nx := 1\n```\n", true},
		{"frontmatter table", "---\nt: x\n---\n\n| a | b |\n| --- | --- |\n", true},
	}
	for _, tc := range cases {
		got := liveUnsafe(tc.body)
		if (got != "") != tc.unsafe {
			t.Errorf("%s: liveUnsafe = %q, want unsafe %v", tc.name, got, tc.unsafe)
		}
	}
}

func TestLiveModeRefused(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Table.md"), "| a | b |\n| --- | --- |\n| 1 | 2 |\n")
	a := &app{sidebar: true}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Table.md")
	n := a.activeNote()
	a.setMode(n, modeLive)
	if n.mode != modeSource {
		t.Fatalf("mode = %v, want source", n.mode)
	}
	if a.notice == "" {
		t.Fatal("no notice after refusing Live mode")
	}
}

func TestSidebarOps(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Zeta.md"), "# Zeta\n")
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n")
	write(t, filepath.Join(dir, "folder", "Beta.md"), "# Beta\n")

	a := &app{sidebar: true}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	// Move Alpha into folder via the sidebar drag-target path.
	a.moveEntry("Alpha.md", "folder")
	if _, err := os.Stat(filepath.Join(dir, "folder", "Alpha.md")); err != nil {
		t.Fatalf("Alpha did not move into folder: %v", err)
	}
	// The active-note retarget only matters if it was open; open then move.
	a.openPath("folder/Alpha.md")
	a.moveEntry("folder/Alpha.md", "")
	if _, err := os.Stat(filepath.Join(dir, "Alpha.md")); err != nil {
		t.Fatalf("Alpha did not move back to root: %v", err)
	}
	if got := a.activeNote().rel; got != "Alpha.md" {
		t.Fatalf("active tab not retargeted: %q", got)
	}
	// duplicate and copy-path operate on the active note.
	a.duplicateEntry("Alpha.md")
	if _, err := os.Stat(filepath.Join(dir, "Alpha copy.md")); err != nil {
		t.Fatalf("duplicate missing: %v", err)
	}
	// Sort by modified time keeps folders before files.
	a.toggleSort()
	ordered := a.order(a.v.nodes)
	if len(ordered) == 0 || !ordered[0].dir {
		t.Fatalf("folders should sort first, got %+v", ordered)
	}
	for _, n := range ordered[1:] {
		if n.dir {
			t.Fatalf("folder %q sorted after a file", n.name)
		}
	}
	// Rename via the shared helper.
	a.renameEntry("Zeta.md")
	if !a.promptOpen || a.promptKind != promptRename || a.promptTarget != "Zeta.md" {
		t.Fatalf("renameEntry did not arm the prompt: %+v", a)
	}
}

func TestSidebarDragMove(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n")
	write(t, filepath.Join(dir, "folder", "Beta.md"), "# Beta\n")

	a := &app{sidebar: true, width: 232}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	*a.dirOpen("folder") = true
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()

	src, ok := tt.Find("Alpha.md")
	if !ok {
		t.Fatalf("Alpha row not found; texts = %q", tt.Texts())
	}
	dst, ok := tt.Find("Beta.md")
	if !ok {
		t.Fatalf("folder row not found")
	}
	// Drag Alpha onto the folder's Beta row: press, pass the drag threshold,
	// release over a row inside the folder.
	tt.Press(src.X+src.W/2, src.Y+src.H/2)
	tt.Frame()
	tt.Move(src.X+src.W/2+15, src.Y+src.H/2+15)
	tt.Frame()
	tt.Move(dst.X+dst.W/2, dst.Y+dst.H/2)
	tt.Frame()
	tt.Release(dst.X+dst.W/2, dst.Y+dst.H/2)
	tt.Frame()
	tt.Frame()
	if _, err := os.Stat(filepath.Join(dir, "folder", "Alpha.md")); err != nil {
		t.Fatalf("drag did not move Alpha into folder: %v", err)
	}
}

// TestSidebarMultiSelect covers Cmd-click multi-selection, Shift ranges, and
// moving several selected rows at once.
func TestSidebarMultiSelect(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n")
	write(t, filepath.Join(dir, "Beta.md"), "# Beta\n")
	write(t, filepath.Join(dir, "Gamma.md"), "# Gamma\n")
	write(t, filepath.Join(dir, "folder", "Delta.md"), "# Delta\n")

	a := &app{sidebar: true, width: 232}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()

	click := func(mods ui.Modifiers, rel string) {
		b, ok := a.rowBoxes[rel]
		if !ok {
			t.Fatalf("no row box for %s", rel)
		}
		tt.ClickAtWith(mods, b.X+b.W/2, b.Y+b.H/2)
		tt.Frame()
		tt.Frame()
	}
	click(0, "Alpha.md")
	if got := a.selectedRels(""); len(got) != 1 || got[0] != "Alpha.md" {
		t.Fatalf("plain click selection = %v", got)
	}
	click(ui.Cmd, "Beta.md")
	if got := a.selectedRels(""); len(got) != 2 {
		t.Fatalf("cmd-click selection = %v, want 2", got)
	}
	// Shift-click extends the visible range from the anchor.
	click(ui.Shift, "Gamma.md")
	if got := len(a.selectedRels("")); got != 3 {
		t.Fatalf("shift-click selection = %d, want 3", got)
	}
	// Cmd+A selects everything visible.
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Frame()
	tt.Frame()
	if got := len(a.selectedRels("")); got != 5 {
		t.Fatalf("select-all = %d, want 5", got)
	}
	// Multi-select then move: all three files land in folder.
	a.clearSel()
	click(ui.Cmd, "Alpha.md")
	click(ui.Cmd, "Beta.md")
	click(ui.Cmd, "Gamma.md")
	a.dragRel = "" // no explicit source; moveSelection takes the selection
	a.moveSelection("", "folder")
	for _, name := range []string{"Alpha.md", "Beta.md", "Gamma.md"} {
		if _, err := os.Stat(filepath.Join(dir, "folder", name)); err != nil {
			t.Fatalf("%s did not move: %v", name, err)
		}
	}
	// Multi-select then trash.
	*a.dirOpen("folder") = true
	tt.Frame()
	click(ui.Cmd, "folder/Alpha.md")
	click(ui.Cmd, "folder/Beta.md")
	a.trashSelection()
	if _, err := os.Stat(filepath.Join(dir, "folder", "Alpha.md")); !os.IsNotExist(err) {
		t.Fatalf("Alpha not trashed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "folder", "Beta.md")); !os.IsNotExist(err) {
		t.Fatalf("Beta not trashed: %v", err)
	}
}

// TestSidebarFillsAndBlankClick guards the sidebar stretching to the bottom of
// its pane and a click on the empty list area clearing the selection.
func TestSidebarFillsAndBlankClick(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n")
	write(t, filepath.Join(dir, "Beta.md"), "# Beta\n")

	a := &app{sidebar: true, backlinks: true, width: 232}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Alpha.md")
	tt := ui.NewTester(a.view, 1100, 720)
	tt.Frame()

	// The list region reaches from just under the toolbar down to Backlinks.
	var lowest float32
	for _, r := range a.rowBoxes {
		if r.Y+r.H > lowest {
			lowest = r.Y + r.H
		}
	}
	if lowest < 100 {
		t.Fatalf("rows too high; lowest row bottom = %v", lowest)
	}
	// A click well below the last row clears the selection.
	if b, ok := a.rowBoxes["Alpha.md"]; ok {
		tt.ClickAtWith(0, b.X+b.W/2, b.Y+b.H/2)
		tt.Frame()
	}
	if len(a.selectedRels("")) != 1 {
		t.Fatalf("Alpha not selected: %v", a.selectedRels(""))
	}
	tt.ClickAtWith(0, 120, 600)
	tt.Frame()
	tt.Frame()
	if len(a.selectedRels("")) != 0 {
		t.Fatalf("blank click did not clear selection: %v", a.selectedRels(""))
	}
}

func TestViewSmoke(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "# Alpha\n\nHello from the vault.\n")
	write(t, filepath.Join(dir, "folder", "Beta.md"), "A link to [[Alpha]].\n")

	a := &app{sidebar: true, backlinks: true, defaultMode: modeLive}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Alpha.md")
	tt := ui.NewTester(a.view, 1100, 720)
	if !tt.HasText("FILES") || !tt.HasText("Alpha") {
		t.Fatalf("texts = %q", tt.Texts())
	}
	if err := tt.Click("Live"); err != nil {
		t.Fatal(err)
	}
	if got := a.activeNote().mode; got != modeLive {
		t.Fatalf("mode = %v, want live", got)
	}
	if err := tt.Click("Read"); err != nil {
		t.Fatal(err)
	}
	if got := a.activeNote().mode; got != modeRead {
		t.Fatalf("mode = %v, want read", got)
	}
	if !tt.HasText("Hello from the vault.") {
		t.Fatalf("preview texts = %q", tt.Texts())
	}
	tt.Key(ui.Cmd, ui.KeyN)
	if len(a.open) != 2 || a.activeNote().mode != modeLive {
		t.Fatalf("open = %d, mode = %v", len(a.open), a.activeNote().mode)
	}
}

func TestLinkGraph(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "Links to [[Beta]] and [[Gamma|alias]].\n")
	write(t, filepath.Join(dir, "folder", "Beta.md"), "Back to [[Alpha]], and to [[Beta]] itself.\n")
	write(t, filepath.Join(dir, "Gamma.md"), "A dead end.\n")
	write(t, filepath.Join(dir, "Lonely.md"), "No links at all.\n")
	v, err := scanVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	nodes, edges, index := v.linkGraph()
	if len(nodes) != 4 {
		t.Fatalf("nodes = %d, want 4", len(nodes))
	}
	// Alpha-Beta and Alpha-Gamma: a self link and the duplicate are dropped.
	if len(edges) != 2 {
		t.Fatalf("edges = %+v, want 2", edges)
	}
	alpha := index["Alpha.md"]
	beta := index["folder/Beta.md"]
	gamma := index["Gamma.md"]
	if nodes[alpha].degree != 2 || nodes[beta].degree != 1 || nodes[gamma].degree != 1 {
		t.Fatalf("degrees = %d %d %d", nodes[alpha].degree, nodes[beta].degree, nodes[gamma].degree)
	}
	found := map[[2]int]bool{}
	for _, e := range edges {
		a, b := e.a, e.b
		if a > b {
			a, b = b, a
		}
		found[[2]int{a, b}] = true
	}
	if !found[[2]int{alpha, beta}] || !found[[2]int{alpha, gamma}] {
		t.Fatalf("edges missing: %+v", found)
	}
}

func TestLayoutDeterministic(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "A.md"), "[[B]] [[C]]\n")
	write(t, filepath.Join(dir, "B.md"), "[[C]]\n")
	write(t, filepath.Join(dir, "C.md"), "[[A]]\n")
	write(t, filepath.Join(dir, "D.md"), "[[A]]\n")
	v, _ := scanVault(dir)
	first, edges1, _ := v.linkGraph()
	layoutGraph(first, edges1)
	second, edges2, _ := v.linkGraph()
	layoutGraph(second, edges2)
	for i := range first {
		if first[i].x != second[i].x || first[i].y != second[i].y {
			t.Fatalf("node %d moved between runs: %v,%v vs %v,%v",
				i, first[i].x, first[i].y, second[i].x, second[i].y)
		}
	}
}

func TestGraphViewSmoke(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "[[Beta]]\n")
	write(t, filepath.Join(dir, "Beta.md"), "[[Alpha]]\n")
	a := &app{sidebar: false}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Alpha.md")
	tt := ui.NewTester(a.view, 900, 640)
	tt.Key(ui.Cmd, ui.KeyG)
	if !a.graph {
		t.Fatal("graph did not open")
	}
	if a.graphState == nil || len(a.graphState.nodes) != 2 {
		t.Fatalf("graph state = %+v", a.graphState)
	}
	tt.Key(ui.Cmd, ui.KeyG)
	if a.graph {
		t.Fatal("graph did not close")
	}
}

// TestGraphFitsLargestComponent checks the normalizer frames the biggest
// connected component and rings the orphans around it, rather than letting a
// lone note squash the rest of the graph into a sliver.
func TestGraphFitsLargestComponent(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "A.md"), "[[B]] [[C]] [[D]]\n")
	write(t, filepath.Join(dir, "B.md"), "[[A]] [[C]]\n")
	write(t, filepath.Join(dir, "C.md"), "[[A]] [[B]]\n")
	write(t, filepath.Join(dir, "D.md"), "[[A]]\n")
	write(t, filepath.Join(dir, "Orphan.md"), "alone\n")
	v, _ := scanVault(dir)
	nodes, edges, index := v.linkGraph()
	layoutGraph(nodes, edges)
	// Every node sits inside a comfortable unit box, the orphan included.
	max := 0.0
	for _, n := range nodes {
		max = math.Max(max, math.Max(math.Abs(n.x), math.Abs(n.y)))
	}
	if max > 2.0 {
		t.Fatalf("layout escaped the unit box: max |coord| = %.2f", max)
	}
	// The main component keeps a real extent, not a collapsed sliver.
	extent := 0.0
	for _, rel := range []string{"A.md", "B.md", "C.md", "D.md"} {
		n := nodes[index[rel]]
		extent = math.Max(extent, math.Max(math.Abs(n.x), math.Abs(n.y)))
	}
	if extent < 0.5 {
		t.Fatalf("main component collapsed: extent = %.2f", extent)
	}
	// The orphan is placed outside the main component's frame.
	if o := nodes[index["Orphan.md"]]; math.Hypot(o.x, o.y) < 1.0 {
		t.Fatalf("orphan not ringed: %v %v", o.x, o.y)
	}
}

// TestGraphClickOpensNote checks clicking a node in the graph opens it.
func TestGraphClickOpensNote(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "[[Beta]]\n")
	write(t, filepath.Join(dir, "Beta.md"), "[[Alpha]]\n")
	a := &app{sidebar: false, defaultMode: modeLive}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Beta.md")
	a.toggleGraph()
	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()
	tt.Frame() // the camera fits on the second frame, once the rect is known
	g := a.graphState
	alpha := g.index["Alpha.md"]
	x := float32(g.nodes[alpha].x*g.zoom + g.panX)
	y := float32(g.nodes[alpha].y*g.zoom + g.panY)
	tt.ClickAt(x, y)
	tt.Frame()
	if a.activeNote() == nil || a.activeNote().rel != "Alpha.md" {
		t.Fatalf("active = %+v, want Alpha.md", a.activeNote())
	}
	if a.graph {
		t.Fatal("clicking a node should close the graph")
	}
}

// TestGraphViewOptions covers the Obsidian-style filters: orphan and search
// visibility, and that changing a force relayouts the graph.
func TestGraphViewOptions(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "Alpha.md"), "[[Beta]]\n")
	write(t, filepath.Join(dir, "Beta.md"), "[[Alpha]] [[Gamma]]\n")
	write(t, filepath.Join(dir, "Gamma.md"), "[[Beta]]\n")
	write(t, filepath.Join(dir, "Lonely.md"), "no links\n")
	a := &app{sidebar: false}
	if err := a.setVault(dir, nil); err != nil {
		t.Fatal(err)
	}
	a.openPath("Alpha.md")
	a.toggleGraph()
	tt := ui.NewTester(a.view, 900, 640)
	tt.Frame()
	g := a.graphState
	if g == nil {
		t.Fatal("graph state was not built")
	}

	lonely := g.index["Lonely.md"]
	if a.graphVisible(g, lonely) {
		t.Fatal("orphan should be hidden by default")
	}
	g.showOrphans = true
	if !a.graphVisible(g, lonely) {
		t.Fatal("orphan should show once enabled")
	}
	g.showOrphans = false

	g.filter = "gam"
	if a.graphVisible(g, g.index["Beta.md"]) {
		t.Fatal("filter should hide a non-matching note")
	}
	if !a.graphVisible(g, g.index["Gamma.md"]) {
		t.Fatal("filter should keep a matching note")
	}
	g.filter = ""

	// A force change marks the layout dirty and relayout clears it.
	g.forces.repel = 2.5
	g.forcesDirty = true
	before := g.nodes[g.index["Alpha.md"]]
	a.relayout()
	if g.forcesDirty {
		t.Fatal("relayout should clear the dirty flag")
	}
	if g.nodes[g.index["Alpha.md"]] == before {
		t.Fatal("relayout should move the nodes")
	}
	if g.fitted {
		t.Fatal("relayout should ask for a fresh fit")
	}
}

// TestGraphSettingsRoundTrip checks the graph options survive a save and
// restore, and that out-of-range stored values fall back to the defaults.
func TestGraphSettingsRoundTrip(t *testing.T) {
	g := &graphView{
		showLabels: true, textFade: 0.4, nodeScale: 1.8, linkWidth: 2.2,
		forces: graphForces{center: 0.7, repel: 2.1, link: 0.5, linkRange: 1.9},
	}
	s := g.settings()
	g2 := &graphView{}
	g2.applySettings(s)
	if !g2.showLabels || g2.textFade != 0.4 || g2.nodeScale != 1.8 ||
		g2.linkWidth != 2.2 || g2.forces != g.forces {
		t.Fatalf("round trip lost settings: %+v", g2.settings())
	}
	if !g2.forcesDirty {
		t.Fatal("applying settings should ask for a relayout")
	}
	// Junk values snap back to the defaults.
	bad := &graphView{}
	bad.applySettings(&graphSettings{TextFade: 9, Repel: -3, NodeScale: 0.01})
	if bad.textFade != 0 || bad.forces.repel != 0 || bad.nodeScale != 0 {
		t.Fatalf("out-of-range values should be ignored: %+v", bad)
	}
	// A nil site is safe.
	(&graphView{}).applySettings(nil)
}

// TestGraphLayoutIsRound checks the simulation settles into a round cloud
// rather than the rows a pure 1/d² repulsion collapses into: the spread of
// node radii from the centre, and that no two visible nodes overlap.
func TestGraphLayoutIsRound(t *testing.T) {
	dir := t.TempDir()
	// A ring of notes, each linking its neighbours: a shape whose natural
	// layout is circular.
	const count = 24
	for i := 0; i < count; i++ {
		a := (i + 1) % count
		b := (i + count - 1) % count
		write(t, filepath.Join(dir, fmt.Sprintf("N%02d.md", i)),
			fmt.Sprintf("[[N%02d]] [[N%02d]]\n", a, b))
	}
	v, err := scanVault(dir)
	if err != nil {
		t.Fatal(err)
	}
	nodes, edges, _ := v.linkGraph()
	layoutGraph(nodes, edges)

	// No two nodes may sit on top of each other.
	for i := range nodes {
		for j := i + 1; j < len(nodes); j++ {
			d := math.Hypot(nodes[i].x-nodes[j].x, nodes[i].y-nodes[j].y)
			if d < 0.05 {
				t.Fatalf("%s and %s overlap: %.3f apart", nodes[i].rel, nodes[j].rel, d)
			}
		}
	}
	// The layout must use both axes: a lattice in rows leaves the vertical
	// spread much smaller than the horizontal one.
	var minX, maxX, minY, maxY float64
	minX, minY = math.Inf(1), math.Inf(1)
	maxX, maxY = math.Inf(-1), math.Inf(-1)
	for i := range nodes {
		minX, maxX = math.Min(minX, nodes[i].x), math.Max(maxX, nodes[i].x)
		minY, maxY = math.Min(minY, nodes[i].y), math.Max(maxY, nodes[i].y)
	}
	w, h := maxX-minX, maxY-minY
	if h < w*0.4 || w < h*0.4 {
		t.Fatalf("layout is a strip, not a cloud: %.2f x %.2f", w, h)
	}
}

// TestGraphNodeRadius checks dot sizes stay small and grow with degree.
func TestGraphNodeRadius(t *testing.T) {
	g := &graphView{zoom: 240, nodeScale: 1}
	leaf := g.nodeRadius(&graphNode{degree: 1})
	hub := g.nodeRadius(&graphNode{degree: 30})
	big := g.nodeRadius(&graphNode{degree: 10000})
	if leaf < 2 || leaf > 6 {
		t.Fatalf("leaf radius = %.1f, want a small dot", leaf)
	}
	if !(hub > leaf) {
		t.Fatalf("hub radius %.1f should exceed leaf %.1f", hub, leaf)
	}
	if big > 13.01 {
		t.Fatalf("radius should be capped, got %.1f", big)
	}
	// Zooming in enlarges the dots; zooming out shrinks them, with a floor.
	g.zoom = 480
	if g.nodeRadius(&graphNode{degree: 1}) <= leaf {
		t.Fatal("zooming in should enlarge a dot")
	}
	g.zoom = 1
	if g.nodeRadius(&graphNode{degree: 1}) < 2.2 {
		t.Fatal("dots should not vanish when zoomed far out")
	}
}
