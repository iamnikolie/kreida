package cloud

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"slices"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/images"
	"github.com/iamnikolie/kreida/internal/paths"
)

// Upload is the body of a board upload: what the local /api/boards/<name> answers (the normalized board and its card
// positions), with the source of every live card blanked. A card's image is its hash, type and size; the bytes go up
// separately, and only those in Images.
type Upload struct {
	Board *board.Board    `json:"board"`
	Pos   json.RawMessage `json:"pos"`
	// Images are the checked images this board names: all the image bytes that may leave with it.
	Images map[string]images.File `json:"-"`
	// Problems are the cards that go up without their picture or with a picture changed, for the owner's log.
	Problems []images.Problem `json:"-"`
}

// Snapshot reads a board and its positions into the body of an upload. The command or URL behind a live card stays
// on this machine: every node's live.source is "". So does the path behind a card's image: only its hash goes in the
// body. A card whose image cannot be used goes up without one.
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

	set := images.Public(b, p.Dir)

	return Upload{Board: b, Pos: board.ReadPos(p.PosFile(name)), Images: set.Files, Problems: slices.Concat(set.Problems, set.Notes)}, nil
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
