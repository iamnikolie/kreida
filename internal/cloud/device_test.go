package cloud_test

import (
	"context"
	"net/http"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pollScript answers the token polls of a fake device flow from a list: a code for a 400, "token" for the approval.
func pollScript(f *fakeCloud, interval int, steps ...string) {
	var mu sync.Mutex

	n := 0

	f.respond = func(c call, w http.ResponseWriter) bool {
		switch c.Path {
		case "/api/cli/device":
			reply(w, 200, `{"device_code":"dev123","user_code":"ABCD-2345","verification_url":"`+f.srv.URL+`/owner/device?code=ABCD-2345","expires_in":600,"interval":`+strconv.Itoa(interval)+`}`)
		case "/api/cli/device/token":
			mu.Lock()
			step := "token"
			if n < len(steps) {
				step = steps[n]
			}
			n++
			mu.Unlock()

			switch step {
			case "token":
				reply(w, 200, `{"token":"`+token+`","name":"laptop"}`)
			case "500":
				reply(w, 500, `{"status":"error","message":"boom"}`)
			default:
				reply(w, 400, `{"error":"`+step+`","message":"ignored when error is there"}`)
			}
		default:
			return false
		}

		return true
	}
}

// anonymous is a client without a token, as the device login runs.
func anonymous(t *testing.T, f *fakeCloud) *cloud.Client {
	t.Helper()

	cl, err := cloud.NewClient(f.srv.URL, "")
	require.NoError(t, err)

	return cl
}

// waits records the delays a login asked for and returns at once.
type waits struct {
	mu sync.Mutex
	ds []time.Duration
}

func (w *waits) wait(ctx context.Context, d time.Duration) error {
	w.mu.Lock()
	w.ds = append(w.ds, d)
	w.mu.Unlock()

	return ctx.Err()
}

func TestDeviceLoginPollsAtTheIntervalAndSlowDownAddsFiveSeconds(t *testing.T) {
	f := newFake(t)
	pollScript(f, 5, "authorization_pending", "slow_down", "authorization_pending", "token")

	var shown cloud.Device

	w := &waits{}

	tok, err := anonymous(t, f).Login(context.Background(), "my-mac", func(d cloud.Device) { shown = d }, w.wait)
	require.NoError(t, err)
	assert.Equal(t, cloud.DeviceToken{Token: token, Name: "laptop"}, tok)

	assert.Equal(t, "ABCD-2345", shown.UserCode)
	assert.Equal(t, f.srv.URL+"/owner/device?code=ABCD-2345", shown.VerificationURL)

	assert.Equal(t, []time.Duration{5 * time.Second, 5 * time.Second, 10 * time.Second, 10 * time.Second}, w.ds)

	calls := f.all()
	require.Len(t, calls, 5)
	assert.JSONEq(t, `{"client":"my-mac"}`, calls[0].Body)
	assert.Empty(t, calls[0].Auth, "the device login needs no token")

	for _, c := range calls[1:] {
		assert.JSONEq(t, `{"device_code":"dev123"}`, c.Body)
	}
}

func TestDeviceLoginStopsWhenDeniedOrExpired(t *testing.T) {
	f := newFake(t)
	pollScript(f, 5, "authorization_pending", "access_denied")

	w := &waits{}
	_, err := anonymous(t, f).Login(context.Background(), "c", nil, w.wait)
	assert.ErrorIs(t, err, cloud.ErrDenied)
	assert.Len(t, w.ds, 2)

	f = newFake(t)
	pollScript(f, 5, "expired_token", "token")

	w = &waits{}
	_, err = anonymous(t, f).Login(context.Background(), "c", nil, w.wait)
	assert.ErrorIs(t, err, cloud.ErrExpired)
	assert.Len(t, w.ds, 1, "no poll after the code expired")
}

func TestDeviceLoginDefaultsTheIntervalToFiveSeconds(t *testing.T) {
	f := newFake(t)
	pollScript(f, 0, "token")

	w := &waits{}
	_, err := anonymous(t, f).Login(context.Background(), "c", nil, w.wait)
	require.NoError(t, err)
	assert.Equal(t, []time.Duration{5 * time.Second}, w.ds)
}

func TestDeviceLoginWaitsOutAFailingPollButNotForever(t *testing.T) {
	f := newFake(t)
	pollScript(f, 5, "500", "500", "token")

	_, err := anonymous(t, f).Login(context.Background(), "c", nil, (&waits{}).wait)
	require.NoError(t, err, "two bad polls in a row are waited out")

	f = newFake(t)
	pollScript(f, 5, "500", "500", "500", "500", "500", "token")

	_, err = anonymous(t, f).Login(context.Background(), "c", nil, (&waits{}).wait)
	var api *cloud.APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, 500, api.Status)
}

func TestDeviceLoginHonoursTheContext(t *testing.T) {
	f := newFake(t)
	pollScript(f, 5, "authorization_pending")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := anonymous(t, f).Login(ctx, "c", nil, nil)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestSleepEndsWithTheContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	assert.ErrorIs(t, cloud.Sleep(ctx, time.Minute), context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
	assert.NoError(t, cloud.Sleep(context.Background(), time.Millisecond))
}

func TestDeviceStartWithoutACodeFails(t *testing.T) {
	f := newFake(t)
	f.respond = func(c call, w http.ResponseWriter) bool { reply(w, 200, `{}`); return true }

	_, err := anonymous(t, f).StartDevice(context.Background(), "c")
	assert.ErrorContains(t, err, "without a code")
}
