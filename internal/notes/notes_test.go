package notes_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var t0 = time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)

func scratch(t *testing.T) paths.Paths {
	t.Helper()

	home := t.TempDir()

	return paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}
}

func TestAddListAndTheFileShape(t *testing.T) {
	p := scratch(t)

	got, err := notes.List(p, "b", false)
	require.NoError(t, err)
	assert.Equal(t, []notes.Note{}, got, "no file is no notes, not nil")

	n1, err := notes.Add(p, "b", "a1", "  Rename this card\n", t0)
	require.NoError(t, err)
	assert.Equal(t, notes.Note{ID: "n1", Node: "a1", Text: "Rename this card", Created: "2026-10-30T09:15:00Z"}, n1)

	n2, err := notes.Add(p, "b", "", "The whole board is too wide", t0.Add(time.Minute))
	require.NoError(t, err)
	assert.Equal(t, "n2", n2.ID)
	assert.Equal(t, "", n2.Node, "no node is the whole board")

	raw, err := os.ReadFile(p.NotesFile("b"))
	require.NoError(t, err)

	var file struct {
		Next  int              `json:"next"`
		Notes []map[string]any `json:"notes"`
	}

	require.NoError(t, json.Unmarshal(raw, &file))
	assert.Equal(t, 3, file.Next)
	require.Len(t, file.Notes, 2)
	assert.Equal(t, map[string]any{"id": "n1", "node": "a1", "text": "Rename this card", "author": "", "created": "2026-10-30T09:15:00Z"}, file.Notes[0], "no board, no resolved while open")

	got, err = notes.List(p, "b", false)
	require.NoError(t, err)
	assert.Equal(t, []notes.Note{n1, n2}, got, "oldest first")

	other, err := notes.List(p, "other", false)
	require.NoError(t, err)
	assert.Empty(t, other, "notes belong to their board")

	info, err := os.Stat(p.NotesFile("b"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o644), info.Mode().Perm())
}

func TestResolveHidesTheNoteAndIdsAreNeverReused(t *testing.T) {
	p := scratch(t)

	_, err := notes.Add(p, "b", "a", "one", t0)
	require.NoError(t, err)

	_, err = notes.Add(p, "b", "a", "two", t0)
	require.NoError(t, err)

	require.NoError(t, notes.Resolve(p, "b", "n2", t0.Add(time.Hour)))

	open, err := notes.List(p, "b", false)
	require.NoError(t, err)
	require.Len(t, open, 1)
	assert.Equal(t, "n1", open[0].ID)

	all, err := notes.List(p, "b", true)
	require.NoError(t, err)
	require.Len(t, all, 2)
	assert.Equal(t, "2026-10-30T10:15:00Z", all[1].Resolved)
	assert.Equal(t, "", all[0].Resolved)

	n3, err := notes.Add(p, "b", "", "three", t0)
	require.NoError(t, err)
	assert.Equal(t, "n3", n3.ID, "a resolved id is not handed out again")

	// the newest note is resolved: a gone one must not give its number back either
	require.NoError(t, notes.Resolve(p, "b", "n3", t0))

	n4, err := notes.Add(p, "b", "", "four", t0)
	require.NoError(t, err)
	assert.Equal(t, "n4", n4.ID)
}

func TestResolveIsIdempotentAndKeepsTheFirstTime(t *testing.T) {
	p := scratch(t)

	_, err := notes.Add(p, "b", "", "one", t0)
	require.NoError(t, err)

	require.NoError(t, notes.Resolve(p, "b", "n1", t0.Add(time.Hour)))
	require.NoError(t, notes.Resolve(p, "b", "n1", t0.Add(2*time.Hour)))

	all, err := notes.List(p, "b", true)
	require.NoError(t, err)
	require.Len(t, all, 1)
	assert.Equal(t, "2026-10-30T10:15:00Z", all[0].Resolved)

	assert.EqualError(t, notes.Resolve(p, "b", "n9", t0), "no note n9")
	assert.EqualError(t, notes.Resolve(p, "nope", "n1", t0), "no note n1", "a board without notes has none")
	assert.NoFileExists(t, p.NotesFile("nope"), "a refused resolve writes nothing")
}

func TestCleanTrimsStripsControlsAndBounds(t *testing.T) {
	for in, want := range map[string]string{
		"  hello \n":                "hello",
		"two\nlines\tand a tab":     "two\nlines\tand a tab",
		"a\x1b[31mred\x07\x00":      "a[31mred",
		"windows\r\nline":           "windows\nline",
		"del\x7f and C1\u0085":      "del and C1",
		strings.Repeat("я", 2000):   strings.Repeat("я", 2000),
		"\u00a0 нужно поправить \n": "нужно поправить",
	} {
		got, err := notes.Clean(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got)
	}

	for _, in := range []string{"", "   \n\t ", "\x00\x1b\x07"} {
		_, err := notes.Clean(in)
		assert.EqualError(t, err, "note is empty", "%q", in)
	}

	_, err := notes.Clean(strings.Repeat("я", 2001))
	assert.EqualError(t, err, "note is longer than 2000 characters", "runes, not bytes")
}

func TestAddRefusesWhatCleanRefuses(t *testing.T) {
	p := scratch(t)

	_, err := notes.Add(p, "b", "", "   ", t0)
	assert.EqualError(t, err, "note is empty")
	assert.NoFileExists(t, p.NotesFile("b"))

	n, err := notes.Add(p, "b", "", "a\x00b", t0)
	require.NoError(t, err)
	assert.Equal(t, "ab", n.Text, "the stored text is the cleaned one")
}

func TestBoardsListsTheOnesWithNotes(t *testing.T) {
	p := scratch(t)

	got, err := notes.Boards(p)
	require.NoError(t, err)
	assert.Empty(t, got, "no boards directory is no boards")

	for _, name := range []string{"zeta", "alpha.v2"} {
		_, err := notes.Add(p, name, "", "x", t0)
		require.NoError(t, err)
	}

	require.NoError(t, os.WriteFile(p.BoardFile("plain"), []byte("title: x\n"), 0o644))
	require.NoError(t, os.WriteFile(p.PosFile("plain"), []byte("{}"), 0o644))

	got, err = notes.Boards(p)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha.v2", "zeta"}, got)
}

func TestConcurrentAddsGetDistinctIdsAndLeaveNoTempFiles(t *testing.T) {
	p := scratch(t)

	var wg sync.WaitGroup

	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := notes.Add(p, "b", "", "x", t0)
			assert.NoError(t, err)
		}()
	}

	wg.Wait()

	all, err := notes.List(p, "b", true)
	require.NoError(t, err)
	require.Len(t, all, 20)

	seen := map[string]bool{}
	for _, n := range all {
		seen[n.ID] = true
	}

	assert.Len(t, seen, 20)

	entries, err := os.ReadDir(p.Dir)
	require.NoError(t, err)
	require.Len(t, entries, 1, "the temp file is renamed over the notes file")
	assert.Equal(t, "b.notes.json", entries[0].Name())
}

func TestAFileThatIsNotJSONIsAnErrorAndStaysUntouched(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.MkdirAll(p.Dir, 0o755))
	require.NoError(t, os.WriteFile(p.NotesFile("b"), []byte("not json"), 0o644))

	_, err := notes.List(p, "b", false)
	assert.ErrorContains(t, err, "b.notes.json is not valid JSON")

	_, err = notes.Add(p, "b", "", "x", t0)
	assert.Error(t, err)

	raw, err := os.ReadFile(p.NotesFile("b"))
	require.NoError(t, err)
	assert.Equal(t, "not json", string(raw), "a file we cannot read is not overwritten")
}

func TestANextBehindTheNotesIsRepaired(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.MkdirAll(p.Dir, 0o755))
	require.NoError(t, os.WriteFile(p.NotesFile("b"), []byte(`{"next":1,"notes":[{"id":"n7","node":"","text":"old","author":"","created":"2026-10-30T09:15:00Z"}]}`), 0o644))

	n, err := notes.Add(p, "b", "", "new", t0)
	require.NoError(t, err)
	assert.Equal(t, "n8", n.ID, "an id already in the file is never handed out again")
}
