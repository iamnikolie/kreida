package cloud_test

import (
	"bytes"
	"context"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// syncBuf is a log buffer the Syncer's goroutines and the test can both touch.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.b.String()
}

// newSyncer returns a Syncer with a short debounce over a scratch home. It is closed when the test ends.
func newSyncer(t *testing.T, p paths.Paths) (*cloud.Syncer, *syncBuf) {
	t.Helper()

	logs := &syncBuf{}
	s := cloud.NewSyncer(p, live.New(p), slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug})))
	s.Debounce, s.Tick, s.Backoff = 20*time.Millisecond, 10*time.Millisecond, 20*time.Millisecond

	t.Cleanup(s.Close)

	return s, logs
}

func writeBoard(t *testing.T, p paths.Paths, name, text string) {
	t.Helper()
	require.NoError(t, os.WriteFile(p.BoardFile(name), []byte(text), 0o644))
}

func TestChangedPushesAPublishedBoardOnceAfterTheQuietMoment(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	s, logs := newSyncer(t, p)

	for range 5 {
		s.Changed("demo")
	}

	eventually(t, "the push", func() bool { return len(f.puts("demo")) == 1 })
	never(t, "a second push", func() bool { return len(f.puts("demo")) > 1 })

	assert.Contains(t, f.puts("demo")[0], `"title":"One"`)
	assert.Equal(t, "Bearer "+token, f.all()[0].Auth)
	assert.Contains(t, logs.String(), "cloud push")
	assert.NotContains(t, logs.String(), token, "the token is never logged")

	// the same content again (a touched file, a rewritten position file) is not worth a call
	s.Changed("demo")
	never(t, "a push of the same body", func() bool { return len(f.puts("demo")) > 1 })

	writeBoard(t, p, "demo", "title: Two\n")
	s.Changed("demo")
	eventually(t, "the push of the new body", func() bool { return len(f.puts("demo")) == 2 })
	assert.Contains(t, f.puts("demo")[1], `"title":"Two"`)
}

func TestChangedIgnoresBoardsThatAreNotPublishedAndLoginsThatAreOff(t *testing.T) {
	f, p := newFake(t), scratch(t)
	writeBoard(t, p, "demo", "title: One\n")
	writeBoard(t, p, "other", "title: Other\n")

	s, _ := newSyncer(t, p)

	s.Changed("demo") // no cloud.json at all
	never(t, "a push without a login", func() bool { return len(f.all()) > 0 })

	require.NoError(t, cloud.Config{URL: f.srv.URL, Published: []string{"demo"}}.Save(p))
	s.Changed("demo") // a login without a token
	never(t, "a push without a token", func() bool { return len(f.all()) > 0 })

	login(t, p, f, "demo")
	s.Changed("other") // logged in, but this board is not published
	s.Changed("../x")
	s.Changed("")
	never(t, "a push of a board that is not published", func() bool { return len(f.all()) > 0 })
}

func TestTheLoginIsReadOnEveryPush(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f)
	writeBoard(t, p, "demo", "title: One\n")

	s, _ := newSyncer(t, p)

	s.Changed("demo")
	never(t, "a push before publish", func() bool { return len(f.all()) > 0 })

	// another process publishes the board
	login(t, p, f, "demo")
	s.Changed("demo")
	eventually(t, "the push after publish", func() bool { return len(f.puts("demo")) == 1 })

	// ... and unpublishes it again
	login(t, p, f)
	writeBoard(t, p, "demo", "title: Two\n")
	s.Changed("demo")
	never(t, "a push after unpublish", func() bool { return len(f.puts("demo")) > 1 })

	// ... and logs out
	require.NoError(t, os.Remove(p.CloudFile()))
	login(t, p, f, "demo")
	require.NoError(t, os.Remove(p.CloudFile()))
	s.Changed("demo")
	never(t, "a push after logout", func() bool { return len(f.puts("demo")) > 1 })
}

func TestAFailedPushIsLoggedAndTriedAgainAfterRetryAfter(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	var tries atomic.Int32

	var first atomic.Int64

	f.respond = func(c call, w http.ResponseWriter) bool {
		if c.Method != "PUT" || strings.HasSuffix(c.Path, "/live") {
			return false
		}

		if tries.Add(1) == 1 {
			first.Store(time.Now().UnixNano())
			w.Header().Set("Retry-After", "1")
			reply(w, 429, `{"status":"error","message":"slow down"}`)

			return true
		}

		return false
	}

	s, logs := newSyncer(t, p)
	s.Changed("demo")

	eventually(t, "the retry", func() bool { return tries.Load() == 2 })
	assert.GreaterOrEqual(t, time.Since(time.Unix(0, first.Load())), 900*time.Millisecond, "Retry-After is honoured")
	assert.Contains(t, logs.String(), "slow down")
	assert.Contains(t, logs.String(), "level=WARN")
}

func TestAServerErrorIsRetriedAndAClientErrorIsNot(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo", "big")
	writeBoard(t, p, "demo", "title: One\n")
	writeBoard(t, p, "big", "title: Big\n")

	var demo atomic.Int32

	f.respond = func(c call, w http.ResponseWriter) bool {
		switch c.Path {
		case "/api/cli/boards/demo":
			if demo.Add(1) < 3 {
				reply(w, 503, `{"status":"error","message":"down"}`)

				return true
			}
		case "/api/cli/boards/big":
			reply(w, 413, `{"status":"error","message":"board is too big"}`)

			return true
		}

		return false
	}

	s, logs := newSyncer(t, p)
	s.Changed("demo")
	s.Changed("big")

	eventually(t, "demo to succeed on the third try", func() bool { return demo.Load() == 3 })
	never(t, "a retry of the too big board", func() bool { return len(f.puts("big")) > 1 })
	assert.Contains(t, logs.String(), "board is too big")
}

func TestABoardThatDoesNotParseIsLoggedNotPushed(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "nodes: { a: { colour: red } }\n")

	s, logs := newSyncer(t, p)
	s.Changed("demo")

	eventually(t, "the warning", func() bool { return strings.Contains(logs.String(), "invalid board") })
	never(t, "a push", func() bool { return len(f.all()) > 0 })
}

func TestStartBringsPublishedBoardsUpToDate(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "a", "b")
	writeBoard(t, p, "a", "title: A\n")
	writeBoard(t, p, "b", "title: B\n")

	s, _ := newSyncer(t, p)
	s.Only = "a"
	s.Start()

	eventually(t, "a to be pushed", func() bool { return len(f.puts("a")) == 1 })
	never(t, "b to be pushed", func() bool { return len(f.puts("b")) > 0 })
}

func TestPublishRecordsTheBoardAndPrimesTheSyncer(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f)
	writeBoard(t, p, "demo", "title: One\n")

	s, _ := newSyncer(t, p)

	pub, err := s.Publish(context.Background(), "demo")
	require.NoError(t, err)
	assert.Equal(t, "demo", pub.Name)
	require.Len(t, pub.Links, 1)

	c, err := cloud.Load(p)
	require.NoError(t, err)
	assert.Equal(t, []string{"demo"}, c.Published)
	assert.Equal(t, token, c.Token, "the login is kept")

	s.Changed("demo")
	never(t, "a second upload of the same body", func() bool { return len(f.puts("demo")) > 1 })

	// publishing again is a request: it uploads even when nothing changed
	_, err = s.Publish(context.Background(), "demo")
	require.NoError(t, err)
	assert.Len(t, f.puts("demo"), 2)
}

func TestPublishNeedsALoginAndReportsFailures(t *testing.T) {
	f, p := newFake(t), scratch(t)
	writeBoard(t, p, "demo", "title: One\n")

	s, _ := newSyncer(t, p)

	_, err := s.Publish(context.Background(), "demo")
	assert.ErrorIs(t, err, cloud.ErrNotLoggedIn)

	login(t, p, f)
	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 409, `{"status":"error","message":"500 boards per owner"}`)

		return true
	}

	_, err = s.Publish(context.Background(), "demo")
	assert.True(t, cloud.IsStatus(err, 409))

	c, err := cloud.Load(p)
	require.NoError(t, err)
	assert.Empty(t, c.Published, "a failed publish is not recorded")

	_, err = s.Publish(context.Background(), "missing")
	assert.ErrorContains(t, err, "cloud.Snapshot")
}

// clock is a settable clock for the live schedule.
type clock struct{ ns atomic.Int64 }

func newClock() *clock {
	c := &clock{}
	c.ns.Store(time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC).UnixNano())

	return c
}

func (c *clock) now() time.Time          { return time.Unix(0, c.ns.Load()).UTC() }
func (c *clock) advance(d time.Duration) { c.ns.Add(int64(d)) }

func TestLiveValuesArePushedOnTheirScheduleAndOnlyWhenTheyChange(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")

	val := p.Home + "/value.txt"
	require.NoError(t, os.WriteFile(val, []byte("1\n"), 0o644))
	writeBoard(t, p, "demo", "title: L\nnodes:\n  a: { title: A, live: { cmd: \"cat "+val+"\", every: 5 } }\n  b: { title: B, live: { cmd: \"echo hidden\" } }\n")

	clk := newClock()
	runner := live.New(p)
	runner.Now = clk.now

	_, err := runner.Allow("demo", "a") // b stays blocked
	require.NoError(t, err)

	s, _ := newSyncer(t, p)
	s.Live, s.Now = runner, clk.now

	// the board is up already, so Start's own push of it does not send the values again behind the test's back
	_, err = s.Publish(context.Background(), "demo")
	require.NoError(t, err)

	s.Start()

	eventually(t, "the first live push", func() bool { return len(f.lives("demo")) == 1 })

	first := decode(t, f.lives("demo")[0])["values"].(map[string]any)
	assert.Equal(t, "1", first["a"].(map[string]any)["value"])
	assert.Equal(t, false, first["a"].(map[string]any)["blocked"])
	assert.Equal(t, true, first["b"].(map[string]any)["blocked"], "a blocked card is reported, not run")
	assert.NotContains(t, f.lives("demo")[0], "cat ", "no source goes up")
	assert.NotContains(t, f.lives("demo")[0], "hidden")

	// the schedule comes round, the value is the same: nothing to say
	clk.advance(6 * time.Second)
	never(t, "a push of unchanged values", func() bool { return len(f.lives("demo")) > 1 })

	// the value changes
	require.NoError(t, os.WriteFile(val, []byte("2\n"), 0o644))
	clk.advance(6 * time.Second)

	eventually(t, "the push of the new value", func() bool { return len(f.lives("demo")) == 2 })
	assert.Equal(t, "2", decode(t, f.lives("demo")[1])["values"].(map[string]any)["a"].(map[string]any)["value"])
}

func TestAPushedBoardSendsItsLiveValuesAgain(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: L\nnodes:\n  a: { title: A, live: { cmd: \"echo 1\" } }\n")

	s, _ := newSyncer(t, p)
	_, err := s.Live.Allow("demo", "")
	require.NoError(t, err)

	require.NoError(t, s.PushLive(context.Background(), "demo"))
	assert.Len(t, f.lives("demo"), 1)

	s.Start()

	eventually(t, "the board to be uploaded", func() bool { return len(f.puts("demo")) == 1 })
	eventually(t, "the values to follow the board", func() bool { return len(f.lives("demo")) == 2 })
}

func TestABoardWithoutLiveCardsPushesNoValues(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: Plain\n")

	s, _ := newSyncer(t, p)
	s.Start()

	eventually(t, "the board", func() bool { return len(f.puts("demo")) == 1 })
	never(t, "live values", func() bool { return len(f.lives("demo")) > 0 })
}

func TestNothingRunsWithoutAToken(t *testing.T) {
	f, p := newFake(t), scratch(t)
	require.NoError(t, cloud.Config{URL: f.srv.URL, Published: []string{"demo"}}.Save(p))

	marker := p.Home + "/ran"
	writeBoard(t, p, "demo", "title: L\nnodes:\n  a: { title: A, live: { cmd: \"touch "+marker+"\" } }\n")

	s, _ := newSyncer(t, p)
	_, err := s.Live.Allow("demo", "")
	require.NoError(t, err)

	s.Start()
	s.Changed("demo")

	never(t, "any call or any command", func() bool {
		_, err := os.Stat(marker)

		return len(f.all()) > 0 || err == nil
	})
}

func TestCloseStopsEverythingAndIsSafeTwice(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	s, _ := newSyncer(t, p)
	s.Debounce = 300 * time.Millisecond
	s.Start()
	s.Changed("demo")

	s.Close()
	s.Close()
	s.Changed("demo")

	never(t, "a push after Close", func() bool { return len(f.all()) > 0 })
}
