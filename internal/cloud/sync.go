package cloud

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/paths"
)

const (
	// Debounce is how long a change to a board waits for more changes before it is pushed.
	Debounce = 300 * time.Millisecond

	liveTick    = time.Second
	idleLive    = 10 * time.Second
	firstRetry  = 5 * time.Second
	maxBackoff  = 5 * time.Minute
	minLiveWait = time.Second
)

// ErrNotLoggedIn is returned when there is no token to publish with.
var ErrNotLoggedIn = errors.New("requires login to your own configured compatible server")

// noRetry marks a failure that pushing the same thing again cannot fix: a board that does not parse, a refused token.
type noRetry struct{ error }

func (e noRetry) Unwrap() error { return e.error }

// Syncer keeps the published boards of the login in step with their files. Whoever watches the files (the server,
// `kreida publish --watch`) calls Changed; the Syncer pushes the board after a quiet moment, and resolves the live
// cards of published boards on their schedule and pushes the values when they change. A failed push is logged and
// tried again later, never fatal. cloud.json is read on every push, so publish, unpublish and logout from another
// process take effect at once; without a token the Syncer does nothing.
type Syncer struct {
	Paths paths.Paths
	Live  *live.Runner
	Log   *slog.Logger
	// Only limits the Syncer to one board; empty means every published board.
	Only string
	// Debounce, Tick and Backoff are the quiet time before a push, the pace of the live loop and the first retry
	// delay; zero means Debounce, one second and five seconds.
	Debounce, Tick, Backoff time.Duration
	// Now is the clock of the live schedule; nil means time.Now. It should be the clock of Live.
	Now func() time.Time
	// Thumb draws the thumbnails of published boards; nil means none. ThumbEvery is the least time between two
	// thumbnails of a board; zero means ThumbEvery (five minutes).
	Thumb      Renderer
	ThumbEvery time.Duration

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	pushMu  sync.Mutex // one board upload at a time, so an old body never lands after a newer one
	thumbMu sync.Mutex // one thumbnail at a time: each is a Chrome, and it must not hold up the pushes

	mu     sync.Mutex
	closed bool
	timer  *time.Timer
	due    map[string]time.Time // boards waiting for their push
	sent   map[string][32]byte  // hash of the last body the cloud accepted, by board
	fails  map[string]int       // failed board pushes in a row
	states map[string]*liveState
	thumbs map[string]*thumbState
	cfgErr string
}

// liveState is the live loop's memory of one published board.
type liveState struct {
	next  time.Time // when to resolve the board's cards again
	sig   string    // what the cloud has: ids with their value, error and blocked flag
	fails int
	busy  bool
}

// NewSyncer returns a Syncer for the login in p. It does nothing until Changed or Start is called.
func NewSyncer(p paths.Paths, lv *live.Runner, log *slog.Logger) *Syncer {
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Syncer{
		Paths: p, Live: lv, Log: log, ctx: ctx, cancel: cancel,
		due: map[string]time.Time{}, sent: map[string][32]byte{}, fails: map[string]int{},
		states: map[string]*liveState{}, thumbs: map[string]*thumbState{},
	}
}

func (s *Syncer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}

	return time.Now()
}

func orDefault(d, def time.Duration) time.Duration {
	if d > 0 {
		return d
	}

	return def
}

// Start brings every published board up to date once and starts resolving their live cards.
func (s *Syncer) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	s.wg.Add(1)

	go s.run()
}

// Close stops the Syncer and waits for what it has in flight.
func (s *Syncer) Close() {
	s.mu.Lock()
	s.closed = true

	if s.timer != nil {
		s.timer.Stop()
	}

	for _, th := range s.thumbs {
		if th.timer != nil {
			th.timer.Stop()
		}
	}

	s.mu.Unlock()

	s.cancel()
	s.wg.Wait()
}

// config reads cloud.json. A file that cannot be read is logged once per message and counts as no login.
func (s *Syncer) config() (Config, bool) {
	c, err := Load(s.Paths)

	s.mu.Lock()
	defer s.mu.Unlock()

	if err != nil {
		if msg := err.Error(); msg != s.cfgErr {
			s.cfgErr = msg

			s.Log.Warn("cloud config", "err", err)
		}

		return Config{}, false
	}

	s.cfgErr = ""

	return c, true
}

// active says whether the Syncer looks after the board: it is the one asked for and it is published by a login.
func (s *Syncer) active(cfg Config, name string) bool {
	return cfg.LoggedIn() && cfg.Has(name) && (s.Only == "" || s.Only == name) && ValidName(name) == nil
}

// client is the Client of the login in cloud.json, read now.
func (s *Syncer) client() (*Client, error) {
	cfg, err := Load(s.Paths)
	if err != nil {
		return nil, noRetry{err}
	}

	return clientOf(cfg)
}

// clientOf is the Client of a login; a failure to make one is not worth a retry.
func clientOf(cfg Config) (*Client, error) {
	if !cfg.LoggedIn() {
		return nil, noRetry{ErrNotLoggedIn}
	}

	cl, err := ClientFor(cfg)
	if err != nil {
		return nil, noRetry{err}
	}

	return cl, nil
}

// ----- boards

// Publish uploads the board now and records it as published, so the Syncer keeps it up to date from here on. It
// answers the board with its links.
func (s *Syncer) Publish(ctx context.Context, name string) (Published, error) {
	cl, err := s.client()
	if err != nil {
		return Published{}, errors.Unwrap(err)
	}

	pub, _, err := s.push(ctx, cl, name, true)
	if err != nil {
		return Published{}, err
	}

	// read again: another process may have changed the list while the upload ran
	cfg, err := Load(s.Paths)
	if err != nil {
		return pub, err
	}

	return pub, cfg.Add(name).Save(s.Paths)
}

// push uploads the board unless the cloud already has this very body (and force is not set). It reports whether it
// uploaded.
func (s *Syncer) push(ctx context.Context, cl *Client, name string, force bool) (Published, bool, error) {
	s.pushMu.Lock()
	defer s.pushMu.Unlock()

	up, err := Snapshot(s.Paths, name)
	if err != nil {
		return Published{}, false, noRetry{err}
	}

	body, err := up.JSON()
	if err != nil {
		return Published{}, false, noRetry{err}
	}

	sum := sha256.Sum256(body)

	s.mu.Lock()
	same := s.sent[name] == sum
	s.mu.Unlock()

	if same && !force {
		return Published{}, false, nil
	}

	pub, err := cl.Publish(ctx, name, body)
	if err != nil {
		return Published{}, false, err
	}

	s.mu.Lock()
	s.sent[name] = sum

	// the cloud may drop the live values of a board when it gets a new body: send them again
	if st := s.states[name]; st != nil {
		st.sig, st.next = "", time.Time{}
	}

	s.mu.Unlock()

	return pub, true, nil
}

// Changed says a board's file or positions changed. It never blocks: the push happens after the quiet moment, and
// only for a board that is published.
func (s *Syncer) Changed(name string) {
	cfg, ok := s.config()
	if !ok || !s.active(cfg, name) {
		return
	}

	s.schedule(name, time.Now().Add(orDefault(s.Debounce, Debounce)), true)
}

// schedule puts the board in the queue to be pushed at `at`. A new change (restart) moves a board's turn to `at`; a
// retry only takes a turn the board does not have yet.
func (s *Syncer) schedule(name string, at time.Time, restart bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	if _, queued := s.due[name]; queued && !restart {
		return
	}

	s.due[name] = at

	s.arm()
}

// arm points the timer at the earliest turn. The caller holds mu.
func (s *Syncer) arm() {
	var first time.Time

	for _, at := range s.due {
		if first.IsZero() || at.Before(first) {
			first = at
		}
	}

	if first.IsZero() {
		return
	}

	d := max(time.Until(first), 0)

	if s.timer == nil {
		s.timer = time.AfterFunc(d, s.flush)

		return
	}

	s.timer.Reset(d)
}

// flush pushes the boards whose turn has come.
func (s *Syncer) flush() {
	s.mu.Lock()

	if s.closed {
		s.mu.Unlock()

		return
	}

	s.wg.Add(1)
	defer s.wg.Done()

	var names []string

	now := time.Now()

	for name, at := range s.due {
		if !at.After(now) {
			names = append(names, name)

			delete(s.due, name)
		}
	}

	s.arm()
	s.mu.Unlock()

	slices.Sort(names)

	for _, name := range names {
		if s.ctx.Err() != nil {
			return
		}

		s.pushQuiet(name)
	}
}

// pushQuiet is a push from the queue: it reads the login again, and a failure is logged and queued for a retry.
func (s *Syncer) pushQuiet(name string) {
	cfg, ok := s.config()
	if !ok || !s.active(cfg, name) {
		return
	}

	cl, err := clientOf(cfg)
	if err == nil {
		var uploaded bool

		_, uploaded, err = s.push(s.ctx, cl, name, false)
		if err == nil {
			s.mu.Lock()
			delete(s.fails, name)
			s.mu.Unlock()

			if uploaded {
				s.Log.Info("cloud push", "board", name)
				s.thumbLater(name)
			}

			return
		}
	}

	if s.ctx.Err() != nil {
		return
	}

	s.Log.Warn("cloud push", "board", name, "err", err)

	if !retryable(err) {
		return
	}

	s.mu.Lock()
	s.fails[name]++
	n := s.fails[name]
	s.mu.Unlock()

	s.schedule(name, time.Now().Add(s.delay(n, err)), false)
}

// retryable says whether pushing again later may work: a network failure, a busy or failing cloud.
func retryable(err error) bool {
	var nr noRetry
	if errors.As(err, &nr) {
		return false
	}

	var api *APIError
	if errors.As(err, &api) {
		return api.Status >= 500 || api.Status == http.StatusRequestTimeout
	}

	return true
}

// delay is how long to wait after the n-th failure in a row: doubling from the first retry up to five minutes, and
// never less than what the cloud asked for in Retry-After.
func (s *Syncer) delay(n int, err error) time.Duration {
	d := orDefault(s.Backoff, firstRetry)

	for i := 1; i < n && d < maxBackoff; i++ {
		d *= 2
	}

	d = min(d, maxBackoff)

	var rl *RateLimitError
	if errors.As(err, &rl) {
		d = max(d, rl.RetryAfter)
	}

	return d
}

// ----- live values

func (s *Syncer) run() {
	defer s.wg.Done()

	// bring the published boards up to date: they may have changed while nobody was watching
	if cfg, ok := s.config(); ok && cfg.LoggedIn() {
		for _, name := range cfg.Published {
			if s.active(cfg, name) {
				s.schedule(name, time.Now().Add(orDefault(s.Debounce, Debounce)), true)
			}
		}
	}

	t := time.NewTicker(orDefault(s.Tick, liveTick))
	defer t.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-t.C:
			s.tick()
		}
	}
}

// tick starts a resolve for every published board whose cards are due.
func (s *Syncer) tick() {
	cfg, ok := s.config()

	var names []string

	if ok && cfg.LoggedIn() {
		for _, name := range cfg.Published {
			if s.active(cfg, name) {
				names = append(names, name)
			}
		}
	}

	now := s.now()

	s.mu.Lock()

	for name := range s.states {
		if !slices.Contains(names, name) {
			delete(s.states, name)
			delete(s.sent, name)
		}
	}

	for name, th := range s.thumbs {
		if !slices.Contains(names, name) && !th.busy {
			if th.timer != nil {
				th.timer.Stop()
			}

			delete(s.thumbs, name)
		}
	}

	var start []string

	for _, name := range names {
		st := s.states[name]
		if st == nil {
			st = &liveState{}
			s.states[name] = st
		}

		if !st.busy && !now.Before(st.next) {
			st.busy = true

			start = append(start, name)
		}
	}

	s.mu.Unlock()

	for _, name := range start {
		// not waited for by Close: a command can run for its whole timeout, and nothing it does outlives the Syncer
		go s.liveOnce(name)
	}
}

// liveOnce resolves a board's live cards and pushes the values if they differ from what the cloud has.
func (s *Syncer) liveOnce(name string) {
	s.mu.Lock()
	st := s.states[name]
	s.mu.Unlock()

	if st == nil {
		return
	}

	next := s.pushValues(name, st)

	s.mu.Lock()
	st.busy = false
	st.next = next
	s.mu.Unlock()
}

// pushValues does one round for a board and says when the next one is due.
func (s *Syncer) pushValues(name string, st *liveState) time.Time {
	now := s.now()

	vs, err := s.Live.Values(name)
	if err != nil || s.ctx.Err() != nil {
		// the board does not parse or is gone: nothing to resolve until it changes
		return now.Add(idleLive)
	}

	vals := LiveValues(vs.WithoutSource())
	sig := signature(vals)

	s.mu.Lock()
	changed := sig != st.sig
	s.mu.Unlock()

	if changed {
		if err := s.pushLive(s.ctx, name, vals); err != nil {
			if s.ctx.Err() != nil {
				return now
			}

			s.mu.Lock()
			st.fails++
			n := st.fails
			s.mu.Unlock()

			s.Log.Warn("cloud live", "board", name, "err", err)

			return now.Add(s.delay(n, err))
		}

		s.mu.Lock()
		st.sig, st.fails = sig, 0
		s.mu.Unlock()

		s.Log.Debug("cloud live", "board", name, "cards", len(vals))
	}

	return now.Add(nextWait(vs))
}

// nextWait is the shortest refresh time among the cards that were resolved.
func nextWait(vs live.ValueSet) time.Duration {
	wait := idleLive

	found := false

	for _, v := range vs {
		if v.Every <= 0 {
			continue
		}

		d := time.Duration(v.Every * float64(time.Second))
		if !found || d < wait {
			wait, found = d, true
		}
	}

	return max(wait, minLiveWait)
}

// signature is what a set of values says apart from when it was taken: a refresh that finds the same values is not
// worth a call to the cloud. A board without live cards has the empty signature.
func signature(vals map[string]LiveVal) string {
	ids := make([]string, 0, len(vals))
	for id := range vals {
		ids = append(ids, id)
	}

	slices.Sort(ids)

	var b strings.Builder

	for _, id := range ids {
		v := vals[id]
		fmt.Fprintf(&b, "%q %t %q %q\n", id, v.Blocked, v.Value, v.Error)
	}

	return b.String()
}

// PushLive resolves the board's live cards now and uploads the values, and remembers them as what the cloud has.
func (s *Syncer) PushLive(ctx context.Context, name string) error {
	vs, err := s.Live.Values(name)
	if err != nil {
		return err
	}

	vals := LiveValues(vs.WithoutSource())

	if err := s.pushLive(ctx, name, vals); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	st := s.states[name]
	if st == nil {
		st = &liveState{}
		s.states[name] = st
	}

	st.sig = signature(vals)

	return nil
}

func (s *Syncer) pushLive(ctx context.Context, name string, vals map[string]LiveVal) error {
	cl, err := s.client()
	if err != nil {
		return err
	}

	return cl.PushLive(ctx, name, vals)
}
