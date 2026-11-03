package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const macChrome = "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"

// ErrNoChrome is returned when no Chrome or Chromium can be found.
var ErrNoChrome = errors.New("no Chrome found (macOS app or google-chrome/chromium on PATH)")

// EnvChrome names the Chrome binary to use instead of looking for one.
const EnvChrome = "KREIDA_CHROME"

// FindChrome returns the Chrome binary: $KREIDA_CHROME when it is set, else the macOS app, else google-chrome,
// google-chrome-stable, chromium or chromium-browser from PATH.
func FindChrome() (string, error) {
	if p := os.Getenv(EnvChrome); p != "" {
		if st, err := os.Stat(p); err != nil || st.IsDir() {
			return "", fmt.Errorf("%s=%s is not a file: %w", EnvChrome, p, ErrNoChrome)
		}

		return p, nil
	}

	if runtime.GOOS == "darwin" {
		if _, err := os.Stat(macChrome); err == nil {
			return macChrome, nil
		}
	}

	for _, name := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}

	return "", ErrNoChrome
}

// PNG opens a page in headless Chrome and writes a 1920x1200 screenshot. Chrome gets a throwaway profile, so a
// running Chrome of the same user cannot swallow the request.
func PNG(chrome, pageURL, out string) error {
	return shoot(context.Background(), chrome, pageURL, out, "--window-size=1920,1200")
}

// shoot runs headless Chrome on a page with the window flags and waits for its screenshot, at most 30 s and not past
// ctx.
func shoot(ctx context.Context, chrome, pageURL, out string, window ...string) error {
	profile, err := os.MkdirTemp("", "kreida-chrome-")
	if err != nil {
		return fmt.Errorf("render.PNG: %w", err)
	}

	defer os.RemoveAll(profile)

	run, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	args := append([]string{"--headless=new", "--disable-gpu", "--hide-scrollbars"}, window...)
	args = append(args, "--virtual-time-budget=8000", "--timeout=15000", "--user-data-dir="+profile, "--screenshot="+out, pageURL)

	cmd := exec.CommandContext(run, chrome, args...)
	// a helper process of a killed Chrome may keep stderr open
	cmd.WaitDelay = 2 * time.Second

	var stderr bytes.Buffer

	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}

		if run.Err() != nil {
			return errors.New("chrome did not finish in 30 s")
		}

		if msg := strings.TrimSpace(stderr.String()); msg != "" {
			return errors.New(msg)
		}

		return fmt.Errorf("render.PNG: %w", err)
	}

	if st, err := os.Stat(out); err != nil || st.Size() == 0 {
		return errors.New("chrome wrote no screenshot")
	}

	return nil
}
