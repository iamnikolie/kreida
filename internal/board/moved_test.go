package board_test

import (
	"sort"
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const movedBase = `groups:
  g: G
  h: H
nodes:
  a: { title: A, group: g }
  b: { title: B, group: g }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
  y: Y
edges:
  - a -> b
  - x -> y
`

func TestDisplaced(t *testing.T) {
	cases := []struct {
		name, after string
		want        []string
	}{
		{"nothing changed", movedBase, nil},
		{"text and labels change nothing", `groups: { g: Group, h: H }
nodes:
  a: { title: A2, text: more, kind: core, group: g }
  b: { title: B, group: g }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
  y: Y
edges:
  - a -> b: now labelled
  - x -.-> y
`, nil},
		{"a new card outside groups", movedBase + "  - y -> z\n", []string{"y", "z"}},
		{"a new card in a group moves its group mates", `groups: { g: G, h: H }
nodes:
  a: { title: A, group: g }
  b: { title: B, group: g }
  e: { title: E, group: g }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
  y: Y
edges: [ a -> b, x -> y ]
`, []string{"a", "b", "e"}},
		{"a card moved to another group", `groups: { g: G, h: H }
nodes:
  a: { title: A, group: g }
  b: { title: B, group: h }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
  y: Y
edges: [ a -> b, x -> y ]
`, []string{"a", "b", "c", "d"}},
		{"a removed card: its neighbour and the gone id", `groups: { g: G, h: H }
nodes:
  a: { title: A, group: g }
  b: { title: B, group: g }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
edges: [ a -> b ]
`, []string{"x", "y"}},
		{"an added edge", movedBase + "  - c -> x\n", []string{"c", "x"}},
		{"a removed edge", `groups: { g: G, h: H }
nodes:
  a: { title: A, group: g }
  b: { title: B, group: g }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
  y: Y
edges: [ x -> y ]
`, []string{"a", "b"}},
		{"a group moved into another", `groups: { g: G, h: { label: H, parent: g } }
nodes:
  a: { title: A, group: g }
  b: { title: B, group: g }
  c: { title: C, group: h }
  d: { title: D, group: h }
  x: X
  y: Y
edges: [ a -> b, x -> y ]
`, []string{"a", "b", "h"}},
	}

	before, err := board.Parse(movedBase)
	require.NoError(t, err)

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			after, err := board.Parse(c.after)
			require.NoError(t, err)

			var got []string
			for id := range board.Displaced(before, after) {
				got = append(got, id)
			}

			sort.Strings(got)
			assert.Equal(t, c.want, got)
		})
	}
}
