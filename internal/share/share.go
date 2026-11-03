// Package share keeps read-only share links: a token that opens one board. The CLI writes the store, the server reads
// it on every request, so the file is the single source of truth and a revoked link stops working at once.
package share

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"sort"
	"time"

	"github.com/iamnikolie/kreida/internal/history"
	"github.com/iamnikolie/kreida/internal/paths"
)

var tokenRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,}$`)

// Link is one share link.
type Link struct {
	Token   string
	Board   string
	Created string
}

// Store reads and writes <home>/shares.json.
type Store struct{ Paths paths.Paths }

type stored struct {
	Board   string `json:"board"`
	Created string `json:"created"`
}

// All lists the links in the order they were made. A missing or unreadable file is an empty list.
func (s Store) All() []Link {
	var m map[string]stored

	if data, err := os.ReadFile(s.Paths.SharesFile()); err == nil {
		_ = json.Unmarshal(data, &m)
	}

	out := make([]Link, 0, len(m))
	for token, l := range m {
		out = append(out, Link{Token: token, Board: l.Board, Created: l.Created})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Created != out[j].Created {
			return out[i].Created < out[j].Created
		}

		return out[i].Token < out[j].Token
	})

	return out
}

func (s Store) save(links []Link) error {
	m := map[string]stored{}
	for _, l := range links {
		m[l.Token] = stored{l.Board, l.Created}
	}

	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return fmt.Errorf("share.save: %w", err)
	}

	if err := os.MkdirAll(s.Paths.Home, 0o755); err != nil {
		return fmt.Errorf("share.save: %w", err)
	}

	if err := os.WriteFile(s.Paths.SharesFile(), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("share.save: %w", err)
	}

	return nil
}

// Share returns the board's link token, making one if there is none: one link per board.
func (s Store) Share(board string) (string, error) {
	links := s.All()

	for _, l := range links {
		if l.Board == board {
			return l.Token, nil
		}
	}

	raw := make([]byte, 18)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("share.Share: %w", err)
	}

	token := base64.RawURLEncoding.EncodeToString(raw)
	links = append(links, Link{Token: token, Board: board, Created: history.ISO(time.Now())})

	if err := s.save(links); err != nil {
		return "", err
	}

	return token, nil
}

// Unshare revokes the board's links and returns how many there were.
func (s Store) Unshare(board string) (int, error) {
	var keep []Link

	dropped := 0

	for _, l := range s.All() {
		if l.Board == board {
			dropped++
		} else {
			keep = append(keep, l)
		}
	}

	if err := s.save(keep); err != nil {
		return 0, err
	}

	return dropped, nil
}

// BoardOf is the board a token opens, or "" for an unknown or malformed token.
func (s Store) BoardOf(token string) string {
	if !tokenRe.MatchString(token) {
		return ""
	}

	for _, l := range s.All() {
		if l.Token == token {
			return l.Board
		}
	}

	return ""
}
