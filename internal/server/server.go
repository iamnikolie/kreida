// Package server is the HTTP side of kreida: the editor API for this machine's own pages, read-only share links,
// live change events (SSE) and the embedded page.
package server

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/iamnikolie/kreida/internal/history"
	"github.com/iamnikolie/kreida/internal/live"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/share"
)

// maxBody bounds a request body; the page sends board text and small JSON.
const maxBody = 16 << 20

// Config says where the server keeps boards and what it serves.
type Config struct {
	Paths paths.Paths
	// Port is the port browsers use to reach the server; it is part of the Host/Origin check. Zero means "the
	// port of the listener passed to Serve".
	Port int
	// LAN listens on all interfaces; other machines then reach share links and the page assets only.
	LAN bool
	// Dist is the built page: index.html at its root. Nil or empty serves a "page not built" message.
	Dist fs.FS
	Log  *slog.Logger
	// Thumb draws the thumbnails of the boards published to kreida cloud; nil means headless Chrome showing Dist.
	Thumb cloud.Renderer
}

// Server serves boards from a directory.
type Server struct {
	cfg    Config
	log    *slog.Logger
	hist   history.Store
	shares share.Store
	live   *live.Runner
	// cloud pushes the boards published to kreida cloud; it does nothing while cloud.json has no token.
	cloud *cloud.Syncer

	editMu sync.Mutex // one board edit at a time: read-modify-write of a file must not interleave

	mu        sync.Mutex
	loopHosts map[string]bool
	clients   map[*client]struct{}
	pending   map[string]struct{}
	timer     *time.Timer
	done      chan struct{}
	closeOnce sync.Once
	stopWatch func()

	// pendingNotes are the boards whose notes file changed: the page hears of them, no snapshot and no cloud push
	// follow. Guarded by mu like pending.
	pendingNotes map[string]struct{}
}

// New prepares a server: it makes sure the boards directory exists, saves a first history version of every board and
// starts watching the directory for changes. With a kreida cloud login it also keeps the published boards in step.
func New(cfg Config) (*Server, error) {
	if err := cfg.Paths.Ensure(); err != nil {
		return nil, fmt.Errorf("server.New: %w", err)
	}

	lg := cfg.Log
	if lg == nil {
		lg = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	runner := live.New(cfg.Paths)

	sy := cloud.NewSyncer(cfg.Paths, runner, lg)

	sy.Thumb = cfg.Thumb
	if sy.Thumb == nil {
		sy.Thumb = cloud.ChromeThumbs(cfg.Paths, cfg.Dist)
	}

	s := &Server{
		cfg: cfg, log: lg,
		hist:    history.Store{Paths: cfg.Paths},
		shares:  share.Store{Paths: cfg.Paths},
		live:    runner,
		cloud:   sy,
		clients: map[*client]struct{}{},
		pending: map[string]struct{}{},
		done:    make(chan struct{}),

		pendingNotes: map[string]struct{}{},
	}

	s.setPort(cfg.Port)

	entries, err := os.ReadDir(cfg.Paths.Dir)
	if err != nil {
		return nil, fmt.Errorf("server.New: %w", err)
	}

	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".yaml"); ok {
			if _, err := s.hist.Snapshot(name); err != nil {
				s.log.Warn("history snapshot", "board", name, "err", err)
			}
		}
	}

	stop, err := s.watch()
	if err != nil {
		return nil, err
	}

	s.stopWatch = stop
	s.cloud.Start()

	return s, nil
}

func (s *Server) setPort(port int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.loopHosts = map[string]bool{
		fmt.Sprintf("127.0.0.1:%d", port): true,
		fmt.Sprintf("localhost:%d", port): true,
		fmt.Sprintf("[::1]:%d", port):     true,
	}
}

// Close stops watching and ends every event stream.
func (s *Server) Close() {
	s.closeOnce.Do(func() {
		close(s.done)

		if s.stopWatch != nil {
			s.stopWatch()
		}

		s.cloud.Close()
	})
}

// Serve serves on the listener until ctx is done, then shuts down.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	if s.cfg.Port == 0 {
		if addr, ok := ln.Addr().(*net.TCPAddr); ok {
			s.setPort(addr.Port)
		}
	}

	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return fmt.Errorf("server.Serve: %w", err)
	case <-ctx.Done():
	}

	s.Close()

	sctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := srv.Shutdown(sctx); err != nil {
		_ = srv.Close()
	}

	if err := <-errc; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("server.Serve: %w", err)
	}

	return nil
}

// Handler is the whole HTTP surface.
func (s *Server) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.log.Error("handler panic", "path", r.URL.Path, "panic", rec)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": fmt.Sprint(rec)})
			}
		}()

		parts := splitPath(r)

		switch {
		case at(parts, 0) == "api" && at(parts, 1) == "share":
			s.shared(w, r, parts)
		case at(parts, 0) == "s" || at(parts, 0) == "assets":
			s.static(w, r)
		case !s.local(r):
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "only share links are reachable from other machines"})
		case at(parts, 0) == "api" && at(parts, 1) == "events":
			s.events(w, r, "", false)
		case at(parts, 0) == "api" && at(parts, 1) == "boards":
			s.api(w, r, parts)
		case at(parts, 0) == "api" && at(parts, 1) == "health":
			writeJSON(w, http.StatusOK, health{OK: true, Dir: s.cfg.Paths.Dir, PID: os.Getpid(), LAN: s.cfg.LAN})
		default:
			s.static(w, r)
		}
	})
}

// health is the body of /api/health; `kreida up` reads it to see which mode the running server is in.
type health struct {
	OK  bool   `json:"ok"`
	Dir string `json:"dir"`
	PID int    `json:"pid"`
	LAN bool   `json:"lan"`
}

// at returns the i-th path segment, or "" when there is none.
func at(parts []string, i int) string {
	if i < len(parts) {
		return parts[i]
	}

	return ""
}

// splitPath splits the path as sent (percent-encoding intact, dot segments resolved) into its non-empty segments.
func splitPath(r *http.Request) []string {
	var out []string

	for _, seg := range strings.Split(r.URL.EscapedPath(), "/") {
		switch strings.ToLower(seg) {
		case "":
		case ".", "%2e":
		case "..", ".%2e", "%2e.", "%2e%2e":
			if len(out) > 0 {
				out = out[:len(out)-1]
			}
		default:
			out = append(out, seg)
		}
	}

	return out
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(v); err != nil {
		buf.Reset()
		buf.WriteString(`{"error":"cannot encode response"}`)

		code = http.StatusInternalServerError
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(bytes.TrimSuffix(buf.Bytes(), []byte("\n")))
}

func errJSON(w http.ResponseWriter, code int, err error) {
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func exists(file string) bool {
	_, err := os.Stat(file)

	return err == nil
}

func readBody(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	b, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	return b, nil
}

// cleanStaticPath turns a URL path into a name inside the page's file system.
func cleanStaticPath(p string) string {
	return strings.TrimPrefix(path.Clean("/"+p), "/")
}
