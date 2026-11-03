package mermaid

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/iamnikolie/kreida/internal/board"
)

var colors = map[string]string{
	"src": "#4a7fd4", "core": "#d4943a", "out": "#4f9a62", "warn": "#d0574b",
	"info": "#8a6fd1", "who": "#222", "note": "#d8c25a", "p2": "#9b968c",
}

var reservedIDs = map[string]bool{"end": true, "graph": true, "subgraph": true, "style": true, "class": true}

func quote(s string) string { return strings.ReplaceAll(s, `"`, "#quot;") }

func isSafeID(id string) bool {
	if id == "" {
		return false
	}

	for i := 0; i < len(id); i++ {
		c := id[i]
		letter := c >= 'A' && c <= 'Z' || c >= 'a' && c <= 'z' || c == '_'

		if i == 0 && !letter || i > 0 && !letter && !(c >= '0' && c <= '9') {
			return false
		}
	}

	return true
}

// ToMermaid writes a board as a flowchart. Ids that Mermaid would misread become n1, n2, ...
func ToMermaid(b *board.Board) (string, error) {
	safe := map[string]string{}
	used := map[string]bool{}

	sid := func(id string) string {
		if s, ok := safe[id]; ok {
			return s
		}

		s := "n" + strconv.Itoa(len(safe)+1)
		if isSafeID(id) && !reservedIDs[id] {
			s = id
		}

		for used[s] {
			s += "_"
		}

		safe[id] = s
		used[s] = true

		return s
	}

	var out []string

	if b.Title != "" {
		out = append(out, "%% title: "+b.Title)
	}

	if b.Sub != "" {
		out = append(out, "%% sub: "+b.Sub)
	}

	if len(b.Legend) > 0 {
		items := make([]string, len(b.Legend))
		for i, l := range b.Legend {
			items[i] = l.Kind + " " + l.Label
		}

		out = append(out, "%% legend: "+strings.Join(items, " | "))
	}

	dir := b.Direction
	if dir == "TB" {
		dir = "TD"
	}

	out = append(out, "flowchart "+dir)

	nodeLine := func(n board.Node) string {
		var parts []string

		for _, p := range []string{n.Title, n.Text} {
			if p != "" {
				parts = append(parts, p)
			}
		}

		for _, f := range n.Fields {
			line := f.Name
			if f.Ref != nil && *f.Ref != "" {
				line = f.Name + " → " + *f.Ref
			}

			if line != "" {
				parts = append(parts, line)
			}
		}

		open, closer := "[", "]"

		switch {
		case n.Shape != nil && *n.Shape == "pill" || n.Kind == "who":
			open, closer = "([", "])"
		case n.Icon != nil && *n.Icon == "database":
			open, closer = "[(", ")]"
		}

		cls := ""
		if kinds[n.Kind] && n.Kind != "plain" {
			cls = ":::" + n.Kind
		}

		return sid(n.ID) + open + `"` + quote(strings.Join(parts, "<br/>")) + `"` + closer + cls
	}

	open := map[string]bool{}

	var emit func(parent *string, pad string) error

	emit = func(parent *string, pad string) error {
		for _, g := range b.Groups {
			if !sameRef(g.Parent, parent) {
				continue
			}

			if open[g.ID] {
				return fmt.Errorf("mermaid: group %s is inside itself", g.ID)
			}

			open[g.ID] = true

			out = append(out, pad+"subgraph "+sid(g.ID)+`["`+quote(g.Label)+`"]`)

			if g.Direction != nil && *g.Direction != "" {
				out = append(out, pad+"  direction "+*g.Direction)
			}

			id := g.ID
			if err := emit(&id, pad+"  "); err != nil {
				return err
			}

			out = append(out, pad+"end")
			delete(open, g.ID)
		}

		for _, n := range b.Nodes {
			if sameRef(n.Group, parent) {
				out = append(out, pad+nodeLine(n))
			}
		}

		return nil
	}

	if err := emit(nil, "  "); err != nil {
		return "", err
	}

	for _, e := range b.Edges {
		op := "-->"

		switch {
		case e.Hidden:
			op = "~~~"
		case e.Dashed:
			op = "-.->"
		case e.Thick:
			op = "==>"
		}

		lbl := ""
		if e.Label != "" && !e.Hidden {
			lbl = `|"` + quote(e.Label) + `"|`
		}

		out = append(out, "  "+sid(e.From)+" "+op+lbl+" "+sid(e.To))
	}

	seen := map[string]bool{}

	for _, n := range b.Nodes {
		k := n.Kind
		if seen[k] || colors[k] == "" {
			continue
		}

		seen[k] = true

		line := "  classDef " + k + " stroke:" + colors[k] + ",stroke-width:1.6px"
		if k == "p2" {
			line += ",stroke-dasharray:5 4"
		}

		out = append(out, line)
	}

	return strings.Join(out, "\n") + "\n", nil
}

// sameRef compares a node's or group's optional reference to the parent being emitted (nil = top level).
func sameRef(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}

	return *a == *b
}
