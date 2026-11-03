package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Why a device login ended without a token.
var (
	// ErrDenied is returned when the owner denied the login.
	ErrDenied = errors.New("login denied")
	// ErrExpired is returned when the code ran out before the owner approved it.
	ErrExpired = errors.New("login code expired")
)

const (
	defaultInterval = 5 * time.Second
	slowDownBy      = 5 * time.Second
	// maxPollFailures is how many failed polls in a row end a login: a hiccup of the network is waited out, a
	// cloud that is gone is not.
	maxPollFailures = 5
)

// Device is the start of a device login: the code the owner approves in the browser.
type Device struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURL string `json:"verification_url"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// DeviceToken is the answer of an approved device login: the CLI token, shown once.
type DeviceToken struct {
	Token string `json:"token"`
	Name  string `json:"name"`
}

// StartDevice asks the cloud for a device and user code. client names this machine on the approval page.
func (c *Client) StartDevice(ctx context.Context, client string) (Device, error) {
	body, err := json.Marshal(map[string]string{"client": client})
	if err != nil {
		return Device{}, fmt.Errorf("cloud.StartDevice: %w", err)
	}

	var d Device
	if err := c.call(ctx, http.MethodPost, "/api/cli/device", body, &d); err != nil {
		return Device{}, err
	}

	if d.DeviceCode == "" || d.UserCode == "" || d.VerificationURL == "" {
		return Device{}, errors.New("cloud.StartDevice: the cloud answered without a code")
	}

	return d, nil
}

// pollError is the RFC 8628 error of a token poll: authorization_pending, slow_down, access_denied, expired_token.
type pollError struct{ code string }

func (e *pollError) Error() string { return "cloud: " + e.code }

// PollDevice asks once whether the owner approved the device. Not yet approved is a *pollError whose code says
// what to do; any other failure is the usual APIError or a transport error.
func (c *Client) PollDevice(ctx context.Context, deviceCode string) (DeviceToken, error) {
	body, err := json.Marshal(map[string]string{"device_code": deviceCode})
	if err != nil {
		return DeviceToken{}, fmt.Errorf("cloud.PollDevice: %w", err)
	}

	var t DeviceToken

	err = c.send(ctx, http.MethodPost, "/api/cli/device/token", body, &t, true)

	var api *APIError
	if errors.As(err, &api) && api.Status == http.StatusBadRequest {
		return DeviceToken{}, &pollError{code: api.Message}
	}

	return t, err
}

// Waiter sleeps between polls; it returns early with the context's error when the context ends.
type Waiter func(ctx context.Context, d time.Duration) error

// Sleep is the Waiter that really waits.
func Sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()

	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Login runs a whole device login: it starts one, hands the code to show (the verification URL and user code for
// the owner), then polls at the cloud's interval until the owner approves. slow_down makes the interval 5 s longer;
// a denied or expired login ends it. wait nil means Sleep.
func (c *Client) Login(ctx context.Context, client string, show func(Device), wait Waiter) (DeviceToken, error) {
	if wait == nil {
		wait = Sleep
	}

	d, err := c.StartDevice(ctx, client)
	if err != nil {
		return DeviceToken{}, err
	}

	if show != nil {
		show(d)
	}

	interval := time.Duration(d.Interval) * time.Second
	if interval <= 0 {
		interval = defaultInterval
	}

	// the code lives ExpiresIn seconds; the deadline only matters to a cloud that never says expired_token
	var deadline time.Time
	if d.ExpiresIn > 0 {
		deadline = time.Now().Add(time.Duration(d.ExpiresIn)*time.Second + interval)
	}

	failures := 0

	for {
		if err := wait(ctx, interval); err != nil {
			return DeviceToken{}, err
		}

		if !deadline.IsZero() && time.Now().After(deadline) {
			return DeviceToken{}, ErrExpired
		}

		t, err := c.PollDevice(ctx, d.DeviceCode)
		if err == nil {
			return t, nil
		}

		var pe *pollError
		if errors.As(err, &pe) {
			failures = 0

			switch pe.code {
			case "authorization_pending":
			case "slow_down":
				interval += slowDownBy
			case "access_denied":
				return DeviceToken{}, ErrDenied
			case "expired_token":
				return DeviceToken{}, ErrExpired
			default:
				return DeviceToken{}, fmt.Errorf("cloud.Login: %w", pe)
			}

			continue
		}

		if ctx.Err() != nil {
			return DeviceToken{}, ctx.Err()
		}

		// a rate limit or a bad moment of the network: wait it out, but not forever
		var rl *RateLimitError
		if errors.As(err, &rl) {
			interval = max(interval, rl.RetryAfter)
		}

		if failures++; failures >= maxPollFailures {
			return DeviceToken{}, err
		}
	}
}
