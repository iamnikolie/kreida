package cloud

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/iamnikolie/kreida/internal/paths"
)

const (
	// RequestTimeout bounds one call to the cloud, from dial to the last byte of the answer.
	RequestTimeout = 30 * time.Second
	// MaxThumb is the largest thumbnail the cloud takes, in bytes.
	MaxThumb     = 256 << 10
	maxReply     = 4 << 20
	maxRedirects = 3
	userAgent    = "kreida"
)

// Link is one public address of a published board.
type Link struct {
	ID        string  `json:"id"`
	URL       string  `json:"url"`
	ExpiresAt *string `json:"expires_at"`
	Passcode  bool    `json:"passcode"`
}

// Published is a board as the cloud keeps it.
type Published struct {
	Name      string `json:"name"`
	Title     string `json:"title"`
	UpdatedAt string `json:"updated_at"`
	Links     []Link `json:"links"`
}

// Whoami is the owner a token belongs to.
type Whoami struct {
	Email     string `json:"email"`
	TokenName string `json:"token_name"`
}

// LiveVal is what a live card shows in the cloud: kreida's live.Value without its source.
type LiveVal struct {
	Value   string `json:"value"`
	Error   string `json:"error"`
	Blocked bool   `json:"blocked"`
	At      string `json:"at"`
}

// LiveValues turns resolved values into what goes up. The source of a card is not copied, so it cannot leave.
func LiveValues(vs live.ValueSet) map[string]LiveVal {
	out := make(map[string]LiveVal, len(vs))

	for id, v := range vs {
		lv := LiveVal{Blocked: v.Blocked, At: v.At}

		if v.Value != nil {
			lv.Value = *v.Value
		}

		if v.Error != nil {
			lv.Error = *v.Error
		}

		out[id] = lv
	}

	return out
}

// APIError is a failed call: the cloud's status and its message.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("cloud: %s (HTTP %d)", e.Message, e.Status)
}

// RateLimitError is a 429. RetryAfter is what the cloud asked for in Retry-After; zero when it did not say.
type RateLimitError struct {
	RetryAfter time.Duration
	Message    string
}

func (e *RateLimitError) Error() string {
	if e.RetryAfter > 0 {
		return fmt.Sprintf("cloud: %s (HTTP 429, retry in %s)", e.Message, e.RetryAfter.Round(time.Second))
	}

	return fmt.Sprintf("cloud: %s (HTTP 429)", e.Message)
}

// Client talks to the CLI API of the cloud. Token is empty for the device login, which needs none.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// NewClient returns a Client for a cloud address (see NormalizeURL). Its HTTP client has explicit timeouts and
// refuses to follow a redirect to another origin, so the token never goes anywhere but the address it was given for.
func NewClient(baseURL, token string) (*Client, error) {
	base, err := NormalizeURL(baseURL)
	if err != nil {
		return nil, err
	}

	return &Client{BaseURL: base, Token: token, HTTP: newHTTP()}, nil
}

// ClientFor returns the Client of a login: its address (KREIDA_CLOUD_URL wins) and token.
func ClientFor(c Config) (*Client, error) { return NewClient(c.URL, c.Token) }

func newHTTP() *http.Client {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.DialContext = (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	tr.TLSHandshakeTimeout = 10 * time.Second
	tr.ResponseHeaderTimeout = 20 * time.Second

	return &http.Client{Transport: tr, Timeout: RequestTimeout, CheckRedirect: sameOrigin}
}

// sameOrigin lets a redirect stay on the origin of the first request and nothing else.
func sameOrigin(req *http.Request, via []*http.Request) error {
	if len(via) > maxRedirects {
		return errors.New("too many redirects")
	}

	first := via[0].URL
	if req.URL.Scheme != first.Scheme || req.URL.Host != first.Host {
		return fmt.Errorf("refusing a redirect to %s://%s", req.URL.Scheme, req.URL.Host)
	}

	return nil
}

// ValidName checks a board name for the cloud: a local board name of at most 100 characters, and not a dot segment
// (it ends up in a URL path).
func ValidName(name string) error {
	if !paths.ValidName(name) || len(name) > 100 || name == "." || name == ".." {
		if name == "" {
			name = "(none)"
		}

		return fmt.Errorf("bad board name: %s", name)
	}

	return nil
}

// call sends one request. body is JSON (nil for none); a 2xx answer is decoded into out when out is not nil.
func (c *Client) call(ctx context.Context, method, path string, body []byte, out any) error {
	return c.send(ctx, method, path, body, out, false)
}

// send is call; errorFirst reads the text of a failure from the body's `error` before its `message` (the device
// token endpoint answers RFC 8628 codes there).
func (c *Client) send(ctx context.Context, method, path string, body []byte, out any, errorFirst bool) error {
	return c.do(ctx, method, path, "application/json", body, out, errorFirst)
}

// do is send with the content type of the body.
func (c *Client) do(ctx context.Context, method, path, contentType string, body []byte, out any, errorFirst bool) error {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}

	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rd)
	if err != nil {
		return fmt.Errorf("cloud.call: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", userAgent)

	if body != nil {
		req.Header.Set("Content-Type", contentType)
	}

	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("cloud.call: %w", err)
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return fmt.Errorf("cloud.call: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return failure(resp, data, errorFirst)
	}

	if out == nil || len(bytes.TrimSpace(data)) == 0 {
		return nil
	}

	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("cloud.call: bad answer from %s: %w", path, err)
	}

	return nil
}

// failure turns an error answer into an APIError, or a RateLimitError for a 429. The text is the body's `message`
// (the cloud's responder shape), else its `error`, else the status text.
func failure(resp *http.Response, data []byte, errorFirst bool) error {
	msg := errorText(data, errorFirst)
	if msg == "" {
		msg = strings.ToLower(http.StatusText(resp.StatusCode))
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return &RateLimitError{RetryAfter: retryAfter(resp.Header.Get("Retry-After")), Message: msg}
	}

	return &APIError{Status: resp.StatusCode, Message: msg}
}

// errorText reads the text of an error body: `message` then `error`, or `error` then `message` when errorFirst (the
// device login keeps RFC 8628's {"error": "authorization_pending"}).
func errorText(data []byte, errorFirst bool) string {
	var b struct {
		Message string `json:"message"`
		Error   string `json:"error"`
	}

	if json.Unmarshal(data, &b) != nil {
		return ""
	}

	if errorFirst {
		return cmp.Or(b.Error, b.Message)
	}

	return cmp.Or(b.Message, b.Error)
}

// retryAfter reads a Retry-After header: seconds, or an HTTP date. Zero when absent or unreadable.
func retryAfter(h string) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}

	if n, err := strconv.ParseFloat(h, 64); err == nil && n >= 0 {
		return time.Duration(n * float64(time.Second))
	}

	if t, err := http.ParseTime(h); err == nil {
		return max(0, time.Until(t))
	}

	return 0
}

// escape makes a board name safe for a URL path.
func escape(name string) string { return url.PathEscape(name) }

// Whoami asks which owner the token belongs to. It is also the check that a pasted token works.
func (c *Client) Whoami(ctx context.Context) (Whoami, error) {
	var w Whoami

	return w, c.call(ctx, http.MethodGet, "/api/cli/whoami", nil, &w)
}

// Boards lists the owner's published boards.
func (c *Client) Boards(ctx context.Context) ([]Published, error) {
	var out []Published

	return out, c.call(ctx, http.MethodGet, "/api/cli/boards", nil, &out)
}

// Publish uploads a board (see Snapshot) and returns it as the cloud now keeps it. A first publish makes its first
// link.
func (c *Client) Publish(ctx context.Context, name string, body []byte) (Published, error) {
	if err := ValidName(name); err != nil {
		return Published{}, err
	}

	var p Published

	return p, c.call(ctx, http.MethodPut, "/api/cli/boards/"+escape(name), body, &p)
}

// PushLive replaces the live values of a published board.
func (c *Client) PushLive(ctx context.Context, name string, values map[string]LiveVal) error {
	if err := ValidName(name); err != nil {
		return err
	}

	if values == nil {
		values = map[string]LiveVal{}
	}

	body, err := json.Marshal(map[string]any{"values": values})
	if err != nil {
		return fmt.Errorf("cloud.PushLive: %w", err)
	}

	return c.call(ctx, http.MethodPut, "/api/cli/boards/"+escape(name)+"/live", body, nil)
}

// pngSignature starts every PNG; the cloud checks the leading bytes of a thumbnail for it.
const pngSignature = "\x89PNG\r\n\x1a\n"

// PutThumb replaces the thumbnail of a published board with a PNG of at most MaxThumb bytes. A board that is not
// published is an *APIError with status 404; a picture the cloud finds too large, 413.
func (c *Client) PutThumb(ctx context.Context, name string, png []byte) error {
	if err := ValidName(name); err != nil {
		return err
	}

	switch {
	case !bytes.HasPrefix(png, []byte(pngSignature)):
		return noRetry{errors.New("cloud.PutThumb: not a PNG")}
	case len(png) > MaxThumb:
		return noRetry{fmt.Errorf("cloud.PutThumb: %d KiB is over the %d KiB limit", (len(png)+1023)/1024, MaxThumb>>10)}
	}

	return c.do(ctx, http.MethodPut, "/api/cli/boards/"+escape(name)+"/thumb", "image/png", png, nil, false)
}

// Unpublish removes a board and all its links from the cloud. A board that is not published is an *APIError with
// status 404.
func (c *Client) Unpublish(ctx context.Context, name string) error {
	if err := ValidName(name); err != nil {
		return err
	}

	return c.call(ctx, http.MethodDelete, "/api/cli/boards/"+escape(name), nil, nil)
}

// allQuery is the query that asks for the resolved comments too.
func allQuery(all bool) string {
	if all {
		return "?all=1"
	}

	return ""
}

// Comments lists the comments clients left on a published board: the open ones, or all of them with their resolved
// time. A comment is a notes.Note whose id starts with c_ and whose author the cloud sets. A board that is not
// published is an *APIError with status 404.
func (c *Client) Comments(ctx context.Context, name string, all bool) ([]notes.Note, error) {
	if err := ValidName(name); err != nil {
		return nil, err
	}

	var out []notes.Note

	return out, c.call(ctx, http.MethodGet, "/api/cli/boards/"+escape(name)+"/comments"+allQuery(all), nil, &out)
}

// AllComments lists the comments on all the owner's boards, each with its board (the name it was published under).
func (c *Client) AllComments(ctx context.Context, all bool) ([]notes.Note, error) {
	var out []notes.Note

	return out, c.call(ctx, http.MethodGet, "/api/cli/comments"+allQuery(all), nil, &out)
}

// ResolveComment closes a comment on a published board. Closing a closed one is fine.
func (c *Client) ResolveComment(ctx context.Context, name, id string) error {
	if err := ValidName(name); err != nil {
		return err
	}

	if rest, ok := strings.CutPrefix(id, "c_"); !ok || rest == "" {
		return fmt.Errorf("bad comment id: %s", id)
	}

	return c.call(ctx, http.MethodPost, "/api/cli/boards/"+escape(name)+"/comments/"+escape(id)+"/resolve", nil, nil)
}

// IsStatus reports whether err is a failed call with the given HTTP status.
func IsStatus(err error, status int) bool {
	var api *APIError
	if errors.As(err, &api) {
		return api.Status == status
	}

	var rl *RateLimitError

	return status == http.StatusTooManyRequests && errors.As(err, &rl)
}
