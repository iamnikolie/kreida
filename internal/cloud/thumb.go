package cloud

import (
	"context"
	"errors"
	"io/fs"
	"time"

	"github.com/iamnikolie/kreida/internal/images"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/render"
)

// ThumbEvery is the least time between two thumbnails of one board.
const ThumbEvery = 5 * time.Minute

// ErrNoRenderer is returned when the Syncer has nothing to draw thumbnails with.
var ErrNoRenderer = errors.New("no thumbnail renderer")

// Renderer draws a board as a PNG of at most MaxThumb bytes, from the board as it is on this machine now.
type Renderer func(ctx context.Context, name string) ([]byte, error)

// ChromeThumbs is the Renderer of the boards in p: the page in dist shown by headless Chrome (render.Thumb). With no
// Chrome on the machine it fails with render.ErrNoChrome. The board is read like an upload reads it, so no live
// command or address reaches the picture.
func ChromeThumbs(p paths.Paths, dist fs.FS) Renderer {
	return func(ctx context.Context, name string) ([]byte, error) {
		chrome, err := render.FindChrome()
		if err != nil {
			return nil, err
		}

		up, err := Snapshot(p, name)
		if err != nil {
			return nil, err
		}

		pic, err := render.Thumb(ctx, chrome, dist, render.Data{Name: name, Board: up.Board, Pos: up.Pos, Images: images.Set{Files: up.Images}.DataURLs()})
		if err != nil {
			return nil, err
		}

		return render.Shrink(pic, MaxThumb)
	}
}

// thumbState is the Syncer's memory of one published board's thumbnail.
type thumbState struct {
	last  time.Time   // when the latest attempt started
	stale bool        // the board changed since the thumbnail was drawn
	timer *time.Timer // the attempt that waits for its turn
	busy  bool
	err   string // the message of the latest failure, so the same one is logged once
}

// thumbOf is the state of a board, made when there is none. The caller holds mu.
func (s *Syncer) thumbOf(name string) *thumbState {
	th := s.thumbs[name]
	if th == nil {
		th = &thumbState{}
		s.thumbs[name] = th
	}

	return th
}

func (s *Syncer) thumbEvery() time.Duration { return orDefault(s.ThumbEvery, ThumbEvery) }

// PushThumb draws the board and uploads the picture now. The board counts as having a fresh thumbnail from here on,
// so the Syncer does not draw it again before ThumbEvery has passed. A failure is only returned, never logged: the
// same message is not logged again when it comes back.
func (s *Syncer) PushThumb(ctx context.Context, name string) error {
	if s.Thumb == nil {
		return ErrNoRenderer
	}

	cl, err := s.client()
	if err != nil {
		return errors.Unwrap(err)
	}

	err = s.thumb(ctx, cl, name)
	if err != nil {
		s.mu.Lock()
		s.thumbOf(name).err = err.Error()
		s.mu.Unlock()
	}

	return err
}

// thumb draws a board and uploads it, one at a time. A failure to draw is not worth a retry.
func (s *Syncer) thumb(ctx context.Context, cl *Client, name string) error {
	s.thumbMu.Lock()
	defer s.thumbMu.Unlock()

	s.mu.Lock()
	th := s.thumbOf(name)
	th.last, th.stale = time.Now(), false
	s.mu.Unlock()

	png, err := s.Thumb(ctx, name)
	if err != nil {
		return noRetry{err}
	}

	return cl.PutThumb(ctx, name, png)
}

// thumbLater says a board's body changed, so its thumbnail is out of date. It never blocks. The picture is drawn
// after the quiet that ThumbEvery asks for, once for every change that came meanwhile.
func (s *Syncer) thumbLater(name string) {
	if s.Thumb == nil {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.closed {
		return
	}

	th := s.thumbOf(name)
	th.stale = true

	if th.timer == nil && !th.busy {
		s.thumbArm(name, th, th.last.Add(s.thumbEvery()))
	}
}

// thumbArm sets the board's attempt for `at`. The caller holds mu.
func (s *Syncer) thumbArm(name string, th *thumbState, at time.Time) {
	th.timer = time.AfterFunc(max(time.Until(at), 0), func() { s.thumbRun(name) })
}

// thumbRun is an attempt from the queue: it reads the login again, and a failure is logged.
func (s *Syncer) thumbRun(name string) {
	s.mu.Lock()

	th := s.thumbs[name]
	if s.closed || th == nil {
		s.mu.Unlock()

		return
	}

	s.wg.Add(1)
	defer s.wg.Done()

	th.timer, th.busy, th.stale = nil, true, false
	s.mu.Unlock()

	err := s.thumbOnce(name)

	s.mu.Lock()
	defer s.mu.Unlock()

	th.busy = false

	if s.closed || s.ctx.Err() != nil {
		return
	}

	// a failure that may pass (the network, a busy cloud) gets another go after the quiet; the rest waits for the
	// board to change
	if err != nil && retryable(err) {
		th.stale = true
	}

	if !th.stale || th.timer != nil {
		return
	}

	at := th.last.Add(s.thumbEvery())

	var rl *RateLimitError
	if errors.As(err, &rl) {
		if wait := time.Now().Add(rl.RetryAfter); wait.After(at) {
			at = wait
		}
	}

	s.thumbArm(name, th, at)
}

// thumbOnce draws and uploads one board if it is still published by a login, and logs the outcome.
func (s *Syncer) thumbOnce(name string) error {
	cfg, ok := s.config()
	if !ok || !s.active(cfg, name) {
		return nil
	}

	cl, err := clientOf(cfg)
	if err == nil {
		err = s.thumb(s.ctx, cl, name)
	}

	s.mu.Lock()
	th := s.thumbOf(name)
	seen := err != nil && th.err == err.Error()

	th.err = ""
	if err != nil {
		th.err = err.Error()
	}

	s.mu.Unlock()

	switch {
	case err == nil:
		s.Log.Info("cloud thumb", "board", name)
	case s.ctx.Err() != nil:
	case seen:
		s.Log.Debug("cloud thumb", "board", name, "err", err)
	default:
		s.Log.Warn("cloud thumb", "board", name, "err", err)
	}

	return err
}
