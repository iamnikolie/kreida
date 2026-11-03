package board

import (
	"slices"
	"sort"
)

// place is where a card or group sits in a board: its parent group ("" at the top), its links and its siblings.
type place struct {
	parent string
	links  []string // "from>to" of every edge to or from it
	cell   string   // its quadrant on a matrix board
}

func places(b *Board) (map[string]place, map[string][]string) {
	out := map[string]place{}

	for _, g := range b.Groups {
		p := place{}
		if g.Parent != nil {
			p.parent = *g.Parent
		}

		out[g.ID] = p
	}

	for _, n := range b.Nodes {
		p := place{cell: n.X + "-" + n.Y}
		if n.Group != nil {
			p.parent = *n.Group
		}

		out[n.ID] = p
	}

	for _, e := range b.Edges {
		for _, id := range []string{e.From, e.To} {
			if p, ok := out[id]; ok {
				p.links = append(p.links, e.From+">"+e.To)
				out[id] = p
			}
		}
	}

	members := map[string][]string{}

	for id, p := range out {
		sort.Strings(p.links)
		p.links = slices.Compact(p.links)
		out[id] = p

		if p.parent != "" {
			members[p.parent] = append(members[p.parent], id)
		}
	}

	for _, m := range members {
		sort.Strings(m)
	}

	return out, members
}

// Displaced lists the ids whose dragged position no longer fits after the board changed from prev to next: a card
// or group that is new, sits in another group, gained or lost a link, or belongs to a group that gained or lost
// members — and every id that is gone. Positions of everything else still hold.
func Displaced(prev, next *Board) map[string]bool {
	was, wasMembers := places(prev)
	now, nowMembers := places(next)
	out := map[string]bool{}

	for id, p := range now {
		q, ok := was[id]
		if !ok || p.parent != q.parent || p.cell != q.cell || !slices.Equal(p.links, q.links) ||
			p.parent != "" && !slices.Equal(nowMembers[p.parent], wasMembers[p.parent]) {
			out[id] = true
		}
	}

	for id := range was {
		if _, ok := now[id]; !ok {
			out[id] = true
		}
	}

	return out
}
