package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/history"
	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/templates"
)

type boardItem struct {
	Name  string  `json:"name"`
	Title string  `json:"title"`
	Mtime float64 `json:"mtime"`
}

// boards lists the boards, most recently changed first.
func (s *Server) boards() []boardItem {
	out := []boardItem{}

	entries, err := os.ReadDir(s.cfg.Paths.Dir)
	if err != nil {
		return out
	}

	for _, e := range entries {
		name, ok := strings.CutSuffix(e.Name(), ".yaml")
		if !ok {
			continue
		}

		info, err := os.Stat(s.cfg.Paths.BoardFile(name))
		if err != nil {
			continue
		}

		item := boardItem{Name: name, Mtime: float64(info.ModTime().UnixNano()) / 1e6}

		if text, err := os.ReadFile(s.cfg.Paths.BoardFile(name)); err == nil {
			if b, err := board.Parse(string(text)); err == nil {
				item.Title = b.Title
			}
		}

		out = append(out, item)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].Mtime > out[j].Mtime })

	return out
}

type boardResp struct {
	Name  string          `json:"name"`
	Board *board.Board    `json:"board,omitempty"`
	Pos   json.RawMessage `json:"pos,omitempty"`
	Error string          `json:"error,omitempty"`
}

// boardResponse reads a board for the page: the normalized board and its card positions, or the parse error.
func (s *Server) boardResponse(name string) boardResp {
	text, err := os.ReadFile(s.cfg.Paths.BoardFile(name))
	if err != nil {
		return boardResp{Name: name, Error: err.Error()}
	}

	b, err := board.Parse(string(text))
	if err != nil {
		return boardResp{Name: name, Error: err.Error()}
	}

	return boardResp{Name: name, Board: b, Pos: board.ReadPos(s.cfg.Paths.PosFile(name))}
}

// api is /api/boards[/<name>[/<sub>[/<id>]]].
func (s *Server) api(w http.ResponseWriter, r *http.Request, parts []string) {
	name, sub, id := at(parts, 2), at(parts, 3), at(parts, 4)
	p := s.cfg.Paths

	if name == "" {
		writeJSON(w, http.StatusOK, s.boards())

		return
	}

	if !paths.ValidName(name) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad name"})

		return
	}

	file := p.BoardFile(name)

	if sub == "" && r.Method == http.MethodPost {
		s.editMu.Lock()
		defer s.editMu.Unlock()

		if exists(file) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "board " + name + " exists"})

			return
		}

		if err := os.WriteFile(file, []byte(templates.Default(name)), 0o644); err != nil {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})

		return
	}

	if !exists(file) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "no board " + name})

		return
	}

	switch {
	case sub == "undo" && r.Method == http.MethodPost:
		s.editMu.Lock()
		defer s.editMu.Unlock()

		if err := s.hist.Undo(name); err != nil {
			errJSON(w, http.StatusConflict, unwrapUndo(err))

			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case sub == "history" && r.Method == http.MethodGet:
		v, err := s.hist.List(name)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		if v == nil {
			v = []history.Version{}
		}

		writeJSON(w, http.StatusOK, v)
	case sub == "live" && r.Method == http.MethodGet:
		vs, err := s.live.Values(name)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, http.StatusOK, vs)
	case sub == "" && r.Method == http.MethodGet:
		writeJSON(w, http.StatusOK, s.boardResponse(name))
	case sub == "style" && r.Method == http.MethodPut:
		s.putStyle(w, r, name)
	case sub == "pos" && r.Method == http.MethodPut:
		body, err := readBody(w, r)
		if err != nil {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		if err := os.WriteFile(p.PosFile(name), body, 0o644); err != nil {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case sub == "pos" && r.Method == http.MethodDelete:
		if err := os.Remove(p.PosFile(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			errJSON(w, http.StatusInternalServerError, err)

			return
		}

		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	case sub == "node" && id != "" && r.Method == http.MethodPatch:
		s.patchNode(w, r, name, id)
	case sub == "notes" && r.Method == http.MethodGet:
		s.listNotes(w, name)
	case sub == "notes" && r.Method == http.MethodPost:
		s.addNote(w, r, name)
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

// unwrapUndo gives the plain message the API and CLI show for "nothing to undo".
func unwrapUndo(err error) error {
	if errors.Is(err, history.ErrNothingToUndo) {
		return history.ErrNothingToUndo
	}

	return err
}

// putStyle switches the board's style (sketch or the default clean) in the file, keeping the rest of the text.
func (s *Server) putStyle(w http.ResponseWriter, r *http.Request, name string) {
	body, err := readBody(w, r)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err)

		return
	}

	var req struct{ Style string }
	if err := json.Unmarshal(body, &req); err != nil {
		errJSON(w, http.StatusInternalServerError, err)

		return
	}

	var style *string
	if req.Style == "sketch" {
		sketch := "sketch"
		style = &sketch
	}

	s.editMu.Lock()
	defer s.editMu.Unlock()

	if err := s.rewrite(name, func(text string) (string, error) { return board.SetTop(text, "style", style) }); err != nil {
		errJSON(w, http.StatusInternalServerError, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// patchNode rewrites one card's title/text in the file, keeping the rest of the text.
func (s *Server) patchNode(w http.ResponseWriter, r *http.Request, name, rawID string) {
	id, err := url.PathUnescape(rawID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "URI malformed"})

		return
	}

	body, err := readBody(w, r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err)

		return
	}

	// a missing or null field is left alone
	var req struct{ Title, Text *string }
	if err := json.Unmarshal(body, &req); err != nil {
		errJSON(w, http.StatusBadRequest, err)

		return
	}

	title, text := req.Title, req.Text

	s.editMu.Lock()
	defer s.editMu.Unlock()

	if err := s.rewrite(name, func(t string) (string, error) { return board.PatchNode(t, id, title, text) }); err != nil {
		errJSON(w, http.StatusBadRequest, err)

		return
	}

	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// listNotes answers with the open notes the human wrote on the board for the agent.
func (s *Server) listNotes(w http.ResponseWriter, name string) {
	list, err := notes.List(s.cfg.Paths, name, false)
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err)

		return
	}

	writeJSON(w, http.StatusOK, list)
}

// addNote saves a note on a card or group of the board, or on the whole board when node is empty. Notes are for the
// agent: they stay out of the board's file and out of what a share link shows.
func (s *Server) addNote(w http.ResponseWriter, r *http.Request, name string) {
	body, err := readBody(w, r)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err)

		return
	}

	var req struct{ Node, Text string }
	if err := json.Unmarshal(body, &req); err != nil {
		errJSON(w, http.StatusBadRequest, err)

		return
	}

	text, err := notes.Clean(req.Text)
	if err != nil {
		errJSON(w, http.StatusBadRequest, err)

		return
	}

	if req.Node != "" {
		if err := s.hasNode(name, req.Node); err != nil {
			errJSON(w, http.StatusBadRequest, err)

			return
		}
	}

	n, err := notes.Add(s.cfg.Paths, name, req.Node, text, time.Now())
	if err != nil {
		errJSON(w, http.StatusInternalServerError, err)

		return
	}

	writeJSON(w, http.StatusCreated, n)
}

// hasNode checks that id is a card or group of the board as it parses now.
func (s *Server) hasNode(name, id string) error {
	text, err := os.ReadFile(s.cfg.Paths.BoardFile(name))
	if err != nil {
		return err
	}

	b, err := board.Parse(string(text))
	if err != nil {
		return err
	}

	for _, n := range b.Nodes {
		if n.ID == id {
			return nil
		}
	}

	for _, g := range b.Groups {
		if g.ID == id {
			return nil
		}
	}

	return fmt.Errorf("no card or group %s on this board", id)
}

// rewrite edits a board file with fn and writes the result back if it still parses as a board.
func (s *Server) rewrite(name string, fn func(text string) (string, error)) error {
	file := s.cfg.Paths.BoardFile(name)

	text, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	out, err := fn(string(text))
	if err != nil {
		return err
	}

	if _, err := board.Parse(out); err != nil {
		return err
	}

	return os.WriteFile(file, []byte(out), 0o644)
}
