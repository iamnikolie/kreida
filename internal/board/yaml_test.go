package board

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// libyaml takes "?" in a flow collection for a key indicator; boards use it as text ("Запускается по расписанию?").
func TestFlowQuestionMarks(t *testing.T) {
	cases := map[string]string{
		"{ title: Запускается вручную или по расписанию?, kind: note }": `{"title":"Запускается вручную или по расписанию?","kind":"note"}`,
		`{ title: a? b, x: "q?" }`: `{"title":"a? b","x":"q?"}`,
		`[ why?, "it's?", 'да?' ]`: `["why?","it's?","да?"]`,
		"title: Так? # и это?":     `{"title":"Так?"}`,
		"nodes:\n  q: { title: Кто? — Редактор, text: it's ok? }": `{"nodes":{"q":{"title":"Кто? — Редактор","text":"it's ok?"}}}`,
	}

	for in, want := range cases {
		doc, err := parseYAML(in)
		require.NoError(t, err, in)

		v, err := decode(doc)
		require.NoError(t, err, in)

		got, err := json.Marshal(v)
		require.NoError(t, err)
		assert.JSONEq(t, want, string(got), in)
	}

	b, err := Parse("nodes:\n  q1: { title: Запускается вручную или по расписанию?, kind: note }\n")
	require.NoError(t, err)
	assert.Equal(t, "Запускается вручную или по расписанию?", b.Nodes[0].Title)
}

func TestFlowQuestionMarksKeepComments(t *testing.T) {
	doc, err := parseYAML("a: { t: b? } # c?\n")
	require.NoError(t, err)
	assert.Equal(t, "# c?", doc.Content[0].Content[1].LineComment)
}

func TestFlowQuestionMarksSurviveAnEdit(t *testing.T) {
	in := "nodes:\n  q: { title: Кто?, text: it's ok? }   # да?\n"

	out, spliced, err := edit(in, planPatchNode("q", sp("Кто? Редактор?"), nil))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, "nodes:\n  q: { title: Кто? Редактор?, text: it's ok? }   # да?\n", out)
}

func TestYAMLRules(t *testing.T) {
	_, err := Parse("title: a\ntitle: b\n")
	assert.True(t, IsSyntax(err), "a repeated key: %v", err)

	_, err = Parse("title: a\n---\ntitle: b\n")
	assert.True(t, IsSyntax(err), "two documents: %v", err)

	_, err = Parse("nodes: { a: A\n")
	assert.True(t, IsSyntax(err))

	_, err = Parse("- a\n")
	assert.False(t, IsSyntax(err))
	assert.EqualError(t, err, "board must be a YAML map")
}
