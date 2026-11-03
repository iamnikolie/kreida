package board

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

var (
	dirs     = map[string]bool{"LR": true, "RL": true, "TB": true, "BT": true}
	nodeKeys = keySet("id", "title", "text", "kind", "group", "icon", "shape", "width", "step", "fields", "live", "cols",
		"rows", "x", "y")
	liveKeys  = keySet("cmd", "url", "every", "path", "label")
	groupKeys = keySet("id", "label", "title", "kind", "direction", "parent", "nodes", "collapsed")
	stepKeys  = keySet("title", "show", "hide")
	cellKeys  = keySet("text", "score")
	axisKeys  = keySet("label", "low", "high")
	askKeys   = keySet("text", "done", "on")
	termKeys  = keySet("text", "aka")
	ends      = map[string]bool{"low": true, "high": true}
	corners   = []string{"low-low", "low-high", "high-low", "high-high"}

	// "name", "name | note", "city_id -> cities | note" (FK), "x -.-> t" (optional FK)
	fieldRe = regexp.MustCompile(`^(.+?)(?:\s*(-\.->|->)\s*([^|]+?))?(?:\s+\|\s+(.+))?$`)
	arrowRe = regexp.MustCompile(`\s*(-\.->|->|~~~|=>)\s*`)
	// "a -> b: label" — the label follows the first colon after the last hop's target
	edgeLabelRe = regexp.MustCompile(`^(.*?(?:->|~~~|=>)\s*[^:]+?)\s*:\s*(.+)$`)
)

func keySet(keys ...string) map[string]bool {
	m := make(map[string]bool, len(keys))
	for _, k := range keys {
		m[k] = true
	}

	return m
}

// Parse reads board text. A YAML syntax problem comes back as a *SyntaxError, a broken rule of the format as an
// *Error; both print the message the user should see.
func Parse(text string) (b *Board, err error) {
	doc, err := parseYAML(text)
	if err != nil {
		return nil, err
	}

	defer func() {
		if r := recover(); r != nil {
			e, ok := r.(*Error)
			if !ok {
				panic(r)
			}

			b, err = nil, e
		}
	}()

	return build(deref(rootOf(doc))), nil
}

// fail stops the parse with a rule error; Parse recovers it.
func fail(format string, a ...any) { panic(&Error{Msg: fmt.Sprintf(format, a...)}) }

// text is a scalar's text: "" for a missing or null value; a map or a list where text belongs is an error.
func text(n *yaml.Node, what string) string {
	n = deref(n)

	switch {
	case isNull(n):
		return ""
	case n.Kind != yaml.ScalarNode:
		fail("%s must be text, not a map or a list", what)
	}

	return n.Value
}

// textPtr is text, nil when there is none.
func textPtr(n *yaml.Node, what string) *string {
	if s := text(n, what); s != "" {
		return &s
	}

	return nil
}

// number reads a finite number; ok is false for anything else.
func number(n *yaml.Node, what string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.TrimSpace(text(n, what)), 64)

	return f, err == nil && !math.IsInf(f, 0) && !math.IsNaN(f)
}

// flag reads a yes/no option: anything but empty, false and 0 is yes.
func flag(n *yaml.Node, what string) bool {
	s := text(n, what)

	return s != "" && s != "0" && !strings.EqualFold(s, "false")
}

// strict fails loudly on a key the format does not know: an unquoted comma inside { } silently splits a value into a
// junk key otherwise.
func strict(what string, m *yaml.Node, keys map[string]bool) {
	for i := 0; i < len(m.Content); i += 2 {
		if k := text(m.Content[i], what+" key"); !keys[k] {
			fail("%s: unknown key \"%s\" — quote values that contain commas or colons", what, k)
		}
	}
}

// splitTitle turns "Title | secondary text" into its two parts.
func splitTitle(s string) (title, text string) {
	title, text, _ = strings.Cut(s, " | ")

	return strings.TrimSpace(title), strings.TrimSpace(text)
}

type entry struct {
	id string
	v  *yaml.Node
}

// entries reads a section given as a map (id -> value) or as a list of values that carry their own id (n0, n1, ...
// when they do not).
func entries(n *yaml.Node, what string) []entry {
	var out []entry

	switch n = deref(n); {
	case isNull(n):
	case n.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			out = append(out, entry{text(n.Content[i], what+" id"), deref(n.Content[i+1])})
		}
	case n.Kind == yaml.SequenceNode:
		for i, item := range n.Content {
			id := text(get(item, "id"), what+" id")
			if id == "" {
				id = "n" + strconv.Itoa(i)
			}

			out = append(out, entry{id, deref(item)})
		}
	default:
		fail("%s must be a map (id: value) or a list", what)
	}

	return out
}

// list is the items of a sequence; a missing value is an empty list.
func list(n *yaml.Node, what string) []*yaml.Node {
	switch n = deref(n); {
	case isNull(n):
		return nil
	case n.Kind != yaml.SequenceNode:
		fail("%s must be a list", what)
	}

	return n.Content
}

func buildLive(id string, n *yaml.Node) *Live {
	if isNull(n) {
		return nil
	}

	if n.Kind != yaml.MappingNode {
		fail("node %s: live must be a map {cmd or url, every}", id)
	}

	what := "node " + id + " live"
	strict(what, n, liveKeys)

	cmd, url := text(get(n, "cmd"), what+" cmd"), text(get(n, "url"), what+" url")
	if (cmd == "") == (url == "") {
		fail("node %s: live needs exactly one of cmd, url", id)
	}

	l := &Live{Every: 30, Path: textPtr(get(n, "path"), what+" path"), Label: text(get(n, "label"), what+" label")}

	if cmd != "" {
		l.Source = "cmd:" + cmd
	} else if strings.HasPrefix(url, "http://") || strings.HasPrefix(url, "https://") {
		l.Source = "url:" + url
	} else {
		fail("node %s: live url must be http(s)", id)
	}

	// every: seconds between runs, at least 5
	if f, ok := number(get(n, "every"), what+" every"); ok && f != 0 {
		l.Every = math.Max(5, math.Min(f, 1e9))
	}

	return l
}

// field reads a table field. An unquoted "name: note" arrives from YAML as a one-key map.
func field(n *yaml.Node, what string) Field {
	s := ""

	if n.Kind == yaml.MappingNode && len(n.Content) >= 2 {
		s = text(n.Content[0], what)
		if note := text(n.Content[1], what); note != "" {
			s += " | " + note
		}
	} else {
		s = text(n, what)
	}

	m := fieldRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return Field{Name: "?"}
	}

	f := Field{Name: strings.TrimSpace(m[1]), Dashed: m[2] == "-.->", Note: strings.TrimSpace(m[4])}
	if m[3] != "" {
		ref := strings.TrimSpace(m[3])
		f.Ref = &ref
	}

	return f
}

func buildNode(id string, n *yaml.Node) Node {
	node := Node{ID: id, Title: id, Kind: "plain"}

	switch {
	case isNull(n):
		return node
	case n.Kind == yaml.ScalarNode:
		node.Title, node.Text = splitTitle(n.Value)
		if node.Title == "" && node.Text == "" {
			node.Title = id
		}

		return node
	case n.Kind != yaml.MappingNode:
		fail("node %s must be \"Title | text\" or a map", id)
	}

	what := "node " + id
	strict(what, n, nodeKeys)

	if s := text(get(n, "id"), what+" id"); s != "" {
		node.ID, node.Title = s, s
	}

	title, body := get(n, "title"), get(n, "text")

	switch {
	case !isNull(title) && isNull(body):
		node.Title, node.Text = splitTitle(text(title, what+" title"))
	case !isNull(title):
		node.Title, node.Text = text(title, what+" title"), text(body, what+" text")
	default:
		node.Text = text(body, what+" text")
	}

	if k := text(get(n, "kind"), what+" kind"); k != "" {
		node.Kind = k
	}

	node.Group = textPtr(get(n, "group"), what+" group")
	node.Icon = textPtr(get(n, "icon"), what+" icon")
	node.Shape = textPtr(get(n, "shape"), what+" shape")

	if w, ok := number(get(n, "width"), what+" width"); ok && w != 0 {
		node.Width = &w
	}

	if s, ok := number(get(n, "step"), what+" step"); ok {
		s += 0 // -0 is 0
		node.Step = &s
	}

	if fs := get(n, "fields"); !isNull(fs) {
		node.Fields = []Field{}
		seen := map[string]bool{}

		for _, item := range list(fs, what+" fields") {
			f := field(deref(item), what+" field")
			if seen[f.Name] {
				fail("node %s: field \"%s\" twice", node.ID, f.Name)
			}

			seen[f.Name] = true
			node.Fields = append(node.Fields, f)
		}
	}

	node.Live = buildLive(node.ID, get(n, "live"))
	node.Cols, node.Rows = buildTable(node.ID, n)

	if node.Rows != nil && node.Fields != nil {
		fail("node %s: fields or cols/rows, not both", node.ID)
	}

	node.X, node.Y = text(get(n, "x"), what+" x"), text(get(n, "y"), what+" y")

	return node
}

// buildTable reads a table card's `cols:` (header texts) and `rows:` (lists of cells). Rows are padded to the widest
// of cols and rows; a row wider than cols is an error.
func buildTable(id string, n *yaml.Node) ([]string, [][]Cell) {
	what := "node " + id
	cn, rn := get(n, "cols"), get(n, "rows")

	if isNull(cn) && isNull(rn) {
		return nil, nil
	}

	var cols []string
	for _, c := range list(cn, what+": cols") {
		cols = append(cols, text(c, what+" col"))
	}

	rows := [][]Cell{}
	width := len(cols)

	for i, r := range list(rn, what+": rows") {
		if r = deref(r); r.Kind != yaml.SequenceNode {
			fail("node %s: row %d must be a list of cells", id, i+1)
		}

		row := make([]Cell, 0, len(r.Content))
		for _, c := range r.Content {
			row = append(row, cell(deref(c), fmt.Sprintf("%s row %d", what, i+1)))
		}

		if cols != nil && len(row) > len(cols) {
			fail("node %s: row %d has %d cells, cols has %d", id, i+1, len(row), len(cols))
		}

		width = max(width, len(row))
		rows = append(rows, row)
	}

	for i := range rows {
		for len(rows[i]) < width {
			rows[i] = append(rows[i], Cell{})
		}
	}

	return cols, rows
}

// cell reads a table cell: text, or {text, score}.
func cell(n *yaml.Node, what string) Cell {
	if isNull(n) || n.Kind != yaml.MappingNode {
		return Cell{Text: text(n, what)}
	}

	strict(what+" cell", n, cellKeys)

	c := Cell{Text: text(get(n, "text"), what+" cell text")}

	if s := get(n, "score"); !isNull(s) {
		f, ok := number(s, what+" score")
		if !ok {
			fail("%s: score must be a number", what)
		}

		c.Score = &f
	}

	return c
}

// buildAxis reads a matrix axis: {label, low, high}; the ends default to the words low and high.
func buildAxis(n *yaml.Node, what string) Axis {
	a := Axis{Low: "low", High: "high"}

	switch n = deref(n); {
	case isNull(n):
		return a
	case n.Kind == yaml.ScalarNode:
		a.Label = n.Value

		return a
	}

	if n.Kind != yaml.MappingNode {
		fail("%s must be a label or {label, low, high}", what)
	}

	strict(what, n, axisKeys)
	a.Label = text(get(n, "label"), what+" label")

	if s := text(get(n, "low"), what+" low"); s != "" {
		a.Low = s
	}

	if s := text(get(n, "high"), what+" high"); s != "" {
		a.High = s
	}

	return a
}

func corner(s, what string) string {
	if !slices.Contains(corners, s) {
		fail("%s must be one of %s", what, strings.Join(corners, ", "))
	}

	return s
}

// buildMatrix reads the 2×2 of `layout: matrix` and checks that every card in it names its quadrant right.
func buildMatrix(root *yaml.Node, nodes []Node) *Matrix {
	m := &Matrix{X: buildAxis(get(root, "x"), "x"), Y: buildAxis(get(root, "y"), "y"), Quadrants: map[string]string{}}

	if s := text(get(root, "hot"), "hot"); s != "" {
		m.Hot = corner(s, "hot")
	}

	switch q := deref(get(root, "quadrants")); {
	case isNull(q):
	case q.Kind != yaml.MappingNode:
		fail("quadrants must be a map (corner: label)")
	default:
		for i := 0; i+1 < len(q.Content); i += 2 {
			k := text(q.Content[i], "quadrants key")
			m.Quadrants[corner(k, fmt.Sprintf("quadrants: %q", k))] = text(q.Content[i+1], "quadrants label")
		}
	}

	for _, n := range nodes {
		for _, v := range [][2]string{{"x", n.X}, {"y", n.Y}} {
			if v[1] != "" && !ends[v[1]] {
				fail("node %s: %s must be low or high", n.ID, v[0])
			}
		}
	}

	return m
}

// buildQuestions reads `questions:`: a list of texts or {text, done, on}.
func buildQuestions(n *yaml.Node, known func(string) bool) []Question {
	var out []Question

	for i, item := range list(n, "questions") {
		what := fmt.Sprintf("question %d", i+1)
		q := Question{}

		if item = deref(item); !isNull(item) && item.Kind == yaml.MappingNode {
			strict(what, item, askKeys)
			q = Question{Text: text(get(item, "text"), what+" text"), Done: flag(get(item, "done"), what+" done"),
				On: text(get(item, "on"), what+" on")}
		} else {
			q.Text = text(item, what)
		}

		if q.Text = strings.TrimSpace(q.Text); q.Text == "" {
			fail("%s needs text", what)
		}

		if q.On != "" && !known(q.On) {
			fail("%s: unknown node or group \"%s\"", what, q.On)
		}

		out = append(out, q)
	}

	return out
}

// buildTerms reads `terms:`: term -> definition, or term -> {text, aka: [aliases]}.
func buildTerms(n *yaml.Node) []Term {
	var out []Term

	switch n = deref(n); {
	case isNull(n):
		return nil
	case n.Kind != yaml.MappingNode:
		fail("terms must be a map (term: definition)")
	}

	for i := 0; i+1 < len(n.Content); i += 2 {
		term := text(n.Content[i], "term")
		what := "term " + term
		t, v := Term{Term: term, Aka: []string{}}, deref(n.Content[i+1])

		if !isNull(v) && v.Kind == yaml.MappingNode {
			strict(what, v, termKeys)
			t.Text = text(get(v, "text"), what+" text")

			for _, a := range list(get(v, "aka"), what+" aka") {
				t.Aka = append(t.Aka, text(a, what+" aka"))
			}
		} else {
			t.Text = text(v, what)
		}

		if t.Text = strings.TrimSpace(t.Text); t.Text == "" {
			fail("%s needs a definition", what)
		}

		out = append(out, t)
	}

	return out
}

// buildGroup reads a group and the ids its `nodes:` list claims.
func buildGroup(id string, n *yaml.Node) (Group, []string) {
	g := Group{ID: id, Label: id}

	switch {
	case isNull(n):
		return g, nil
	case n.Kind == yaml.ScalarNode:
		g.Label = n.Value

		return g, nil
	case n.Kind != yaml.MappingNode:
		fail("group %s must be a label or a map", id)
	}

	what := "group " + id
	strict(what, n, groupKeys)

	if s := text(get(n, "id"), what+" id"); s != "" {
		g.ID, g.Label = s, s
	}

	if s := text(get(n, "label"), what+" label"); s != "" {
		g.Label = s
	} else if s := text(get(n, "title"), what+" title"); s != "" {
		g.Label = s
	}

	g.Kind = textPtr(get(n, "kind"), what+" kind")
	g.Parent = textPtr(get(n, "parent"), what+" parent")
	g.Collapsed = flag(get(n, "collapsed"), what+" collapsed")

	if d := text(get(n, "direction"), what+" direction"); dirs[d] {
		g.Direction = &d
	}

	var members []string
	for _, m := range list(get(n, "nodes"), what+" nodes") {
		members = append(members, text(m, what+" nodes"))
	}

	return g, members
}

// edgeString reads "a -> b -> c: label", "a -.-> b" (dashed), "s1 ~~~ s2" (invisible, layout only), "a => b"
// (thick). The label goes on the last hop.
func edgeString(s string) []Edge {
	label := ""

	if m := edgeLabelRe.FindStringSubmatch(s); m != nil {
		s, label = m[1], strings.TrimSpace(m[2])
	}

	ops := arrowRe.FindAllStringSubmatchIndex(s, -1)

	var out []Edge

	from := s
	if len(ops) > 0 {
		from = s[:ops[0][0]]
	}

	for i, m := range ops {
		end := len(s)
		if i+1 < len(ops) {
			end = ops[i+1][0]
		}

		op, to := s[m[2]:m[3]], s[m[1]:end]
		out = append(out, Edge{From: strings.TrimSpace(from), To: strings.TrimSpace(to),
			Dashed: op == "-.->", Hidden: op == "~~~", Thick: op == "=>"})
		from = to
	}

	if len(out) > 0 {
		out[len(out)-1].Label = label
	}

	return out
}

// buildEdge reads one item of `edges:`: a string, [from, to, label], {from, to, ...} or the one-key map YAML makes of
// an unquoted "a -> b: label".
func buildEdge(n *yaml.Node) []Edge {
	const what = "edge"

	switch n.Kind {
	case yaml.SequenceNode:
		at := func(i int) string {
			if i < len(n.Content) {
				return text(n.Content[i], what)
			}

			return ""
		}

		return []Edge{{From: at(0), To: at(1), Label: at(2)}}
	case yaml.MappingNode:
		if len(n.Content) == 2 && isNull(get(n, "from")) {
			if k := text(n.Content[0], what); arrowRe.MatchString(k) {
				return edgeString(k + ": " + text(n.Content[1], what))
			}
		}

		return []Edge{{From: text(get(n, "from"), what+" from"), To: text(get(n, "to"), what+" to"),
			Label:  text(get(n, "label"), what+" label"),
			Dashed: flag(get(n, "dashed"), what), Hidden: flag(get(n, "hidden"), what),
			Thick: flag(get(n, "thick"), what), Animated: flag(get(n, "animated"), what)}}
	}

	return edgeString(text(n, what))
}

// ids reads a step's show/hide: a list of ids or "a, b".
func ids(n *yaml.Node, what string) []string {
	out := []string{}

	if n = deref(n); !isNull(n) && n.Kind == yaml.SequenceNode {
		for _, item := range n.Content {
			out = append(out, text(item, what))
		}

		return out
	}

	for _, s := range strings.Split(text(n, what), ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}

	return out
}

// buildStep reads a presentation step: {title, show: [ids], hide: [ids]} or just the ids to show.
func buildStep(i int, n *yaml.Node) Step {
	what := "step " + strconv.Itoa(i+1)

	if isNull(n) || n.Kind != yaml.MappingNode {
		return Step{Show: ids(n, what), Hide: []string{}}
	}

	strict(what, n, stepKeys)

	return Step{Title: text(get(n, "title"), what+" title"), Show: ids(get(n, "show"), what+" show"),
		Hide: ids(get(n, "hide"), what+" hide")}
}

// buildLegend reads {kind: label} or a list of "kind label words" / {kind, label}.
func buildLegend(n *yaml.Node) []LegendItem {
	out := []LegendItem{}

	switch n = deref(n); {
	case isNull(n):
	case n.Kind == yaml.MappingNode:
		for i := 0; i+1 < len(n.Content); i += 2 {
			out = append(out, LegendItem{text(n.Content[i], "legend kind"), text(n.Content[i+1], "legend label")})
		}
	default:
		for _, item := range list(n, "legend") {
			switch item = deref(item); {
			case isNull(item):
			case item.Kind == yaml.MappingNode:
				out = append(out, LegendItem{text(get(item, "kind"), "legend kind"), text(get(item, "label"), "legend label")})
			default:
				kind, label, _ := strings.Cut(strings.Join(strings.Fields(text(item, "legend")), " "), " ")
				out = append(out, LegendItem{kind, label})
			}
		}
	}

	return out
}

func build(root *yaml.Node) *Board {
	if root == nil || root.Kind != yaml.MappingNode {
		fail("board must be a YAML map")
	}

	b := &Board{
		Title: text(get(root, "title"), "title"), Sub: text(get(root, "sub"), "sub"),
		Direction: "LR", Layout: "auto", Style: "clean",
		Legend: buildLegend(get(root, "legend")),
		Groups: []Group{}, Nodes: []Node{}, Edges: []Edge{}, Steps: []Step{},
	}

	if d := text(get(root, "direction"), "direction"); dirs[d] {
		b.Direction = d
	}

	if l := text(get(root, "layout"), "layout"); l == "lanes" || l == "matrix" || l == "radial" {
		b.Layout = l
	}

	if l := strings.ToLower(text(get(root, "lang"), "lang")); l == "ru" || l == "uk" || l == "en" {
		b.Lang = l
	}

	if text(get(root, "style"), "style") == "sketch" {
		b.Style = "sketch"
	}

	isGroup := map[string]bool{}
	claims := map[string]string{} // node id -> the group whose `nodes:` lists it

	for _, e := range entries(get(root, "groups"), "groups") {
		g, members := buildGroup(e.id, e.v)
		for _, m := range members {
			claims[m] = g.ID
		}

		b.Groups = append(b.Groups, g)
		isGroup[g.ID] = true
	}

	index := map[string]int{}

	for _, e := range entries(get(root, "nodes"), "nodes") {
		n := buildNode(e.id, e.v)
		if isGroup[n.ID] {
			fail("id \"%s\" is both a node and a group", n.ID)
		}

		index[n.ID] = len(b.Nodes)
		b.Nodes = append(b.Nodes, n)
	}

	for id, g := range claims {
		if i, ok := index[id]; ok {
			b.Nodes[i].Group = &g
		}
	}

	for _, item := range list(get(root, "edges"), "edges") {
		for _, e := range buildEdge(deref(item)) {
			if e.From != "" && e.To != "" {
				b.Edges = append(b.Edges, e)
			}
		}
	}

	// FK fields draw their own edge: referenced table (its id row, if any) -> the FK row. One -> many.
	for _, n := range b.Nodes {
		for _, f := range n.Fields {
			if f.Ref != nil && *f.Ref != "" {
				b.Edges = append(b.Edges, Edge{From: *f.Ref, To: n.ID, Dashed: f.Dashed, ToPort: f.Name})
			}
		}
	}

	// unknown ids in edges become plain cards
	for _, e := range b.Edges {
		for _, id := range []string{e.From, e.To} {
			if _, ok := index[id]; !ok && !isGroup[id] {
				n := Node{ID: id, Kind: "plain"}
				n.Title, n.Text = splitTitle(id)
				index[id] = len(b.Nodes)
				b.Nodes = append(b.Nodes, n)
			}
		}
	}

	for i := range b.Edges {
		e := &b.Edges[i]
		e.ID = fmt.Sprintf("e%d-%s-%s", i, e.From, e.To)

		if e.ToPort == "" {
			continue
		}

		e.ID += "-" + e.ToPort

		if j, ok := index[e.From]; ok {
			for _, f := range b.Nodes[j].Fields {
				if f.Name == "id" {
					e.FromPort = "id"
				}
			}
		}
	}

	if b.Layout == "matrix" {
		b.Matrix = buildMatrix(root, b.Nodes)
	}

	known := func(id string) bool { _, ok := index[id]; return ok || isGroup[id] }
	b.Questions = buildQuestions(get(root, "questions"), known)
	b.Terms = buildTerms(get(root, "terms"))

	for i, item := range list(get(root, "steps"), "steps") {
		s := buildStep(i, deref(item))

		for _, id := range append(append([]string{}, s.Show...), s.Hide...) {
			if _, ok := index[id]; !ok && !isGroup[id] {
				fail("step %d: unknown node or group \"%s\"", i+1, id)
			}
		}

		b.Steps = append(b.Steps, s)
	}

	return b
}
