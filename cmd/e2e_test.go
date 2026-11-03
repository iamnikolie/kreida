package cmd_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// bin builds the real binary once per test that needs it.
func bin(t *testing.T) string {
	t.Helper()

	out := filepath.Join(t.TempDir(), "kreida")
	b := exec.Command("go", "build", "-o", out, "..")
	b.Stderr = os.Stderr
	require.NoError(t, b.Run())

	return out
}

func freePort(t *testing.T) int {
	t.Helper()

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	defer ln.Close()

	return ln.Addr().(*net.TCPAddr).Port
}

func get(t *testing.T, url string) (int, string) {
	t.Helper()

	resp, err := http.Get(url)
	require.NoError(t, err)

	defer resp.Body.Close()

	b, _ := io.ReadAll(resp.Body)

	return resp.StatusCode, string(b)
}

// TestBinaryEndToEnd starts the real binary as a background server on a random port in a scratch home (never port
// 7788, never ~/.kreida) and drives it through the CLI.
func TestBinaryEndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and starts the binary")
	}

	exe := bin(t)
	home := t.TempDir()
	port := freePort(t)
	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	kreida := func(stdin string, args ...string) (string, error) {
		c := exec.Command(exe, args...)
		c.Env = append(os.Environ(), "KREIDA_HOME="+home, "KREIDA_DIR=", fmt.Sprintf("KREIDA_PORT=%d", port), "KREIDA_LAN=")
		c.Stdin = strings.NewReader(stdin)

		out, err := c.CombinedOutput()

		return string(out), err
	}

	t.Cleanup(func() { _, _ = kreida("", "down") })

	out, err := kreida("", "down")
	require.NoError(t, err)
	assert.Equal(t, "not running\n", out)

	_, err = kreida("title: Shared board\nnodes:\n  a: { title: A, live: { cmd: \"echo 7\" } }\n", "set", "pub")
	require.NoError(t, err)

	out, err = kreida("", "up")
	require.NoError(t, err, out)
	assert.Equal(t, base+"\n", out)

	pid, err := os.ReadFile(filepath.Join(home, "server.pid"))
	require.NoError(t, err)
	assert.NotEmpty(t, strings.TrimSpace(string(pid)))

	code, body := get(t, base+"/api/health")
	assert.Equal(t, 200, code)

	var health map[string]any
	require.NoError(t, json.Unmarshal([]byte(body), &health))
	assert.Equal(t, true, health["ok"])
	assert.Equal(t, filepath.Join(home, "boards"), health["dir"])
	assert.Equal(t, false, health["lan"])

	log, _ := os.ReadFile(filepath.Join(home, "server.log"))
	assert.Contains(t, string(log), fmt.Sprintf("kreida on :%d  boards: %s", port, filepath.Join(home, "boards")))

	code, body = get(t, base+"/api/boards/pub")
	assert.Equal(t, 200, code)
	assert.Contains(t, body, `"title":"Shared board"`)

	// a second `up` finds the server and leaves it alone
	out, err = kreida("", "up")
	require.NoError(t, err)
	assert.Equal(t, base+"\n", out)

	pid2, _ := os.ReadFile(filepath.Join(home, "server.pid"))
	assert.Equal(t, string(pid), string(pid2))

	// a board written by the CLI is seen by the running server and snapshotted by its watcher
	_, err = kreida("title: Shared board 2\n", "set", "pub")
	require.NoError(t, err)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, hist := get(t, base+"/api/boards/pub/history"); strings.Count(hist, `"id"`) >= 2 {
			break
		}

		time.Sleep(50 * time.Millisecond)
	}

	_, hist := get(t, base+"/api/boards/pub/history")
	assert.GreaterOrEqual(t, strings.Count(hist, `"id"`), 2, hist)

	// a share link
	out, err = kreida("", "share", "pub")
	require.NoError(t, err, out)

	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.Len(t, lines, 2, out)
	assert.True(t, strings.HasPrefix(lines[0], base+"/s/"), lines[0])
	assert.Equal(t, "(this machine only; kreida share <name> --lan to reach it from the local network)", lines[1])

	token := strings.TrimPrefix(lines[0], base+"/s/")
	code, body = get(t, base+"/api/share/"+token)
	assert.Equal(t, 200, code)
	assert.Contains(t, body, "Shared board 2")

	out, err = kreida("", "shares")
	require.NoError(t, err)
	assert.Contains(t, out, "pub                      /s/"+token)

	out, err = kreida("", "unshare", "pub")
	require.NoError(t, err)
	assert.Equal(t, "revoked\n", out)

	code, _ = get(t, base+"/api/share/"+token)
	assert.Equal(t, 404, code)

	// `up --lan` restarts the server in LAN mode and plain `up` goes back to local only
	out, err = kreida("", "up", "--lan")
	require.NoError(t, err, out)
	assert.True(t, strings.HasPrefix(out, base+"\nLAN: share links only — "), out)

	_, body = get(t, base+"/api/health")
	assert.Contains(t, body, `"lan":true`)

	pid3, _ := os.ReadFile(filepath.Join(home, "server.pid"))
	assert.NotEqual(t, string(pid), string(pid3), "a server in the other mode is replaced")

	out, err = kreida("", "up")
	require.NoError(t, err, out)
	assert.Equal(t, base+"\n", out)

	_, body = get(t, base+"/api/health")
	assert.Contains(t, body, `"lan":false`)

	out, err = kreida("", "down")
	require.NoError(t, err)
	assert.Equal(t, "stopped\n", out)

	deadline = time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := http.Get(base + "/api/health"); err != nil {
			return
		}

		time.Sleep(50 * time.Millisecond)
	}

	t.Fatal("the server kept running after `down`")
}

// TestBinaryPublishesToTheCloudAndTheServerKeepsItInSync drives the real binary against a fake cloud: login, publish,
// then an edit that the background server pushes by itself, and no more pushes after unpublish.
func TestBinaryPublishesToTheCloudAndTheServerKeepsItInSync(t *testing.T) {
	if testing.Short() {
		t.Skip("builds and starts the binary")
	}

	exe := bin(t)
	home := t.TempDir()
	port := freePort(t)
	f := newCloudFake(t)

	// a Chrome that only writes a PNG-looking file, so the thumbnails go the whole way without a real browser
	chrome := filepath.Join(t.TempDir(), "chrome")
	require.NoError(t, os.WriteFile(chrome, []byte("#!/bin/sh\nfor a in \"$@\"; do case \"$a\" in --screenshot=*) printf '\\211PNG\\r\\n\\032\\nfake' > \"${a#--screenshot=}\";; esac; done\n"), 0o755))

	kreida := func(stdin string, args ...string) (string, error) {
		c := exec.Command(exe, args...)
		c.Env = append(os.Environ(), "KREIDA_HOME="+home, "KREIDA_DIR=", fmt.Sprintf("KREIDA_PORT=%d", port), "KREIDA_LAN=", "KREIDA_CLOUD_URL=", "KREIDA_CHROME="+chrome)
		c.Stdin = strings.NewReader(stdin)

		out, err := c.CombinedOutput()

		return string(out), err
	}

	t.Cleanup(func() { _, _ = kreida("", "down") })

	out, err := kreida("", "cloud", "login", "--token", cloudToken, "--url", f.srv.URL)
	require.NoError(t, err, out)
	assert.Equal(t, "logged in as me@example.com ("+f.srv.URL+")\n", out)

	_, err = kreida("title: First\nnodes:\n  a: { title: A, live: { cmd: \"echo 7\" } }\n", "set", "pub")
	require.NoError(t, err)

	_, err = kreida("", "live", "pub", "--allow")
	require.NoError(t, err)

	out, err = kreida("", "up")
	require.NoError(t, err, out)

	out, err = kreida("", "publish", "pub")
	require.NoError(t, err, out)
	assert.Equal(t, f.srv.URL+"/b/aaa\n", out)
	require.Len(t, f.seen("PUT /api/cli/boards/pub "), 1)
	assert.Equal(t, []string{"PUT /api/cli/boards/pub/thumb \x89PNG\r\n\x1a\nfake"}, f.seen("PUT /api/cli/boards/pub/thumb"), "the publish draws and uploads the thumbnail")

	info, err := os.Stat(filepath.Join(home, "cloud.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	// the running server, a separate process, pushes the edit
	_, err = kreida("title: Second\nnodes:\n  a: { title: A, live: { cmd: \"echo 7\" } }\n", "set", "pub")
	require.NoError(t, err)

	waitFor(t, "the server's push of the edit", func() bool {
		puts := f.seen("PUT /api/cli/boards/pub ")

		return len(puts) >= 2 && strings.Contains(puts[len(puts)-1], `"title":"Second"`)
	})

	waitFor(t, "the server's thumbnail of the edit", func() bool { return len(f.seen("PUT /api/cli/boards/pub/thumb")) >= 2 })

	for _, p := range f.seen("PUT /api/cli/boards/pub ") {
		assert.NotContains(t, p, "echo 7", "the command never leaves")
	}

	for _, l := range f.seen("PUT /api/cli/boards/pub/live") {
		assert.NotContains(t, l, "echo")
	}

	out, err = kreida("", "unpublish", "pub")
	require.NoError(t, err, out)
	assert.Equal(t, "unpublished\n", out)

	n := len(f.seen("PUT /api/cli/boards/pub "))

	_, err = kreida("title: Third\n", "set", "pub")
	require.NoError(t, err)
	time.Sleep(1500 * time.Millisecond)
	assert.Len(t, f.seen("PUT /api/cli/boards/pub "), n, "an unpublished board is not pushed")

	log, _ := os.ReadFile(filepath.Join(home, "server.log"))
	assert.NotContains(t, string(log), cloudToken)
}
