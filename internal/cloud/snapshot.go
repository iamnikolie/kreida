package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/paths"
)

// Upload is the body of a board upload: what the local /api/boards/<name> answers (the normalized board and its card
// positions), with the source of every live card blanked.
type Upload struct {
	Board *board.Board    `json:"board"`
	Pos   json.RawMessage `json:"pos"`
}

// Snapshot reads a board and its positions into the body of an upload. The command or URL behind a live card stays
// on this machine: every node's live.source is "".
func Snapshot(p paths.Paths, name string) (Upload, error) {
	if err := ValidName(name); err != nil {
		return Upload{}, err
	}

	text, err := os.ReadFile(p.BoardFile(name))
	if err != nil {
		return Upload{}, fmt.Errorf("cloud.Snapshot: %w", err)
	}

	b, err := board.Parse(string(text))
	if err != nil {
		return Upload{}, fmt.Errorf("cloud.Snapshot: invalid board: %w", err)
	}

	for i := range b.Nodes {
		if b.Nodes[i].Live != nil {
			b.Nodes[i].Live.Source = ""
		}
	}

	return Upload{Board: b, Pos: board.ReadPos(p.PosFile(name))}, nil
}

// JSON is the upload as sent, encoded like the local API encodes a board.
func (u Upload) JSON() ([]byte, error) {
	var buf bytes.Buffer

	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)

	if err := enc.Encode(u); err != nil {
		return nil, fmt.Errorf("cloud.Upload.JSON: %w", err)
	}

	return bytes.TrimSuffix(buf.Bytes(), []byte("\n")), nil
}
