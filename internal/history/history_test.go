package history_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/history"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newStore(t *testing.T) (history.Store, paths.Paths) {
	t.Helper()

	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}
	require.NoError(t, p.Ensure())

	tick := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	return history.Store{Paths: p, Now: func() time.Time {
		tick = tick.Add(time.Second)

		return tick
	}}, p
}

func put(t *testing.T, p paths.Paths, name, text string) {
	t.Helper()
	require.NoError(t, os.WriteFile(p.BoardFile(name), []byte(text), 0o644))
}

func TestSnapshotSkipsUnchangedText(t *testing.T) {
	s, p := newStore(t)

	ok, err := s.Snapshot("missing")
	require.NoError(t, err)
	assert.False(t, ok, "no board file, nothing to keep")

	put(t, p, "b", "title: one\n")
	ok, _ = s.Snapshot("b")
	assert.True(t, ok)
	ok, _ = s.Snapshot("b")
	assert.False(t, ok, "same text twice is one version")

	put(t, p, "b", "title: two\n")
	ok, _ = s.Snapshot("b")
	assert.True(t, ok)

	v, err := s.List("b")
	require.NoError(t, err)
	require.Len(t, v, 2)
	assert.Equal(t, "2026-01-01T00:00:01.000Z", v[0].Time)
}

func TestUndoWalksBackOneVersionAtATime(t *testing.T) {
	s, p := newStore(t)

	for _, text := range []string{"title: one\n", "title: two\n", "title: three\n"} {
		put(t, p, "b", text)
		_, err := s.Snapshot("b")
		require.NoError(t, err)
	}

	require.NoError(t, s.Undo("b"))
	got, _ := os.ReadFile(p.BoardFile("b"))
	assert.Equal(t, "title: two\n", string(got))

	require.NoError(t, s.Undo("b"))
	got, _ = os.ReadFile(p.BoardFile("b"))
	assert.Equal(t, "title: one\n", string(got))

	assert.ErrorIs(t, s.Undo("b"), history.ErrNothingToUndo)
	assert.EqualError(t, s.Undo("b"), "nothing to undo")
}

func TestUndoSavesUnsnapshottedWorkFirst(t *testing.T) {
	s, p := newStore(t)

	put(t, p, "b", "title: saved\n")
	_, _ = s.Snapshot("b")
	put(t, p, "b", "title: edited, never snapshotted\n")

	require.NoError(t, s.Undo("b"))
	got, _ := os.ReadFile(p.BoardFile("b"))
	assert.Equal(t, "title: saved\n", string(got))

	// the edit was a version of its own before it was dropped, so undo went back one step, not two
	v, _ := s.List("b")
	assert.Len(t, v, 1)
}

func TestKeepsTheLast200(t *testing.T) {
	s, p := newStore(t)

	for i := 0; i < history.Keep+5; i++ {
		put(t, p, "b", "title: v"+time.Duration(i).String()+"\n")
		_, err := s.Snapshot("b")
		require.NoError(t, err)
	}

	v, err := s.List("b")
	require.NoError(t, err)
	assert.Len(t, v, history.Keep)
	assert.Equal(t, "2026-01-01T00:00:06.000Z", v[0].Time, "the five oldest were dropped")
}

func TestTwoSavesInOneMillisecondAreTwoVersions(t *testing.T) {
	_, p := newStore(t)
	frozen := history.Store{Paths: p, Now: func() time.Time { return time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) }}

	for _, text := range []string{"title: a\n", "title: b\n", "title: c\n"} {
		put(t, p, "b", text)
		ok, err := frozen.Snapshot("b")
		require.NoError(t, err)
		assert.True(t, ok)
	}

	v, err := frozen.List("b")
	require.NoError(t, err)
	assert.Len(t, v, 3)
	assert.Less(t, v[0].ID, v[1].ID, "names keep sorting in save order")
	assert.Less(t, v[1].ID, v[2].ID)
}

func TestNewVersionDropsDisplacedPositions(t *testing.T) {
	s, p := newStore(t)
	pos := func() string {
		data, err := os.ReadFile(p.PosFile("b"))
		if os.IsNotExist(err) {
			return "(none)"
		}

		require.NoError(t, err)

		return string(data)
	}

	put(t, p, "b", "nodes: { a: A, b: B, c: C }\nedges: [ a -> b ]\n")
	_, err := s.Snapshot("b")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(p.PosFile("b"), []byte(`{"a":{"x":1,"y":2},"b":{"x":3,"y":4},"c":{"x":5,"y":6}}`), 0o644))

	put(t, p, "b", "nodes: { a: A2, b: B, c: C }\nedges: [ a -> b ]\n")
	_, err = s.Snapshot("b")
	require.NoError(t, err)
	assert.Equal(t, `{"a":{"x":1,"y":2},"b":{"x":3,"y":4},"c":{"x":5,"y":6}}`, pos(), "a new title moves nothing")

	put(t, p, "b", "nodes: { a: A2, b: B, c: C }\nedges: [ a -> b, c -> d ]\n")
	_, err = s.Snapshot("b")
	require.NoError(t, err)
	assert.Equal(t, `{"a":{"x":1,"y":2},"b":{"x":3,"y":4}}`, pos(), "c got a neighbour")

	require.NoError(t, os.WriteFile(p.PosFile("b"), []byte(`{"a":{"x":1,"y":2},"b":{"x":3,"y":4},"d":{"x":5,"y":6}}`), 0o644))
	require.NoError(t, s.Undo("b"))
	assert.Equal(t, `{"a":{"x":1,"y":2},"b":{"x":3,"y":4}}`, pos(), "undo drops the card that is gone again")

	put(t, p, "b", "nodes: [ {\n") // broken while being edited: the next valid text is compared with the last valid one
	_, err = s.Snapshot("b")
	require.NoError(t, err)
	assert.Equal(t, `{"a":{"x":1,"y":2},"b":{"x":3,"y":4}}`, pos())

	put(t, p, "b", "nodes: { a: A2, b: B, c: C }\nedges: [ c -> d ]\n")
	_, err = s.Snapshot("b")
	require.NoError(t, err)
	assert.Equal(t, "(none)", pos(), "a and b lost their link; an empty set leaves no file")
}
