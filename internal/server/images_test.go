package server_test

import (
	"bytes"
	"image"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func picture(t *testing.T, w, h int) []byte {
	t.Helper()

	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))))

	return b.Bytes()
}

// shaOf is the hash the page got for card a.
func shaOf(t *testing.T, r reply) string {
	t.Helper()

	nodes := r.json(t)["board"].(map[string]any)["nodes"].([]any)
	img, ok := nodes[0].(map[string]any)["image"].(map[string]any)
	require.True(t, ok, r.body)

	return img["sha256"].(string)
}

func TestTheEditorGetsAHashAndTheBytesNeverThePath(t *testing.T) {
	h := start(t)
	data := picture(t, 4, 2)
	require.NoError(t, os.WriteFile(filepath.Join(h.paths.Dir, "secret-name.png"), data, 0o644))
	h.put("x", "nodes:\n  a: { title: A, image: secret-name.png }\n")

	r := h.do("GET", "/api/boards/x", "")
	assert.NotContains(t, r.body, "secret-name")
	sha := shaOf(t, r)

	img := h.do("GET", "/api/boards/x/images/"+sha, "")
	require.Equal(t, 200, img.status)
	assert.Equal(t, string(data), img.body)
	assert.Equal(t, "image/png", img.header.Get("Content-Type"))
	assert.Equal(t, "nosniff", img.header.Get("X-Content-Type-Options"))
	assert.Equal(t, "default-src 'none'; sandbox", img.header.Get("Content-Security-Policy"))
	assert.Equal(t, "private, no-cache", img.header.Get("Cache-Control"))

	assert.Equal(t, 200, h.do("HEAD", "/api/boards/x/images/"+sha, "").status)

	again := h.do("GET", "/api/boards/x/images/"+sha, "", "If-None-Match", `"`+sha+`"`)
	assert.Equal(t, http.StatusNotModified, again.status)

	assert.Equal(t, 404, h.do("GET", "/api/boards/x/images/"+strings.Repeat("a", 64), "").status)
	assert.Equal(t, 404, h.do("GET", "/api/boards/x/images/secret-name.png", "").status)
	assert.Equal(t, 404, h.do("GET", "/api/boards/x/images/..%2Fsecret-name.png", "").status)
}

func TestTheEditorSaysWhyACardHasNoPictureAShareLinkDoesNot(t *testing.T) {
	h := start(t)
	h.put("x", "nodes:\n  a: { title: A, image: gone-file.png }\n")

	r := h.do("GET", "/api/boards/x", "")
	assert.Contains(t, r.body, `"image":{"error":"missing"}`)
	assert.NotContains(t, r.body, "gone-file")

	link, err := h.shares.Share("x")
	require.NoError(t, err)

	s := h.do("GET", "/api/share/"+link, "")
	require.Equal(t, 200, s.status)
	assert.NotContains(t, s.body, `"image"`)
	assert.NotContains(t, s.body, "gone-file")
}

func TestAShareLinkReachesOnlyTheImagesOfItsBoardAsItIsNow(t *testing.T) {
	h := start(t)
	require.NoError(t, os.WriteFile(filepath.Join(h.paths.Dir, "a.png"), picture(t, 4, 2), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(h.paths.Dir, "b.png"), picture(t, 8, 8), 0o644))
	h.put("shared", "nodes:\n  a: { title: A, image: a.png }\n")
	h.put("other", "nodes:\n  a: { title: A, image: b.png }\n")

	link, err := h.shares.Share("shared")
	require.NoError(t, err)

	mine := shaOf(t, h.do("GET", "/api/share/"+link, ""))
	theirs := shaOf(t, h.do("GET", "/api/boards/other", ""))

	foreign := []string{"Host", "kreida.example.com", "Origin", "https://kreida.example.com"}

	assert.Equal(t, 200, h.do("GET", "/api/share/"+link+"/images/"+mine, "", foreign...).status)
	assert.Equal(t, 404, h.do("GET", "/api/share/"+link+"/images/"+theirs, "", foreign...).status)
	assert.Equal(t, 404, h.do("GET", "/api/share/nosuchlink0000000/images/"+mine, "", foreign...).status)
	assert.Equal(t, 403, h.do("GET", "/api/boards/shared/images/"+mine, "", foreign...).status, "the editor's route stays the editor's")

	// the card drops the picture: the link no longer reaches it
	h.put("shared", "nodes:\n  a: { title: A }\n")
	assert.Equal(t, 404, h.do("GET", "/api/share/"+link+"/images/"+mine, "").status)
}

func TestAChangedImageFileReachesThePageAndOnlyAPublishedBoardReachesTheCloud(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t, func(c *server.Config) { c.ImagePoll = 10 * time.Millisecond })

	pic := filepath.Join(t.TempDir(), "outside.png")
	private := filepath.Join(t.TempDir(), "private.png")

	require.NoError(t, os.WriteFile(pic, picture(t, 4, 2), 0o644))
	require.NoError(t, os.WriteFile(private, picture(t, 4, 3), 0o644))
	h.put("pub", "nodes:\n  a: { title: A, image: "+pic+" }\n")
	h.put("private", "nodes:\n  a: { title: A, image: "+private+" }\n")

	h.loginTo(f, "pub")

	waitFor(t, func() bool { return strings.Contains(h.do("GET", "/api/boards/pub", "").body, `"w":4`) })

	before := shaOf(t, h.do("GET", "/api/boards/pub", ""))

	s := h.stream("/api/events")
	drain(t, s)

	require.NoError(t, os.WriteFile(pic, picture(t, 6, 3), 0o644))
	require.NoError(t, os.WriteFile(private, picture(t, 7, 3), 0o644))

	waitFor(t, func() bool { return strings.Contains(s.next(t, time.Second), `"name":"pub"`) })

	assert.NotEqual(t, before, shaOf(t, h.do("GET", "/api/boards/pub", "")))

	waitFor(t, func() bool {
		ups := f.uploads("pub")
		return len(ups) > 0 && strings.Contains(ups[len(ups)-1], `"w":6`)
	})
	assert.NotContains(t, strings.Join(f.uploads("pub"), ""), pic)
	assert.Empty(t, f.uploads("private"))
}
