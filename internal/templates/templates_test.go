package templates_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/templates"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTitlesSurviveQuoting: every template with any title is a valid board that reads the title back.
func TestTitlesSurviveQuoting(t *testing.T) {
	titles := []string{"Plain title", "Title: with colon", "a,b", "Привет, мир", "- dash", "#hash", "  padded  ",
		"100%", `"Quoted" & <b>`, "it's", "123", "true", "~", "a | b", "Why?", "[draft] plan", "{x}", ""}

	for _, tpl := range templates.All() {
		for _, title := range titles {
			t.Run(tpl.Name+"/"+title, func(t *testing.T) {
				text := tpl.YAML(title)

				b, err := board.Parse(text)
				require.NoError(t, err, text)
				assert.Equal(t, title, b.Title)

				// a card title is trimmed and split at " | " (title | text)
				if tpl.Name == "mindmap" && title == strings.TrimSpace(title) && !strings.Contains(title, " | ") {
					assert.Equal(t, title, b.Nodes[0].Title, "the root card carries the title")
				}
			})
		}
	}
}

// TestList pins what `kreida templates` prints.
func TestList(t *testing.T) {
	var sb strings.Builder

	for _, tpl := range templates.All() {
		fmt.Fprintf(&sb, "%-14s %s\n", tpl.Name, tpl.About)
	}

	want, err := os.ReadFile(filepath.Join("testdata", "list.txt"))
	require.NoError(t, err)
	assert.Equal(t, string(want), sb.String())
}

func TestDefaultIsFlow(t *testing.T) {
	flow, ok := templates.Get("flow")
	require.True(t, ok)
	assert.Equal(t, flow.YAML("myboard"), templates.Default("myboard"))

	_, ok = templates.Get("nope")
	assert.False(t, ok)
}
