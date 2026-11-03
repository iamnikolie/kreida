package render_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/iamnikolie/kreida/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeChrome is a shell script that answers like Chrome: it writes `png` to the --screenshot file and notes its
// arguments and the page it was pointed at (read while the page file exists) in `seen`.
func fakeChrome(t *testing.T, png string) (chrome, seen string) {
	t.Helper()

	dir := t.TempDir()
	seen = filepath.Join(dir, "seen")
	chrome = filepath.Join(dir, "chrome")

	script := `#!/bin/sh
for a in "$@"; do
  case "$a" in
    --screenshot=*) out="${a#--screenshot=}" ;;
    file://*) page="${a#file://}"; page="${page%%\?*}" ;;
  esac
done
echo "$@" > "` + seen + `"
cat "$page" >> "` + seen + `"
` + png + `
`

	require.NoError(t, os.WriteFile(chrome, []byte(script), 0o755))

	return chrome, seen
}

func TestThumbShowsThePageInAWindowTwiceTheSizeScaledByHalf(t *testing.T) {
	chrome, seen := fakeChrome(t, `printf '\211PNG\r\n\032\nfake' > "$out"`)

	png, err := render.Thumb(context.Background(), chrome, dist(), data(t, "title: Demo\nnodes: { a: A }\n"))
	require.NoError(t, err)
	assert.Equal(t, "\x89PNG\r\n\x1a\nfake", string(png))

	got, err := os.ReadFile(seen)
	require.NoError(t, err)

	args, page, _ := strings.Cut(string(got), "\n")
	assert.Contains(t, args, "--headless=new")
	assert.Contains(t, args, "--window-size=1280,800")
	assert.Contains(t, args, "--force-device-scale-factor=0.5")
	assert.Regexp(t, `file:///[^ ]*/board\.html\?present=1&static=1&theme=light$`, strings.TrimSpace(args))
	assert.Contains(t, page, `window.__KREIDA__=`, "the page is the standalone one, the board inside")
	assert.Contains(t, page, `"title":"Demo"`)
}

func TestThumbLeavesNothingBehind(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)

	chrome, _ := fakeChrome(t, `printf '\211PNG\r\n\032\n' > "$out"`)

	_, err := render.Thumb(context.Background(), chrome, dist(), data(t, "title: Demo\n"))
	require.NoError(t, err)

	left, err := os.ReadDir(tmp)
	require.NoError(t, err)
	assert.Empty(t, left, "the page, the picture and Chrome's profile are removed")
}

func TestThumbFailures(t *testing.T) {
	ctx := context.Background()

	// no page built
	chrome, _ := fakeChrome(t, `printf 'x' > "$out"`)
	_, err := render.Thumb(ctx, chrome, nil, data(t, "title: Demo\n"))
	assert.ErrorContains(t, err, "page not built")

	_, err = render.Thumb(ctx, chrome, fstest.MapFS{".keep": {}}, data(t, "title: Demo\n"))
	assert.ErrorContains(t, err, "page not built")

	// Chrome fails with a message
	chrome, _ = fakeChrome(t, `echo "boom" >&2; exit 1`)
	_, err = render.Thumb(ctx, chrome, dist(), data(t, "title: Demo\n"))
	assert.EqualError(t, err, "boom")

	// Chrome exits without a picture
	chrome, _ = fakeChrome(t, `true`)
	_, err = render.Thumb(ctx, chrome, dist(), data(t, "title: Demo\n"))
	assert.EqualError(t, err, "chrome wrote no screenshot")

	// Chrome that is not there
	_, err = render.Thumb(ctx, filepath.Join(t.TempDir(), "nope"), dist(), data(t, "title: Demo\n"))
	assert.Error(t, err)
}

func TestThumbStopsWithItsContext(t *testing.T) {
	chrome, _ := fakeChrome(t, `sleep 30`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := render.Thumb(ctx, chrome, dist(), data(t, "title: Demo\n"))

	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.Less(t, time.Since(start), 5*time.Second)
}
