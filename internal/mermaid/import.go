// Package mermaid converts between Mermaid diagrams and boards. Import reads flowchart/graph and sequenceDiagram into
// board YAML; export writes a normalized board as a flowchart. `%% title:`,
// `%% sub:` and `%% legend:` comments carry what Mermaid has no syntax for.
package mermaid

import (
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var kinds = map[string]bool{"src": true, "core": true, "out": true, "warn": true, "info": true, "who": true, "note": true, "p2": true, "plain": true}

var dirMap = map[string]string{"TD": "TB", "TB": "TB", "LR": "LR", "RL": "RL", "BT": "BT"}

type shape struct{ open, close, name string }

// Node shapes, longest opener first.
var shapes = []shape{
	{"(((", ")))", "pill"}, {"([", "])", "pill"}, {"[[", "]]", ""}, {"[(", ")]", "db"}, {"((", "))", "pill"},
	{"{{", "}}", ""}, {"[/", "/]", ""}, {"[\\", "\\]", ""}, {"[/", "\\]", ""}, {"[\\", "/]", ""},
	{"[", "]", ""}, {"(", ")", ""}, {"{", "}", ""}, {">", "]", ""},
}

var (
	quotedRe = regexp.MustCompile(`^"((?s:.*))"$`)
	brRe     = regexp.MustCompile(`(?i)<br\s*/?>|\\n`)
	tagRe    = regexp.MustCompile(`<[^>]+>`)
	classRe  = regexp.MustCompile(`^:::([\w-]+)`)
	ampRe    = regexp.MustCompile(`^\s*&\s*`)
	// -->  ---  -.->  -.-  ==>  ===  ~~~  --o  --x  <-->, optional |label|
	opRe = regexp.MustCompile(`^\s*<?(-{2,}|={2,}|-\.+-|~{3,})(>|o|x)?(?:\s*\|([^|]*)\|)?`)
	// -- text -->   -. text .->   == text ==>   (the trailing group stands in for a lookahead: it is not consumed)
	opTextRe = regexp.MustCompile(`^\s*<?(--|-\.|==)\s+(.+?)\s+(-{2,}|\.-+|={2,})(>|o|x)?(\s|$|[\p{L}\p{N}_"])`)

	metaRe      = regexp.MustCompile(`^\s*%%\s*(title|sub|legend)\s*:\s*(.+)$`)
	commentRe   = regexp.MustCompile(`%%.*$`)
	headRe      = regexp.MustCompile(`^(?:flowchart|graph)\s*(\w+)?`)
	subgraphRe  = regexp.MustCompile(`^subgraph\s+(.+)$`)
	subgraphSp  = regexp.MustCompile(`^(\S+?)\s+(\[.*\])$`)
	directionRe = regexp.MustCompile(`^direction\s+(\w+)`)
	classLineRe = regexp.MustCompile(`^class\s+([\w,\s-]+?)\s+([\w-]+)$`)
	skipRe      = regexp.MustCompile(`^(classDef|style|linkStyle|click|accTitle|accDescr)\b`)
	partRe      = regexp.MustCompile(`^(?:participant|actor)\s+(\S+)(?:\s+as\s+(.+))?$`)
	messageRe   = regexp.MustCompile(`^(\S+?)\s*(-{1,2}(?:>>|>|x|\)))\s*[+-]?(\S+?)\s*:\s*(.*)$`)
	notIDRe     = regexp.MustCompile(`[^\p{L}\p{N}_]+`)
)

type labelParts struct{ title, text string }

// label reads "<b>Title</b><br/><span>text</span>" into a title and a secondary text.
func label(s string) labelParts {
	t := strings.TrimSpace(s)
	t = quotedRe.ReplaceAllString(t, "$1")
	t = strings.ReplaceAll(t, "#quot;", `"`)
	t = strings.ReplaceAll(t, "#amp;", "&")

	var lines []string

	for _, l := range brRe.Split(t, -1) {
		if l = strings.TrimSpace(tagRe.ReplaceAllString(l, "")); l != "" {
			lines = append(lines, l)
		}
	}

	var p labelParts
	if len(lines) > 0 {
		p.title = lines[0]
		p.text = strings.Join(lines[1:], " · ")
	}

	return p
}

// scanID returns the length of the id at the start of s: letters, digits and underscores, with hyphens allowed
// unless one starts an arrow (A-->B).
func scanID(s string) int {
	i := 0

	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])

		switch {
		case unicode.IsLetter(r) || unicode.IsNumber(r) || r == '_':
			i += w
		case r == '-' && i > 0 && (i+1 >= len(s) || !strings.ContainsRune("-.>=", rune(s[i+1]))):
			i++
		default:
			return i
		}
	}

	return i
}

type pnode struct {
	id       string
	hasLabel bool
	title    string
	text     string
	shape    string
	cls      string
}

func skipSpace(s string, i int) int {
	for i < len(s) {
		r, w := utf8.DecodeRuneInString(s[i:])
		if !unicode.IsSpace(r) {
			break
		}

		i += w
	}

	return i
}

func readNode(s string, i int) (pnode, int, bool) {
	i = skipSpace(s, i)

	n := scanID(s[i:])
	if n == 0 {
		return pnode{}, 0, false
	}

	node := pnode{id: s[i : i+n]}
	i += n

	for _, sh := range shapes {
		if !strings.HasPrefix(s[i:], sh.open) {
			continue
		}

		j := i + len(sh.open)

		var text string

		if j < len(s) && s[j] == '"' {
			q := strings.IndexByte(s[j+1:], '"')
			if q < 0 {
				continue
			}

			q += j + 1
			text, j = s[j+1:q], q+1

			if !strings.HasPrefix(s[j:], sh.close) {
				continue
			}
		} else {
			k := strings.Index(s[j:], sh.close)
			if k < 0 {
				continue
			}

			k += j
			text, j = s[j:k], k
		}

		l := label(text)
		node.hasLabel, node.title, node.text, node.shape = true, l.title, l.text, sh.name
		i = j + len(sh.close)

		break
	}

	if m := classRe.FindStringSubmatch(s[i:]); m != nil {
		node.cls = m[1]
		i += len(m[0])
	}

	return node, i, true
}

type fnode struct {
	id, title, text, kind, group, icon, shape string
}

// orderedMap is a map that remembers insertion order.
type orderedMap[V any] struct {
	keys []string
	vals map[string]V
}

func newOrdered[V any]() *orderedMap[V] { return &orderedMap[V]{vals: map[string]V{}} }

func (m *orderedMap[V]) has(k string) bool { _, ok := m.vals[k]; return ok }

func (m *orderedMap[V]) set(k string, v V) {
	if !m.has(k) {
		m.keys = append(m.keys, k)
	}

	m.vals[k] = v
}

func (m *orderedMap[V]) delete(k string) {
	if !m.has(k) {
		return
	}

	delete(m.vals, k)

	for i, key := range m.keys {
		if key == k {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)

			break
		}
	}
}

type fgroup struct {
	label     string
	parent    string
	direction string
}

type fedge struct{ from, to, text, kind string }

type collector struct {
	nodes  *orderedMap[*fnode]
	groups *orderedMap[*fgroup]
	edges  []fedge
	stack  []string
}

func (c *collector) node(n pnode) {
	cur := c.nodes.vals[n.id]
	if cur == nil {
		cur = &fnode{id: n.id}
	}

	if n.hasLabel {
		cur.title, cur.text = n.title, n.text
	}

	if n.shape == "pill" {
		cur.shape = "pill"
	}

	if n.shape == "db" {
		cur.icon = "database"
	}

	if n.cls != "" && kinds[n.cls] {
		cur.kind = n.cls
	}

	if !c.nodes.has(n.id) && len(c.stack) > 0 && !c.groups.has(n.id) {
		cur.group = c.stack[len(c.stack)-1]
	}

	c.nodes.set(n.id, cur)
}

// statement reads "A & B --> C -- x --> D" into nodes and edges.
func statement(s string, c *collector) {
	i := 0

	group := func() []string {
		var ids []string

		for {
			n, next, ok := readNode(s, i)
			if !ok {
				return ids
			}

			c.node(n)
			ids = append(ids, n.id)
			i = next

			amp := ampRe.FindString(s[i:])
			if amp == "" {
				return ids
			}

			i += len(amp)
		}
	}

	left := group()
	if len(left) == 0 {
		return
	}

	for {
		rest := s[i:]

		var op, text string

		var n int

		if m := opTextRe.FindStringSubmatchIndex(rest); m != nil {
			op = rest[m[2]:m[3]] + rest[m[6]:m[7]]
			text = rest[m[4]:m[5]]
			n = m[10] // the lookahead character is not part of the match
		} else if m := opRe.FindStringSubmatchIndex(rest); m != nil {
			op = rest[m[2]:m[3]]

			if m[6] >= 0 {
				text = rest[m[6]:m[7]]
			}

			n = m[1]
		} else {
			break
		}

		i += n

		right := group()
		if len(right) == 0 {
			break
		}

		kind := ""

		switch {
		case strings.Contains(op, "~"):
			kind = "hidden"
		case strings.Contains(op, "."):
			kind = "dashed"
		case strings.Contains(op, "="):
			kind = "thick"
		}

		t := label(text).title
		for _, a := range left {
			for _, b := range right {
				c.edges = append(c.edges, fedge{a, b, t, kind})
			}
		}

		left = right
	}
}

func flowchart(lines []string, dir string) doc {
	c := &collector{nodes: newOrdered[*fnode](), groups: newOrdered[*fgroup]()}
	classes := newOrdered[string]()

	for _, line := range lines {
		if m := subgraphRe.FindStringSubmatch(line); m != nil {
			c.subgraph(m[1])

			continue
		}

		if line == "end" {
			if len(c.stack) > 0 {
				c.stack = c.stack[:len(c.stack)-1]
			}

			continue
		}

		if m := directionRe.FindStringSubmatch(line); m != nil {
			if len(c.stack) > 0 {
				if d, ok := dirMap[m[1]]; ok {
					c.groups.vals[c.stack[len(c.stack)-1]].direction = d
				}
			}

			continue
		}

		if m := classLineRe.FindStringSubmatch(line); m != nil {
			for _, id := range strings.Split(m[1], ",") {
				classes.set(strings.TrimSpace(id), m[2])
			}

			continue
		}

		if skipRe.MatchString(line) {
			continue
		}

		statement(line, c)
	}

	for _, id := range classes.keys {
		if cls := classes.vals[id]; c.nodes.has(id) && kinds[cls] {
			c.nodes.vals[id].kind = cls
		}
	}

	for _, id := range c.groups.keys { // edges to a subgraph point at the group, not a new card
		c.nodes.delete(id)
	}

	d, ok := dirMap[dir]
	if !ok {
		d = "TB"
	}

	out := doc{head: []kv{{"direction", d}}, edges: []string{}}

	for _, id := range c.groups.keys {
		g := c.groups.vals[id]
		out.groups = append(out.groups, item{id: id, attrs: []kv{{"label", g.label}, {"parent", g.parent}, {"direction", g.direction}}})
	}

	for _, id := range c.nodes.keys {
		n := c.nodes.vals[id]
		title := n.title
		if title == "" {
			title = n.id
		}

		out.nodes = append(out.nodes, item{id: id, attrs: []kv{{"title", title}, {"text", n.text}, {"kind", n.kind},
			{"group", n.group}, {"icon", n.icon}, {"shape", n.shape}}})
	}

	for _, e := range c.edges {
		out.edges = append(out.edges, edgeString(e))
	}

	return out
}

// subgraph opens a group: "subgraph id[Label]", "subgraph id [Label]" or "subgraph Some title".
func (c *collector) subgraph(rest string) {
	whole := strings.TrimSpace(rest)
	src := whole

	if sp := subgraphSp.FindStringSubmatch(whole); sp != nil {
		src = sp[1] + sp[2]
	}

	n, end, ok := readNode(src, 0)
	full := ok && end >= len(src)

	id, lbl := notIDRe.ReplaceAllString(whole, "_"), whole

	if full {
		id = n.id

		lbl = id
		if n.hasLabel {
			lbl = n.title
		}
	}

	g := &fgroup{label: lbl}
	if len(c.stack) > 0 {
		g.parent = c.stack[len(c.stack)-1]
	}

	c.groups.set(id, g)
	c.nodes.delete(id)
	c.stack = append(c.stack, id)
}

func edgeString(e fedge) string {
	op := "->"

	switch e.kind {
	case "hidden":
		op = "~~~"
	case "dashed":
		op = "-.->"
	case "thick":
		op = "=>"
	}

	s := e.from + " " + op + " " + e.to
	if e.text != "" {
		s += ": " + e.text
	}

	return s
}

// sequence turns participants into lanes and message i into a card in the sender's lane at column i.
func sequence(lines []string) doc {
	parts := newOrdered[string]()
	out := doc{head: []kv{{"layout", "lanes"}}, edges: []string{}}

	who := func(id string) string {
		if !parts.has(id) {
			parts.set(id, id)
		}

		return id
	}

	prev := ""

	for _, line := range lines {
		if m := partRe.FindStringSubmatch(line); m != nil {
			name := m[1]
			if m[2] != "" {
				name = m[2]
			}

			parts.set(m[1], strings.TrimSpace(name))

			continue
		}

		m := messageRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}

		from, to := who(m[1]), who(m[3])
		step := len(out.nodes)
		id := fmt.Sprintf("m%d", step+1)

		title := label(m[4]).title
		if title == "" {
			title = "…"
		}

		kind := ""
		if strings.HasPrefix(m[2], "--") {
			kind = "p2"
		}

		out.nodes = append(out.nodes, item{id: id, attrs: []kv{{"title", title}, {"text", "→ " + parts.vals[to]},
			{"group", from}, {"step", step}, {"kind", kind}}})

		if prev != "" {
			out.edges = append(out.edges, prev+" -> "+id)
		}

		prev = id
	}

	for _, id := range parts.keys {
		out.groups = append(out.groups, item{id: id, attrs: []kv{{"label", parts.vals[id]}}})
	}

	return out
}

// FromMermaid reads a Mermaid flowchart/graph or sequenceDiagram into board YAML.
func FromMermaid(text string) (string, error) {
	meta := map[string]string{}

	var lines []string

	for _, raw := range strings.Split(strings.TrimPrefix(text, "\ufeff"), "\n") {
		if m := metaRe.FindStringSubmatch(raw); m != nil {
			meta[m[1]] = strings.TrimSpace(m[2])

			continue
		}

		for _, part := range strings.Split(commentRe.ReplaceAllString(raw, ""), ";") {
			if l := strings.TrimSpace(part); l != "" {
				lines = append(lines, l)
			}
		}
	}

	head := ""
	if len(lines) > 0 {
		head, lines = lines[0], lines[1:]
	}

	var d doc

	switch m := headRe.FindStringSubmatch(head); {
	case m != nil:
		dir := m[1]
		if dir == "" {
			dir = "TB"
		}

		d = flowchart(lines, dir)
	case strings.HasPrefix(head, "sequenceDiagram"):
		d = sequence(lines)
	default:
		if r := []rune(head); len(r) > 40 {
			head = string(r[:40])
		}

		return "", fmt.Errorf("mermaid: only flowchart/graph and sequenceDiagram are supported (got \"%s\")", head)
	}

	d.title, d.sub = meta["title"], meta["sub"]

	if meta["legend"] != "" {
		for _, it := range strings.Split(meta["legend"], "|") {
			kind, lbl, _ := strings.Cut(strings.Join(strings.Fields(it), " "), " ")
			d.legend = append(d.legend, kv{kind, lbl})
		}
	}

	return d.yaml(), nil
}
