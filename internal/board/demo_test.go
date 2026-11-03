package board_test

import (
	"path/filepath"
	"regexp"
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The landing page of kreida cloud embeds these boards (demo/README.md): one per language, sketch style, told in
// steps, and nothing that runs - a visitor's page must never execute a command or fetch a URL.
func TestLandingDemoBoards(t *testing.T) {
	cyrillic := regexp.MustCompile(`\p{Cyrillic}`)
	russianOnly := regexp.MustCompile(`[ыэъёЫЭЪЁ]`)
	ukrainian := regexp.MustCompile(`[іїєґІЇЄҐ]`)

	for _, lang := range []string{"ru", "uk", "en"} {
		t.Run(lang, func(t *testing.T) {
			text := readFile(t, filepath.Join("..", "..", "demo", "landing-"+lang+".yaml"))

			b, err := board.Parse(text)
			require.NoError(t, err)

			assert.Equal(t, "sketch", b.Style)
			assert.NotEmpty(t, b.Title)
			assert.GreaterOrEqual(t, len(b.Steps), 4, "one step per stage")
			assert.LessOrEqual(t, len(b.Steps), 6)
			assert.GreaterOrEqual(t, len(b.Nodes), 5)
			assert.LessOrEqual(t, len(b.Nodes), 7)

			ids := map[string]bool{}
			for _, n := range b.Nodes {
				ids[n.ID] = true

				assert.Nil(t, n.Live, "%s: a demo card runs nothing", n.ID)
			}

			for i, s := range b.Steps {
				assert.NotEmpty(t, s.Title, "step %d", i+1)
				assert.LessOrEqual(t, len([]rune(s.Title)), 28, "step %d: a short title", i+1)

				for _, id := range append(append([]string{}, s.Show...), s.Hide...) {
					assert.True(t, ids[id], "step %d names %q, which is no card", i+1, id)
				}
			}

			switch lang {
			case "en":
				assert.False(t, cyrillic.MatchString(text), "en has no Cyrillic")
			case "uk":
				assert.True(t, ukrainian.MatchString(text), "uk is written in Ukrainian")
				assert.False(t, russianOnly.MatchString(text), "uk has no ы э ъ ё")
			case "ru":
				assert.True(t, cyrillic.MatchString(text))
			}
		})
	}
}

// TestCarryDemoBoards: the boards that show the table, note, questions, glossary, matrix, radial and folding.
func TestCarryDemoBoards(t *testing.T) {
	for name, layout := range map[string]string{"carry-flow": "auto", "carry-matrix": "matrix", "carry-mindmap": "radial"} {
		t.Run(name, func(t *testing.T) {
			b, err := board.Parse(readFile(t, filepath.Join("..", "..", "demo", name+".yaml")))
			require.NoError(t, err)
			assert.Equal(t, layout, b.Layout)
			assert.NotEmpty(t, b.Nodes)
		})
	}
}
