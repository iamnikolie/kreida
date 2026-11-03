package cloud_test

import (
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const liveBoard = `title: Live <board>
nodes:
  a: { title: Orders, live: { cmd: "curl -s https://internal.example/token-abc", every: 30, label: today } }
  b: { title: Health, live: { url: "https://user:secret@internal.example/health", path: data.ok } }
  c: Plain
edges: [ a -> b ]
`

func TestSnapshotLeavesTheNotesForTheAgentAtHome(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.BoardFile("demo"), []byte("title: Demo\nnodes: { a: A }\n"), 0o644))
	require.NoError(t, os.WriteFile(p.NotesFile("demo"), []byte(`{"next":2,"notes":[{"id":"n1","node":"a","text":"private thought","author":"","created":"2026-10-30T09:15:00Z"}]}`), 0o644))

	up, err := cloud.Snapshot(p, "demo")
	require.NoError(t, err)

	body, err := up.JSON()
	require.NoError(t, err)
	assert.NotContains(t, string(body), "private thought")
	assert.ElementsMatch(t, []string{"board", "pos"}, slices.Collect(maps.Keys(decode(t, string(body)))))
}

func TestSnapshotBlanksEveryLiveSource(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.BoardFile("demo"), []byte(liveBoard), 0o644))
	require.NoError(t, os.WriteFile(p.PosFile("demo"), []byte("{\n \"a\": {\"x\": 1, \"y\": 2}\n}\n"), 0o644))

	up, err := cloud.Snapshot(p, "demo")
	require.NoError(t, err)

	body, err := up.JSON()
	require.NoError(t, err)

	for _, leak := range []string{"curl", "internal.example", "secret", "token-abc"} {
		assert.NotContains(t, string(body), leak)
	}

	m := decode(t, string(body))
	assert.ElementsMatch(t, []string{"board", "pos"}, slices.Collect(maps.Keys(m)))
	assert.Equal(t, map[string]any{"a": map[string]any{"x": 1.0, "y": 2.0}}, m["pos"])

	b := m["board"].(map[string]any)
	assert.Equal(t, "Live <board>", b["title"], "markup characters are not escaped")

	for _, key := range []string{"groups", "nodes", "edges", "steps", "legend"} {
		assert.IsType(t, []any{}, b[key], key+" is an array, as the cloud requires")
	}

	nodes := b["nodes"].([]any)
	require.Len(t, nodes, 3)

	a := nodes[0].(map[string]any)["live"].(map[string]any)
	assert.Equal(t, "", a["source"])
	assert.Equal(t, 30.0, a["every"])
	assert.Equal(t, "today", a["label"])

	bl := nodes[1].(map[string]any)["live"].(map[string]any)
	assert.Equal(t, "", bl["source"])
	assert.Equal(t, "data.ok", bl["path"])

	assert.Nil(t, nodes[2].(map[string]any)["live"])

	// the parse is the one the local API serves: asking again gives the same bytes
	again, err := cloud.Snapshot(p, "demo")
	require.NoError(t, err)

	body2, err := again.JSON()
	require.NoError(t, err)
	assert.Equal(t, string(body), string(body2))
}

func TestSnapshotWithoutPositionsHasAnEmptyObject(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.BoardFile("demo"), []byte("title: T\n"), 0o644))

	up, err := cloud.Snapshot(p, "demo")
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(up.Pos))

	require.NoError(t, os.WriteFile(p.PosFile("demo"), []byte("not json"), 0o644))

	up, err = cloud.Snapshot(p, "demo")
	require.NoError(t, err)
	assert.JSONEq(t, `{}`, string(up.Pos))
}

func TestSnapshotRefusesBadNamesMissingAndBrokenBoards(t *testing.T) {
	p := scratch(t)

	_, err := cloud.Snapshot(p, "../etc")
	assert.ErrorContains(t, err, "bad board name")

	_, err = cloud.Snapshot(p, "nope")
	assert.ErrorContains(t, err, "cloud.Snapshot")

	require.NoError(t, os.WriteFile(p.BoardFile("bad"), []byte("nodes: { a: { colour: red } }\n"), 0o644))

	_, err = cloud.Snapshot(p, "bad")
	assert.ErrorContains(t, err, "invalid board")
	assert.True(t, strings.Contains(err.Error(), "unknown key"), err.Error())
}
