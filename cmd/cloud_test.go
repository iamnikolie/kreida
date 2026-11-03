package cmd_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/cmd"
	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const cloudToken = "kc_0123456789abcdefghijklmnopqrstuvwxyzABCDEFG"

// cloudFake stands in for the cloud's CLI API and records the requests as "METHOD /path body".
type cloudFake struct {
	srv *httptest.Server
	// answer overrides the default answer of a request; true means it wrote one.
	answer func(method, path string, w http.ResponseWriter) bool

	mu    sync.Mutex
	calls []string
	polls int
}

func newCloudFake(t *testing.T) *cloudFake {
	t.Helper()

	f := &cloudFake{}

	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)

		f.mu.Lock()
		f.calls = append(f.calls, r.Method+" "+r.URL.RequestURI()+" "+string(body))
		f.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")

		if f.answer != nil && f.answer(r.Method, r.URL.Path, w) {
			return
		}

		switch {
		case r.URL.Path == "/api/cli/whoami":
			if r.Header.Get("Authorization") != "Bearer "+cloudToken {
				w.WriteHeader(401)
				_, _ = io.WriteString(w, `{"status":"error","message":"bad token"}`)

				return
			}

			_, _ = io.WriteString(w, `{"email":"me@example.com","token_name":"laptop"}`)
		case r.URL.Path == "/api/cli/device":
			_, _ = io.WriteString(w, `{"device_code":"dev1","user_code":"ABCD-2345","verification_url":"`+f.srv.URL+`/owner/device?code=ABCD-2345","expires_in":600,"interval":5}`)
		case r.URL.Path == "/api/cli/device/token":
			f.mu.Lock()
			f.polls++
			n := f.polls
			f.mu.Unlock()

			if n < 3 {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":"authorization_pending"}`)

				return
			}

			_, _ = io.WriteString(w, `{"token":"`+cloudToken+`","name":"laptop"}`)
		case r.URL.Path == "/api/cli/boards":
			_, _ = io.WriteString(w, `[{"name":"demo","title":"Demo","updated_at":"x","links":[{"id":"1","url":"`+f.srv.URL+`/b/aaa","expires_at":null,"passcode":false},{"id":"2","url":"`+f.srv.URL+`/b/bbb","expires_at":"2026-12-01T00:00:00Z","passcode":true}]},{"name":"bare","title":"","updated_at":"x","links":[]}]`)
		case strings.HasSuffix(r.URL.Path, "/live"), strings.HasSuffix(r.URL.Path, "/thumb"):
			w.WriteHeader(204)
		case r.Method == "PUT":
			_, _ = io.WriteString(w, `{"name":"demo","title":"T","updated_at":"x","links":[{"id":"1","url":"`+f.srv.URL+`/b/aaa","expires_at":null,"passcode":false}]}`)
		case r.Method == "DELETE":
			w.WriteHeader(204)
		default:
			w.WriteHeader(404)
		}
	}))
	t.Cleanup(f.srv.Close)

	return f
}

// seen lists the recorded requests that start with prefix.
func (f *cloudFake) seen(prefix string) []string {
	f.mu.Lock()
	defer f.mu.Unlock()

	var out []string

	for _, c := range f.calls {
		if strings.HasPrefix(c, prefix) {
			out = append(out, c)
		}
	}

	return out
}

// cloudEnv is a scratch environment pointed at a fake cloud.
func cloudEnv(t *testing.T) (*env, *cloudFake) {
	t.Helper()

	t.Setenv(cloud.EnvURL, "")

	return newEnv(t), newCloudFake(t)
}

func (e *env) login(f *cloudFake, published ...string) {
	e.t.Helper()

	require.NoError(e.t, os.MkdirAll(e.home, 0o755))
	require.NoError(e.t, cloud.Config{URL: f.srv.URL, Token: cloudToken, Email: "me@example.com", Published: published}.Save(paths.Paths{Home: e.home, Dir: e.dir}))
}

func (e *env) cloudJSON() map[string]any {
	e.t.Helper()

	data, err := os.ReadFile(filepath.Join(e.home, "cloud.json"))
	require.NoError(e.t, err)

	var m map[string]any
	require.NoError(e.t, json.Unmarshal(data, &m))

	return m
}

func TestCloudLoginWithATokenStatusAndLogout(t *testing.T) {
	e, f := cloudEnv(t)

	assert.Equal(t, "not logged in\n", e.ok("", "cloud", "status"))
	assert.Equal(t, "not logged in\n", e.ok("", "cloud", "logout"))

	assert.Contains(t, e.fail("", "cloud", "login", "--token", cloudToken), "cloud endpoint required")
	out := e.ok("", "cloud", "login", "--token", cloudToken, "--url", f.srv.URL+"/")
	assert.Equal(t, "logged in as me@example.com ("+f.srv.URL+")\n", out)

	info, err := os.Stat(filepath.Join(e.home, "cloud.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())

	saved := e.cloudJSON()
	assert.Equal(t, f.srv.URL, saved["url"], "--url is saved, without the trailing slash")
	assert.Equal(t, cloudToken, saved["token"])
	assert.Equal(t, "me@example.com", saved["email"])
	assert.Equal(t, []any{}, saved["published"])

	t.Setenv(cloud.EnvURL, "https://other.example")
	status := e.ok("", "cloud", "status")
	assert.Equal(t, "cloud      "+f.srv.URL+"\nuser       me@example.com\npublished  (none)\n", status)
	assert.NotContains(t, status, cloudToken)
	assert.NotContains(t, out, cloudToken)

	assert.Contains(t, e.ok("", "cloud", "logout"), "logged out")
	assert.NoFileExists(t, filepath.Join(e.home, "cloud.json"))
}

func TestCloudLoginRefusesABadTokenAndABadURL(t *testing.T) {
	e, f := cloudEnv(t)

	assert.Equal(t, "a cloud token starts with kc_", e.fail("", "cloud", "login", "--token", "nope", "--url", f.srv.URL))

	msg := e.fail("", "cloud", "login", "--token", "kc_wrong", "--url", f.srv.URL)
	assert.Equal(t, "token not accepted: cloud: bad token (HTTP 401)", msg)
	assert.NotContains(t, msg, "kc_wrong")
	assert.NoFileExists(t, filepath.Join(e.home, "cloud.json"))

	assert.Contains(t, e.fail("", "cloud", "login", "--token", cloudToken, "--url", "http://kreida.example"), "must not travel in clear text")
	assert.Contains(t, e.fail("", "cloud", "login", "--token", cloudToken, "--url", "nonsense"), "bad cloud url")
	assert.Equal(t, "unknown command cloud nope (kreida help)", e.fail("", "cloud", "nope"))
}

func TestCloudLoginUsesTheEnvURLAndKeepsPublishedForTheSameOwner(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")

	t.Setenv(cloud.EnvURL, f.srv.URL)

	e.ok("", "cloud", "login", "--token", cloudToken)
	assert.Equal(t, []any{"demo"}, e.cloudJSON()["published"], "a new token of the same owner keeps the list")

	other := newCloudFake(t)
	other.answer = func(method, path string, w http.ResponseWriter) bool {
		if path == "/api/cli/whoami" {
			_, _ = io.WriteString(w, `{"email":"someone@else.example","token_name":"x"}`)

			return true
		}

		return false
	}

	e.ok("", "cloud", "login", "--token", cloudToken, "--url", other.srv.URL)
	assert.Equal(t, []any{}, e.cloudJSON()["published"], "another owner's login starts with no boards")
	assert.Equal(t, other.srv.URL, e.cloudJSON()["url"])
}

// runWith runs a command line with a device-login wait that returns at once.
func (e *env) runWith(ctx context.Context, wait cloud.Waiter, stdin string, args ...string) (string, string, error) {
	e.t.Helper()

	var out, errOut lockedBuffer

	app := &cmd.App{Out: &out, Err: &errOut, In: strings.NewReader(stdin), Dist: testDist, Wait: wait, Thumb: e.thumbs()}
	root := cmd.NewRoot(app)
	root.SetArgs(args)

	err := root.ExecuteContext(ctx)

	return out.String(), errOut.String(), err
}

type lockedBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *lockedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.Write(p)
}

func (l *lockedBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.b.String()
}

func instant(ctx context.Context, d time.Duration) error { return ctx.Err() }

func TestCloudLoginByDeviceCode(t *testing.T) {
	e, f := cloudEnv(t)

	out, errOut, err := e.runWith(context.Background(), instant, "", "cloud", "login", "--url", f.srv.URL)
	require.NoError(t, err)

	assert.Equal(t, "open  "+f.srv.URL+"/owner/device?code=ABCD-2345\ncode  ABCD-2345\nlogged in as me@example.com ("+f.srv.URL+")\n", out)
	assert.Contains(t, errOut, "waiting for the approval")
	assert.NotContains(t, out+errOut, cloudToken)

	assert.Equal(t, cloudToken, e.cloudJSON()["token"])
	assert.Equal(t, "me@example.com", e.cloudJSON()["email"])
	assert.Len(t, f.seen("POST /api/cli/device/token"), 3, "two pending polls, then the token")
	assert.Contains(t, f.seen("POST /api/cli/device ")[0], `"client":`)
}

func TestCloudLoginByDeviceCodeDeniedKeepsNothing(t *testing.T) {
	e, f := cloudEnv(t)
	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if path == "/api/cli/device/token" {
			w.WriteHeader(400)
			_, _ = io.WriteString(w, `{"error":"access_denied"}`)

			return true
		}

		return false
	}

	_, _, err := e.runWith(context.Background(), instant, "", "cloud", "login", "--url", f.srv.URL)
	assert.EqualError(t, err, "login denied")
	assert.NoFileExists(t, filepath.Join(e.home, "cloud.json"))
}

func TestPublishUploadsWithoutSourcesAndPrintsTheLinks(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)

	e.ok("title: Demo\nnodes:\n  a: { title: A, live: { cmd: \"echo 7\" } }\n  b: { title: B, live: { cmd: \"echo hidden\" } }\n", "set", "demo")
	e.ok("", "live", "demo", "--allow", "a")

	out := e.ok("", "publish", "demo")
	assert.Equal(t, f.srv.URL+"/b/aaa\n", out)

	puts := f.seen("PUT /api/cli/boards/demo ")
	require.Len(t, puts, 1)
	assert.NotContains(t, puts[0], "echo")
	assert.Contains(t, puts[0], `"title":"Demo"`)
	assert.Contains(t, puts[0], `"source":""`)

	lives := f.seen("PUT /api/cli/boards/demo/live")
	require.Len(t, lives, 1)
	assert.Contains(t, lives[0], `"a":{"value":"7","error":"","blocked":false,"at":"`)
	assert.Contains(t, lives[0], `"b":{"value":"","error":"","blocked":true,"at":""}`, "a blocked card is reported, never run")
	assert.NotContains(t, lives[0], "echo")

	assert.Equal(t, []any{"demo"}, e.cloudJSON()["published"])
	assert.Equal(t, cloudToken, e.cloudJSON()["token"], "the login stays")
}

func TestPublishUploadsAThumbnailAfterTheLinks(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)
	e.ok("title: Demo\n", "set", "demo")

	var drawn []string

	e.thumb = func(ctx context.Context, name string) ([]byte, error) {
		drawn = append(drawn, name)

		return []byte("\x89PNG\r\n\x1a\ndemo"), nil
	}

	out, errOut, err := e.run("", "publish", "demo")
	require.NoError(t, err)
	assert.Equal(t, f.srv.URL+"/b/aaa\n", out)
	assert.Empty(t, errOut)
	assert.Equal(t, []string{"demo"}, drawn)

	thumbs := f.seen("PUT /api/cli/boards/demo/thumb")
	require.Len(t, thumbs, 1)
	assert.Equal(t, "PUT /api/cli/boards/demo/thumb \x89PNG\r\n\x1a\ndemo", thumbs[0])
	assert.NotContains(t, thumbs[0], cloudToken)
}

func TestPublishWithoutAThumbnailWarnsOnceAndStillPublishes(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)
	e.ok("title: Demo\n", "set", "demo")

	// no Chrome on the machine
	e.thumb = func(context.Context, string) ([]byte, error) { return nil, render.ErrNoChrome }

	out, errOut, err := e.run("", "publish", "demo")
	require.NoError(t, err)
	assert.Equal(t, f.srv.URL+"/b/aaa\n", out)
	assert.Equal(t, "kreida: thumbnail not uploaded: no Chrome found (macOS app or google-chrome/chromium on PATH)\n", errOut)
	assert.Empty(t, f.seen("PUT /api/cli/boards/demo/thumb"))
	assert.Equal(t, []any{"demo"}, e.cloudJSON()["published"])

	// a picture that is not one: refused here, never sent
	e.thumb = func(context.Context, string) ([]byte, error) { return []byte("<html>"), nil }

	_, errOut, err = e.run("", "publish", "demo")
	require.NoError(t, err)
	assert.Equal(t, "kreida: thumbnail not uploaded: cloud.PutThumb: not a PNG\n", errOut)
	assert.Empty(t, f.seen("PUT /api/cli/boards/demo/thumb"))

	// the cloud does not take it
	e.thumb = nil
	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if strings.HasSuffix(path, "/thumb") {
			w.WriteHeader(413)
			_, _ = io.WriteString(w, `{"status":"error","message":"thumbnail is too large"}`)

			return true
		}

		return false
	}

	out, errOut, err = e.run("", "publish", "demo")
	require.NoError(t, err)
	assert.Equal(t, f.srv.URL+"/b/aaa\n", out)
	assert.Equal(t, "kreida: thumbnail not uploaded: cloud: thumbnail is too large (HTTP 413)\n", errOut)

	// an unpublished board in the cloud: the thumbnail's 404 is a note too
	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if strings.HasSuffix(path, "/thumb") {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"status":"error","message":"not published"}`)

			return true
		}

		return false
	}

	_, errOut, err = e.run("", "publish", "demo")
	require.NoError(t, err)
	assert.Equal(t, "kreida: thumbnail not uploaded: cloud: not published (HTTP 404)\n", errOut)
}

func TestPublishNotesPasscodeAndExpiry(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)
	e.ok("title: Demo\n", "set", "demo")

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if method == "PUT" && !strings.HasSuffix(path, "/live") {
			_, _ = io.WriteString(w, `{"name":"demo","links":[{"id":"1","url":"https://c.example/b/x","expires_at":"2026-12-01T00:00:00Z","passcode":true},{"id":"2","url":"https://c.example/b/y","expires_at":null,"passcode":true}]}`)

			return true
		}

		return false
	}

	assert.Equal(t, "https://c.example/b/x (passcode, expires 2026-12-01T00:00:00Z)\nhttps://c.example/b/y (passcode)\n", e.ok("", "publish", "demo"))
}

func TestPublishNeedsALoginABoardAndAnAcceptedToken(t *testing.T) {
	e, f := cloudEnv(t)
	e.ok("title: Demo\n", "set", "demo")

	assert.Equal(t, "requires login to your own configured compatible server", e.fail("", "publish", "demo"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "publish", "nope"))
	assert.Equal(t, "bad board name: (none)", e.fail("", "publish"))

	e.login(f)
	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"status":"error","message":"token revoked"}`)

		return true
	}

	assert.Equal(t, "cloud: token revoked (HTTP 401) (sign in again to your configured server)", e.fail("", "publish", "demo"))
	assert.Equal(t, []any{}, e.cloudJSON()["published"], "a refused publish is not recorded")
}

func TestPublishSurvivesALiveFailureAndReportsIt(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)
	e.ok("title: Demo\n", "set", "demo")

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if strings.HasSuffix(path, "/live") {
			w.WriteHeader(500)
			_, _ = io.WriteString(w, `{"status":"error","message":"boom"}`)

			return true
		}

		return false
	}

	out, errOut, err := e.run("", "publish", "demo")
	require.NoError(t, err)
	assert.Equal(t, f.srv.URL+"/b/aaa\n", out)
	assert.Equal(t, "kreida: live values not pushed: cloud: boom (HTTP 500)\n", errOut)
	assert.Equal(t, []any{"demo"}, e.cloudJSON()["published"])
}

func TestUnpublishAndPublished(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo", "keep")

	assert.Equal(t, "demo                     "+f.srv.URL+"/b/aaa  "+f.srv.URL+"/b/bbb (passcode, expires 2026-12-01T00:00:00Z)\n"+
		"bare                     (no links)\n", e.ok("", "published"))

	assert.Equal(t, "unpublished\n", e.ok("", "unpublish", "demo"))
	assert.Len(t, f.seen("DELETE /api/cli/boards/demo"), 1)
	assert.Equal(t, []any{"keep"}, e.cloudJSON()["published"])

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if method == "DELETE" {
			w.WriteHeader(404)
			_, _ = io.WriteString(w, `{"status":"error","message":"not published"}`)

			return true
		}

		return false
	}

	assert.Equal(t, "not published\n", e.ok("", "unpublish", "keep"), "gone up there is gone here too")
	assert.Equal(t, []any{}, e.cloudJSON()["published"])

	assert.Equal(t, "bad board name: bad name", e.fail("", "unpublish", "bad name"))
	assert.Equal(t, "bad board name: ..", e.fail("", "unpublish", ".."))

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"status":"error","message":"boom"}`)

		return true
	}

	e.login(f, "demo")
	assert.Equal(t, "cloud: boom (HTTP 500)", e.fail("", "unpublish", "demo"))
	assert.Equal(t, []any{"demo"}, e.cloudJSON()["published"], "a failed unpublish keeps the board listed")
}

func TestPublishedAndUnpublishNeedALogin(t *testing.T) {
	e, _ := cloudEnv(t)

	assert.Equal(t, "requires login to your own configured compatible server", e.fail("", "published"))
	assert.Equal(t, "requires login to your own configured compatible server", e.fail("", "unpublish", "demo"))

	e, f := cloudEnv(t)
	e.login(f)
	f.answer = func(method, path string, w http.ResponseWriter) bool {
		_, _ = io.WriteString(w, `[]`)

		return true
	}

	assert.Equal(t, "nothing published\n", e.ok("", "published"))
}

func TestPublishWatchPushesEveryChangeAndStopsOnCancel(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)
	e.ok("title: One\n", "set", "demo")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	type result struct {
		out, errOut string
		err         error
	}

	done := make(chan result, 1)

	go func() {
		out, errOut, err := e.runWith(ctx, nil, "", "publish", "demo", "--watch")
		done <- result{out, errOut, err}
	}()

	waitFor(t, "the first upload", func() bool { return len(f.seen("PUT /api/cli/boards/demo ")) == 1 })
	time.Sleep(500 * time.Millisecond) // the watcher is up

	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "demo.yaml"), []byte("title: Two\n"), 0o644))
	waitFor(t, "the upload of the edit", func() bool { return len(f.seen("PUT /api/cli/boards/demo ")) == 2 })
	assert.Contains(t, f.seen("PUT /api/cli/boards/demo ")[1], `"title":"Two"`)

	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "demo.pos.json"), []byte(`{"a":{"x":1,"y":2}}`), 0o644))
	waitFor(t, "the upload of the positions", func() bool { return len(f.seen("PUT /api/cli/boards/demo ")) == 3 })
	assert.Contains(t, f.seen("PUT /api/cli/boards/demo ")[2], `"pos":{"a":{"x":1,"y":2}}`)

	// another board's files are none of its business
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "other.yaml"), []byte("title: x\n"), 0o644))
	time.Sleep(600 * time.Millisecond)
	assert.Len(t, f.seen("PUT /api/cli/boards/demo "), 3)
	assert.Len(t, f.seen("PUT /api/cli/boards/demo/thumb"), 1, "the thumbnail of the publish; the edits are inside its five minutes")
	assert.Empty(t, f.seen("PUT /api/cli/boards/other"))

	cancel()

	select {
	case r := <-done:
		require.NoError(t, r.err, "Ctrl-C ends the watch cleanly")
		assert.Equal(t, f.srv.URL+"/b/aaa\n", r.out)
		assert.Contains(t, r.errOut, "watching demo; Ctrl-C to stop")
		assert.NotContains(t, r.errOut, cloudToken)
	case <-time.After(5 * time.Second):
		t.Fatal("the watch did not stop")
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for %s", what)
}

func TestUnconfiguredCloudHelp(t *testing.T) {
	e, _ := cloudEnv(t)

	help := e.ok("", "help")
	for _, command := range []string{"cloud", "publish", "published", "unpublish"} {
		assert.NotContains(t, help, "kreida "+command)
	}
	loginHelp := e.ok("", "cloud", "login", "--help")
	assert.Contains(t, loginHelp, "your own compatible server")
	assert.Contains(t, loginHelp, "required on first login")
	assert.NotContains(t, loginHelp, "Log in to kreida cloud")
}
