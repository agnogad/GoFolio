package main

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gofolio/internal/doc"
)

// fileNode is one entry of the vault's tree.
type fileNode struct {
	name string
	rel  string
	dir  bool
	mod  time.Time
	kids []*fileNode
}

// vault is a folder of Markdown files, scanned once and refreshed on demand.
type vault struct {
	root     string
	nodes    []*fileNode
	notes    []string // every note, vault-relative, sorted
	drawings []string // every .tldr drawing, vault-relative
}

var errInvalidPath = errors.New("path is outside the vault")

// skipDir reports whether a directory holds app data rather than notes.
func skipDir(name string) bool {
	switch name {
	case ".git", ".obsidian", ".trash", ".gofolio", "node_modules":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// isNote reports whether a file name is a Markdown note.
func isNote(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

// isDraw reports whether a file name is a drawing.
func isDraw(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".tldr")
}

// isVaultFile reports whether a file is one the vault shows: a note or a
// drawing.
func isVaultFile(name string) bool {
	return isNote(name) || isDraw(name)
}

// scanVault walks root and builds the tree and the flat list of notes.
func scanVault(root string) (*vault, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("not a folder")
	}
	v := &vault{root: root}
	var walk func(dir, rel string) ([]*fileNode, error)
	walk = func(dir, rel string) ([]*fileNode, error) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		var nodes []*fileNode
		for _, e := range entries {
			name := e.Name()
			childRel := name
			if rel != "" {
				childRel = rel + "/" + name
			}
			if e.IsDir() {
				if skipDir(name) {
					continue
				}
				kids, err := walk(filepath.Join(dir, name), childRel)
				if err != nil {
					continue
				}
				// Empty folders stay visible, as they do in Obsidian.
				info, _ := e.Info()
				var mod time.Time
				if info != nil {
					mod = info.ModTime()
				}
				nodes = append(nodes, &fileNode{name: name, rel: childRel, dir: true, mod: mod, kids: kids})
				continue
			}
			if !isVaultFile(name) {
				continue
			}
			info, err := e.Info()
			if err != nil {
				continue
			}
			nodes = append(nodes, &fileNode{name: name, rel: childRel, mod: info.ModTime()})
			if isDraw(name) {
				v.drawings = append(v.drawings, childRel)
			} else {
				v.notes = append(v.notes, childRel)
			}
		}
		sortNodes(nodes)
		return nodes, nil
	}
	nodes, err := walk(root, "")
	if err != nil {
		return nil, err
	}
	v.nodes = nodes
	sort.Strings(v.notes)
	return v, nil
}

func sortNodes(nodes []*fileNode) {
	sort.Slice(nodes, func(i, j int) bool {
		a, b := nodes[i], nodes[j]
		if a.dir != b.dir {
			return a.dir
		}
		return strings.ToLower(a.name) < strings.ToLower(b.name)
	})
}

// abs resolves a vault-relative path, refusing escapes.
func (v *vault) abs(rel string) (string, error) {
	rel = filepath.FromSlash(strings.TrimSpace(rel))
	if rel == "" {
		return "", errInvalidPath
	}
	p := filepath.Join(v.root, rel)
	r, err := filepath.Rel(v.root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errInvalidPath
	}
	return p, nil
}

func (v *vault) has(rel string) bool {
	for _, n := range v.notes {
		if n == rel {
			return true
		}
	}
	for _, d := range v.drawings {
		if d == rel {
			return true
		}
	}
	return false
}

// cleanName keeps a user-typed name a single path segment.
func cleanName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, ".md")
	name = strings.TrimSuffix(name, ".markdown")
	name = strings.TrimSuffix(name, ".tldr")
	name = strings.Map(func(r rune) rune {
		if strings.ContainsRune(`/\:*?"<>|`, r) || r < 32 {
			return -1
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" || name == "." || name == ".." {
		return "", errors.New("enter a name")
	}
	return name, nil
}

// uniqueRel returns a free vault-relative path for name under parent.
func (v *vault) uniqueRel(parent, name, ext string) string {
	base := name
	for i := 2; ; i++ {
		rel := name + ext
		if parent != "" {
			rel = parent + "/" + rel
		}
		if !v.exists(rel) {
			return rel
		}
		name = base + " " + itoa(i)
	}
}

func (v *vault) exists(rel string) bool {
	p, err := v.abs(rel)
	if err != nil {
		return true
	}
	_, err = os.Stat(p)
	return err == nil
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

// createNote makes an empty note under parent and returns its path.
func (v *vault) createNote(parent, name string) (string, error) {
	if name == "" {
		name = "Untitled"
	}
	name, err := cleanName(name)
	if err != nil {
		return "", err
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".md", ".markdown":
		name = name[:len(name)-len(filepath.Ext(name))]
	}
	rel := v.uniqueRel(parent, name, ".md")
	p, err := v.abs(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, nil, 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

// createDrawing makes a new empty drawing under parent and returns its path.
// The file holds a one-page, empty .tldr document, so a viewer that is not
// this app can still read it.
func (v *vault) createDrawing(parent, name string) (string, error) {
	if name == "" {
		name = "Drawing"
	}
	name, err := cleanName(name)
	if err != nil {
		return "", err
	}
	rel := v.uniqueRel(parent, name, ".tldr")
	p, err := v.abs(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	d := doc.New()
	d.Name = trimExt(pathBase(rel))
	var buf bytes.Buffer
	if err := doc.Write(&buf, d); err != nil {
		return "", err
	}
	if err := os.WriteFile(p, buf.Bytes(), 0o644); err != nil {
		return "", err
	}
	return rel, nil
}

func (v *vault) createFolder(parent, name string) (string, error) {
	name, err := cleanName(name)
	if err != nil {
		return "", err
	}
	rel := v.uniqueRel(parent, name, "")
	p, err := v.abs(rel)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return rel, nil
}

// rename moves a file or folder inside the vault, keeping a note's .md.
func (v *vault) rename(rel, name string) (string, error) {
	p, err := v.abs(rel)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		name, err = cleanName(name)
		if err != nil {
			return "", err
		}
		switch strings.ToLower(filepath.Ext(rel)) {
		case ".markdown":
			name += ".markdown"
		case ".tldr":
			name += ".tldr"
		default:
			name += ".md"
		}
	} else {
		name, err = cleanName(name)
		if err != nil {
			return "", err
		}
	}
	parent := pathDir(rel)
	next := name
	if parent != "" {
		next = parent + "/" + name
	}
	if next == rel {
		return rel, nil
	}
	for i := 2; v.exists(next); i++ {
		base := strings.TrimSuffix(name, filepath.Ext(name))
		ext := filepath.Ext(name)
		n := base + " " + itoa(i) + ext
		next = n
		if parent != "" {
			next = parent + "/" + n
		}
	}
	np, err := v.abs(next)
	if err != nil {
		return "", err
	}
	if err := os.Rename(p, np); err != nil {
		return "", err
	}
	return next, nil
}

// move rel into the folder destDir ("" for the vault root), keeping its name
// and avoiding a clash. A folder cannot move into itself or its own subtree.
func (v *vault) move(rel, destDir string) (string, error) {
	if rel == "" || rel == destDir {
		return rel, nil
	}
	if destDir != "" && strings.HasPrefix(destDir+"/", rel+"/") {
		return "", errors.New("a folder cannot move into itself")
	}
	p, err := v.abs(rel)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(p); err != nil {
		return "", err
	}
	name := pathBase(rel)
	next := name
	if destDir != "" {
		next = destDir + "/" + name
	}
	if next == rel {
		return rel, nil
	}
	ext := ""
	base := name
	if isNote(rel) || isDraw(rel) {
		ext = filepath.Ext(name)
		base = strings.TrimSuffix(name, ext)
	}
	for i := 2; v.exists(next); i++ {
		n := base + " " + itoa(i) + ext
		next = n
		if destDir != "" {
			next = destDir + "/" + n
		}
	}
	np, err := v.abs(next)
	if err != nil {
		return "", err
	}
	if err := os.Rename(p, np); err != nil {
		return "", err
	}
	return next, nil
}

// trash moves an entry to the vault's .trash, as Obsidian does.
func (v *vault) trash(rel string) error {
	p, err := v.abs(rel)
	if err != nil {
		return err
	}
	dir := filepath.Join(v.root, ".trash")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	name := filepath.Base(p)
	dst := filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+name)
	for i := 2; ; i++ {
		if _, err := os.Stat(dst); errors.Is(err, fs.ErrNotExist) {
			break
		}
		dst = filepath.Join(dir, time.Now().Format("20060102-150405")+"-"+itoa(i)+"-"+name)
	}
	return os.Rename(p, dst)
}

func pathDir(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[:i]
	}
	return ""
}

func pathBase(rel string) string {
	if i := strings.LastIndex(rel, "/"); i >= 0 {
		return rel[i+1:]
	}
	return rel
}

func trimExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// --- frontmatter ---------------------------------------------------------

// splitFrontmatter separates a leading YAML block from the body. front keeps
// its markers and trailing newline, so it can be written back untouched.
func splitFrontmatter(src string) (front, body string) {
	if !strings.HasPrefix(src, "---\n") {
		return "", src
	}
	rest := src[4:]
	end := 0
	for end <= len(rest) {
		line := rest[end:]
		nl := strings.IndexByte(line, '\n')
		var text string
		if nl < 0 {
			text = line
		} else {
			text = line[:nl]
		}
		if strings.TrimRight(text, " \t\r") == "---" || strings.TrimRight(text, " \t\r") == "..." {
			if nl < 0 {
				return src[:4+end+len(line)], ""
			}
			cut := 4 + end + nl + 1
			return src[:cut], src[cut:]
		}
		if nl < 0 {
			break
		}
		end += nl + 1
	}
	return "", src
}

func eolOf(src string) string {
	if strings.Contains(src, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

// withEOL writes canonical \n text with the file's original line endings.
func withEOL(src, eol string) string {
	if eol == "\n" || !strings.Contains(src, "\n") {
		return src
	}
	return strings.ReplaceAll(strings.ReplaceAll(src, "\r\n", "\n"), "\n", eol)
}

// writeFileAtomic replaces path in one step and keeps its permissions.
func writeFileAtomic(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".gofolio-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	ok := false
	defer func() {
		if !ok {
			os.Remove(tmp)
		}
	}()
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	ok = true
	return nil
}

// --- wikilinks -----------------------------------------------------------

var (
	wikiRe     = regexp.MustCompile(`\[\[([^\]\|#]+)(?:#([^\]\|]+))?(?:\|([^\]]+))?\]\]`)
	embedRe    = regexp.MustCompile(`!\[\[([^\]\|#]+)(?:\|([^\]]+))?\]\]`)
	noteLinkRe = regexp.MustCompile(`\[([^\]]*)\]\(note:([^)#\s]+)(?:#([^)]+))?\)`)
)

// linkify turns [[Note]] and [[Note|Alias]] into ordinary Markdown links the
// editors and the preview can open, and ![[drawing.tldr]] into an image the
// preview draws on the spot.
func linkify(body string) string {
	body = embedRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := embedRe.FindStringSubmatch(m)
		target := strings.TrimSpace(sub[1])
		label := strings.TrimSpace(sub[2])
		if label == "" {
			label = target
		}
		return "![" + label + "](draw:" + pathEscape(target) + ")"
	})
	return wikiRe.ReplaceAllStringFunc(body, func(m string) string {
		sub := wikiRe.FindStringSubmatch(m)
		target := strings.TrimSpace(sub[1])
		section := strings.TrimSpace(sub[2])
		alias := strings.TrimSpace(sub[3])
		label := alias
		if label == "" {
			label = target
			if section != "" {
				label += " › " + section
			}
		}
		dest := "note:" + pathEscape(target)
		if section != "" {
			dest += "#" + pathEscape(section)
		}
		return "[" + label + "](" + dest + ")"
	})
}

// unlinkify writes note: links back as Obsidian wikilinks.
func unlinkify(md string) string {
	return noteLinkRe.ReplaceAllStringFunc(md, func(m string) string {
		sub := noteLinkRe.FindStringSubmatch(m)
		label, target, section := sub[1], pathUnescape(sub[2]), pathUnescape(sub[3])
		out := target
		if section != "" {
			out += "#" + section
		}
		if label != "" && label != target && label != target+" › "+section {
			out += "|" + label
		}
		return "[[" + out + "]]"
	})
}

// pathEscape keeps spaces and slashes readable in a link destination.
func pathEscape(s string) string {
	r := strings.NewReplacer(" ", "%20", ")", "%29", "(", "%28", "\n", "%0A")
	return r.Replace(s)
}

func pathUnescape(s string) string {
	r := strings.NewReplacer("%20", " ", "%29", ")", "%28", "(", "%0A", "\n")
	return r.Replace(s)
}

// resolveLink finds the note a link points at: a vault path, a path relative
// to the link's note, or a bare file name.
func (v *vault) resolveLink(fromRel, dest string) (string, bool) {
	dest = strings.TrimSpace(dest)
	dest = strings.TrimPrefix(dest, "note:")
	if i := strings.IndexByte(dest, '#'); i >= 0 {
		dest = dest[:i]
	}
	dest = pathUnescape(dest)
	if dest == "" {
		return "", false
	}
	if v.has(dest) {
		return dest, true
	}
	if from := pathDir(fromRel); from != "" {
		if rel := from + "/" + dest; v.has(rel) {
			return rel, true
		}
	}
	base := strings.ToLower(pathBase(dest))
	for _, n := range v.notes {
		if strings.ToLower(pathBase(n)) == base {
			return n, true
		}
	}
	// The destination may name a note in a subfolder without its extension,
	// as "Projects/Roadmap" does: compare its last segment, extension
	// stripped, against each note's name. Comparing the whole path here
	// would never match, since a note is looked up by its basename.
	noext := strings.ToLower(trimExt(pathBase(dest)))
	for _, n := range v.notes {
		if strings.ToLower(trimExt(pathBase(n))) == noext {
			return n, true
		}
	}
	return "", false
}

// --- search --------------------------------------------------------------

type searchHit struct {
	rel    string
	line   int // 1-based
	start  int // byte offset of the match
	length int
	text   string
}

// searchVault looks through note contents, case-insensitively.
func searchVault(v *vault, query string, limit int) []searchHit {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil
	}
	re, err := regexp.Compile("(?i)" + regexp.QuoteMeta(query))
	if err != nil {
		return nil
	}
	var hits []searchHit
	for _, rel := range v.notes {
		p, err := v.abs(rel)
		if err != nil {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || info.Size() > 2<<20 {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.ReplaceAll(string(b), "\r\n", "\n")
		for _, loc := range re.FindAllStringIndex(content, 20) {
			lineStart := strings.LastIndexByte(content[:loc[0]], '\n') + 1
			lineEnd := strings.IndexByte(content[loc[1]:], '\n')
			if lineEnd < 0 {
				lineEnd = len(content)
			} else {
				lineEnd += loc[1]
			}
			text := strings.TrimSpace(content[lineStart:lineEnd])
			if len(text) > 180 {
				text = text[:180] + "…"
			}
			hits = append(hits, searchHit{
				rel:    rel,
				line:   strings.Count(content[:loc[0]], "\n") + 1,
				start:  loc[0],
				length: loc[1] - loc[0],
				text:   text,
			})
			if len(hits) >= limit {
				return hits
			}
		}
	}
	return hits
}

// --- backlinks -----------------------------------------------------------

type backlink struct {
	rel  string
	line string
}

// backlinksFor finds notes that link to rel with [[...]] or a .md link.
func backlinksFor(v *vault, rel string) []backlink {
	base := trimExt(pathBase(rel))
	full := trimExt(rel)
	var out []backlink
	for _, other := range v.notes {
		if other == rel {
			continue
		}
		p, err := v.abs(other)
		if err != nil {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || info.Size() > 2<<20 {
			continue
		}
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		content := strings.ReplaceAll(string(b), "\r\n", "\n")
		needles := []string{
			"[[" + base + "]]", "[[" + base + "|", "[[" + base + "#",
			"[[" + full + "]]", "[[" + full + "|", "[[" + full + "#",
			"](" + base + ".md)", "](" + full + ".md)",
		}
		for _, n := range needles {
			if i := strings.Index(content, n); i >= 0 {
				lineStart := strings.LastIndexByte(content[:i], '\n') + 1
				lineEnd := strings.IndexByte(content[i:], '\n')
				if lineEnd < 0 {
					lineEnd = len(content)
				} else {
					lineEnd += i
				}
				text := strings.TrimSpace(content[lineStart:lineEnd])
				if len(text) > 140 {
					text = text[:140] + "…"
				}
				out = append(out, backlink{rel: other, line: text})
				break
			}
		}
		if len(out) >= 40 {
			break
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].rel) < strings.ToLower(out[j].rel)
	})
	return out
}
