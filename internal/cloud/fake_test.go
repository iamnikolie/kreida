package cloud_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/require"
)

// token has the shape of a CLI token: kc_ and 43 characters.
const token = "kc_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"

// call is one request the fake cloud saw.
type call struct {
	Method, Path, Auth string
	Body               string
	Type               string // Content-Type
}

// fakeCloud stands in for the cloud's CLI API: it records every request and answers like the real one unless
// respond answers first (true means it did).
type fakeCloud struct {
	t       *testing.T
	srv     *httptest.Server
	respond func(c call, w http.ResponseWriter) bool

	mu    sync.Mutex
	calls []call
}

func newFake(t *testing.T) *fakeCloud {
	t.Helper()

	f := &fakeCloud{t: t}

	f.srv = httptest.NewServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)

	return f
}

func (f *fakeCloud) serve(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c := call{Method: r.Method, Path: r.URL.EscapedPath(), Auth: r.Header.Get("Authorization"), Body: string(body), Type: r.Header.Get("Content-Type")}

	f.mu.Lock()
	f.calls = append(f.calls, c)
	f.mu.Unlock()

	if f.respond != nil && f.respond(c, w) {
		return
	}

	switch {
	case c.Method == "GET" && c.Path == "/api/cli/whoami":
		reply(w, 200, `{"email":"me@example.com","token_name":"laptop"}`)
	case c.Method == "GET" && c.Path == "/api/cli/boards":
		reply(w, 200, `[{"name":"demo","title":"Demo","updated_at":"2026-10-29T12:00:00Z","links":[{"id":"1","url":"`+f.srv.URL+`/b/tok","expires_at":null,"passcode":false}]}]`)
	case c.Method == "PUT" && (strings.HasSuffix(c.Path, "/live") || strings.HasSuffix(c.Path, "/thumb")):
		w.WriteHeader(204)
	case c.Method == "PUT":
		name := strings.TrimPrefix(c.Path, "/api/cli/boards/")
		reply(w, 200, `{"name":"`+name+`","title":"T","updated_at":"2026-10-29T12:00:00Z","links":[{"id":"1","url":"`+f.srv.URL+`/b/tok","expires_at":null,"passcode":false}]}`)
	case c.Method == "DELETE":
		w.WriteHeader(204)
	default:
		reply(w, 404, `{"status":"error","message":"no route","request_id":"r1"}`)
	}
}

func reply(w http.ResponseWriter, code int, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = io.WriteString(w, body)
}

// count is how many requests used the method on a path ending in suffix ("" for any path).
func (f *fakeCloud) count(method, suffix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()

	n := 0

	for _, c := range f.calls {
		if c.Method == method && strings.HasSuffix(c.Path, suffix) {
			n++
		}
	}

	return n
}

// puts lists the bodies of the board uploads (not the live ones) to a board.
func (f *fakeCloud) puts(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []string

	for _, c := range f.calls {
		if c.Method == "PUT" && c.Path == "/api/cli/boards/"+name {
			out = append(out, c.Body)
		}
	}

	return out
}

// thumbs lists the bodies of the thumbnail uploads to a board.
func (f *fakeCloud) thumbs(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []string

	for _, c := range f.calls {
		if c.Method == "PUT" && c.Path == "/api/cli/boards/"+name+"/thumb" {
			out = append(out, c.Body)
		}
	}

	return out
}

// lives lists the bodies of the live uploads to a board.
func (f *fakeCloud) lives(name string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []string

	for _, c := range f.calls {
		if c.Method == "PUT" && c.Path == "/api/cli/boards/"+name+"/live" {
			out = append(out, c.Body)
		}
	}

	return out
}

func (f *fakeCloud) all() []call {
	f.mu.Lock()
	defer f.mu.Unlock()

	return append([]call(nil), f.calls...)
}

// scratch is a home and boards directory in a temporary directory, never ~/.kreida.
func scratch(t *testing.T) paths.Paths {
	t.Helper()

	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}
	require.NoError(t, p.Ensure())

	return p
}

// login saves a login for the fake cloud with the boards published.
func login(t *testing.T, p paths.Paths, f *fakeCloud, published ...string) {
	t.Helper()

	require.NoError(t, cloud.Config{URL: f.srv.URL, Token: token, Email: "me@example.com", Published: published}.Save(p))
}

// eventually waits for cond, polling; the Syncer works in the background.
func eventually(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

// never checks that cond stays false for a moment.
func never(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(deadline) {
		if cond() {
			t.Fatalf("unexpected: %s", what)
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func decode(t *testing.T, s string) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(s), &m), s)

	return m
}
