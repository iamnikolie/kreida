package board

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImageIsAKnownKeyAndItsPathNeverReachesJSON(t *testing.T) {
	b, err := Parse("nodes:\n  a: { title: A, image: /Users/someone/private/a.png }\n")
	require.NoError(t, err)
	assert.Equal(t, "/Users/someone/private/a.png", b.Nodes[0].ImagePath)
	assert.Nil(t, b.Nodes[0].Image, "parsing looks at no file")

	data, err := json.Marshal(b)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "private")
	assert.NotContains(t, string(data), "image", "a card without a picture says nothing about one")
}

func TestABadImageValueIsRefusedWithoutRepeatingIt(t *testing.T) {
	for yaml, want := range map[string]string{
		"a: { title: A, image: https://secret.example/x.png }":          "not a URL",
		`a: { title: A, image: "data:image/png;base64,c2VjcmV0" }`:      "not a URL",
		"a: { title: A, image: /secret/x.png, fields: [id] }":           "not a table",
		"a: { title: A, image: /secret/x.png, cols: [c], rows: [[1]] }": "not a table",
		"a: { title: A, kind: who, image: /secret/x.png }":              "not a pill",
		"a: { title: A, shape: pill, image: /secret/x.png }":            "not a pill",
	} {
		_, err := Parse("nodes:\n  " + yaml + "\n")
		require.Error(t, err, yaml)
		assert.Contains(t, err.Error(), want, yaml)
		assert.NotContains(t, err.Error(), "secret", yaml)
	}

	_, err := Parse("nodes:\n  a: { title: A, image: { path: x.png } }\n")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "image must be text")
}
