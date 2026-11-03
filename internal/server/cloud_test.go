package server_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/server"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cloudToken = "kc_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"

// fakeCloud records the uploads a server makes. Its answers are the cloud's: a Published for a board, 204 for live.
type fakeCloud struct {
	srv *httptest.Server

	mu     sync.Mutex
	boards map[string][]string // board uploads by name
	lives  map[string][]string // live uploads by name
	thumbs map[string][]string // thumbnail uploads by name
	auths  []string
	status int // when set, every upload answers this status
}

func newFakeCloud(t *testing.T) *fakeCloud {
	t.Helper()

	f := &fakeCloud{boards: map[string][]string{}, lives: map[string][]string{}, thumbs: map[string][]string{}}

	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		name := strings.TrimPrefix(r.URL.Path, "/api/cli/boards/")
		name, isLive := strings.CutSuffix(name, "/live")
		name, isThumb := strings.CutSuffix(name, "/thumb")

		f.mu.Lock()
		defer f.mu.Unlock()

		f.auths = append(f.auths, r.Header.Get("Authorization"))

		if f.status != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(f.status)
			_, _ = io.WriteString(w, `{"status":"error","message":"cloud is down"}`)

			return
		}

		if isLive {
			f.lives[name] = append(f.lives[name], string(body))
			w.WriteHeader(http.StatusNoContent)

			return
		}

		if isThumb {
			f.thumbs[name] = append(f.thumbs[name], string(body))
			w.WriteHeader(http.StatusNoContent)

			return
		}

		f.boards[name] = append(f.boards[name], string(body))

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"`+name+`","title":"","updated_at":"","links":[]}`)
	}))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeCloud) uploads(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.boards[name]...)
}

func (f *fakeCloud) thumbUploads(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.thumbs[name]...)
}

func (f *fakeCloud) liveUploads(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]string(nil), f.lives[name]...)
}

func (f *fakeCloud) requests() int {
	f.mu.Lock()
	defer f.mu.Unlock()

	return len(f.auths)
}

func (f *fakeCloud) setStatus(code int) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.status = code
}

// loginTo writes a cloud.json for the harness's home.
func (h *harness) loginTo(f *fakeCloud, published ...string) {
	h.t.Helper()

	require.NoError(h.t, cloud.Config{URL: f.srv.URL, Token: cloudToken, Email: "me@example.com", Published: published}.Save(h.paths))
}

func within(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func TestAPublishedBoardIsPushedWhenItsFileOrPositionsChange(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)
	h.put("pub", "title: One\n")
	h.put("private", "title: Private\n")
	h.loginTo(f, "pub")

	time.Sleep(200 * time.Millisecond) // the setup writes settle

	h.put("pub", "title: Two\n")
	within(t, "the upload of the changed file", func() bool {
		ups := f.uploads("pub")

		return len(ups) > 0 && strings.Contains(ups[len(ups)-1], `"title":"Two"`)
	})

	n := len(f.uploads("pub"))

	require.NoError(t, os.WriteFile(h.paths.PosFile("pub"), []byte(`{"a":{"x":5,"y":6}}`), 0o644))
	within(t, "the upload of the positions", func() bool {
		ups := f.uploads("pub")

		return len(ups) > n && strings.Contains(ups[len(ups)-1], `"pos":{"a":{"x":5,"y":6}}`)
	})

	h.put("private", "title: Private 2\n")
	time.Sleep(600 * time.Millisecond)
	assert.Empty(t, f.uploads("private"), "a board that is not published stays home")
}

func TestANoteNeverReachesTheCloud(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)
	h.put("pub", "nodes: { a: A }\n")

	time.Sleep(200 * time.Millisecond) // the setup writes settle
	h.loginTo(f, "pub")

	h.put("pub", "nodes: { a: A2 }\n")
	within(t, "the upload of the change", func() bool { return len(f.uploads("pub")) > 0 })
	time.Sleep(600 * time.Millisecond)

	before := f.requests()

	s := h.stream("/api/events")
	require.Equal(t, ": hi", s.next(t, 2*time.Second))
	drain(t, s)

	require.Equal(t, 201, h.do("POST", "/api/boards/pub/notes", `{"node":"a","text":"for the agent"}`).status)
	require.Equal(t, `data: {"name":"pub"}`, s.next(t, 3*time.Second))

	time.Sleep(600 * time.Millisecond)
	assert.Equal(t, before, f.requests(), "a note makes no upload, no live push, no thumbnail")
}

func TestAPublishedBoardGetsOneThumbnailPerWindowAfterAChange(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)
	h.put("pub", "title: One\n")
	h.put("private", "title: Private\n")

	time.Sleep(200 * time.Millisecond) // the setup writes settle
	h.loginTo(f, "pub")

	h.put("pub", "title: Two\n")
	within(t, "the thumbnail", func() bool { return len(f.thumbUploads("pub")) == 1 })
	assert.Equal(t, "\x89PNG\r\n\x1a\nstub", f.thumbUploads("pub")[0])

	// the next change is pushed at once; its thumbnail waits for the end of the five minutes
	h.put("pub", "title: Three\n")
	within(t, "the push of the change", func() bool {
		ups := f.uploads("pub")

		return len(ups) > 0 && strings.Contains(ups[len(ups)-1], `"title":"Three"`)
	})
	time.Sleep(600 * time.Millisecond)
	assert.Len(t, f.thumbUploads("pub"), 1)

	h.put("private", "title: Private 2\n")
	time.Sleep(600 * time.Millisecond)
	assert.Empty(t, f.thumbUploads("private"), "a board that is not published stays home")
}

func TestTheUploadIsWhatTheLocalAPIAnswersWithoutSources(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)
	h.loginTo(f, "pub")

	h.put("pub", "title: Pub\nnodes:\n  a: { title: A, live: { cmd: \"echo secret-command\" } }\n  b: B\nedges: [ a -> b ]\n")
	require.NoError(t, os.WriteFile(h.paths.PosFile("pub"), []byte(`{"b":{"x":1,"y":2}}`), 0o644))

	within(t, "an upload with the positions", func() bool {
		ups := f.uploads("pub")

		return len(ups) > 0 && strings.Contains(ups[len(ups)-1], `"pos":{"b"`)
	})

	ups := f.uploads("pub")
	sent := ups[len(ups)-1]
	assert.NotContains(t, sent, "secret-command")

	var up, local map[string]any
	require.NoError(t, json.Unmarshal([]byte(sent), &up))
	require.NoError(t, json.Unmarshal([]byte(h.get("/api/boards/pub").body), &local))

	// the local answer carries the source and the name; the upload has neither
	node := local["board"].(map[string]any)["nodes"].([]any)[0].(map[string]any)
	assert.Equal(t, "cmd:echo secret-command", node["live"].(map[string]any)["source"])
	node["live"].(map[string]any)["source"] = ""
	delete(local, "name")

	assert.Equal(t, local, up)
}

func TestNothingIsSentWithoutAToken(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)

	require.NoError(t, cloud.Config{URL: f.srv.URL, Published: []string{"pub"}}.Save(h.paths))
	h.put("pub", "title: One\nnodes:\n  a: { title: A, live: { cmd: \"touch "+h.paths.Home+"/ran\" } }\n")
	_, err := live.New(h.paths).Allow("pub", "")
	require.NoError(t, err)

	h.put("pub", "title: Two\nnodes:\n  a: { title: A, live: { cmd: \"touch "+h.paths.Home+"/ran\" } }\n")
	time.Sleep(1500 * time.Millisecond)

	assert.Zero(t, f.requests())
	assert.NoFileExists(t, h.paths.Home+"/ran", "no live command runs for the cloud")
}

func TestLiveValuesOfAPublishedBoardArePushed(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)

	h.put("pub", "title: L\nnodes:\n  a: { title: A, live: { cmd: \"echo 42\" } }\n")
	_, err := live.New(h.paths).Allow("pub", "")
	require.NoError(t, err)

	h.loginTo(f, "pub")

	within(t, "the live upload", func() bool { return len(f.liveUploads("pub")) > 0 })

	body := f.liveUploads("pub")[0]
	assert.Contains(t, body, `"value":"42"`)
	assert.NotContains(t, body, "echo", "the command stays home")
	assert.NotContains(t, body, "cmd:")
}

func TestAFailingCloudNeverBreaksTheServer(t *testing.T) {
	f := newFakeCloud(t)
	f.setStatus(500)

	logs := &lockedBuf{}
	h := start(t, func(c *server.Config) { c.Log = slog.New(slog.NewTextHandler(logs, nil)) })
	h.put("pub", "title: One\n")
	h.loginTo(f, "pub")

	s := h.stream("/api/events")
	require.Equal(t, ": hi", s.next(t, 2*time.Second))
	drain(t, s)

	h.put("pub", "title: Two\n")

	assert.Equal(t, `data: {"name":"pub"}`, s.next(t, 3*time.Second), "the page still hears of the change")
	assert.Equal(t, 200, h.get("/api/boards/pub").status)

	within(t, "the failure in the log", func() bool { return strings.Contains(logs.String(), "cloud is down") })
	assert.Contains(t, logs.String(), "cloud push")
	assert.Contains(t, logs.String(), "level=WARN")
	assert.NotContains(t, logs.String(), cloudToken)

	// the cloud comes back: the push is retried without another change
	f.setStatus(0)
	within(t, "the retried upload", func() bool { return len(f.uploads("pub")) > 0 })
}

func TestLoginAfterTheServerStartedIsPickedUp(t *testing.T) {
	f := newFakeCloud(t)
	h := start(t)
	h.put("pub", "title: One\n")

	time.Sleep(200 * time.Millisecond)
	assert.Zero(t, f.requests())

	h.loginTo(f, "pub")
	h.put("pub", "title: Two\n")

	within(t, "the upload", func() bool { return len(f.uploads("pub")) > 0 })

	f.mu.Lock()
	assert.Equal(t, "Bearer "+cloudToken, f.auths[0])
	f.mu.Unlock()
}

func TestStartingTheServerPushesThePublishedBoardsOnce(t *testing.T) {
	f := newFakeCloud(t)
	start(t, func(c *server.Config) {
		require.NoError(t, c.Paths.Ensure())
		require.NoError(t, os.WriteFile(c.Paths.BoardFile("pub"), []byte("title: Before\n"), 0o644))
		require.NoError(t, cloud.Config{URL: f.srv.URL, Token: cloudToken, Published: []string{"pub"}}.Save(c.Paths))
	})

	within(t, "the catch-up upload", func() bool {
		ups := f.uploads("pub")

		return len(ups) == 1 && strings.Contains(ups[0], `"title":"Before"`)
	})
}

// lockedBuf is a log buffer safe for the server's goroutines.
type lockedBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.Write(p)
}

func (l *lockedBuf) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.String()
}
