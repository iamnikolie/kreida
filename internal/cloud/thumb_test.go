package cloud_test

import (
	"context"
	"errors"
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
	"github.com/iamnikolie/kreida/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drawing is a renderer that "draws" the title of the board file as it is when it is called, and counts its calls.
func drawing(p paths.Paths, calls *atomic.Int32) cloud.Renderer {
	return func(ctx context.Context, name string) ([]byte, error) {
		calls.Add(1)

		text, err := os.ReadFile(p.BoardFile(name))
		if err != nil {
			return nil, err
		}

		return append(png(0), text...), nil
	}
}

// got waits for the renderer to say it has started.
func got(t *testing.T, started <-chan struct{}) {
	t.Helper()

	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the renderer")
	}
}

// warns counts the warnings about thumbnails in a log.
func warns(logs *syncBuf) int { return strings.Count(logs.String(), `level=WARN msg="cloud thumb"`) }

func TestPushThumbUploadsThePictureAfterPublish(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f)
	writeBoard(t, p, "demo", "title: One\n")

	var calls atomic.Int32

	s, logs := newSyncer(t, p)
	s.Thumb = drawing(p, &calls)

	_, err := s.Publish(context.Background(), "demo")
	require.NoError(t, err)
	require.NoError(t, s.PushThumb(context.Background(), "demo"))

	require.Len(t, f.thumbs("demo"), 1)
	assert.Equal(t, "\x89PNG\r\n\x1a\ntitle: One\n", f.thumbs("demo")[0])

	last := f.all()[len(f.all())-1]
	assert.Equal(t, "image/png", last.Type)
	assert.Equal(t, "Bearer "+token, last.Auth)
	assert.EqualValues(t, 1, calls.Load())
	assert.NotContains(t, logs.String(), token)
}

func TestPushThumbWithoutARendererOrALoginSaysSo(t *testing.T) {
	f, p := newFake(t), scratch(t)
	writeBoard(t, p, "demo", "title: One\n")

	s, _ := newSyncer(t, p)
	assert.ErrorIs(t, s.PushThumb(context.Background(), "demo"), cloud.ErrNoRenderer)

	s.Thumb = func(context.Context, string) ([]byte, error) { return png(0), nil }
	assert.ErrorIs(t, s.PushThumb(context.Background(), "demo"), cloud.ErrNotLoggedIn)

	login(t, p, f, "demo")
	s.Thumb = func(context.Context, string) ([]byte, error) { return nil, render.ErrNoChrome }
	assert.ErrorIs(t, s.PushThumb(context.Background(), "demo"), render.ErrNoChrome)

	assert.Empty(t, f.all(), "nothing reached the cloud")
}

func TestAPictureTheCloudWouldRefuseNeverLeavesTheMachine(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	s, _ := newSyncer(t, p)

	s.Thumb = func(context.Context, string) ([]byte, error) { return png(cloud.MaxThumb), nil }
	assert.ErrorContains(t, s.PushThumb(context.Background(), "demo"), "over the 256 KiB limit")

	s.Thumb = func(context.Context, string) ([]byte, error) { return []byte("<html>"), nil }
	assert.ErrorContains(t, s.PushThumb(context.Background(), "demo"), "not a PNG")

	assert.Empty(t, f.all())
}

func TestThumbnailsAreDrawnOncePerWindowAfterABoardChange(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	var calls atomic.Int32

	s, _ := newSyncer(t, p)
	s.Thumb, s.ThumbEvery = drawing(p, &calls), 600*time.Millisecond

	// the first change is drawn at once
	s.Changed("demo")
	eventually(t, "the first thumbnail", func() bool { return len(f.thumbs("demo")) == 1 })
	assert.Contains(t, f.thumbs("demo")[0], "title: One")

	// changes inside the window push the board, and wait for the window's end together
	for _, title := range []string{"Two", "Three", "Four"} {
		writeBoard(t, p, "demo", "title: "+title+"\n")
		s.Changed("demo")
		eventually(t, "the push of "+title, func() bool { return strings.Contains(strings.Join(f.puts("demo"), ""), `"title":"`+title+`"`) })
	}

	never(t, "a second thumbnail inside the window", func() bool { return len(f.thumbs("demo")) > 1 })
	eventually(t, "the second thumbnail at the window's end", func() bool { return len(f.thumbs("demo")) == 2 })
	assert.Contains(t, f.thumbs("demo")[1], "title: Four", "it shows the board as it is then")

	never(t, "a third thumbnail", func() bool { return len(f.thumbs("demo")) > 2 })
	assert.EqualValues(t, 2, calls.Load())

	// the same body again is not a change
	s.Changed("demo")
	never(t, "a thumbnail of an unchanged board", func() bool { return len(f.thumbs("demo")) > 2 })
}

func TestLiveValuesNeverDrawAThumbnail(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")

	val := p.Home + "/value.txt"
	require.NoError(t, os.WriteFile(val, []byte("1\n"), 0o644))
	writeBoard(t, p, "demo", "title: L\nnodes:\n  a: { title: A, live: { cmd: \"cat "+val+"\", every: 5 } }\n")

	clk := newClock()
	runner := live.New(p)
	runner.Now = clk.now

	_, err := runner.Allow("demo", "a")
	require.NoError(t, err)

	var calls atomic.Int32

	s, _ := newSyncer(t, p)
	s.Live, s.Now = runner, clk.now
	s.Thumb, s.ThumbEvery = drawing(p, &calls), 50*time.Millisecond
	s.Start()

	eventually(t, "the board, its values and its thumbnail", func() bool {
		return len(f.puts("demo")) == 1 && len(f.lives("demo")) >= 1 && len(f.thumbs("demo")) == 1
	})

	lives := len(f.lives("demo"))

	for _, v := range []string{"2\n", "3\n"} {
		require.NoError(t, os.WriteFile(val, []byte(v), 0o644))
		clk.advance(6 * time.Second)

		want := lives + 1
		eventually(t, "the push of the value "+v, func() bool { return len(f.lives("demo")) >= want })

		lives = len(f.lives("demo"))
	}

	never(t, "a thumbnail for a value", func() bool { return len(f.thumbs("demo")) > 1 })
	assert.Len(t, f.puts("demo"), 1)
}

func TestAThumbnailNeverHoldsUpAPush(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	started, release := make(chan struct{}), make(chan struct{})

	s, _ := newSyncer(t, p)
	s.Thumb = func(ctx context.Context, name string) ([]byte, error) {
		close(started)

		select {
		case <-release:
			return png(0), nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	s.Changed("demo")
	got(t, started)

	// Chrome is busy; the next change still goes up
	writeBoard(t, p, "demo", "title: Two\n")
	s.Changed("demo")
	eventually(t, "the push while a thumbnail is drawn", func() bool { return len(f.puts("demo")) == 2 })
	assert.Empty(t, f.thumbs("demo"))

	close(release)
	eventually(t, "the thumbnail", func() bool { return len(f.thumbs("demo")) == 1 })
}

func TestCloseStopsADrawingThatIsInFlight(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	started := make(chan struct{})

	s, _ := newSyncer(t, p)
	s.Thumb = func(ctx context.Context, name string) ([]byte, error) {
		close(started)
		<-ctx.Done()

		return nil, ctx.Err()
	}

	s.Changed("demo")
	got(t, started)

	done := make(chan struct{})

	go func() { s.Close(); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Close waits for the drawing")
	}

	assert.Empty(t, f.thumbs("demo"))
}

func TestAFailedDrawingDoesNotBreakPushesAndIsWarnedOnce(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	var calls atomic.Int32

	s, logs := newSyncer(t, p)
	s.ThumbEvery = 30 * time.Millisecond
	s.Thumb = func(context.Context, string) ([]byte, error) {
		calls.Add(1)

		return nil, render.ErrNoChrome
	}

	for i, title := range []string{"One", "Two", "Three"} {
		writeBoard(t, p, "demo", "title: "+title+"\n")
		s.Changed("demo")
		eventually(t, "the push of "+title, func() bool { return len(f.puts("demo")) == i+1 })
		eventually(t, "the attempt of "+title, func() bool { return int(calls.Load()) == i+1 })
		time.Sleep(50 * time.Millisecond)
	}

	assert.Empty(t, f.thumbs("demo"))
	assert.Equal(t, 1, warns(logs), "the same message is logged once:\n"+logs.String())
	assert.Contains(t, logs.String(), "no Chrome found")

	// a different failure is a new message
	s.Thumb = func(context.Context, string) ([]byte, error) { return nil, errors.New("chrome did not finish in 30 s") }

	writeBoard(t, p, "demo", "title: Four\n")
	s.Changed("demo")
	eventually(t, "the second kind of warning", func() bool { return warns(logs) == 2 })

	// and it is not tried again without a change: no Chrome is no Chrome
	never(t, "an attempt without a change", func() bool { return int(calls.Load()) > 3 })
}

func TestWithoutARendererABoardChangeJustPushes(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	s, logs := newSyncer(t, p)
	s.Changed("demo")

	eventually(t, "the push", func() bool { return len(f.puts("demo")) == 1 })
	never(t, "a thumbnail", func() bool { return len(f.thumbs("demo")) > 0 })
	assert.NotContains(t, logs.String(), "cloud thumb")
}

func TestAThumbnailTheCloudCouldNotTakeIsTriedAgainOnlyWhenItMayWork(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	var (
		mu    sync.Mutex
		times []time.Time
		codes = []int{503, 429, 204}
	)

	f.respond = func(c call, w http.ResponseWriter) bool {
		if !strings.HasSuffix(c.Path, "/thumb") {
			return false
		}

		mu.Lock()
		defer mu.Unlock()

		code := codes[min(len(times), len(codes)-1)]
		times = append(times, time.Now())

		switch code {
		case 429:
			w.Header().Set("Retry-After", "1")
			reply(w, 429, `{"status":"error","message":"slow down"}`)
		case 503:
			reply(w, 503, `{"status":"error","message":"cloud is down"}`)
		default:
			w.WriteHeader(code)
		}

		return true
	}

	var calls atomic.Int32

	s, logs := newSyncer(t, p)
	s.Thumb, s.ThumbEvery = drawing(p, &calls), 50*time.Millisecond
	s.Changed("demo")

	eventually(t, "three attempts", func() bool { return len(f.thumbs("demo")) == 3 })
	never(t, "a fourth attempt", func() bool { return len(f.thumbs("demo")) > 3 })

	mu.Lock()
	defer mu.Unlock()

	assert.Less(t, times[1].Sub(times[0]), 500*time.Millisecond, "a failing cloud is asked again after the window")
	assert.GreaterOrEqual(t, times[2].Sub(times[1]), 900*time.Millisecond, "Retry-After is kept")
	assert.Equal(t, 2, warns(logs), logs.String())
	assert.Len(t, f.puts("demo"), 1, "the board itself was pushed once")
}

func TestAThumbnailOfABoardThatIsNotPublishedIsNotTriedAgain(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	f.respond = func(c call, w http.ResponseWriter) bool {
		if !strings.HasSuffix(c.Path, "/thumb") {
			return false
		}

		reply(w, 404, `{"status":"error","message":"not published"}`)

		return true
	}

	var calls atomic.Int32

	s, logs := newSyncer(t, p)
	s.Thumb, s.ThumbEvery = drawing(p, &calls), 30*time.Millisecond
	s.Changed("demo")

	eventually(t, "the attempt", func() bool { return len(f.thumbs("demo")) == 1 })
	never(t, "another attempt", func() bool { return len(f.thumbs("demo")) > 1 })
	assert.Equal(t, 1, warns(logs))
}

func TestAnUnpublishedBoardGetsNoThumbnail(t *testing.T) {
	f, p := newFake(t), scratch(t)
	login(t, p, f, "demo")
	writeBoard(t, p, "demo", "title: One\n")

	var calls atomic.Int32

	s, _ := newSyncer(t, p)
	s.Thumb, s.ThumbEvery = drawing(p, &calls), 300*time.Millisecond

	s.Changed("demo")
	eventually(t, "the first thumbnail", func() bool { return len(f.thumbs("demo")) == 1 })

	// a change, then the board is unpublished before the window ends
	writeBoard(t, p, "demo", "title: Two\n")
	s.Changed("demo")
	eventually(t, "the push", func() bool { return len(f.puts("demo")) == 2 })
	login(t, p, f)

	never(t, "a thumbnail of an unpublished board", func() bool { return len(f.thumbs("demo")) > 1 })
	assert.EqualValues(t, 1, calls.Load())
}
