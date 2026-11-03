package board

import (
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

const utf8BOM = "\xef\xbb\xbf"

// errUnsupported marks a layout the text splicer does not handle; the caller falls back to re-encoding the tree.
var errUnsupported = errors.New("board.splice: layout not supported")

type stepKind int

const (
	stepSet    stepKind = iota // replace the value of an existing pair with a string
	stepAdd                    // append a pair
	stepDelete                 // remove a pair
	stepRoot                   // the document is empty: create a map holding one pair
)

// step is one change to a map of the document, expressed on the parsed tree.
type step struct {
	kind stepKind
	m    *yaml.Node // the mapping that holds (or will hold) the pair
	idx  int        // index of the pair's key in m.Content (stepSet, stepDelete)
	key  string     // stepAdd, stepRoot
	val  string
}

// source gives byte offsets for the line/column positions the YAML parser reports.
type source struct {
	text  string
	lines []int // byte offset of the start of each line
	nl    string
}

func newSource(text string) (*source, error) {
	s := &source{text: text, nl: "\n"}

	if strings.Contains(text, "\r\n") {
		s.nl = "\r\n"
	}

	if strings.Contains(strings.ReplaceAll(text, "\r\n", ""), "\r") {
		return nil, errUnsupported
	}

	start := 0
	if strings.HasPrefix(text, utf8BOM) {
		start = len(utf8BOM)
	}

	s.lines = []int{start}

	for i := 0; i < len(text); i++ {
		if text[i] == '\n' {
			s.lines = append(s.lines, i+1)
		}
	}

	return s, nil
}

// offset converts a 1-based line and rune column to a byte offset.
func (s *source) offset(line, col int) (int, bool) {
	if line < 1 || line > len(s.lines) || col < 1 {
		return 0, false
	}

	p := s.lines[line-1]

	for c := 1; c < col; c++ {
		if p >= len(s.text) || s.text[p] == '\n' {
			return 0, false
		}

		_, w := utf8.DecodeRuneInString(s.text[p:])
		p += w
	}

	return p, true
}

func (s *source) at(n *yaml.Node) (int, bool) { return s.offset(n.Line, n.Column) }

type textEdit struct {
	start, end int
	repl       string
}

// scalarSpan finds the source text of a scalar or an alias (`*name`). It handles plain single-line and quoted
// scalars without tags or anchors; anything else (block scalars, multi-line plain scalars) is reported as unsupported.
func (s *source) scalarSpan(n *yaml.Node) (int, int, error) {
	if n.Kind == yaml.AliasNode {
		if start, ok := s.at(n); ok && strings.HasPrefix(s.text[start:], "*"+n.Value) {
			return start, start + 1 + len(n.Value), nil
		}

		return 0, 0, errUnsupported
	}

	if n.Kind != yaml.ScalarNode || n.Anchor != "" || n.Style&(yaml.TaggedStyle|yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return 0, 0, errUnsupported
	}

	start, ok := s.at(n)
	if !ok {
		return 0, 0, errUnsupported
	}

	t := s.text

	switch {
	case n.Style&(yaml.DoubleQuotedStyle|yaml.SingleQuotedStyle) != 0:
		if end, err := quotedEnd(t, start); err == nil {
			return start, end + 1, nil
		}
	case n.Value == "":
		// an empty value sits right after its colon
		if start > 0 && t[start-1] == ':' {
			return start, start, nil
		}
	case !strings.ContainsAny(n.Value, "\n\r") && strings.HasPrefix(t[start:], n.Value):
		return start, start + len(n.Value), nil
	}

	return 0, 0, errUnsupported
}

// flowEnd returns the index of the bracket closing the flow collection that opens at start.
func (s *source) flowEnd(start int) (int, error) {
	t := s.text
	if start >= len(t) || (t[start] != '{' && t[start] != '[') {
		return 0, errUnsupported
	}

	depth := 0
	prev := byte(0) // last significant character, to tell a quote that opens a scalar from an apostrophe inside one

	for i := start; i < len(t); i++ {
		c := t[i]

		switch {
		case c == '{' || c == '[':
			depth++
			prev = c
		case c == '}' || c == ']':
			depth--
			if depth == 0 {
				return i, nil
			}

			prev = c
		case c == '#' && i > 0 && strings.IndexByte(" \t\n", t[i-1]) >= 0:
			j := strings.IndexByte(t[i:], '\n')
			if j < 0 {
				return 0, errUnsupported
			}

			i += j
		case (c == '"' || c == '\'') && strings.IndexByte("{[,:", prev) >= 0:
			end, err := quotedEnd(t, i)
			if err != nil {
				return 0, err
			}

			i = end
			prev = c
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		default:
			prev = c
		}
	}

	return 0, errUnsupported
}

// quotedEnd returns the index of the closing quote of the quoted scalar opening at i.
func quotedEnd(t string, i int) (int, error) {
	if i >= len(t) || t[i] != '"' && t[i] != '\'' {
		return 0, errUnsupported
	}

	q := t[i]

	for j := i + 1; j < len(t); j++ {
		switch {
		case q == '"' && t[j] == '\\':
			j++
		case t[j] == q:
			if q == '\'' && j+1 < len(t) && t[j+1] == '\'' {
				j++

				continue
			}

			return j, nil
		}
	}

	return 0, errUnsupported
}

// nodeEnd is the offset just past the last character of a node's content.
func (s *source) nodeEnd(n *yaml.Node) (int, error) {
	switch n.Kind {
	case yaml.ScalarNode, yaml.AliasNode:
		_, end, err := s.scalarSpan(n)

		return end, err
	case yaml.MappingNode, yaml.SequenceNode:
		if n.Style&yaml.FlowStyle != 0 {
			start, ok := s.at(n)
			if !ok {
				return 0, errUnsupported
			}

			end, err := s.flowEnd(start)

			return end + 1, err
		}

		if len(n.Content) == 0 {
			return 0, errUnsupported
		}

		return s.nodeEnd(n.Content[len(n.Content)-1])
	}

	return 0, errUnsupported
}

func (s *source) setEdit(st step) (textEdit, error) {
	v := st.m.Content[st.idx+1]
	inFlow := st.m.Style&yaml.FlowStyle != 0

	start, end, err := s.scalarSpan(v)
	if err != nil {
		return textEdit{}, err
	}

	repl := formatScalar(st.val, v.Style, inFlow)
	if start == end { // `key:` with nothing after the colon
		repl = " " + repl
	}

	return textEdit{start, end, repl}, nil
}

// addEdit appends pairs to a map: before the closing brace of a flow map, after the last line of a block map.
func (s *source) addEdit(m *yaml.Node, pairs []step) (textEdit, error) {
	t := s.text

	if m.Style&yaml.FlowStyle != 0 {
		open, ok := s.at(m)
		if !ok {
			return textEdit{}, errUnsupported
		}

		cb, err := s.flowEnd(open)
		if err != nil {
			return textEdit{}, err
		}

		if strings.Contains(t[open:cb], "#") && len(m.Content) > 0 {
			return textEdit{}, errUnsupported
		}

		items := make([]string, len(pairs))
		for i, p := range pairs {
			items[i] = p.key + ": " + formatScalar(p.val, 0, true)
		}

		j := cb
		for j > open+1 && strings.IndexByte(" \t\r\n", t[j-1]) >= 0 {
			j--
		}

		switch {
		case t[j-1] == '{': // empty map
			return textEdit{open + 1, cb, " " + strings.Join(items, ", ") + " "}, nil
		case t[j-1] == ',':
			return textEdit{j, j, " " + strings.Join(items, ", ")}, nil
		}

		return textEdit{j, j, ", " + strings.Join(items, ", ")}, nil
	}

	if len(m.Content) == 0 {
		return textEdit{}, errUnsupported
	}

	end, err := s.nodeEnd(m.Content[len(m.Content)-1])
	if err != nil {
		return textEdit{}, err
	}

	indent := strings.Repeat(" ", m.Content[0].Column-1)

	var b strings.Builder

	for _, p := range pairs {
		b.WriteString(indent + p.key + ": " + formatScalar(p.val, 0, false) + s.nl)
	}

	nl := strings.IndexByte(t[end:], '\n')
	if nl < 0 { // the last line has no line break: add one first, and none at the end
		return textEdit{len(t), len(t), s.nl + strings.TrimSuffix(b.String(), s.nl)}, nil
	}

	at := end + nl + 1

	return textEdit{at, at, b.String()}, nil
}

// deleteEdit removes a pair: its lines in a block map, the pair and one comma in a flow map.
func (s *source) deleteEdit(m *yaml.Node, idx int) (textEdit, error) {
	t := s.text
	key, val := m.Content[idx], m.Content[idx+1]

	keyStart, ok := s.at(key)
	if !ok {
		return textEdit{}, errUnsupported
	}

	valEnd, err := s.nodeEnd(val)
	if err != nil {
		return textEdit{}, err
	}

	if m.Style&yaml.FlowStyle == 0 {
		lineStart := s.lines[key.Line-1]
		if strings.TrimLeft(t[lineStart:keyStart], " ") != "" {
			return textEdit{}, errUnsupported // the key shares its line with a sequence dash or a bracket
		}

		nl := strings.IndexByte(t[valEnd:], '\n')
		if nl < 0 {
			return textEdit{lineStart, len(t), ""}, nil
		}

		return textEdit{lineStart, valEnd + nl + 1, ""}, nil
	}

	open, ok := s.at(m)
	if !ok {
		return textEdit{}, errUnsupported
	}

	cb, err := s.flowEnd(open)
	if err != nil {
		return textEdit{}, err
	}

	if strings.Contains(t[open:cb], "#") {
		return textEdit{}, errUnsupported
	}

	if len(m.Content) == 2 {
		return textEdit{open + 1, cb, ""}, nil
	}

	after := valEnd
	for after < cb && strings.IndexByte(" \t\r\n", t[after]) >= 0 {
		after++
	}

	if after < cb && t[after] == ',' {
		after++
		for after < cb && (t[after] == ' ' || t[after] == '\t') {
			after++
		}

		return textEdit{keyStart, after, ""}, nil
	}

	before := keyStart
	for before > open+1 && strings.IndexByte(" \t\r\n", t[before-1]) >= 0 {
		before--
	}

	if before > open && t[before-1] == ',' {
		return textEdit{before - 1, valEnd, ""}, nil
	}

	return textEdit{}, errUnsupported
}

// splice applies steps to the text, leaving every byte outside the touched spans as it was.
func splice(text string, steps []step) (string, error) {
	src, err := newSource(text)
	if err != nil {
		return "", err
	}

	var edits []textEdit

	for i := 0; i < len(steps); {
		st := steps[i]

		switch st.kind {
		case stepSet:
			e, err := src.setEdit(st)
			if err != nil {
				return "", err
			}

			edits = append(edits, e)
			i++
		case stepDelete:
			e, err := src.deleteEdit(st.m, st.idx)
			if err != nil {
				return "", err
			}

			edits = append(edits, e)
			i++
		case stepAdd:
			j := i
			for j < len(steps) && steps[j].kind == stepAdd && steps[j].m == st.m {
				j++
			}

			e, err := src.addEdit(st.m, steps[i:j])
			if err != nil {
				return "", err
			}

			edits = append(edits, e)
			i = j
		case stepRoot:
			out := text
			if out != "" && !strings.HasSuffix(out, "\n") {
				out += src.nl
			}

			if strings.TrimSpace(out) != "" {
				out += src.nl
			}

			return out + st.key + ": " + formatScalar(st.val, 0, false) + src.nl, nil
		}
	}

	sort.SliceStable(edits, func(a, b int) bool { return edits[a].start < edits[b].start })

	var b strings.Builder

	pos := 0

	for _, e := range edits {
		if e.start < pos || e.end < e.start {
			return "", errUnsupported
		}

		b.WriteString(text[pos:e.start])
		b.WriteString(e.repl)
		pos = e.end
	}

	b.WriteString(text[pos:])

	return b.String(), nil
}
