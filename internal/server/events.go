package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	debounce  = 60 * time.Millisecond
	pingEvery = 20 * time.Second
)

// client is one open event stream. only is the board it may hear about; all streams hear everything.
type client struct {
	ch   chan string
	only string
	all  bool
}

// watch watches the boards directory. Changes to a board file or its position file are collected for 60 ms, then
// each changed board gets a history snapshot and an event, and the cloud syncer hears of it. A board whose notes
// changed gets the event only: notes are no version of the board and never go to the cloud.
func (s *Server) watch() (func(), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("server.watch: %w", err)
	}

	if err := w.Add(s.cfg.Paths.Dir); err != nil {
		_ = w.Close()

		return nil, fmt.Errorf("server.watch: %w", err)
	}

	go func() {
		for {
			select {
			case ev, ok := <-w.Events:
				if !ok {
					return
				}

				if ev.Op == fsnotify.Chmod {
					continue
				}

				s.changed(filepath.Base(ev.Name))
			case err, ok := <-w.Errors:
				if !ok {
					return
				}

				s.log.Warn("watch", "err", err)
			case <-s.done:
				return
			}
		}
	}()

	return func() { _ = w.Close() }, nil
}

// changed records a changed file and (re)starts the debounce timer.
func (s *Server) changed(file string) {
	if file == "" || file == "." {
		return
	}

	// the temp file a notes write is renamed from
	if strings.HasSuffix(file, ".tmp") {
		return
	}

	// one suffix at most: "x.pos.json", "x.notes.json" and "x.yaml" all belong to board x
	name, isNotes := file, false
	if n, ok := strings.CutSuffix(file, ".pos.json"); ok {
		name = n
	} else if n, ok := strings.CutSuffix(file, ".notes.json"); ok {
		name, isNotes = n, true
	} else if n, ok := strings.CutSuffix(file, ".yaml"); ok {
		name = n
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if isNotes {
		s.pendingNotes[name] = struct{}{}
	} else {
		s.pending[name] = struct{}{}
	}

	if s.timer != nil {
		s.timer.Stop()
	}

	s.timer = time.AfterFunc(debounce, s.flush)
}

func (s *Server) flush() {
	s.mu.Lock()
	names := make([]string, 0, len(s.pending))

	for n := range s.pending {
		names = append(names, n)
	}

	// a board that changed too is told below with its snapshot
	var noted []string

	for n := range s.pendingNotes {
		if _, ok := s.pending[n]; !ok {
			noted = append(noted, n)
		}
	}

	s.pending = map[string]struct{}{}
	s.pendingNotes = map[string]struct{}{}
	s.mu.Unlock()

	for _, n := range names {
		if _, err := s.hist.Snapshot(n); err != nil {
			s.log.Warn("history snapshot", "board", n, "err", err)
		}

		s.send(n)
	}

	for _, n := range noted {
		s.send(n)
	}

	// after every stream has its event; Changed only queues the board, the push happens later on the syncer's own
	// goroutine
	for _, n := range names {
		s.cloud.Changed(n)
	}
}

// send tells every stream that may hear about the board that it changed.
func (s *Server) send(name string) {
	data, _ := json.Marshal(map[string]string{"name": name})
	msg := "data: " + string(data) + "\n\n"

	s.mu.Lock()
	defer s.mu.Unlock()

	for c := range s.clients {
		if !c.all && c.only != name {
			continue
		}

		select {
		case c.ch <- msg:
		default: // a stream that cannot keep up misses an event; the page refetches on the next one
		}
	}
}

// events streams change events as SSE. Share links pass the board they may hear about.
func (s *Server) events(w http.ResponseWriter, r *http.Request, only string, scoped bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)

		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	_, _ = w.Write([]byte(": hi\n\n"))
	flusher.Flush()

	c := &client{ch: make(chan string, 16), only: only, all: !scoped}

	s.mu.Lock()
	s.clients[c] = struct{}{}
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		delete(s.clients, c)
		s.mu.Unlock()
	}()

	ping := time.NewTicker(pingEvery)
	defer ping.Stop()

	for {
		select {
		case msg := <-c.ch:
			_, _ = w.Write([]byte(msg))
			flusher.Flush()
		case <-ping.C:
			_, _ = w.Write([]byte(": ping\n\n"))
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-s.done:
			return
		}
	}
}
