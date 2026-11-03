package board_test

import (
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFieldsForeignKeyDrawsEdgeFromReferencedIDRow(t *testing.T) {
	b, err := board.Parse(`nodes:
  cities: { fields: [id, name] }
  models: { fields: [id, "city_id -> cities | note", "agency_id -.-> agencies"] }
`)
	require.NoError(t, err)

	type fk struct {
		From, To string
		FromPort string
		ToPort   string
		Dashed   bool
	}

	var got []fk

	for _, e := range b.Edges {
		if e.ToPort != "" {
			got = append(got, fk{From: e.From, To: e.To, FromPort: e.FromPort, ToPort: e.ToPort, Dashed: e.Dashed})
		}
	}

	assert.Equal(t, []fk{
		{"cities", "models", "id", "city_id", false},
		{"agencies", "models", "", "agency_id", true},
	}, got)

	var models board.Node

	for _, n := range b.Nodes {
		if n.ID == "models" {
			models = n
		}
	}

	assert.Equal(t, "note", models.Fields[1].Note)

	_, err = board.Parse("nodes:\n  t: { fields: [id, id] }\n")
	assert.ErrorContains(t, err, "twice")
}

func TestStepsParsedShorthandAndUnknownIDsRejected(t *testing.T) {
	b, err := board.Parse(`groups: { now: Сейчас, later: С ассистентом }
nodes: { a: { group: now }, b: { group: later }, c: C }
steps:
  - { title: Сейчас, show: [now] }
  - { title: С ассистентом, show: later, hide: [now] }
  - c
`)
	require.NoError(t, err)
	assert.Equal(t, []board.Step{
		{Title: "Сейчас", Show: []string{"now"}, Hide: []string{}},
		{Title: "С ассистентом", Show: []string{"later"}, Hide: []string{"now"}},
		{Title: "", Show: []string{"c"}, Hide: []string{}},
	}, b.Steps)

	_, err = board.Parse("nodes: { a: A }\nsteps: [ { show: [zz] } ]\n")
	assert.ErrorContains(t, err, "step 1: unknown")

	_, err = board.Parse("nodes: { a: A }\nsteps: [ { shw: [a] } ]\n")
	assert.ErrorContains(t, err, "unknown key")
}

func TestAnIDCannotBeBothANodeAndAGroup(t *testing.T) {
	_, err := board.Parse("groups: { g: G }\nnodes: { g: G }\n")
	assert.ErrorContains(t, err, "both a node and a group")
}

func TestLiveParsedAndValidated(t *testing.T) {
	b, err := board.Parse("nodes:\n  a: { title: A, live: { cmd: \"echo 1\", every: 1 } }\n")
	require.NoError(t, err)
	require.NotNil(t, b.Nodes[0].Live)
	assert.Equal(t, board.Live{Source: "cmd:echo 1", Every: 5, Path: nil, Label: ""}, *b.Nodes[0].Live)

	_, err = board.Parse("nodes:\n  a: { live: { cmd: x, url: \"http://x\" } }\n")
	assert.ErrorContains(t, err, "exactly one")

	_, err = board.Parse("nodes:\n  a: { live: { url: \"file:///etc/passwd\" } }\n")
	assert.ErrorContains(t, err, "http")

	_, err = board.Parse("nodes:\n  a: { live: { cmd: x, shell: y } }\n")
	assert.ErrorContains(t, err, "unknown key")
}

// Edge forms and defaults the page relies on.

func TestEdgeFormsAndAutoCreatedNodes(t *testing.T) {
	b, err := board.Parse("edges:\n  - a -> b -> c: both hops end here\n  - { from: c, to: d, animated: true }\n")
	require.NoError(t, err)
	require.Len(t, b.Edges, 3)
	assert.Equal(t, "", b.Edges[0].Label)
	assert.Equal(t, "both hops end here", b.Edges[1].Label)
	assert.True(t, b.Edges[2].Animated)
	assert.Equal(t, "e2-c-d", b.Edges[2].ID)
	assert.Len(t, b.Nodes, 4)
}

func TestPanicFreeOnNonsense(t *testing.T) {
	for _, in := range []string{"\x00", "{{{", "a: *b", "? - a\n  : b\n", "!!binary x", "- - - - -"} {
		_, _ = board.Parse(in)
	}
}
