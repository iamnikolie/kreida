package cloud_test

import (
	"bytes"
	"context"
	"image"
	imagepng "image/png"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/images"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func picture(t *testing.T, w, h int) []byte {
	t.Helper()

	var b bytes.Buffer
	require.NoError(t, imagepng.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))))

	return b.Bytes()
}

// imageCloud answers like the agreed cloud: a board whose images it lacks gets a 409 naming them.
type imageCloud struct {
	*fakeCloud

	mu     sync.Mutex
	stored map[string]bool
}

func newImageCloud(t *testing.T) *imageCloud {
	t.Helper()

	c := &imageCloud{fakeCloud: newFake(t), stored: map[string]bool{}}

	c.respond = func(in call, w http.ResponseWriter) bool {
		c.mu.Lock()
		defer c.mu.Unlock()

		if i := strings.Index(in.Path, "/images/"); in.Method == "PUT" && i >= 0 {
			c.stored[in.Path[i+len("/images/"):]] = true
			w.WriteHeader(204)

			return true
		}

		if in.Method != "PUT" || strings.HasSuffix(in.Path, "/live") || strings.HasSuffix(in.Path, "/thumb") {
			return false
		}

		var missing []string

		nodes, _ := decode(c.t, in.Body)["board"].(map[string]any)["nodes"].([]any)

		for _, n := range nodes {
			if img, ok := n.(map[string]any)["image"].(map[string]any); ok {
				if sha := img["sha256"].(string); !c.stored[sha] {
					missing = append(missing, `"`+sha+`"`)
				}
			}
		}

		if len(missing) > 0 {
			reply(w, 409, `{"status":"error","message":"images missing","missing_images":[`+strings.Join(missing, ",")+`]}`)

			return true
		}

		return false
	}

	return c
}

func boardWithImage(t *testing.T, p paths.Paths, name, file string, data []byte) images.File {
	t.Helper()

	path := filepath.Join(p.Dir, file)
	require.NoError(t, os.WriteFile(path, data, 0o644))
	require.NoError(t, os.WriteFile(p.BoardFile(name), []byte("title: T\nnodes:\n  a: { title: A, image: "+file+" }\n"), 0o644))

	f, err := images.Load(path)
	require.NoError(t, err)

	return f
}

func TestAnUploadCarriesTheHashAndNeverThePath(t *testing.T) {
	p := scratch(t)
	f := boardWithImage(t, p, "x", "private-name.png", picture(t, 4, 2))

	up, err := cloud.Snapshot(p, "x")
	require.NoError(t, err)

	body, err := up.JSON()
	require.NoError(t, err)
	assert.Contains(t, string(body), `"image":{"sha256":"`+f.SHA256+`","type":"image/png","w":4,"h":2}`)
	assert.NotContains(t, string(body), "private-name")
	assert.NotContains(t, string(body), p.Dir)
	assert.Contains(t, up.Images, f.SHA256)
}

func TestACardWithABrokenImageGoesUpWithoutOne(t *testing.T) {
	p := scratch(t)
	require.NoError(t, os.WriteFile(p.BoardFile("x"), []byte("nodes:\n  a: { title: A, image: gone.png }\n"), 0o644))

	up, err := cloud.Snapshot(p, "x")
	require.NoError(t, err)

	body, err := up.JSON()
	require.NoError(t, err)
	assert.NotContains(t, string(body), `"image"`)
	assert.NotContains(t, string(body), "gone")
	assert.Len(t, up.Problems, 1)
}

func TestPublishPutsTheMissingImageUpThenTheBoardAgain(t *testing.T) {
	p := scratch(t)
	f := boardWithImage(t, p, "x", "a.png", picture(t, 4, 2))
	c := newImageCloud(t)
	login(t, p, c.fakeCloud)

	s, _ := newSyncer(t, p)
	_, err := s.Publish(context.Background(), "x")
	require.NoError(t, err)

	calls := c.all()
	require.Len(t, calls, 3)
	assert.Equal(t, "/api/cli/boards/x", calls[0].Path)
	assert.Equal(t, "/api/cli/boards/x/images/"+f.SHA256, calls[1].Path)
	assert.Equal(t, "image/png", calls[1].Type)
	assert.Equal(t, string(f.Bytes), calls[1].Body)
	assert.Equal(t, "Bearer "+token, calls[1].Auth)
	assert.Equal(t, "/api/cli/boards/x", calls[2].Path)

	// published for good: a publish that finds the bytes up there costs one call
	_, err = s.Publish(context.Background(), "x")
	require.NoError(t, err)
	assert.Equal(t, 1, c.count("PUT", "/images/"+f.SHA256))
}

func TestOnlyTheBoardsOwnBytesGoWhateverTheCloudAsks(t *testing.T) {
	p := scratch(t)
	f := boardWithImage(t, p, "x", "a.png", picture(t, 4, 2))
	other := boardWithImage(t, p, "other", "b.png", picture(t, 5, 5))
	c := newFake(t)
	login(t, p, c)

	c.respond = func(in call, w http.ResponseWriter) bool {
		if in.Method == "PUT" && in.Path == "/api/cli/boards/x" {
			reply(w, 409, `{"message":"images missing","missing_images":["`+other.SHA256+`","`+strings.Repeat("0", 64)+`"]}`)

			return true
		}

		return false
	}

	s, _ := newSyncer(t, p)
	_, err := s.Publish(context.Background(), "x")
	require.Error(t, err)
	assert.True(t, cloud.IsStatus(err, 409))
	assert.Equal(t, 0, c.count("PUT", "/images/"+other.SHA256))
	assert.Equal(t, 0, c.count("PUT", "/images/"+f.SHA256))
	assert.Len(t, c.puts("x"), 1, "a list with nothing of ours is not asked again")
}

func TestAnImageChangeTakesTheRoadOfABoardChange(t *testing.T) {
	p := scratch(t)
	first := boardWithImage(t, p, "pub", "a.png", picture(t, 4, 2))
	boardWithImage(t, p, "private", "secret.png", picture(t, 9, 9))
	c := newImageCloud(t)
	login(t, p, c.fakeCloud, "pub")

	s, _ := newSyncer(t, p)
	s.Start()

	eventually(t, "the first push", func() bool { return c.count("PUT", "/images/"+first.SHA256) == 1 && len(c.puts("pub")) == 2 })

	second := picture(t, 6, 3)
	require.NoError(t, os.WriteFile(filepath.Join(p.Dir, "a.png"), second, 0o644))

	s.Changed("pub")

	eventually(t, "the new image", func() bool { return len(c.puts("pub")) == 4 })
	assert.Equal(t, 1, c.count("PUT", "/images/"+first.SHA256), "the old bytes do not go again")

	// a board off the list sends nothing, whatever changes
	s.Changed("private")

	never(t, "a board off the list went up", func() bool {
		for _, in := range c.all() {
			if strings.Contains(in.Path, "/private") {
				return true
			}
		}

		return false
	})
}

func TestAPushThatBrokeOffGoesOnWhereItStopped(t *testing.T) {
	p := scratch(t)
	f := boardWithImage(t, p, "x", "a.png", picture(t, 4, 2))
	c := newImageCloud(t)
	login(t, p, c.fakeCloud, "x")

	inner := c.respond
	tries := 0

	c.respond = func(in call, w http.ResponseWriter) bool {
		if strings.Contains(in.Path, "/images/") {
			tries++
			if tries == 1 {
				w.Header().Set("Retry-After", "0")
				reply(w, 429, `{"message":"slow down"}`)

				return true
			}
		}

		return inner(in, w)
	}

	s, _ := newSyncer(t, p)
	s.Changed("x")

	eventually(t, "the second try", func() bool { return c.count("PUT", "/images/"+f.SHA256) == 2 && len(c.puts("x")) == 3 })
}
