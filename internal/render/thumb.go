package render

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
)

// ThumbWidth and ThumbHeight are the size of a thumbnail in pixels.
const (
	ThumbWidth  = 640
	ThumbHeight = 400
)

// Thumb draws a board as a small PNG: the standalone page (see HTML) in a throwaway file, shown by the Chrome binary
// the way `kreida png` shows it (light theme, the whole board) in a window twice the size of the picture and scaled
// by half, so the board is laid out like on a desktop. The page needs no server, and the board comes from the caller,
// so no live command or address is in it.
func Thumb(ctx context.Context, chrome string, dist fs.FS, d Data) ([]byte, error) {
	if dist == nil {
		return nil, errors.New("render.Thumb: page not built (run make build)")
	}

	html, err := HTML(dist, d)
	if err != nil {
		return nil, err
	}

	dir, err := os.MkdirTemp("", "kreida-thumb-")
	if err != nil {
		return nil, fmt.Errorf("render.Thumb: %w", err)
	}

	defer os.RemoveAll(dir)

	page := filepath.Join(dir, "board.html")
	if err := os.WriteFile(page, []byte(html), 0o600); err != nil {
		return nil, fmt.Errorf("render.Thumb: %w", err)
	}

	out := filepath.Join(dir, "thumb.png")
	size := fmt.Sprintf("--window-size=%d,%d", 2*ThumbWidth, 2*ThumbHeight)

	if err := shoot(ctx, chrome, thumbURL(page), out, size, "--force-device-scale-factor=0.5"); err != nil {
		return nil, err
	}

	png, err := os.ReadFile(out)
	if err != nil {
		return nil, fmt.Errorf("render.Thumb: %w", err)
	}

	return png, nil
}

// thumbURL is the address of a page file with the query of `kreida png`: the whole board, no controls, light.
func thumbURL(page string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(page), RawQuery: "present=1&static=1&theme=light"}).String()
}
