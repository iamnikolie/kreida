package mermaid_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/mermaid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// update rewrites the expected outputs under testdata (and web/test/fixtures/mermaid-flow.json) from the code.
var update = flag.Bool("update", false, "rewrite golden files from the current code")

func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

func exists(p string) bool {
	_, err := os.Stat(p)

	return err == nil
}

// golden compares got with the file at path (as JSON values when asJSON), or writes it with -update.
func golden(t *testing.T, path string, got []byte, asJSON bool) {
	t.Helper()

	if *update {
		require.NoError(t, os.WriteFile(path, got, 0o644))

		return
	}

	if !asJSON {
		assert.Equal(t, read(t, path), string(got), path)

		return
	}

	var w, g any

	require.NoError(t, json.Unmarshal([]byte(read(t, path)), &w), path)
	require.NoError(t, json.Unmarshal(got, &g))
	assert.Equal(t, w, g, path)
}

func indented(t *testing.T, v any) []byte {
	t.Helper()

	data, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)

	return append(data, '\n')
}

func load(t *testing.T, text string) *board.Board {
	t.Helper()

	text, err := mermaid.FromMermaid(text)
	require.NoError(t, err)

	b, err := board.Parse(text)
	require.NoError(t, err, text)

	return b
}

func TestFlowchartShapesClassesChainsFanOutLabelledAndStyledEdges(t *testing.T) {
	b := load(t, `%% title: T
flowchart LR
  A[Start] -->|"yes"| B(Round) --> C{Decide}
  C == big ==> D[(DB)]
  A & B -.-> E:::warn
  my-node-->F
  C -- no --> A
  subgraph g1 [Group one]
    direction TB
    X["<b>Title</b><br/>second line"]
  end
  class F out`)

	assert.Equal(t, "T", b.Title)
	assert.Equal(t, "LR", b.Direction)

	n := map[string]board.Node{}
	for _, x := range b.Nodes {
		n[x.ID] = x
	}

	assert.Equal(t, "database", *n["D"].Icon)
	assert.Equal(t, "warn", n["E"].Kind)
	assert.Equal(t, "out", n["F"].Kind)
	assert.Equal(t, []string{"Title", "second line", "g1"}, []string{n["X"].Title, n["X"].Text, *n["X"].Group})

	require.Len(t, b.Groups, 1)
	assert.Equal(t, "g1", b.Groups[0].ID)
	assert.Equal(t, "Group one", b.Groups[0].Label)
	assert.Equal(t, "TB", *b.Groups[0].Direction)

	var e []string

	for _, x := range b.Edges {
		s := fmt.Sprintf("%s>%s:%s", x.From, x.To, x.Label)
		if x.Dashed {
			s += ":d"
		}

		if x.Thick {
			s += ":t"
		}

		e = append(e, s)
	}

	assert.Equal(t, []string{"A>B:yes", "B>C:", "C>D:big:t", "A>E::d", "B>E::d", "my-node>F:", "C>A:no"}, e)
}

func TestSequenceParticipantsBecomeLanesMessagesOrderedCards(t *testing.T) {
	b := load(t, "sequenceDiagram\n  participant K as Редактор\n  C->>K: Запрос\n  K-->>C: Ответ")

	assert.Equal(t, "lanes", b.Layout)

	var labels []string
	for _, g := range b.Groups {
		labels = append(labels, g.Label)
	}

	assert.Equal(t, []string{"Редактор", "C"}, labels)

	require.Len(t, b.Nodes, 2)
	assert.Equal(t, []any{"Запрос", "C", 0.0}, []any{b.Nodes[0].Title, *b.Nodes[0].Group, *b.Nodes[0].Step})
	assert.Equal(t, []any{"Ответ", "K", 1.0}, []any{b.Nodes[1].Title, *b.Nodes[1].Group, *b.Nodes[1].Step})
}

func TestExportImportRoundTripKeepsTheBoard(t *testing.T) {
	b := load(t, read(t, filepath.Join("testdata", "import", "flow-file.mmd")))
	assert.Len(t, b.Nodes, 12, "S1 ~~~ S2 links groups, it adds no cards")

	text, err := mermaid.ToMermaid(b)
	require.NoError(t, err)

	again := load(t, text)

	type card struct{ ID, Title, Text, Kind, Group string }

	cards := func(b *board.Board) []card {
		var out []card

		for _, n := range b.Nodes {
			g := ""
			if n.Group != nil {
				g = *n.Group
			}

			out = append(out, card{n.ID, n.Title, n.Text, n.Kind, g})
		}

		return out
	}
	links := func(b *board.Board) [][3]string {
		var out [][3]string
		for _, e := range b.Edges {
			out = append(out, [3]string{e.From, e.To, e.Label})
		}

		return out
	}

	assert.Equal(t, cards(b), cards(again))
	assert.Equal(t, links(b), links(again))
	assert.Equal(t, b.Title, again.Title)
}

func TestUnsupportedDiagramTypesFailLoudly(t *testing.T) {
	_, err := mermaid.FromMermaid("pie\n \"a\": 1")
	assert.ErrorContains(t, err, "only flowchart")
	assert.EqualError(t, err, `mermaid: only flowchart/graph and sequenceDiagram are supported (got "pie")`)
}

// TestGoldenImport imports every diagram under testdata/import and compares the board it reads as (or the error)
// and the Mermaid exported from that board. flow-file is also the page's fixture mermaid-flow.
func TestGoldenImport(t *testing.T) {
	dir := filepath.Join("testdata", "import")
	files, err := filepath.Glob(filepath.Join(dir, "*.mmd"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".mmd")
		if strings.HasSuffix(name, ".export") { // the expected export of another input, not an input
			continue
		}

		t.Run(name, func(t *testing.T) {
			text, err := mermaid.FromMermaid(read(t, f))
			if *update {
				for _, ext := range []string{".err.json", ".json", ".export.mmd"} {
					_ = os.Remove(filepath.Join(dir, name+ext))
				}
			}

			if err != nil {
				if !*update {
					require.True(t, exists(filepath.Join(dir, name+".err.json")), "unexpected error: %v", err)
				}

				golden(t, filepath.Join(dir, name+".err.json"), indented(t, map[string]string{"message": err.Error()}), true)

				return
			}

			require.False(t, exists(filepath.Join(dir, name+".err.json")), "expected an error")

			b, err := board.Parse(text)
			require.NoError(t, err, "imported YAML must be a valid board:\n%s", text)
			golden(t, filepath.Join(dir, name+".json"), indented(t, b), true)

			if name == "flow-file" {
				golden(t, filepath.Join("..", "..", "web", "test", "fixtures", "mermaid-flow.json"), indented(t, b), true)
			}

			out, err := mermaid.ToMermaid(b)
			require.NoError(t, err)
			golden(t, filepath.Join(dir, name+".export.mmd"), []byte(out), false)
		})
	}
}

// TestGoldenExport exports every board the parser tests use.
func TestGoldenExport(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "export", "*.mmd"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".mmd")

		t.Run(name, func(t *testing.T) {
			var text string

			for _, d := range []string{"cases", "fixtures"} {
				if p := filepath.Join("..", "board", "testdata", d, name+".yaml"); exists(p) {
					text = read(t, p)
				}
			}

			require.NotEmpty(t, text, "input board for %s", name)

			b, err := board.Parse(text)
			require.NoError(t, err)

			out, err := mermaid.ToMermaid(b)
			require.NoError(t, err)
			golden(t, f, []byte(out), false)
		})
	}
}

func TestExportStopsOnTwoGroupsWithOneIDThatContainEachOther(t *testing.T) {
	b, err := board.Parse("groups:\n  - { id: g, label: A }\n  - { id: g, label: B, parent: g }\n")
	require.NoError(t, err)

	_, err = mermaid.ToMermaid(b)
	assert.Error(t, err, "it must not hang or crash here")
}
