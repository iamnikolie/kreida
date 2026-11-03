package live_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setup(t *testing.T) (*live.Runner, paths.Paths) {
	t.Helper()

	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}
	require.NoError(t, p.Ensure())

	return live.New(p), p
}

func put(t *testing.T, p paths.Paths, yaml string) {
	t.Helper()
	require.NoError(t, os.WriteFile(p.BoardFile("b"), []byte(yaml), 0o644))
}

func str(s *string) string {
	if s == nil {
		return "<nil>"
	}

	return *s
}

func TestACommandRunsOnlyAfterItIsAllowed(t *testing.T) {
	r, p := setup(t)
	marker := filepath.Join(p.Home, "ran")

	put(t, p, fmt.Sprintf("nodes:\n  a: { live: { cmd: \"touch %s; echo 42\" } }\n", marker))

	vs, err := r.Values("b")
	require.NoError(t, err)

	a, _ := vs["a"], true
	assert.Equal(t, live.Value{Blocked: true, Source: "cmd:touch " + marker + "; echo 42"}, a)
	assert.NoFileExists(t, marker)

	added, err := r.Allow("b", "")
	require.NoError(t, err)
	assert.Len(t, added, 1)

	vs, _ = r.Values("b")
	a, _ = vs["a"], true
	assert.Equal(t, "42", str(a.Value))
	assert.FileExists(t, marker)

	put(t, p, "nodes:\n  a: { live: { cmd: \"echo changed\" } }\n")

	srcs, err := r.Sources("b")
	require.NoError(t, err)
	assert.False(t, srcs[0].Allowed, "an edited command is not the one that was allowed")

	vs, _ = r.Values("b")
	a, _ = vs["a"], true
	assert.True(t, a.Blocked)

	n, err := r.Revoke("b")
	require.NoError(t, err)
	assert.GreaterOrEqual(t, n, 1)
}

func TestURLWithAJSONPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"count":17,"items":["x","y"],"obj":{"b":1,"a":2}}}`))
	}))
	t.Cleanup(srv.Close)

	r, p := setup(t)

	for path, want := range map[string]string{"data.count": "17", "data.items.1": "y", "data.nope": "", "data.obj": `{"a":2,"b":1}`, "": `{"data":{"count":17,"items":["x","y"],"obj":{"a":2,"b":1}}}`} {
		put(t, p, fmt.Sprintf("nodes:\n  a: { live: { url: \"%s/x\", path: \"%s\" } }\n", srv.URL, path))
		_, err := r.Allow("b", "")
		require.NoError(t, err)

		r2 := live.New(p) // a fresh cache for each path
		vs, err := r2.Values("b")
		require.NoError(t, err)

		a, _ := vs["a"], true
		assert.Equal(t, want, str(a.Value), "path %q", path)
		assert.Nil(t, a.Error)
	}
}

func TestURLErrorsAndPlainText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/500":
			http.Error(w, "boom", http.StatusInternalServerError)
		case "/text":
			_, _ = w.Write([]byte("  ok\nsecond line\n"))
		case "/str":
			_, _ = w.Write([]byte(`"just a string"`))
		}
	}))
	t.Cleanup(srv.Close)

	r, p := setup(t)

	put(t, p, fmt.Sprintf("nodes:\n  a: { live: { url: \"%s/500\" } }\n  b: { live: { url: \"%s/text\" } }\n  c: { live: { url: \"%s/str\" } }\n  d: { live: { url: \"http://127.0.0.1:1/x\" } }\n", srv.URL, srv.URL, srv.URL))
	_, err := r.Allow("b", "")
	require.NoError(t, err)

	vs, err := r.Values("b")
	require.NoError(t, err)

	a, _ := vs["a"], true
	assert.Equal(t, "HTTP 500", str(a.Error))
	assert.Nil(t, a.Value)

	b, _ := vs["b"], true
	assert.Equal(t, "ok", str(b.Value), "first line, trimmed")

	c, _ := vs["c"], true
	assert.Equal(t, "just a string", str(c.Value))

	d, _ := vs["d"], true
	assert.NotNil(t, d.Error, "a refused connection is an error value, not a failure of the call")
}

func TestCommandOutputIsOneShortLine(t *testing.T) {
	r, p := setup(t)

	long := strings.Repeat("я", 300)
	put(t, p, fmt.Sprintf("nodes:\n  a: { live: { cmd: \"printf '\\\\n  %s\\\\nsecond\\\\n'\" } }\n  b: { live: { cmd: \"echo oops >&2; exit 3\" } }\n  c: { live: { cmd: \"exit 4\" } }\n  d: { live: { cmd: \"true\" } }\n", long))
	_, err := r.Allow("b", "")
	require.NoError(t, err)

	vs, err := r.Values("b")
	require.NoError(t, err)

	a, _ := vs["a"], true
	assert.Equal(t, strings.Repeat("я", 200), str(a.Value))

	b, _ := vs["b"], true
	assert.Equal(t, "oops", str(b.Error))
	assert.Nil(t, b.Value)

	c, _ := vs["c"], true
	assert.Equal(t, "Command failed: exit 4", str(c.Error))

	d, _ := vs["d"], true
	require.NotNil(t, d.Value)
	assert.Equal(t, "", *d.Value, "an empty output is an empty value")
}

func TestCommandRunsInTheStateDirectory(t *testing.T) {
	r, p := setup(t)

	put(t, p, "nodes:\n  a: { live: { cmd: pwd } }\n")
	_, err := r.Allow("b", "")
	require.NoError(t, err)

	vs, _ := r.Values("b")
	a, _ := vs["a"], true

	want, _ := filepath.EvalSymlinks(p.Home)
	got, _ := filepath.EvalSymlinks(str(a.Value))
	assert.Equal(t, want, got)
}

func TestCommandIsKilledAfterTheTimeout(t *testing.T) {
	r, p := setup(t)
	r.Timeout = 300 * time.Millisecond

	put(t, p, "nodes:\n  a: { live: { cmd: \"sleep 30\" } }\n")
	_, err := r.Allow("b", "")
	require.NoError(t, err)

	start := time.Now()
	vs, _ := r.Values("b")
	a, _ := vs["a"], true

	assert.Equal(t, "Command failed: sleep 30", str(a.Error))
	assert.Less(t, time.Since(start), 5*time.Second)
}

func TestOutputBeyondTheBufferIsAnError(t *testing.T) {
	r, p := setup(t)

	put(t, p, "nodes:\n  a: { live: { cmd: \"yes | head -c 200000\" } }\n")
	_, err := r.Allow("b", "")
	require.NoError(t, err)

	vs, _ := r.Values("b")
	a, _ := vs["a"], true
	assert.Equal(t, "stdout maxBuffer length exceeded", str(a.Error))
}

// A value is fresh for the card's `every` seconds, and viewers asking meanwhile share one run.
func TestValuesAreCachedPerCard(t *testing.T) {
	r, p := setup(t)

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	var mu sync.Mutex

	r.Now = func() time.Time {
		mu.Lock()
		defer mu.Unlock()

		return now
	}
	advance := func(d time.Duration) {
		mu.Lock()
		defer mu.Unlock()

		now = now.Add(d)
	}

	counter := filepath.Join(p.Home, "count")
	put(t, p, fmt.Sprintf("nodes:\n  a: { live: { cmd: \"echo x >> %s; wc -l < %s\", every: 10 } }\n", counter, counter))
	_, err := r.Allow("b", "")
	require.NoError(t, err)

	value := func() string {
		vs, err := r.Values("b")
		require.NoError(t, err)

		a, _ := vs["a"], true

		return str(a.Value)
	}

	var wg sync.WaitGroup

	var runs atomic.Int32

	for i := 0; i < 5; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			if value() == "1" {
				runs.Add(1)
			}
		}()
	}

	wg.Wait()
	assert.EqualValues(t, 5, runs.Load(), "five viewers, one run")

	advance(10 * time.Second)
	assert.Equal(t, "1", value(), "still fresh at exactly `every` seconds")

	advance(time.Second)
	assert.Equal(t, "2", value(), "stale after `every` seconds")

	vs, _ := r.Values("b")
	a, _ := vs["a"], true
	assert.Equal(t, "2026-01-01T00:00:11.000Z", a.At)
	assert.EqualValues(t, 10, a.Every)
}

func TestAllowOneNodeAndTheJSONOfAValueSet(t *testing.T) {
	r, p := setup(t)

	put(t, p, "nodes:\n  b2: { live: { cmd: echo two } }\n  b1: { live: { cmd: echo one } }\n  7: { live: { cmd: echo seven } }\n")

	added, err := r.Allow("b", "b1")
	require.NoError(t, err)
	require.Len(t, added, 1)
	assert.Equal(t, "b1", added[0].Node)

	again, err := r.Allow("b", "b1")
	require.NoError(t, err)
	assert.Empty(t, again, "nothing new to allow")

	vs, err := r.Values("b")
	require.NoError(t, err)

	raw, err := json.Marshal(vs)
	require.NoError(t, err)
	assert.Contains(t, string(raw), `"7":{"blocked":true,"source":"cmd:echo seven"}`)

	raw, err = json.Marshal(vs.WithoutSource())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "echo", "a share link sees values, never the commands")
	assert.Contains(t, string(raw), `"b2":{"blocked":true}`)

	data, err := os.ReadFile(p.AllowFile())
	require.NoError(t, err)
	assert.Contains(t, string(data), "\"source\": \"cmd:echo one\"")

	st, _ := os.Stat(p.AllowFile())
	assert.Equal(t, os.FileMode(0o600), st.Mode().Perm())
}

func TestAnInvalidBoardIsAnError(t *testing.T) {
	r, p := setup(t)

	put(t, p, "nodes:\n  a: { live: { cmd: x, url: \"http://x\" } }\n")

	_, err := r.Values("b")
	assert.ErrorContains(t, err, "exactly one")

	_, err = r.Sources("missing")
	assert.Error(t, err)
}
