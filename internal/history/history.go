// Package history keeps every distinct saved version of a board and walks back through them (`kreida undo`). Each
// change of version also drops the dragged card positions the change made stale.
//
// Versions live in <home>/history/<board>/<unix ms>.yaml, 200 at most; the oldest are dropped first.
package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/iamnikolie/kreida/internal/paths"
)

// Keep is how many versions of a board are kept.
const Keep = 200

// ErrNothingToUndo is returned by Undo when there is no earlier version.
var ErrNothingToUndo = errors.New("nothing to undo")

// Version is one saved version.
type Version struct {
	ID   string `json:"id"`
	Time string `json:"time"`
}

// Store reads and writes the history of boards under a Paths root.
type Store struct {
	Paths paths.Paths
	// Now is the clock; nil means time.Now. Tests set it to get distinct version names.
	Now func() time.Time
}

func (s Store) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}

	return time.Now()
}

func (s Store) versions(name string) ([]string, error) {
	entries, err := os.ReadDir(s.Paths.HistoryDir(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("history.versions: %w", err)
	}

	var out []string

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".yaml") {
			out = append(out, e.Name())
		}
	}

	sort.Strings(out)

	return out, nil
}

// Snapshot saves the board's current text as a new version unless it equals the latest one. It reports whether a
// version was written; a board without a file has nothing to save. A new version forgets the dragged positions it
// displaces (see board.Displaced), compared with the newest earlier version that is a valid board.
func (s Store) Snapshot(name string) (bool, error) {
	text, err := os.ReadFile(s.Paths.BoardFile(name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("history.Snapshot: %w", err)
	}

	v, err := s.versions(name)
	if err != nil {
		return false, err
	}

	dir := s.Paths.HistoryDir(name)

	if len(v) > 0 {
		last, err := os.ReadFile(filepath.Join(dir, v[len(v)-1]))
		if err == nil && string(last) == string(text) {
			return false, nil
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false, fmt.Errorf("history.Snapshot: %w", err)
	}

	// Versions are named by the millisecond they were saved in. Two saves in one millisecond must not overwrite each
	// other, and names must keep sorting in save order.
	ms := s.now().UnixMilli()

	if len(v) > 0 {
		if last, err := strconv.ParseInt(strings.TrimSuffix(v[len(v)-1], ".yaml"), 10, 64); err == nil && ms <= last {
			ms = last + 1
		}
	}

	file := strconv.FormatInt(ms, 10) + ".yaml"
	if err := os.WriteFile(filepath.Join(dir, file), text, 0o644); err != nil {
		return false, fmt.Errorf("history.Snapshot: %w", err)
	}

	for _, old := range v[:max(0, len(v)+1-Keep)] {
		if err := os.Remove(filepath.Join(dir, old)); err != nil {
			return true, fmt.Errorf("history.Snapshot: %w", err)
		}
	}

	if _, err := os.Stat(s.Paths.PosFile(name)); err == nil && len(v) > 0 {
		return true, s.dropDisplaced(name, s.previous(name, v), string(text))
	}

	return true, nil
}

// List returns the saved versions, oldest first.
func (s Store) List(name string) ([]Version, error) {
	v, err := s.versions(name)
	if err != nil {
		return nil, err
	}

	out := make([]Version, 0, len(v))

	for _, f := range v {
		id := strings.TrimSuffix(f, ".yaml")
		ms, _ := strconv.ParseInt(id, 10, 64)
		out = append(out, Version{ID: id, Time: ISO(time.UnixMilli(ms))})
	}

	return out, nil
}

// Undo restores the version before the latest and drops the latest, so repeated undo keeps walking back. The current
// text is saved first, so nothing typed since the last version is lost.
func (s Store) Undo(name string) error {
	if _, err := s.Snapshot(name); err != nil {
		return err
	}

	v, err := s.versions(name)
	if err != nil {
		return err
	}

	if len(v) < 2 {
		return ErrNothingToUndo
	}

	dir := s.Paths.HistoryDir(name)

	prev, err := os.ReadFile(filepath.Join(dir, v[len(v)-2]))
	if err != nil {
		return fmt.Errorf("history.Undo: %w", err)
	}

	latest, err := os.ReadFile(filepath.Join(dir, v[len(v)-1]))
	if err != nil {
		return fmt.Errorf("history.Undo: %w", err)
	}

	if err := os.Remove(filepath.Join(dir, v[len(v)-1])); err != nil {
		return fmt.Errorf("history.Undo: %w", err)
	}

	if err := os.WriteFile(s.Paths.BoardFile(name), prev, 0o644); err != nil {
		return fmt.Errorf("history.Undo: %w", err)
	}

	return s.dropDisplaced(name, string(latest), string(prev))
}

// ISO formats a time as UTC with milliseconds: 2026-01-02T15:04:05.000Z.
func ISO(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.000Z") }
