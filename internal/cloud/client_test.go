package cloud_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func clientFor(t *testing.T, f *fakeCloud) *cloud.Client {
	t.Helper()

	cl, err := cloud.NewClient(f.srv.URL, token)
	require.NoError(t, err)

	return cl
}

func TestWhoamiSendsTheBearerToken(t *testing.T) {
	f := newFake(t)

	w, err := clientFor(t, f).Whoami(context.Background())
	require.NoError(t, err)
	assert.Equal(t, cloud.Whoami{Email: "me@example.com", TokenName: "laptop"}, w)

	calls := f.all()
	require.Len(t, calls, 1)
	assert.Equal(t, "Bearer "+token, calls[0].Auth)
}

func TestClientHasExplicitTimeouts(t *testing.T) {
	cl, err := cloud.NewClient("https://c.example/", token)
	require.NoError(t, err)

	assert.Equal(t, "https://c.example", cl.BaseURL)
	assert.Equal(t, cloud.RequestTimeout, cl.HTTP.Timeout)
	assert.NotNil(t, cl.HTTP.CheckRedirect)

	tr := cl.HTTP.Transport.(*http.Transport)
	assert.NotZero(t, tr.TLSHandshakeTimeout)
	assert.NotZero(t, tr.ResponseHeaderTimeout)

	_, err = cloud.NewClient("http://c.example", token)
	assert.Error(t, err, "the token does not travel over plain http to another machine")
}

func TestErrorTextComesFromMessageThenError(t *testing.T) {
	for name, tc := range map[string]struct {
		code int
		body string
		want string
	}{
		"house responder": {400, `{"status":"error","message":"board is too big","request_id":"r1"}`, "board is too big"},
		"old shape":       {409, `{"error":"500 boards per owner"}`, "500 boards per owner"},
		"message wins":    {400, `{"message":"from message","error":"from error"}`, "from message"},
		"no json":         {502, `<html>bad gateway</html>`, "bad gateway"},
		"empty":           {401, ``, "unauthorized"},
	} {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			f.respond = func(c call, w http.ResponseWriter) bool { reply(w, tc.code, tc.body); return true }

			_, err := clientFor(t, f).Whoami(context.Background())

			var api *cloud.APIError
			require.ErrorAs(t, err, &api)
			assert.Equal(t, tc.code, api.Status)
			assert.Equal(t, tc.want, api.Message)
			assert.True(t, cloud.IsStatus(err, tc.code))
			assert.NotContains(t, err.Error(), token)
		})
	}
}

func TestRateLimitCarriesRetryAfter(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		w.Header().Set("Retry-After", "7")
		reply(w, 429, `{"status":"error","message":"slow down"}`)

		return true
	}

	_, err := clientFor(t, f).Publish(context.Background(), "demo", []byte(`{}`))

	var rl *cloud.RateLimitError
	require.ErrorAs(t, err, &rl)
	assert.Equal(t, 7*time.Second, rl.RetryAfter)
	assert.Equal(t, "slow down", rl.Message)
	assert.True(t, cloud.IsStatus(err, 429))
	assert.ErrorContains(t, err, "retry in 7s")

	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 429, `{"error":"slow down"}`)

		return true
	}

	_, err = clientFor(t, f).Publish(context.Background(), "demo", []byte(`{}`))
	require.ErrorAs(t, err, &rl)
	assert.Zero(t, rl.RetryAfter, "no header: the caller picks a delay")

	f.respond = func(c call, w http.ResponseWriter) bool {
		w.Header().Set("Retry-After", time.Now().Add(30*time.Second).UTC().Format(http.TimeFormat))
		reply(w, 429, `{}`)

		return true
	}

	_, err = clientFor(t, f).Publish(context.Background(), "demo", []byte(`{}`))
	require.ErrorAs(t, err, &rl)
	assert.InDelta(t, 30, rl.RetryAfter.Seconds(), 2, "an HTTP date works too")
}

func TestPublishPushLiveUnpublishAndBoards(t *testing.T) {
	f := newFake(t)
	cl := clientFor(t, f)
	ctx := context.Background()

	pub, err := cl.Publish(ctx, "my.board-1", []byte(`{"board":{},"pos":{}}`))
	require.NoError(t, err)
	assert.Equal(t, "my.board-1", pub.Name)
	require.Len(t, pub.Links, 1)
	assert.Equal(t, f.srv.URL+"/b/tok", pub.Links[0].URL)
	assert.Nil(t, pub.Links[0].ExpiresAt)
	assert.False(t, pub.Links[0].Passcode)

	val := "42"
	require.NoError(t, cl.PushLive(ctx, "demo", cloud.LiveValues(live.ValueSet{"a": {Value: &val, At: "2026-10-29T12:00:00Z", Source: "cmd:secret"}})))
	require.NoError(t, cl.Unpublish(ctx, "demo"))

	boards, err := cl.Boards(ctx)
	require.NoError(t, err)
	require.Len(t, boards, 1)
	assert.Equal(t, "Demo", boards[0].Title)

	calls := f.all()
	require.Len(t, calls, 4)
	assert.Equal(t, call{"PUT", "/api/cli/boards/my.board-1", "Bearer " + token, `{"board":{},"pos":{}}`, "application/json"}, calls[0])
	assert.Equal(t, call{"PUT", "/api/cli/boards/demo/live", "Bearer " + token,
		`{"values":{"a":{"value":"42","error":"","blocked":false,"at":"2026-10-29T12:00:00Z"}}}`, "application/json"}, calls[1])
	assert.Equal(t, "DELETE", calls[2].Method)
	assert.NotContains(t, calls[1].Body, "secret")
}

// png is the smallest thing PutThumb takes: the signature and whatever follows.
func png(n int) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, n)...)
}

func TestPutThumbSendsThePNGBytes(t *testing.T) {
	f := newFake(t)
	img := png(100)

	require.NoError(t, clientFor(t, f).PutThumb(context.Background(), "my.board-1", img))

	calls := f.all()
	require.Len(t, calls, 1)
	assert.Equal(t, call{"PUT", "/api/cli/boards/my.board-1/thumb", "Bearer " + token, string(img), "image/png"}, calls[0])
}

func TestPutThumbRefusesWhatTheCloudWouldRefuseBeforeTheNetwork(t *testing.T) {
	f := newFake(t)
	cl := clientFor(t, f)
	ctx := context.Background()

	assert.ErrorContains(t, cl.PutThumb(ctx, "demo", nil), "not a PNG")
	assert.ErrorContains(t, cl.PutThumb(ctx, "demo", []byte("GIF89a")), "not a PNG")
	assert.ErrorContains(t, cl.PutThumb(ctx, "demo", []byte(`{"board":{}}`)), "not a PNG")
	assert.ErrorContains(t, cl.PutThumb(ctx, "demo", png(cloud.MaxThumb)), "over the 256 KiB limit")
	assert.ErrorContains(t, cl.PutThumb(ctx, "a/b", png(10)), "bad board name")
	assert.Empty(t, f.all())

	// the limit itself is fine
	require.NoError(t, cl.PutThumb(ctx, "demo", png(cloud.MaxThumb-8)))
	assert.Len(t, f.all(), 1)
}

func TestPutThumbAnswers(t *testing.T) {
	f := newFake(t)
	cl := clientFor(t, f)

	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 404, `{"status":"error","message":"not published"}`)

		return true
	}

	err := cl.PutThumb(context.Background(), "demo", png(10))
	assert.True(t, cloud.IsStatus(err, 404))
	assert.ErrorContains(t, err, "not published")

	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 413, `{"status":"error","message":"thumbnail is too large"}`)

		return true
	}

	assert.True(t, cloud.IsStatus(cl.PutThumb(context.Background(), "demo", png(10)), 413))

	f.respond = func(c call, w http.ResponseWriter) bool {
		w.Header().Set("Retry-After", "9")
		reply(w, 429, `{"status":"error","message":"slow down"}`)

		return true
	}

	var rl *cloud.RateLimitError
	require.ErrorAs(t, cl.PutThumb(context.Background(), "demo", png(10)), &rl)
	assert.Equal(t, 9*time.Second, rl.RetryAfter)
}

func TestCommentsOfABoardAreTheOpenOnesUnlessAll(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 200, `[{"id":"c_1","node":"a1","text":"Move this up","author":"Anna <anna@client.example>","created":"2026-10-29T12:00:00Z"},`+
			`{"id":"c_2","node":"","text":"Whole board","author":"Anna <anna@client.example>","created":"2026-10-29T13:00:00Z","resolved":"2026-10-30T08:00:00Z"}]`)

		return true
	}

	cl := clientFor(t, f)

	got, err := cl.Comments(context.Background(), "my.board-1", false)
	require.NoError(t, err)
	require.Len(t, got, 2)
	assert.Equal(t, notes.Note{ID: "c_1", Node: "a1", Text: "Move this up", Author: "Anna <anna@client.example>", Created: "2026-10-29T12:00:00Z"}, got[0])
	assert.Equal(t, "2026-10-30T08:00:00Z", got[1].Resolved)

	_, err = cl.Comments(context.Background(), "my.board-1", true)
	require.NoError(t, err)

	calls := f.all()
	require.Len(t, calls, 2)
	assert.Equal(t, call{"GET", "/api/cli/boards/my.board-1/comments", "Bearer " + token, "", ""}, calls[0])
	assert.Equal(t, "/api/cli/boards/my.board-1/comments", calls[1].Path)
	assert.Equal(t, "GET", calls[1].Method)
}

func TestAllCommentsCarryTheirBoard(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 200, `[{"id":"c_1","board":"demo","node":"a1","text":"x","author":"Anna","created":"2026-10-29T12:00:00Z"}]`)

		return true
	}

	cl := clientFor(t, f)

	got, err := cl.AllComments(context.Background(), false)
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "demo", got[0].Board)

	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 200, `[]`)

		return true
	}

	got, err = cl.AllComments(context.Background(), true)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestCommentsAskForAllWithAllOne(t *testing.T) {
	var urls []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urls = append(urls, r.Method+" "+r.URL.RequestURI())
		reply(w, 200, `[]`)
	}))
	defer srv.Close()

	cl, err := cloud.NewClient(srv.URL, token)
	require.NoError(t, err)

	ctx := context.Background()

	_, err = cl.Comments(ctx, "demo", false)
	require.NoError(t, err)

	_, err = cl.Comments(ctx, "demo", true)
	require.NoError(t, err)

	_, err = cl.AllComments(ctx, false)
	require.NoError(t, err)

	_, err = cl.AllComments(ctx, true)
	require.NoError(t, err)

	assert.Equal(t, []string{
		"GET /api/cli/boards/demo/comments",
		"GET /api/cli/boards/demo/comments?all=1",
		"GET /api/cli/comments",
		"GET /api/cli/comments?all=1",
	}, urls)
}

func TestResolveCommentPostsToTheCommentAndAcceptsA204(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		w.WriteHeader(204)

		return true
	}

	cl := clientFor(t, f)

	require.NoError(t, cl.ResolveComment(context.Background(), "my.board-1", "c_9"))
	require.NoError(t, cl.ResolveComment(context.Background(), "my.board-1", "c_9"), "again is fine")

	calls := f.all()
	require.Len(t, calls, 2)
	assert.Equal(t, call{"POST", "/api/cli/boards/my.board-1/comments/c_9/resolve", "Bearer " + token, "", ""}, calls[0])

	require.NoError(t, cl.ResolveComment(context.Background(), "demo", "c_a/b?x"))
	assert.Equal(t, "/api/cli/boards/demo/comments/c_a%2Fb%3Fx/resolve", f.all()[2].Path, "an id stays one path segment")

	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 404, `{"status":"error","message":"no comment"}`)

		return true
	}

	err := cl.ResolveComment(context.Background(), "my.board-1", "c_404")
	assert.True(t, cloud.IsStatus(err, 404))
	assert.ErrorContains(t, err, "no comment")
}

func TestCommentsOfAnUnpublishedBoardAre404(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 404, `{"status":"error","message":"not published"}`)

		return true
	}

	_, err := clientFor(t, f).Comments(context.Background(), "demo", false)
	assert.True(t, cloud.IsStatus(err, 404))
}

func TestBadNamesAndIDsNeverReachTheCloudForComments(t *testing.T) {
	f := newFake(t)
	cl := clientFor(t, f)
	ctx := context.Background()

	for _, name := range []string{"", "..", "a b", "a/b", "../x"} {
		_, err := cl.Comments(ctx, name, false)
		assert.ErrorContains(t, err, "bad board name", name)
		assert.ErrorContains(t, cl.ResolveComment(ctx, name, "c_1"), "bad board name", name)
	}

	for _, id := range []string{"", "n1", "c_", "1"} {
		assert.ErrorContains(t, cl.ResolveComment(ctx, "demo", id), "bad comment id", id)
	}

	assert.Empty(t, f.all())
}

func TestUnpublishOfAnUnpublishedBoardIs404(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		reply(w, 404, `{"status":"error","message":"not published"}`)

		return true
	}

	err := clientFor(t, f).Unpublish(context.Background(), "demo")
	assert.True(t, cloud.IsStatus(err, 404))
	assert.False(t, cloud.IsStatus(err, 500))
	assert.False(t, cloud.IsStatus(errors.New("other"), 404))
}

func TestBadNamesNeverReachTheCloud(t *testing.T) {
	f := newFake(t)
	cl := clientFor(t, f)
	ctx := context.Background()

	for _, name := range []string{"", "..", ".", "a b", "a/b", "../x", "a%2Fb", string(make([]byte, 101))} {
		_, err := cl.Publish(ctx, name, []byte(`{}`))
		assert.ErrorContains(t, err, "bad board name", name)
		assert.Error(t, cl.Unpublish(ctx, name), name)
		assert.Error(t, cl.PushLive(ctx, name, nil), name)
	}

	assert.Empty(t, f.all())
	assert.NoError(t, cloud.ValidName("a.b_c-9"))
}

func TestARedirectToAnotherHostIsRefusedAndKeepsTheToken(t *testing.T) {
	var other atomic.Int32

	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		other.Add(1)
		reply(w, 200, `{"email":"leaked"}`)
	}))
	defer elsewhere.Close()

	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		w.Header().Set("Location", elsewhere.URL+"/api/cli/whoami")
		w.WriteHeader(307)

		return true
	}

	_, err := clientFor(t, f).Whoami(context.Background())
	require.Error(t, err)
	assert.ErrorContains(t, err, "refusing a redirect")
	assert.Zero(t, other.Load(), "nothing reached the other host")
	assert.NotContains(t, err.Error(), token)
}

func TestARedirectOnTheSameOriginIsFollowed(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool {
		if c.Path == "/old/whoami" {
			w.Header().Set("Location", "/api/cli/whoami")
			w.WriteHeader(307)

			return true
		}

		return false
	}

	cl := clientFor(t, f)

	req, err := http.NewRequest(http.MethodGet, cl.BaseURL+"/old/whoami", nil)
	require.NoError(t, err)

	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := cl.HTTP.Do(req)
	require.NoError(t, err)

	defer resp.Body.Close()

	assert.Equal(t, 200, resp.StatusCode)
	assert.Equal(t, "Bearer "+token, f.all()[1].Auth)
}

func TestACanceledContextStopsTheCall(t *testing.T) {
	f := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := clientFor(t, f).Whoami(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
