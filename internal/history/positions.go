package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/iamnikolie/kreida/internal/board"
)

// previous is the text of the newest of versions that is a valid board, or "".
func (s Store) previous(name string, versions []string) string {
	for i := len(versions) - 1; i >= 0; i-- {
		text, err := os.ReadFile(filepath.Join(s.Paths.HistoryDir(name), versions[i]))
		if err != nil {
			continue
		}

		if _, err := board.Parse(string(text)); err == nil {
			return string(text)
		}
	}

	return ""
}

// dropDisplaced forgets the dragged positions (<name>.pos.json) that no longer fit after the board's text changed
// from prev to next: see board.Displaced. Everything else keeps its position. Either text not being a valid board
// changes nothing.
func (s Store) dropDisplaced(name, prev, next string) error {
	file := s.Paths.PosFile(name)

	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("history.dropDisplaced: %w", err)
	}

	var pos map[string]json.RawMessage
	if json.Unmarshal(data, &pos) != nil {
		return nil
	}

	was, perr := board.Parse(prev)
	now, nerr := board.Parse(next)

	if perr != nil || nerr != nil {
		return nil
	}

	gone := board.Displaced(was, now)
	kept := map[string]json.RawMessage{}

	for id, p := range pos {
		if !gone[id] {
			kept[id] = p
		}
	}

	switch {
	case len(kept) == len(pos):
		return nil
	case len(kept) == 0:
		err = os.Remove(file)
	default:
		data, _ = json.Marshal(kept)
		err = os.WriteFile(file, data, 0o644)
	}

	if err != nil {
		return fmt.Errorf("history.dropDisplaced: %w", err)
	}

	return nil
}
