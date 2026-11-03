package board_test

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// update rewrites the expected outputs (testdata/cases/*.json, *.err.json and web/test/fixtures) from the parser:
// go test ./internal/board -update, then review the diff.
var update = flag.Bool("update", false, "rewrite golden files from the current parser")

// goldenErr is an expected error: syntax marks YAML errors, as opposed to broken rules of the format.
type goldenErr struct {
	YAML    bool   `json:"yaml"`
	Message string `json:"message"`
}

func readFile(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)

	return err == nil
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()

	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, append(data, '\n'), 0o644))
}

// golden compares v with the JSON in path as values (key order and spacing do not matter), or writes it with -update.
func golden(t *testing.T, path string, v any) {
	t.Helper()

	if *update {
		writeJSON(t, path, v)

		return
	}

	got, err := json.Marshal(v)
	require.NoError(t, err)

	var w, g any

	require.NoError(t, json.Unmarshal([]byte(readFile(t, path)), &w), path)
	require.NoError(t, json.Unmarshal(got, &g))
	assert.Equal(t, w, g, path)
}

func checkCase(t *testing.T, dir, name string) {
	t.Helper()

	b, err := board.Parse(readFile(t, filepath.Join(dir, name+".yaml")))
	okPath, errPath := filepath.Join(dir, name+".json"), filepath.Join(dir, name+".err.json")

	if *update {
		_ = os.Remove(okPath)
		_ = os.Remove(errPath)
	}

	if err != nil {
		if !*update {
			require.True(t, exists(errPath), "unexpected error: %v", err)
		}

		golden(t, errPath, goldenErr{YAML: board.IsSyntax(err), Message: err.Error()})

		return
	}

	require.False(t, exists(errPath), "expected an error")
	golden(t, okPath, b)
}

func caseNames(t *testing.T, dir string) []string {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	names := make([]string, len(files))
	for i, f := range files {
		names[i] = strings.TrimSuffix(filepath.Base(f), ".yaml")
	}

	return names
}

// TestGoldenParse parses every input under testdata/cases and compares the normalized board (or the error).
func TestGoldenParse(t *testing.T) {
	dir := filepath.Join("testdata", "cases")

	for _, name := range caseNames(t, dir) {
		t.Run(name, func(t *testing.T) { checkCase(t, dir, name) })
	}
}

// TestWebFixtures: web/test/fixtures are the page's contract, the parser's output for testdata/fixtures and for the
// templates (testdata/cases/template-*.yaml).
func TestWebFixtures(t *testing.T) {
	web := filepath.Join("..", "..", "web", "test", "fixtures")
	inputs := map[string]string{}

	for _, name := range caseNames(t, filepath.Join("testdata", "fixtures")) {
		inputs[name] = filepath.Join("testdata", "fixtures", name+".yaml")
	}

	for _, tpl := range []string{"flow", "lanes", "sequence", "mindmap", "matrix", "compare", "schema", "before-after"} {
		inputs["template-"+tpl] = filepath.Join("testdata", "cases", "template-"+tpl+".yaml")
	}

	for name, in := range inputs {
		t.Run(name, func(t *testing.T) {
			b, err := board.Parse(readFile(t, in))
			require.NoError(t, err)
			golden(t, filepath.Join(web, name+".json"), b)
		})
	}
}
