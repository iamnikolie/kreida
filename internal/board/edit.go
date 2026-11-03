package board

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"
)

// planner decides what to change in a parsed document.
type planner func(root *yaml.Node) ([]step, error)

// SetTop sets a top-level key to a string, or drops it when value is nil, keeping comments and formatting of the
// rest of the text.
func SetTop(text, key string, value *string) (string, error) {
	out, _, err := edit(text, planSetTop(key, value))

	return out, err
}

func planSetTop(key string, value *string) planner {
	return func(root *yaml.Node) ([]step, error) {
		switch {
		case root == nil && value != nil:
			return []step{{kind: stepRoot, key: key, val: *value}}, nil
		case root == nil && value == nil:
			return nil, nil
		case root == nil || root.Kind != yaml.MappingNode:
			return nil, fmt.Errorf("board.SetTop: the board is not a YAML map")
		}

		return setPair(root, key, value), nil
	}
}

// PatchNode edits one card's title and/or text in place, keeping comments and formatting of the rest. A nil title or
// text is left alone; an empty text removes the text.
func PatchNode(text, id string, title, body *string) (string, error) {
	out, _, err := edit(text, planPatchNode(id, title, body))

	return out, err
}

func planPatchNode(id string, title, body *string) planner {
	return func(root *yaml.Node) ([]step, error) {
		nodes := get(root, "nodes")

		switch {
		case nodes != nil && nodes.Kind == yaml.MappingNode && findKey(nodes, id) >= 0:
			i := findKey(nodes, id)
			if cur := nodes.Content[i+1]; cur.Kind == yaml.MappingNode {
				return patchMap(cur, title, body), nil
			}

			// "Title | text" in one scalar
			t, b := id, ""
			if cur := deref(nodes.Content[i+1]); !isNull(cur) && cur.Kind == yaml.ScalarNode {
				t, b = splitTitle(cur.Value)
			}

			if title != nil {
				t = *title
			}

			if body != nil {
				b = *body
			}

			if b != "" {
				t += " | " + b
			}

			return []step{{kind: stepSet, m: nodes, idx: i, val: t}}, nil
		case nodes != nil && nodes.Kind == yaml.SequenceNode:
			for _, item := range nodes.Content {
				if item.Kind == yaml.MappingNode && findKey(item, "id") >= 0 && item.Content[findKey(item, "id")+1].Value == id {
					return patchMap(item, title, body), nil
				}
			}
		}

		return nil, fmt.Errorf("node %s is not under nodes:", id)
	}
}

func patchMap(m *yaml.Node, title, body *string) []step {
	var steps []step

	if title != nil {
		steps = append(steps, setPair(m, "title", title)...)
	}

	if body != nil {
		if *body == "" {
			body = nil
		}

		steps = append(steps, setPair(m, "text", body)...)
	}

	return steps
}

// setPair sets a key (replacing the value of an existing pair, or adding one) or, with a nil value, deletes it.
func setPair(m *yaml.Node, key string, value *string) []step {
	i := findKey(m, key)

	switch {
	case value == nil && i >= 0:
		return []step{{kind: stepDelete, m: m, idx: i}}
	case value == nil:
		return nil
	case i >= 0:
		return []step{{kind: stepSet, m: m, idx: i, val: *value}}
	}

	return []step{{kind: stepAdd, m: m, key: key, val: *value}}
}

// edit makes a planned change to board text. The first choice is splicing the new text into the original, which
// leaves every byte outside the touched spans as it was. The splice is kept only when it reads back as the same
// document as the parsed tree with the change applied; otherwise (a layout the splicer does not handle) that tree is
// written out instead: comments survive, blank lines and spacing do not. The boolean says whether the splice was
// used.
func edit(text string, plan planner) (string, bool, error) {
	doc, err := parseYAML(text)
	if err != nil {
		return "", false, err
	}

	steps, err := plan(rootOf(doc))
	if err != nil || len(steps) == 0 {
		return text, true, err
	}

	spliced, serr := splice(text, steps)

	doc = apply(doc, steps)

	want, err := decode(doc)
	if err != nil {
		return "", false, fmt.Errorf("board.edit: %w", err)
	}

	if serr == nil {
		if d, err := parseYAML(spliced); err == nil {
			if got, err := decode(d); err == nil && reflect.DeepEqual(got, want) {
				return spliced, true, nil
			}
		}
	}

	var buf bytes.Buffer

	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)

	if err := enc.Encode(doc); err != nil {
		return "", false, fmt.Errorf("board.edit: %w", err)
	}

	return strings.TrimPrefix(buf.String(), "---\n"), false, nil
}

// decode reads a document as plain values, for comparing two documents.
func decode(doc *yaml.Node) (any, error) {
	var v any
	if doc == nil {
		return v, nil
	}

	err := doc.Decode(&v)

	return v, err
}

// apply performs the steps on the parsed tree; every value it writes is a string.
func apply(doc *yaml.Node, steps []step) *yaml.Node {
	str := func(v string) *yaml.Node { return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v} }

	for _, st := range steps {
		switch st.kind {
		case stepRoot:
			doc = &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{
				{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{str(st.key), str(st.val)}}}}
		case stepAdd:
			st.m.Content = append(st.m.Content, str(st.key), str(st.val))
		case stepSet:
			v, n := st.m.Content[st.idx+1], str(st.val)
			n.Style = v.Style &^ (yaml.LiteralStyle | yaml.FoldedStyle | yaml.TaggedStyle | yaml.FlowStyle)
			n.HeadComment, n.LineComment, n.FootComment = v.HeadComment, v.LineComment, v.FootComment
			st.m.Content[st.idx+1] = n
		case stepDelete:
			st.m.Content = append(st.m.Content[:st.idx:st.idx], st.m.Content[st.idx+2:]...)
		}
	}

	return doc
}
