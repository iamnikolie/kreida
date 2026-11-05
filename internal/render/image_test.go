package render_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math/rand"
	"testing"
	"testing/fstest"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTheStandaloneFileHoldsTheBytesAndNoPath(t *testing.T) {
	dist := fstest.MapFS{"index.html": {Data: []byte("<html><head><title>x</title></head></html>")}}
	b, err := board.Parse("nodes:\n  a: { title: A, image: /Users/someone/private/a.png }\n")
	require.NoError(t, err)

	b.Nodes[0].Image = &board.Image{SHA256: "abc", Type: "image/png", W: 1, H: 1}

	html, err := render.HTML(dist, render.Data{Name: "x", Board: b, Images: map[string]string{"abc": "data:image/png;base64,QUJD"}})
	require.NoError(t, err)
	assert.Contains(t, html, `"images":{"abc":"data:image/png;base64,QUJD"}`)
	assert.Contains(t, html, `"sha256":"abc"`)
	assert.NotContains(t, html, "private")
}

func TestShrinkKeepsAFittingPictureAndFitsANoisyOne(t *testing.T) {
	small := []byte("fits")
	got, err := render.Shrink(small, 10)
	require.NoError(t, err)
	assert.Equal(t, small, got)

	// noise is the worst a photo can do to a thumbnail
	src := image.NewRGBA(image.Rect(0, 0, render.ThumbWidth, render.ThumbHeight))
	rnd := rand.New(rand.NewSource(1))

	for y := range render.ThumbHeight {
		for x := range render.ThumbWidth {
			src.Set(x, y, color.RGBA{uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), uint8(rnd.Intn(256)), 255})
		}
	}

	var pic bytes.Buffer
	require.NoError(t, png.Encode(&pic, src))

	const limit = 256 << 10

	require.Greater(t, pic.Len(), limit)

	got, err = render.Shrink(pic.Bytes(), limit)
	require.NoError(t, err)
	assert.LessOrEqual(t, len(got), limit)
	assert.True(t, bytes.HasPrefix(got, []byte("\x89PNG\r\n\x1a\n")))

	_, err = render.Shrink([]byte("not a png at all"), 1)
	require.Error(t, err)
}
