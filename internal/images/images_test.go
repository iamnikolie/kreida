package images_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/images"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()

	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, w, h))))

	return b.Bytes()
}

func write(t *testing.T, path string, data []byte) string {
	t.Helper()
	require.NoError(t, os.WriteFile(path, data, 0o644))

	return path
}

func reason(t *testing.T, b *board.Board, id string) string {
	t.Helper()

	for _, n := range b.Nodes {
		if n.ID == id {
			require.NotNil(t, n.Image, id)

			return n.Image.Error
		}
	}

	t.Fatalf("no node %s", id)

	return ""
}

func parse(t *testing.T, yaml string) *board.Board {
	t.Helper()

	b, err := board.Parse(yaml)
	require.NoError(t, err)

	return b
}

func TestTheFormatIsJudgedByContentNotByName(t *testing.T) {
	dir := t.TempDir()

	f, err := images.Load(write(t, filepath.Join(dir, "notes.txt"), pngBytes(t, 30, 20)))
	require.NoError(t, err)
	assert.Equal(t, "image/png", f.Type)
	assert.Equal(t, 30, f.W)
	assert.Equal(t, 20, f.H)
	assert.Len(t, f.SHA256, 64)
	assert.True(t, images.ValidSHA(f.SHA256))

	for name, data := range map[string][]byte{
		"secret.png":    []byte("API_KEY=abc\n"),
		"page.jpg":      []byte("<html><script>alert(1)</script>"),
		"drawing.png":   []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`),
		"truncated.png": []byte("\x89PNG\r\n\x1a\nnot really"),
		"empty.png":     {},
		"riff.webp":     []byte("RIFF\x00\x00\x00\x00WEBPshort"),
	} {
		_, err := images.Load(write(t, filepath.Join(dir, name), data))
		require.Error(t, err, name)
		assert.Contains(t, err.Error(), "not a PNG, JPEG, GIF or WebP", name)
	}
}

func TestAllFourFormatsGiveTheirTypeAndSize(t *testing.T) {
	dir := t.TempDir()
	rgba := image.NewRGBA(image.Rect(0, 0, 12, 7))

	var jpg, gf bytes.Buffer

	require.NoError(t, jpeg.Encode(&jpg, rgba, nil))
	require.NoError(t, gif.Encode(&gf, rgba, nil))

	vp8 := make([]byte, 30)
	copy(vp8, "RIFF\x00\x00\x00\x00WEBPVP8 ")
	copy(vp8[23:], "\x9d\x01\x2a")
	binary.LittleEndian.PutUint16(vp8[26:], 12)
	binary.LittleEndian.PutUint16(vp8[28:], 7)

	vp8l := make([]byte, 30)
	copy(vp8l, "RIFF\x00\x00\x00\x00WEBPVP8L")
	vp8l[20] = 0x2f
	binary.LittleEndian.PutUint32(vp8l[21:], 11|6<<14)

	vp8x := make([]byte, 30)
	copy(vp8x, "RIFF\x00\x00\x00\x00WEBPVP8X")
	vp8x[24], vp8x[27] = 11, 6

	for _, c := range []struct {
		typ  string
		data []byte
	}{
		{"image/png", pngBytes(t, 12, 7)},
		{"image/jpeg", jpg.Bytes()},
		{"image/gif", gf.Bytes()},
		{"image/webp", vp8},
		{"image/webp", vp8l},
		{"image/webp", vp8x},
	} {
		f, err := images.Load(write(t, filepath.Join(dir, "file"), c.data))
		require.NoError(t, err, c.typ)
		assert.Equal(t, c.typ, f.Type)
		assert.Equal(t, 12, f.W, c.typ)
		assert.Equal(t, 7, f.H, c.typ)
		assert.True(t, strings.HasPrefix(f.DataURL(), "data:"+c.typ+";base64,"))
	}
}

func TestLinksAreFollowedOnlyIntoAnImage(t *testing.T) {
	dir := t.TempDir()
	real := write(t, filepath.Join(dir, "real.png"), pngBytes(t, 2, 2))
	secret := write(t, filepath.Join(dir, "id_rsa"), []byte("-----BEGIN PRIVATE KEY-----\n"))

	good, bad, dangling := filepath.Join(dir, "good.png"), filepath.Join(dir, "bad.png"), filepath.Join(dir, "gone.png")
	require.NoError(t, os.Symlink(real, good))
	require.NoError(t, os.Symlink(secret, bad))
	require.NoError(t, os.Symlink(filepath.Join(dir, "nothing"), dangling))

	b := parse(t, "nodes:\n  a: {title: A, image: good.png}\n  b: {title: B, image: bad.png}\n  c: {title: C, image: gone.png}\n")
	set := images.Attach(b, dir)

	assert.Equal(t, "", reason(t, b, "a"))
	assert.Equal(t, images.Unsupported, reason(t, b, "b"))
	assert.Equal(t, images.Missing, reason(t, b, "c"))
	assert.Len(t, set.Files, 1)

	for _, f := range set.Files {
		assert.NotContains(t, string(f.Bytes), "PRIVATE KEY")
	}
}

func TestOnlyARegularFileIsRead(t *testing.T) {
	dir := t.TempDir()

	_, err := images.Load(dir)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a regular file")

	fifo := filepath.Join(dir, "pipe.png")
	require.NoError(t, syscall.Mkfifo(fifo, 0o644))

	done := make(chan error, 1)

	go func() {
		_, err := images.Load(fifo)
		done <- err
	}()

	select {
	case err := <-done:
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a regular file")
	case <-time.After(3 * time.Second):
		t.Fatal("a pipe held the read up")
	}

	_, err = images.Load("/dev/zero")
	require.Error(t, err)
}

func TestSizeLimits(t *testing.T) {
	dir := t.TempDir()

	big := append(pngBytes(t, 2, 2), make([]byte, images.MaxFile)...)
	_, err := images.Load(write(t, filepath.Join(dir, "big.png"), big))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "over the 5 MiB limit")

	// a small file can claim a huge picture: the header says so, and the header is what is read
	var bomb bytes.Buffer
	require.NoError(t, png.Encode(&bomb, image.NewGray(image.Rect(0, 0, 1, 1))))

	data := bomb.Bytes()
	binary.BigEndian.PutUint32(data[16:], 10000)
	binary.BigEndian.PutUint32(data[20:], 10000)
	binary.BigEndian.PutUint32(data[29:], crc32.ChecksumIEEE(data[12:29]))

	_, err = images.Load(write(t, filepath.Join(dir, "bomb.png"), data))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "over the 40 MP limit")
}

// noise is a PNG that does not compress: its size is its pixels.
func noise(t *testing.T, w, h int, seed int64) []byte {
	t.Helper()

	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	_, _ = rand.New(rand.NewSource(seed)).Read(img.Pix)

	var b bytes.Buffer
	require.NoError(t, (&png.Encoder{CompressionLevel: png.NoCompression}).Encode(&b, img))

	return b.Bytes()
}

func TestABoardHasACapOfItsOwnAndAFileCountsOnce(t *testing.T) {
	dir := t.TempDir()

	yaml := "nodes:\n"

	for i, id := range []string{"a", "b", "c", "d", "e", "f"} {
		data := noise(t, 1100, 1100, int64(i))
		require.Less(t, len(data), images.MaxFile)
		write(t, filepath.Join(dir, id+".png"), data)
		yaml += "  " + id + ": {title: " + id + ", image: " + id + ".png}\n"
	}

	yaml += "  again: {title: again, image: a.png}\n"

	b := parse(t, yaml)
	set := images.Attach(b, dir)

	assert.Len(t, set.Files, 5, "five files of 4.8 MB fit in 25 MiB, a sixth does not")
	assert.Equal(t, images.TooBig, reason(t, b, "f"))
	assert.Equal(t, "", reason(t, b, "again"), "the same bytes twice cost nothing more")
	assert.Equal(t, b.Nodes[0].Image.SHA256, b.Nodes[6].Image.SHA256)
	require.Len(t, set.Problems, 1)
	assert.Equal(t, "f", set.Problems[0].Node)
}

func TestARelativePathIsRelativeToTheBoardsDirectory(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "img"), 0o755))
	write(t, filepath.Join(dir, "img", "a.png"), pngBytes(t, 2, 2))

	// wherever the command runs from
	t.Chdir(t.TempDir())

	b := parse(t, "nodes:\n  a: {title: A, image: img/a.png}\n")
	assert.Len(t, images.Attach(b, dir).Files, 1)

	assert.Equal(t, filepath.Join(dir, "img/a.png"), images.Resolve(dir, "img/a.png"))
	assert.Equal(t, "/abs/a.png", images.Resolve(dir, "/abs/a.png"))

	home, err := os.UserHomeDir()
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(home, "pics/a.png"), images.Resolve(dir, "~/pics/a.png"))
}

func TestPublicDropsTheReasonAndKeepsTheProblem(t *testing.T) {
	b := parse(t, "nodes:\n  a: {title: A, image: nope.png}\n")
	set := images.Public(b, t.TempDir())

	assert.Nil(t, b.Nodes[0].Image)
	require.Len(t, set.Problems, 1)
	assert.Contains(t, set.Problems[0].String(), "nope.png", "the owner hears which file")
}

func TestTrackerSeesAnImageChangeNotTheFirstLookNorABoardEdit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "x.yaml")
	img := write(t, filepath.Join(dir, "a.png"), pngBytes(t, 2, 2))
	write(t, file, []byte("nodes:\n  a: {title: A, image: a.png}\n"))

	tr := images.NewTracker()
	assert.False(t, tr.Changed("x", file, dir), "the first look")
	assert.False(t, tr.Changed("x", file, dir), "nothing happened")

	write(t, img, pngBytes(t, 3, 3))
	assert.True(t, tr.Changed("x", file, dir), "the image changed")
	assert.False(t, tr.Changed("x", file, dir), "and is told once")

	require.NoError(t, os.Remove(img))
	assert.True(t, tr.Changed("x", file, dir), "the image went away")

	write(t, file, []byte("title: edited\nnodes:\n  a: {title: A, image: a.png}\n"))
	assert.False(t, tr.Changed("x", file, dir), "the board's own change has its own event")

	require.NoError(t, os.Remove(file))
	assert.False(t, tr.Changed("x", file, dir))
}
