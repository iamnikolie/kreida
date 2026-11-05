package server

import (
	"net/http"
	"os"

	"github.com/iamnikolie/kreida/internal/images"
	"github.com/iamnikolie/kreida/internal/paths"
)

// shared is /api/share/<token>[/events|/live|/images/<sha256>]: one board, read-only, reachable from other machines in
// LAN mode.
func (s *Server) shared(w http.ResponseWriter, r *http.Request, parts []string) {
	token, sub := at(parts, 2), at(parts, 3)

	name := s.shares.BoardOf(token)
	if name == "" || !paths.ValidName(name) || !exists(s.cfg.Paths.BoardFile(name)) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "link expired"})

		return
	}

	switch {
	case sub == "events":
		s.events(w, r, name, true)
	case sub == "live" && r.Method == http.MethodGet:
		// clients see values, never the commands or URLs behind them
		vs, err := s.live.Values(name)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, http.StatusOK, vs.WithoutSource())
	case sub == "images" && (r.Method == http.MethodGet || r.Method == http.MethodHead):
		s.image(w, r, name, at(parts, 4), images.Public)
	case sub != "" || r.Method != http.MethodGet:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "read-only"})
	default:
		if _, err := os.Stat(s.cfg.Paths.BoardFile(name)); err != nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "link expired"})

			return
		}

		writeJSON(w, http.StatusOK, s.boardResponse(name, images.Public))
	}
}
