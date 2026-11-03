// Package paths says where kreida keeps its files and which port and mode the server uses. Everything comes from the
// environment (KREIDA_HOME, KREIDA_DIR, KREIDA_PORT, KREIDA_LAN) so a test or a second instance can be pointed at a
// scratch directory without touching the real one.
package paths

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
)

// DefaultPort is the port of the local server unless KREIDA_PORT says otherwise.
const DefaultPort = 7788

var validName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

// ValidName reports whether name may be a board name: word characters, dots and dashes.
func ValidName(name string) bool { return validName.MatchString(name) }

// Paths holds the two roots. Home keeps state (history, shares, allow-list, cloud login, pid, log); Dir keeps the boards.
type Paths struct {
	Home string
	Dir  string
}

// FromEnv reads KREIDA_HOME (default ~/.kreida) and KREIDA_DIR (default <home>/boards).
func FromEnv() (Paths, error) {
	home := os.Getenv("KREIDA_HOME")

	if home == "" {
		h, err := os.UserHomeDir()
		if err != nil {
			return Paths{}, fmt.Errorf("paths.FromEnv: %w", err)
		}

		home = filepath.Join(h, ".kreida")
	}

	dir := os.Getenv("KREIDA_DIR")
	if dir == "" {
		dir = filepath.Join(home, "boards")
	}

	return Paths{Home: home, Dir: dir}, nil
}

// BoardFile is the YAML file of a board.
func (p Paths) BoardFile(name string) string { return filepath.Join(p.Dir, name+".yaml") }

// PosFile holds the manual card positions of a board.
func (p Paths) PosFile(name string) string { return filepath.Join(p.Dir, name+".pos.json") }

// NotesFile holds the notes a human wrote on a board for the agent.
func (p Paths) NotesFile(name string) string { return filepath.Join(p.Dir, name+".notes.json") }

// HistoryDir holds the saved versions of a board.
func (p Paths) HistoryDir(name string) string { return filepath.Join(p.Home, "history", name) }

// SharesFile is the share-link store.
func (p Paths) SharesFile() string { return filepath.Join(p.Home, "shares.json") }

// AllowFile is the live-source allow-list.
func (p Paths) AllowFile() string { return filepath.Join(p.Home, "live-allow.json") }

// CloudFile is the kreida cloud login and the list of published boards.
func (p Paths) CloudFile() string { return filepath.Join(p.Home, "cloud.json") }

// PidFile holds the pid of the background server.
func (p Paths) PidFile() string { return filepath.Join(p.Home, "server.pid") }

// LogFile receives the background server's output.
func (p Paths) LogFile() string { return filepath.Join(p.Home, "server.log") }

// Ensure creates the boards directory.
func (p Paths) Ensure() error {
	if err := os.MkdirAll(p.Dir, 0o755); err != nil {
		return fmt.Errorf("paths.Ensure: %w", err)
	}

	return nil
}

// Port reads KREIDA_PORT.
func Port() (int, error) {
	s := os.Getenv("KREIDA_PORT")
	if s == "" {
		return DefaultPort, nil
	}

	n, err := strconv.Atoi(s)
	if err != nil || n < 1 || n > 65535 {
		return 0, fmt.Errorf("paths.Port: bad KREIDA_PORT %q", s)
	}

	return n, nil
}

// LAN reports KREIDA_LAN=1: listen on all interfaces, share links only.
func LAN() bool { return os.Getenv("KREIDA_LAN") == "1" }
