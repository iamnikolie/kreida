// Package cloud is the CLI side of kreida cloud, the hosted read-only home of a board: the login in
// <home>/cloud.json, the HTTP client of the cloud's CLI API, the device login, the upload body of a board and the
// syncer that keeps published boards in step with their files. Nothing that runs leaves the machine: the source of a
// live card (its command or URL) is never part of an upload.
package cloud

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"slices"
	"strings"

	"github.com/iamnikolie/kreida/internal/paths"
)

// EnvURL names the endpoint override for an explicit login.
const EnvURL = "KREIDA_CLOUD_URL"

// Config is <home>/cloud.json: where the cloud is, who is logged in and which boards are published.
type Config struct {
	URL       string   `json:"url"`
	Token     string   `json:"token"`
	Email     string   `json:"email"`
	Published []string `json:"published"`
}

// Load reads the login. A missing file is an empty Config (not logged in). A file that is readable by others is
// tightened to 0600: it holds a token.
func Load(p paths.Paths) (Config, error) {
	file := p.CloudFile()

	data, err := os.ReadFile(file)
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, nil
	}

	if err != nil {
		return Config{}, fmt.Errorf("cloud.Load: %w", err)
	}

	if info, err := os.Stat(file); err == nil && info.Mode().Perm()&0o077 != 0 {
		_ = os.Chmod(file, 0o600)
	}

	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("cloud.Load: %s is not valid JSON: %w", file, err)
	}

	return c, nil
}

// Save writes the login with mode 0600. It writes a new file and renames it over the old one, so a reader (the
// running server) never sees half a file and a file that was wider than 0600 does not stay so.
func (c Config) Save(p paths.Paths) error {
	if err := os.MkdirAll(p.Home, 0o755); err != nil {
		return fmt.Errorf("cloud.Save: %w", err)
	}

	c.Published = slices.Compact(slices.Sorted(slices.Values(c.Published)))
	if c.Published == nil {
		c.Published = []string{}
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("cloud.Save: %w", err)
	}

	tmp, err := os.CreateTemp(p.Home, ".cloud-*.json")
	if err != nil {
		return fmt.Errorf("cloud.Save: %w", err)
	}

	defer os.Remove(tmp.Name())

	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("cloud.Save: %w", err)
	}

	if _, err := tmp.Write(append(data, '\n')); err != nil {
		_ = tmp.Close()

		return fmt.Errorf("cloud.Save: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("cloud.Save: %w", err)
	}

	if err := os.Rename(tmp.Name(), p.CloudFile()); err != nil {
		return fmt.Errorf("cloud.Save: %w", err)
	}

	return nil
}

// Remove deletes the login and returns whether there was one.
func Remove(p paths.Paths) (bool, error) {
	err := os.Remove(p.CloudFile())
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}

	if err != nil {
		return false, fmt.Errorf("cloud.Remove: %w", err)
	}

	return true, nil
}

// LoggedIn reports whether a token is bound to an explicitly configured endpoint.
func (c Config) LoggedIn() bool { return c.Token != "" && c.URL != "" }

// Has reports whether the board is published.
func (c Config) Has(name string) bool { return slices.Contains(c.Published, name) }

// Add returns the config with the board published.
func (c Config) Add(name string) Config {
	if !c.Has(name) {
		c.Published = append(slices.Clone(c.Published), name)
	}

	return c
}

// Drop returns the config with the board no longer published.
func (c Config) Drop(name string) Config {
	c.Published = slices.DeleteFunc(slices.Clone(c.Published), func(n string) bool { return n == name })

	return c
}

// NormalizeURL checks a cloud address and returns it without a trailing slash. The token travels to this address, so
// it must be https; plain http is for a cloud on this machine only.
func NormalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", fmt.Errorf("cloud.NormalizeURL: bad cloud url %q", raw)
	}

	switch u.Scheme {
	case "https":
	case "http":
		if !loopback(u.Hostname()) {
			return "", fmt.Errorf("cloud.NormalizeURL: %q is not https (the token must not travel in clear text)", raw)
		}
	default:
		return "", fmt.Errorf("cloud.NormalizeURL: bad cloud url %q (https://… expected)", raw)
	}

	u.Path = strings.TrimRight(u.Path, "/")

	return u.String(), nil
}

func loopback(host string) bool {
	if host == "localhost" {
		return true
	}

	ip := net.ParseIP(host)

	return ip != nil && ip.IsLoopback()
}
