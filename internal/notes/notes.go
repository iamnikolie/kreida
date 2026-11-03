// Package notes keeps the notes a human writes on a board for the agent: <board>.notes.json next to the board's YAML.
// A note is about a card or group (Node) or the whole board (Node ""). The agent reads the open ones (`kreida inbox`)
// and closes them once it has acted (`kreida resolve`). Notes never go into the board's YAML and never to a share
// link. The same shape carries the comments clients leave on a published board in kreida cloud.
package notes

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/iamnikolie/kreida/internal/paths"
)

// MaxLen is the longest note, in characters.
const MaxLen = 2000

// Note is one note. Created and Resolved are RFC 3339; Resolved is empty while the note is open. Author is empty for
// a note written on this machine; the cloud sets it on a comment. Board is set only where notes of several boards are
// listed together; the file does not keep it.
type Note struct {
	ID       string `json:"id"`
	Node     string `json:"node"`
	Text     string `json:"text"`
	Author   string `json:"author"`
	Created  string `json:"created"`
	Resolved string `json:"resolved,omitempty"`
	Board    string `json:"board,omitempty"`
}

// file is <board>.notes.json. Next is the number of the next id: ids are n1, n2, … and never reused.
type file struct {
	Next  int    `json:"next"`
	Notes []Note `json:"notes"`
}

// mu makes a read-modify-write of a notes file one step; the server answers requests concurrently.
var mu sync.Mutex

// Plain drops the control characters of text that came from outside, except newline and tab. A terminal must not
// obey what a note says.
func Plain(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) && r != '\n' && r != '\t' {
			return -1
		}

		return r
	}, text)
}

// Clean makes the text of a note: control characters dropped except newline and tab, outer space trimmed. It refuses
// an empty note and one over MaxLen characters.
func Clean(text string) (string, error) {
	text = strings.TrimSpace(Plain(text))

	if text == "" {
		return "", errors.New("note is empty")
	}

	if utf8.RuneCountInString(text) > MaxLen {
		return "", fmt.Errorf("note is longer than %d characters", MaxLen)
	}

	return text, nil
}

// seq is the number in an id like n12.
func seq(id string) (int, bool) {
	num, ok := strings.CutPrefix(id, "n")
	if !ok {
		return 0, false
	}

	n, err := strconv.Atoi(num)

	return n, err == nil
}

// load reads the notes of a board. A missing file is no notes. Next is kept ahead of every id in the file.
func load(p paths.Paths, board string) (file, error) {
	if !paths.ValidName(board) {
		return file{}, fmt.Errorf("notes.load: bad board name: %s", board)
	}

	name := p.NotesFile(board)

	data, err := os.ReadFile(name)
	if errors.Is(err, os.ErrNotExist) {
		return file{Next: 1, Notes: []Note{}}, nil
	}

	if err != nil {
		return file{}, fmt.Errorf("notes.load: %w", err)
	}

	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("notes.load: %s is not valid JSON: %w", name, err)
	}

	f.Next = max(f.Next, 1)

	for _, n := range f.Notes {
		if num, ok := seq(n.ID); ok {
			f.Next = max(f.Next, num+1)
		}
	}

	if f.Notes == nil {
		f.Notes = []Note{}
	}

	return f, nil
}

// save writes a new file and renames it over the old one, so the running server and the CLI never see half a file.
func save(p paths.Paths, board string, f file) error {
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return fmt.Errorf("notes.save: %w", err)
	}

	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("notes.save: %w", err)
	}

	// a name ending in .tmp: the server's watcher does not take it for a change of a board
	tmp, err := os.CreateTemp(p.Dir, "."+board+".notes-*.tmp")
	if err != nil {
		return fmt.Errorf("notes.save: %w", err)
	}

	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("notes.save: %w", err)
	}

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("notes.save: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("notes.save: %w", err)
	}

	if err := os.Rename(tmp.Name(), p.NotesFile(board)); err != nil {
		return fmt.Errorf("notes.save: %w", err)
	}

	return nil
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }

// Add writes a new open note on a card or group (node) of the board, or on the whole board (node ""). The text is
// cleaned first (see Clean); the stored text is the cleaned one. The caller checks that node exists.
func Add(p paths.Paths, board, node, text string, now time.Time) (Note, error) {
	text, err := Clean(text)
	if err != nil {
		return Note{}, err
	}

	mu.Lock()
	defer mu.Unlock()

	f, err := load(p, board)
	if err != nil {
		return Note{}, err
	}

	n := Note{ID: "n" + strconv.Itoa(f.Next), Node: node, Text: text, Created: stamp(now)}

	f.Next++
	f.Notes = append(f.Notes, n)

	return n, save(p, board, f)
}

// List returns the board's notes, oldest first: the open ones, or all of them. No notes is an empty list.
func List(p paths.Paths, board string, all bool) ([]Note, error) {
	mu.Lock()
	defer mu.Unlock()

	f, err := load(p, board)
	if err != nil {
		return nil, err
	}

	out := []Note{}

	for _, n := range f.Notes {
		if all || n.Resolved == "" {
			out = append(out, n)
		}
	}

	return out, nil
}

// Resolve closes a note. Closing a closed note is fine and keeps the first time; an unknown id is an error.
func Resolve(p paths.Paths, board, id string, now time.Time) error {
	mu.Lock()
	defer mu.Unlock()

	f, err := load(p, board)
	if err != nil {
		return err
	}

	for i, n := range f.Notes {
		if n.ID != id {
			continue
		}

		if n.Resolved != "" {
			return nil
		}

		f.Notes[i].Resolved = stamp(now)

		return save(p, board, f)
	}

	return fmt.Errorf("no note %s", id)
}

// Boards lists the boards that have a notes file, by name.
func Boards(p paths.Paths) ([]string, error) {
	entries, err := os.ReadDir(p.Dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}

	if err != nil {
		return nil, fmt.Errorf("notes.Boards: %w", err)
	}

	var out []string

	for _, e := range entries {
		if name, ok := strings.CutSuffix(e.Name(), ".notes.json"); ok && paths.ValidName(name) {
			out = append(out, name)
		}
	}

	sort.Strings(out)

	return out, nil
}
