package mermaid

import (
	"fmt"
	"strings"

	"github.com/iamnikolie/kreida/internal/board"
)

// kv is one key and its value (a string or an int); an empty string is left out.
type kv struct {
	k string
	v any
}

// item is a group or a card: written as `id: Label` when the label or title is all it has, else as a flow map.
type item struct {
	id    string
	attrs []kv
}

// doc is an imported board, in the order its YAML is written.
type doc struct {
	title, sub string
	legend     []kv
	head       []kv // direction or layout
	groups     []item
	nodes      []item
	edges      []string
}

func scalar(v any, inFlow bool) string {
	if s, ok := v.(string); ok {
		return board.FormatScalar(s, inFlow)
	}

	return fmt.Sprint(v)
}

// flowMap writes the non-empty pairs as `{ key: value, ... }`, a later pair replacing an earlier one with its key.
func flowMap(pairs []kv) string {
	var keys []string

	vals := map[string]string{}

	for _, p := range pairs {
		if p.k == "" {
			continue
		}

		if _, ok := vals[p.k]; !ok {
			keys = append(keys, p.k)
		}

		vals[p.k] = board.FormatScalar(p.k, true) + ": " + scalar(p.v, true)
	}

	if len(keys) == 0 {
		return "{}"
	}

	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = vals[k]
	}

	return "{ " + strings.Join(parts, ", ") + " }"
}

// yaml writes the board the way hand-written boards look: top-level keys in block style, one card, group or legend
// per line in flow style, edges as a block list.
func (d doc) yaml() string {
	var b strings.Builder

	for _, p := range []kv{{"title", d.title}, {"sub", d.sub}} {
		if p.v != "" {
			b.WriteString(p.k + ": " + scalar(p.v, false) + "\n")
		}
	}

	if len(d.legend) > 0 {
		b.WriteString("legend: " + flowMap(d.legend) + "\n")
	}

	for _, p := range d.head {
		b.WriteString(p.k + ": " + scalar(p.v, false) + "\n")
	}

	section := func(name string, items []item) {
		if len(items) == 0 {
			b.WriteString(name + ": {}\n")

			return
		}

		b.WriteString(name + ":\n")

		for _, it := range items {
			var set []kv

			for _, a := range it.attrs {
				if a.v != "" {
					set = append(set, a)
				}
			}

			b.WriteString("  " + board.FormatScalar(it.id, false) + ": ")

			if len(set) == 1 && (set[0].k == "title" || set[0].k == "label") {
				b.WriteString(scalar(set[0].v, false) + "\n")
			} else {
				b.WriteString(flowMap(set) + "\n")
			}
		}
	}

	if len(d.groups) > 0 {
		section("groups", d.groups)
	}

	section("nodes", d.nodes)

	if len(d.edges) == 0 {
		b.WriteString("edges: []\n")
	} else {
		b.WriteString("edges:\n")
	}

	for _, e := range d.edges {
		b.WriteString("  - " + board.FormatScalar(e, false) + "\n")
	}

	return b.String()
}
