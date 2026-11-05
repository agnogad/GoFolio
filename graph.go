package main

import (
	"errors"
	"hash/fnv"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/egoist/mygo/ui"
)

var errTooBig = errors.New("file too large")

// graphNode is one note in the link graph.
type graphNode struct {
	rel    string
	label  string
	degree int
	x, y   float64 // laid out position, in graph space (roughly [-1,1])
}

// graphEdge is a link between two notes.
type graphEdge struct {
	a, b int
}

// graphForces are the tunable forces, with Obsidian's names and feel.
type graphForces struct {
	center    float64 // how hard nodes pull to the middle; higher = rounder
	repel     float64 // how hard nodes push each other apart
	link      float64 // how hard a link pulls its two ends together
	linkRange float64 // the natural length of a link
}

func defaultForces() graphForces {
	return graphForces{center: 0.34, repel: 1.0, link: 1.0, linkRange: 1.0}
}

// graphView is the cached result of the vault's links and their layout, so
// the frame does not rebuild either while the pane is open and nothing
// changed.
type graphView struct {
	nodes []graphNode
	edges []graphEdge
	index map[string]int

	// camera, in the painter's window-absolute pixels.
	zoom         float64
	panX, panY   float64
	fitted       bool
	fitW, fitH   float32
	fitOX, fitOY float32

	// view options, mirroring Obsidian's graph settings.
	showLabels  bool
	showOrphans bool
	arrows      bool
	textFade    float64 // 0..1, lower means labels appear sooner when zooming out
	nodeScale   float64 // multiplies every node's radius
	linkWidth   float64 // edge stroke thickness
	forces      graphForces
	forcesDirty bool // re-run the layout on the next frame

	filter  string
	filterH ui.Handle

	settingsOpen bool

	// hovered and clicked are indices into nodes, -1 for none.
	hovered int
	clicked int
	moved   bool

	// dragging a node: held index while the pointer drags it, -1 otherwise.
	held int
}

// linkGraph scans the vault once and returns its notes and the links between
// them, using the same resolution as clicking a link so the graph and the
// links agree.
func (v *vault) linkGraph() ([]graphNode, []graphEdge, map[string]int) {
	index := make(map[string]int, len(v.notes))
	nodes := make([]graphNode, len(v.notes))
	for i, rel := range v.notes {
		index[rel] = i
		nodes[i] = graphNode{rel: rel, label: trimExt(pathBase(rel))}
	}
	var edges []graphEdge
	seen := make(map[[2]int]bool)
	for _, rel := range v.notes {
		p, err := v.abs(rel)
		if err != nil {
			continue
		}
		b, err := readSmallFile(p)
		if err != nil {
			continue
		}
		from := index[rel]
		for _, dest := range outgoingLinks(string(b)) {
			to, ok := v.resolveLink(rel, dest)
			if !ok || to == rel {
				continue
			}
			j, ok := index[to]
			if !ok {
				continue
			}
			a, b := from, j
			if a > b {
				a, b = b, a
			}
			key := [2]int{a, b}
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, graphEdge{a: a, b: b})
			nodes[from].degree++
			nodes[j].degree++
		}
	}
	return nodes, edges, index
}

// outgoingLinks returns every link destination in a note: wikilinks and
// ordinary .md links, in the order they appear.
func outgoingLinks(src string) []string {
	_, body := splitFrontmatter(src)
	var out []string
	for _, m := range wikiRe.FindAllStringSubmatch(body, -1) {
		if target := strings.TrimSpace(m[1]); target != "" {
			out = append(out, target)
		}
	}
	for _, m := range noteLinkRe.FindAllStringSubmatch(body, -1) {
		if target := pathUnescape(m[2]); target != "" {
			out = append(out, target)
		}
	}
	return out
}

// readSmallFile reads a note, refusing files too large to be a note.
func readSmallFile(p string) (string, error) {
	info, err := os.Stat(p)
	if err != nil {
		return "", err
	}
	if info.Size() > 2<<20 {
		return "", errTooBig
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n"), nil
}

// layoutGraph places the nodes with a force simulation, then normalizes the
// result so the camera can frame it the same way whatever the vault's size.
func layoutGraph(nodes []graphNode, edges []graphEdge) {
	layoutGraphForces(nodes, edges, defaultForces())
}

// layoutGraphForces runs the simulation with the given forces. The result is
// scaled into a unit box so the camera framing is stable, whatever the vault.
func layoutGraphForces(nodes []graphNode, edges []graphEdge, f graphForces) {
	simulate(nodes, edges, f)
	normalizeGraph(nodes, edges)
}

// simulate runs a force-directed layout, in the spirit of d3-force as
// Obsidian uses it: a many-body repulsion, a spring on each link, and a
// centering pull. It starts from a deterministic golden-angle spiral seeded
// by each note's path, so the same vault lays out the same way every time,
// and runs a fixed number of steps so the cost is bounded and there is no
// jitter.
func simulate(nodes []graphNode, edges []graphEdge, f graphForces) {
	n := len(nodes)
	if n == 0 {
		return
	}
	if n == 1 {
		nodes[0].x, nodes[0].y = 0, 0
		return
	}
	const steps = 640
	const idealLen = 100.0
	linkK := f.link * 0.55
	linkLen := f.linkRange * idealLen * 0.62
	center := f.center * 0.75

	// A golden-angle spiral seeds the nodes without clustering by index.
	for i := range nodes {
		frac := float64(i) / float64(n)
		a := float64(i) * 2.399963229728653 // golden angle
		r := idealLen * (0.20 + 0.85*math.Sqrt(frac))
		r += idealLen * 0.12 * hashUnit(nodes[i].rel)
		nodes[i].x = math.Cos(a) * r
		nodes[i].y = math.Sin(a) * r
	}

	dxs := make([]float64, n)
	dys := make([]float64, n)
	temp := idealLen * 1.4
	for step := 0; step < steps; step++ {
		for i := range dxs {
			dxs[i], dys[i] = 0, 0
		}
		// Separation. Each note has a personal-space radius; pairs closer
		// than the sum push apart, and the push fades out to nothing at
		// about twice that, so the cloud settles into an even, round blob
		// instead of the unbounded 1/d² shove that flattens it into rows.
		collide := idealLen * 0.62 * math.Sqrt(f.repel)
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				dx := nodes[i].x - nodes[j].x
				dy := nodes[i].y - nodes[j].y
				d2 := dx*dx + dy*dy
				if d2 < 1e-4 {
					dx, dy, d2 = 0.02, 0.02, 4e-4
				}
				d := math.Sqrt(d2)
				if d > collide*1.9 {
					continue
				}
				// Zero at the fade-out distance, strongest when overlapping.
				f2 := (collide*1.9 - d) / (collide * 1.9)
				f2 *= f2 * collide * 0.16 * f.repel
				ux, uy := dx/d, dy/d
				dxs[i] += ux * f2
				dys[i] += uy * f2
				dxs[j] -= ux * f2
				dys[j] -= uy * f2
			}
		}
		// Link springs toward the natural link length.
		for _, e := range edges {
			dx := nodes[e.b].x - nodes[e.a].x
			dy := nodes[e.b].y - nodes[e.a].y
			d := math.Hypot(dx, dy)
			if d < 1e-6 {
				d = 1e-6
			}
			force := linkK * (d - linkLen)
			// A link's strength falls off with how connected its ends are,
			// as d3-force's link force does, so the tightest clusters do not
			// yank their hubs out of shape.
			mass := nodes[e.a].degree
			if nodes[e.b].degree < mass {
				mass = nodes[e.b].degree
			}
			force /= 1 + 0.7*float64(mass)
			ux, uy := dx/d, dy/d
			dxs[e.a] += ux * force
			dys[e.a] += uy * force
			dxs[e.b] -= ux * force
			dys[e.b] -= uy * force
		}
		for i := range nodes {
			dx, dy := dxs[i], dys[i]
			// Centering pulls everything to the middle, which rounds the
			// whole graph off the way Obsidian's center force does.
			dx -= nodes[i].x * center
			dy -= nodes[i].y * center
			d := math.Hypot(dx, dy)
			if d < 1e-6 {
				continue
			}
			lim := math.Min(d, temp)
			nodes[i].x += dx / d * lim
			nodes[i].y += dy / d * lim
		}
		temp *= 0.9905
		if temp < 0.05 {
			temp = 0.05
		}
	}
}

// normalizeGraph recentres the layout on the largest connected component and
// rescales so that component fills a unit box, then tucks the orphan nodes
// onto a ring just outside it. Scaling by the main component instead of the
// whole bounding box stops a single unconnected note from squashing the real
// network into a sliver, and keeps the framing stable for any vault size.
func normalizeGraph(nodes []graphNode, edges []graphEdge) {
	if len(nodes) == 0 {
		return
	}
	comp := connectedComponents(nodes, edges)
	main := 0
	for id, s := range comp.sizes {
		if s > comp.sizes[main] {
			main = id
		}
	}

	minX, minY := math.Inf(1), math.Inf(1)
	maxX, maxY := math.Inf(-1), math.Inf(-1)
	for i := range nodes {
		if comp.id[i] != main {
			continue
		}
		minX = math.Min(minX, nodes[i].x)
		minY = math.Min(minY, nodes[i].y)
		maxX = math.Max(maxX, nodes[i].x)
		maxY = math.Max(maxY, nodes[i].y)
	}
	if math.IsInf(minX, 1) { // no nodes in the main component
		minX, minY, maxX, maxY = 0, 0, 0, 0
	}
	cx, cy := (minX+maxX)/2, (minY+maxY)/2
	extent := math.Max(math.Max(maxX-minX, maxY-minY), 1e-6)
	scale := 1.7 / extent

	for i := range nodes {
		nodes[i].x = (nodes[i].x - cx) * scale
		nodes[i].y = (nodes[i].y - cy) * scale
	}

	// Orphans (and any other component) go on a ring outside the main
	// component. The ring radius clears the furthest main node by a gap, so
	// an isolated note never lands on top of a note it has nothing to do
	// with — the main box's corners reach further out than its half-width.
	reach := 0.0
	for i := range nodes {
		if comp.id[i] != main {
			continue
		}
		reach = math.Max(reach, math.Hypot(nodes[i].x, nodes[i].y))
	}
	if reach == 0 {
		reach = 0.85
	}

	orphans := 0
	for i := range nodes {
		if comp.id[i] != main {
			orphans++
		}
	}
	if orphans == 0 {
		return
	}
	place := 0
	for i := range nodes {
		if comp.id[i] == main {
			continue
		}
		ang := float64(place) / float64(orphans) * 2 * math.Pi
		r := reach + 0.34 + 0.22*float64(place%2)
		nodes[i].x = math.Cos(ang) * r
		nodes[i].y = math.Sin(ang) * r
		place++
	}
}

// components is the connected-component labelling of a graph.
type components struct {
	id    []int
	sizes []int
}

// connectedComponents labels every node with its component id.
func connectedComponents(nodes []graphNode, edges []graphEdge) components {
	c := components{id: make([]int, len(nodes))}
	for i := range c.id {
		c.id[i] = -1
	}
	adj := make([][]int, len(nodes))
	for _, e := range edges {
		adj[e.a] = append(adj[e.a], e.b)
		adj[e.b] = append(adj[e.b], e.a)
	}
	for i := range nodes {
		if c.id[i] >= 0 {
			continue
		}
		id := len(c.sizes)
		size := 0
		stack := []int{i}
		c.id[i] = id
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			size++
			for _, nb := range adj[cur] {
				if c.id[nb] < 0 {
					c.id[nb] = id
					stack = append(stack, nb)
				}
			}
		}
		c.sizes = append(c.sizes, size)
	}
	return c
}

// graphNeighbors returns the set of node indices adjacent to i, including i.
func graphNeighbors(edges []graphEdge, i int) map[int]bool {
	set := map[int]bool{i: true}
	for _, e := range edges {
		if e.a == i {
			set[e.b] = true
		} else if e.b == i {
			set[e.a] = true
		}
	}
	return set
}

// hashUnit returns a stable value in [0,1) from a string.
func hashUnit(s string) float64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	return float64(h.Sum64()%10000) / 10000
}

// nodeRadius is a node's drawn radius in screen pixels: a small base plus a
// gentle bump for the busiest hubs, capped so a dense vault does not become a
// pile of pebbles. It scales mildly with the camera zoom, and with the
// node-size setting. This is close to how Obsidian sizes its dots.
func (g *graphView) nodeRadius(n *graphNode) float64 {
	r := 2.4 + 2.0*math.Sqrt(float64(n.degree))
	if r > 13 {
		r = 13
	}
	r *= g.nodeScale
	r *= g.zoom / 240
	if r < 2.2 {
		r = 2.2
	}
	return r
}

// ensureGraph builds and lays out the graph once, into app state.
func (a *app) ensureGraph() {
	if a.graphState != nil || a.v == nil {
		return
	}
	nodes, edges, index := a.v.linkGraph()
	layoutGraph(nodes, edges)
	g := &graphView{
		nodes:       nodes,
		edges:       edges,
		index:       index,
		zoom:        0,
		hovered:     -1,
		clicked:     -1,
		held:        -1,
		showLabels:  true,
		textFade:    0.18,
		nodeScale:   1.0,
		linkWidth:   1.0,
		forces:      defaultForces(),
		forcesDirty: false,
	}
	g.applySettings(a.graphPrefs)
	a.graphState = g
}

// settings snapshots the graph's display and force options for the session.
func (g *graphView) settings() *graphSettings {
	if g == nil {
		return nil
	}
	return &graphSettings{
		ShowLabels:  g.showLabels,
		ShowOrphans: g.showOrphans,
		Arrows:      g.arrows,
		TextFade:    g.textFade,
		NodeScale:   g.nodeScale,
		LinkWidth:   g.linkWidth,
		Center:      g.forces.center,
		Repel:       g.forces.repel,
		Link:        g.forces.link,
		LinkRange:   g.forces.linkRange,
	}
}

// applySettings restores remembered options, keeping the defaults where the
// stored values are missing or out of range.
func (g *graphView) applySettings(s *graphSettings) {
	if s == nil {
		return
	}
	g.showLabels = s.ShowLabels
	g.showOrphans = s.ShowOrphans
	g.arrows = s.Arrows
	if inRange(s.TextFade, 0, 1) {
		g.textFade = s.TextFade
	}
	if inRange(s.NodeScale, 0.4, 2.5) {
		g.nodeScale = s.NodeScale
	}
	if inRange(s.LinkWidth, 0.4, 3) {
		g.linkWidth = s.LinkWidth
	}
	if inRange(s.Center, 0, 1) {
		g.forces.center = s.Center
	}
	if inRange(s.Repel, 0, 3) {
		g.forces.repel = s.Repel
	}
	if inRange(s.Link, 0, 3) {
		g.forces.link = s.Link
	}
	if inRange(s.LinkRange, 0.3, 3) {
		g.forces.linkRange = s.LinkRange
	}
	g.forcesDirty = true
}

// inRange reports whether v is within [lo, hi].
func inRange(v, lo, hi float64) bool { return v >= lo && v <= hi }

// relayout re-runs the force simulation with the current forces.
func (a *app) relayout() {
	g := a.graphState
	if g == nil {
		return
	}
	nodes, edges, _ := a.v.linkGraph()
	layoutGraphForces(nodes, edges, g.forces)
	g.nodes = nodes
	g.edges = edges
	g.forcesDirty = false
	g.fitted = false
}

// fit centers and scales the graph inside canvas rect r, with a margin.
// The camera lives in the painter's window-absolute space, so the centre
// includes the rect's origin.
func (g *graphView) fit(r ui.Rect) {
	w, h := float64(r.W), float64(r.H)
	if w < 40 {
		w = 900
	}
	if h < 40 {
		h = 600
	}
	g.zoom = math.Min(w, h) / 2.2
	if g.zoom < 60 {
		g.zoom = 60
	}
	g.panX = float64(r.X) + w/2
	g.panY = float64(r.Y) + h/2
	g.fitted = true
}

// zoomBy scales the camera about the pane centre, keeping it in bounds.
func (g *graphView) zoomBy(f float64) {
	g.zoom *= f
	if g.zoom < 40 {
		g.zoom = 40
	}
	if g.zoom > 4000 {
		g.zoom = 4000
	}
}

// graphPane draws the graph and handles its pointer.
func (a *app) graphPane(c *ui.Context) {
	a.ensureGraph()
	t := c.Theme()
	g := a.graphState
	if g == nil || len(g.nodes) == 0 {
		ui.Column(c).Grow(1).Center().Children(func() {
			ui.Text(c, "No notes to graph yet").FontSize(14).TextColor(t.TextMuted)
		})
		return
	}
	if g.forcesDirty {
		a.relayout()
	}
	active := ""
	if n := a.activeNote(); n != nil {
		active = n.rel
	}

	// The pane is one box: the canvas fills it, then the controls sit on top
	// as absolutely positioned siblings, so the pointer still reaches the
	// canvas everywhere but on the controls themselves.
	ui.Box(c).Grow(1).FillWidth().FillHeight().Children(func() {
		canvas := ui.Box(c).Fill().Background(t.Background).Clip().Cursor(ui.CursorGrab)
		a.graphPointer(c, canvas, g)
		canvas.Draw(func(p *ui.Painter, r ui.Rect) {
			a.drawGraph(p, r, g, t, active)
		})
		a.graphOverlay(c, g, t)
	})

	if g.clicked >= 0 {
		rel := g.nodes[g.clicked].rel
		g.clicked = -1
		g.held = -1
		a.graph = false
		a.openPath(rel)
	}
}

// graphOverlay floats the graph controls over the canvas: the count in the
// lower left, the settings cog in the upper right, zoom buttons in the lower
// right, in Obsidian's placement. Controls are absolute so the pointer still
// reaches the canvas everywhere else.
func (a *app) graphOverlay(c *ui.Context, g *graphView, t *ui.Theme) {
	// Lower left: the note and link count.
	ui.Textf(c, "%d notes · %d links", len(g.nodes), len(g.edges)).
		FontSize(11).TextColor(t.TextMuted).Absolute().Left(12).Bottom(10)

	// Upper right: the settings cog.
	ui.Row(c).Gap(2).Padding(3).Radius(9).Absolute().Top(10).Right(10).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.10)).
		Children(func() {
			iconButton(c, iconSettings, "Graph settings", g.settingsOpen, func() {
				g.settingsOpen = !g.settingsOpen
			})
		})

	// Lower right: zoom controls.
	ui.Row(c).Gap(2).Padding(3).Radius(9).Absolute().Bottom(10).Right(10).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 6, 18, 0, ui.RGBA(0, 0, 0, 0.10)).
		Children(func() {
			iconButton(c, iconMinus, "Zoom out", false, func() { g.zoomBy(1 / 1.3) })
			iconButton(c, iconPlus, "Zoom in", false, func() { g.zoomBy(1.3) })
			iconButton(c, iconFit, "Fit to view", false, func() { g.fitted = false })
		})

	if g.settingsOpen {
		a.graphSettings(c, g, t)
	}
}

// graphSettings is the settings panel Obsidian puts behind the cog: Filters,
// Display and Forces, each with the controls that govern the view.
func (a *app) graphSettings(c *ui.Context, g *graphView, t *ui.Theme) {
	panel := ui.Column(c).Width(272).Absolute().Top(48).Right(10).
		MaxHeightPercent(90).
		Padding(12, 16, 12, 16).Gap(10).Radius(10).
		Background(t.Background).Border(1, t.Border).
		Shadow(0, 16, 44, 0, ui.RGBA(0, 0, 0, 0.18))
	changed := false
	panel.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Children(func() {
			ui.Text(c, "Graph settings").FontSize(13.5).FontWeight(650).TextColor(t.Text).Grow(1)
			reset := ui.Text(c, "Reset").FontSize(11.5).Cursor(ui.CursorPointer)
			if reset.Clicked() {
				g.forces = defaultForces()
				g.forcesDirty = true
				g.textFade = 0.18
				g.nodeScale = 1
				g.linkWidth = 1
				g.arrows = false
				g.showOrphans = false
				changed = true
			} else if reset.Hovered() {
				reset.TextColor(t.Text)
			} else {
				reset.TextColor(t.TextMuted)
			}
			iconButton(c, iconX, "Close settings", false, func() { g.settingsOpen = false })
		})
		ui.Box(c).Height(1).FillWidth().Background(t.Border)

		ui.Scroll(c).FillWidth().Grow(1).Children(func() {
			ui.Column(c).FillWidth().Gap(10).Children(func() {
				graphSection(c, t, "Filters")
				ui.SearchField(c, &g.filter).Bind(&g.filterH).
					Placeholder("Search notes").Label("Search notes").Height(26).FillWidth()
				changed = graphToggle(c, t, "Orphans", "Show notes without any links", &g.showOrphans) || changed

				graphSection(c, t, "Display")
				changed = graphToggle(c, t, "Arrows", "Show the direction of each link", &g.arrows) || changed
				changed = graphSlider(c, t, "Text fade threshold", &g.textFade, 0, 1) || changed
				changed = graphSlider(c, t, "Node size", &g.nodeScale, 0.4, 2.5) || changed
				changed = graphSlider(c, t, "Link thickness", &g.linkWidth, 0.4, 3) || changed

				graphSection(c, t, "Forces")
				changed = graphSliderDirty(c, t, "Center force", &g.forces.center, 0, 1, g) || changed
				changed = graphSliderDirty(c, t, "Repel force", &g.forces.repel, 0, 3, g) || changed
				changed = graphSliderDirty(c, t, "Link force", &g.forces.link, 0, 3, g) || changed
				changed = graphSliderDirty(c, t, "Link distance", &g.forces.linkRange, 0.3, 3, g) || changed
			})
		})
	})
	if changed {
		a.saveSoon()
	}
}

// graphSection is a small bold heading inside the settings panel.
func graphSection(c *ui.Context, t *ui.Theme, title string) {
	ui.Text(c, title).FontSize(10.5).FontWeight(650).LetterSpacing(0.8).
		TextColor(t.TextMuted).Padding(4, 0, 0, 0)
}

// graphToggle is a labelled switch row; it reports whether it was flipped.
func graphToggle(c *ui.Context, t *ui.Theme, title, desc string, on *bool) bool {
	changed := false
	ui.Row(c).FillWidth().Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Column(c).Grow(1).Gap(1).Children(func() {
			ui.Text(c, title).FontSize(12).TextColor(t.Text)
			if desc != "" {
				ui.Text(c, desc).FontSize(10.5).TextColor(t.TextMuted)
			}
		})
		changed = ui.Switch(c, on).Changed()
	})
	return changed
}

// graphSlider is a labelled slider with its value shown; it reports whether
// the value changed.
func graphSlider(c *ui.Context, t *ui.Theme, title string, v *float64, lo, hi float64) bool {
	return graphSliderRow(c, t, title, v, lo, hi, false, nil)
}

// graphSliderDirty is a slider that relayouts the graph when it changes.
func graphSliderDirty(c *ui.Context, t *ui.Theme, title string, v *float64, lo, hi float64, g *graphView) bool {
	return graphSliderRow(c, t, title, v, lo, hi, true, g)
}

func graphSliderRow(c *ui.Context, t *ui.Theme, title string, v *float64, lo, hi float64, dirty bool, g *graphView) bool {
	changed := false
	ui.Column(c).FillWidth().Gap(2).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(11.5).TextColor(t.Text).Grow(1)
			ui.Textf(c, "%.2f", *v).FontSize(10.5).TextColor(t.TextMuted)
		})
		if ui.Slider(c, v, lo, hi).FillWidth().Height(18).Changed() {
			changed = true
			if dirty && g != nil {
				g.forcesDirty = true
			}
		}
	})
	return changed
}

// drawGraph paints edges and nodes. The camera is in the painter's
// window-absolute space, so the rect's origin is baked into the pan centre
// and the node positions come out already in painter coordinates.
func (a *app) drawGraph(p *ui.Painter, r ui.Rect, g *graphView, t *ui.Theme, active string) {
	if !g.fitted || g.fitW != r.W || g.fitH != r.H || g.fitOX != r.X || g.fitOY != r.Y {
		g.fitW, g.fitH = r.W, r.H
		g.fitOX, g.fitOY = r.X, r.Y
		g.fit(r)
	}
	sx := func(x float64) float64 { return x*g.zoom + g.panX }
	sy := func(y float64) float64 { return y*g.zoom + g.panY }

	visible := make([]bool, len(g.nodes))
	for i := range g.nodes {
		visible[i] = a.graphVisible(g, i)
	}
	// Hovering a node focuses it: the node and its neighbours stay lit, the
	// rest of the graph fades back, the way Obsidian highlights connections.
	var focus map[int]bool
	if g.hovered >= 0 {
		focus = graphNeighbors(g.edges, g.hovered)
	}
	lit := func(i int) bool {
		return focus == nil || focus[i]
	}

	// Edges behind the nodes.
	for _, e := range g.edges {
		if !visible[e.a] || !visible[e.b] {
			continue
		}
		a1, b1 := g.nodes[e.a], g.nodes[e.b]
		x1, y1 := sx(a1.x), sy(a1.y)
		x2, y2 := sx(b1.x), sy(b1.y)
		col := faint(t)
		width := float32(g.linkWidth)
		switch {
		case focus != nil && focus[e.a] && focus[e.b]:
			col = t.TextMuted
			width = float32(g.linkWidth) + 0.6
		case focus != nil:
			col = faint(t).Alpha(0.14)
		case a1.rel == active || b1.rel == active:
			col = t.TextMuted
		}
		p.Line(float32(x1), float32(y1), float32(x2), float32(y2), width, col)
		if g.arrows && lit(e.a) && lit(e.b) {
			a.drawArrow(p, x1, y1, x2, y2, col, float32(g.linkWidth))
		}
	}

	// Node dots, then the labels above them.
	labelZoom := 30 + g.textFade*800
	showLabels := g.showLabels && (g.zoom >= labelZoom || g.hovered >= 0)
	for i := range g.nodes {
		if !visible[i] {
			continue
		}
		n := &g.nodes[i]
		x, y := sx(n.x), sy(n.y)
		radius := float32(g.nodeRadius(n))
		isActive := n.rel == active
		isHover := i == g.hovered
		col := t.TextMuted
		alpha := float32(1)
		switch {
		case isActive:
			col = t.Text
		case isHover:
			col = t.Text
		case focus != nil && !focus[i]:
			col = faint(t)
			alpha = 0.30
		}
		dot := ui.Rect{X: float32(x) - radius, Y: float32(y) - radius, W: radius * 2, H: radius * 2}
		if isActive || isHover {
			p.Fill(ui.Rect{X: dot.X - 3, Y: dot.Y - 3, W: dot.W + 6, H: dot.H + 6},
				bgTint(t, col), radius+3)
		}
		p.Fill(dot, col.Alpha(alpha), radius)
	}

	if !showLabels {
		return
	}
	// Labels are drawn in priority order — the active note and the hover
	// first, then the busiest hubs — and a label is skipped when it would
	// sit on one already drawn, the way Obsidian thins its labels out.
	order := make([]int, 0, len(g.nodes))
	for i := range g.nodes {
		if visible[i] {
			order = append(order, i)
		}
	}
	rank := func(i int) float64 {
		switch {
		case g.nodes[i].rel == active:
			return 3
		case i == g.hovered:
			return 2
		}
		return 1
	}
	sort.SliceStable(order, func(a, b int) bool {
		ra, rb := rank(order[a]), rank(order[b])
		if ra != rb {
			return ra > rb
		}
		return g.nodes[order[a]].degree > g.nodes[order[b]].degree
	})
	var placed []labelBand
	for _, i := range order {
		n := &g.nodes[i]
		x, y := float32(sx(n.x)), float32(sy(n.y))
		radius := float32(g.nodeRadius(n)) + 2
		label := n.label
		runes := []rune(label)
		if len(runes) > 26 {
			label = string(runes[:25]) + "…"
		}
		lw := float32(len([]rune(label))) * 5.9
		b := labelBand{x0: x - lw/2, x1: x + lw/2, y0: y + radius + 2, y1: y + radius + 15}
		if overlaps(placed, b) {
			continue
		}
		placed = append(placed, b)
		lc := t.TextMuted
		switch {
		case n.rel == active || i == g.hovered:
			lc = t.Text
		case focus != nil && !focus[i]:
			lc = faint(t)
		}
		p.Text(b.x0, b.y0, label, 11, lc)
	}
}

// labelBand is a drawn label's screen rectangle, for collision culling.
type labelBand struct{ x0, x1, y0, y1 float32 }

// overlaps reports whether a label band would sit on any already placed.
func overlaps(placed []labelBand, b labelBand) bool {
	for _, p := range placed {
		if b.x0 < p.x1 && b.x1 > p.x0 && b.y0 < p.y1 && b.y1 > p.y0 {
			return true
		}
	}
	return false
}

// drawArrow paints a small arrowhead partway along a link, angled with it.
func (a *app) drawArrow(p *ui.Painter, x1, y1, x2, y2 float64, col ui.Color, width float32) {
	const ahead = 0.72 // how far along the link the head sits
	tipX := x1 + (x2-x1)*ahead
	tipY := y1 + (y2-y1)*ahead
	ang := math.Atan2(y2-y1, x2-x1)
	size := float64(width) + 4.5
	ha := 0.42 // half-angle of the head
	ax := tipX - size*math.Cos(ang-ha)
	ay := tipY - size*math.Sin(ang-ha)
	bx := tipX - size*math.Cos(ang+ha)
	by := tipY - size*math.Sin(ang+ha)
	p.Line(float32(ax), float32(ay), float32(tipX), float32(tipY), width, col)
	p.Line(float32(bx), float32(by), float32(tipX), float32(tipY), width, col)
}

// bgTint is a soft halo behind a highlighted dot: the theme background laid
// over the dot's colour, so it reads on either appearance.
func bgTint(t *ui.Theme, _ ui.Color) ui.Color {
	if t.Dark {
		return ui.RGBA(26, 26, 25, 0.9)
	}
	return ui.RGBA(252, 252, 251, 0.9)
}

// graphVisible reports whether a node passes the orphan and filter options.
func (a *app) graphVisible(g *graphView, i int) bool {
	n := g.nodes[i]
	if !g.showOrphans && n.degree == 0 {
		return false
	}
	if q := strings.TrimSpace(strings.ToLower(g.filter)); q != "" {
		return strings.Contains(strings.ToLower(n.label), q)
	}
	return true
}

// graphPointer handles panning, node dragging and node picking on the canvas.
func (a *app) graphPointer(c *ui.Context, el ui.Element, g *graphView) {
	// PointerPosition is element-local; the camera is window-absolute, so
	// shift into the canvas's window origin from the last frame's layout.
	px, py, over := el.PointerPosition()
	x := float64(px) + float64(g.fitOX)
	y := float64(py) + float64(g.fitOY)
	g.hovered = -1
	if over {
		if i, ok := g.pick(x, y); ok {
			g.hovered = i
		}
	}
	if el.Dragging() {
		if dx, dy, ok := el.Dragged(); ok {
			if g.held < 0 && !g.moved && g.hovered >= 0 {
				g.held = g.hovered
			}
			if g.held >= 0 {
				g.nodes[g.held].x += float64(dx) / g.zoom
				g.nodes[g.held].y += float64(dy) / g.zoom
			} else {
				g.panX += float64(dx)
				g.panY += float64(dy)
			}
			g.moved = true
		}
	}
	if el.Clicked() && !g.moved {
		if i, ok := g.pick(x, y); ok {
			g.clicked = i
		}
	}
	if !el.Pressed() {
		g.moved = false
		g.held = -1
	}
}

// pick returns the node under a window-absolute point, the smallest hit when
// several overlap.
func (g *graphView) pick(sx, sy float64) (int, bool) {
	best, found := -1, false
	bestD := math.Inf(1)
	for i := range g.nodes {
		n := &g.nodes[i]
		x, y := n.x*g.zoom+g.panX, n.y*g.zoom+g.panY
		radius := math.Max(g.nodeRadius(n), 8)
		d := math.Hypot(sx-x, sy-y)
		if d <= radius && d < bestD {
			best, found, bestD = i, true, d
		}
	}
	return best, found
}
