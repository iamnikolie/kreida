// Package daemon starts, probes and stops the background server that `kreida up` leaves running. The server is this
// same binary started with its hidden `serve` command, detached from the terminal, logging to <home>/server.log and
// recording its pid in <home>/server.pid.
package daemon

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/iamnikolie/kreida/internal/paths"
)

// Control manages the server of one home directory and port.
type Control struct {
	Paths paths.Paths
	Port  int
	// Exe is the binary to start as the server; empty means the running executable.
	Exe string
}

// Health is what a running server reports on /api/health.
type Health struct {
	OK  bool   `json:"ok"`
	Dir string `json:"dir"`
	PID int    `json:"pid"`
	LAN bool   `json:"lan"`
}

// URL is the address of the local server.
func (c Control) URL() string { return fmt.Sprintf("http://127.0.0.1:%d", c.Port) }

// Health asks the server on the port how it is; ok is false when nothing answers.
func (c Control) Health() (Health, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.URL()+"/api/health", nil)
	if err != nil {
		return Health{}, false
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Health{}, false
	}

	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return Health{}, false
	}

	var h Health
	if err := json.NewDecoder(resp.Body).Decode(&h); err != nil {
		return Health{}, false
	}

	return h, true
}

// Stop signals the server whose pid is in the pid file. It reports whether there was one to signal.
func (c Control) Stop() bool {
	data, err := os.ReadFile(c.Paths.PidFile())
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}

	return syscall.Kill(pid, syscall.SIGTERM) == nil
}

// Up makes sure a server is running. With lan set it must run in that mode (LAN or local only): a server in the
// other mode is stopped and started again. A nil lan keeps whatever runs.
func (c Control) Up(lan *bool) error {
	if h, ok := c.Health(); ok {
		if lan == nil || h.LAN == *lan {
			return nil
		}

		c.Stop()

		for i := 0; i < 30; i++ {
			if _, up := c.Health(); !up {
				break
			}

			time.Sleep(100 * time.Millisecond)
		}
	}

	if err := os.MkdirAll(c.Paths.Home, 0o755); err != nil {
		return fmt.Errorf("daemon.Up: %w", err)
	}

	logf, err := os.OpenFile(c.Paths.LogFile(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("daemon.Up: %w", err)
	}

	defer logf.Close()

	exe := c.Exe
	if exe == "" {
		if exe, err = os.Executable(); err != nil {
			return fmt.Errorf("daemon.Up: %w", err)
		}
	}

	lanEnv := ""
	if lan != nil && *lan {
		lanEnv = "1"
	}

	cmd := exec.Command(exe, "serve")
	cmd.Stdout, cmd.Stderr = logf, logf
	cmd.Env = append(os.Environ(), "KREIDA_DIR="+c.Paths.Dir, "KREIDA_HOME="+c.Paths.Home,
		"KREIDA_PORT="+strconv.Itoa(c.Port), "KREIDA_LAN="+lanEnv)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("daemon.Up: %w", err)
	}

	pid := cmd.Process.Pid
	_ = cmd.Process.Release()

	if err := os.WriteFile(c.Paths.PidFile(), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("daemon.Up: %w", err)
	}

	for i := 0; i < 40; i++ {
		if _, ok := c.Health(); ok {
			return nil
		}

		time.Sleep(100 * time.Millisecond)
	}

	return fmt.Errorf("server did not start, see %s", c.Paths.LogFile())
}
