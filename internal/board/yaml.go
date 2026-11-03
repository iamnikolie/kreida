package board

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

// SyntaxError is board text that is not valid YAML (as opposed to a board that parses but breaks a rule).
type SyntaxError struct{ Msg string }

func (e *SyntaxError) Error() string { return e.Msg }

// IsSyntax reports whether err is a YAML syntax error rather than a broken rule of the format.
func IsSyntax(err error) bool {
	var se *SyntaxError

	return errors.As(err, &se)
}

// parseYAML parses board text into its document node, which carries positions and comments; nil for an empty
// stream. More than one document and a key repeated in one map are errors.
func parseYAML(text string) (doc *yaml.Node, err error) {
	defer func() {
		if r := recover(); r != nil {
			doc, err = nil, &SyntaxError{Msg: fmt.Sprintf("yaml: %v", r)}
		}
	}()

	masked, mark := maskFlowQuestions(text)
	dec := yaml.NewDecoder(strings.NewReader(masked))

	var d yaml.Node

	if err := dec.Decode(&d); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, nil
		}

		return nil, &SyntaxError{Msg: err.Error()}
	}

	var second yaml.Node
	if err := dec.Decode(&second); err == nil {
		return nil, &SyntaxError{Msg: "yaml: more than one document (---); a board is one document"}
	} else if !errors.Is(err, io.EOF) {
		return nil, &SyntaxError{Msg: err.Error()}
	}

	if mark != 0 {
		unmask(&d, string(mark))
	}

	if err := uniqueKeys(&d); err != nil {
		return nil, err
	}

	return &d, nil
}

func rootOf(doc *yaml.Node) *yaml.Node {
	if doc == nil || len(doc.Content) == 0 {
		return nil
	}

	return doc.Content[0]
}

// uniqueKeys rejects a key that appears twice in one map: YAML forbids it, and the second would silently win.
func uniqueKeys(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}

		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode {
				continue
			}

			if seen[k.Value] {
				return &SyntaxError{Msg: fmt.Sprintf("yaml: line %d: key %q appears twice in one map", k.Line, k.Value)}
			}

			seen[k.Value] = true
		}
	}

	for _, c := range n.Content {
		if err := uniqueKeys(c); err != nil {
			return err
		}
	}

	return nil
}

// libyaml (and so yaml.v3) reads a "?" inside a flow collection as the explicit-key indicator, so
// `{ title: Запускается вручную или по расписанию?, kind: note }` is a syntax error there although YAML 1.2 takes it as text.
// Such "?" are swapped for a one-byte printable mark that the text does not contain, then restored in the parsed
// tree: lengths stay equal, so node positions and the text-level splicing that relies on them are unaffected. A "?"
// that starts a token is left alone.
func maskFlowQuestions(text string) (string, byte) {
	if !strings.Contains(text, "?") {
		return text, 0
	}

	var mark byte

	for _, c := range []byte("^~$%;") {
		if !strings.ContainsRune(text, rune(c)) {
			mark = c

			break
		}
	}

	if mark == 0 {
		return text, 0
	}

	b := []byte(text)
	depth, quote, comment, changed := 0, byte(0), false, false
	prev := byte('\n') // last non-blank byte outside quotes

	for i := 0; i < len(b); i++ {
		c := b[i]

		switch {
		case comment:
			if c == '\n' {
				comment, prev = false, '\n'
			}
		case quote != 0:
			switch {
			case quote == '"' && c == '\\':
				i++
			case c == quote && quote == '\'' && i+1 < len(b) && b[i+1] == '\'':
				i++
			case c == quote:
				quote, prev = 0, c
			}
		case c == '#' && (i == 0 || b[i-1] == ' ' || b[i-1] == '\t' || b[i-1] == '\n'):
			comment = true
		case (c == '"' || c == '\'') && tokenStart(prev):
			quote = c
		case (c == '{' || c == '[') && tokenStart(prev):
			depth++
			prev = c
		case (c == '}' || c == ']') && depth > 0:
			depth--
			prev = c
		case c == '?' && depth > 0 && !tokenStart(prev):
			b[i], changed = mark, true
			prev = mark
		case c == ' ' || c == '\t':
		default:
			prev = c
		}
	}

	if !changed {
		return text, 0
	}

	return string(b), mark
}

// tokenStart: the byte before a value position — a line start, a flow opener or separator, or an indicator.
func tokenStart(prev byte) bool {
	return strings.IndexByte("\n{[,:-?", prev) >= 0
}

func unmask(n *yaml.Node, mark string) {
	for _, s := range []*string{&n.Value, &n.HeadComment, &n.LineComment, &n.FootComment} {
		*s = strings.ReplaceAll(*s, mark, "?")
	}

	for _, k := range n.Content {
		unmask(k, mark)
	}
}

// deref follows aliases to the node they name.
func deref(n *yaml.Node) *yaml.Node {
	for n != nil && n.Kind == yaml.AliasNode {
		n = n.Alias
	}

	return n
}

// isNull: missing, empty (`key:`), `~` or `null`.
func isNull(n *yaml.Node) bool {
	n = deref(n)

	return n == nil || n.Kind == yaml.ScalarNode && n.ShortTag() == "!!null"
}

// get returns the value under key in a map, or nil.
func get(m *yaml.Node, key string) *yaml.Node {
	m = deref(m)
	if i := findKey(m, key); i >= 0 {
		return deref(m.Content[i+1])
	}

	return nil
}

// findKey returns the index in m.Content of the key named name, or -1.
func findKey(m *yaml.Node, name string) int {
	m = deref(m)
	if m == nil || m.Kind != yaml.MappingNode {
		return -1
	}

	for i := 0; i+1 < len(m.Content); i += 2 {
		if k := m.Content[i]; k.Kind == yaml.ScalarNode && k.Value == name {
			return i
		}
	}

	return -1
}
