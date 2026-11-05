package cmd_test

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func picture(t *testing.T) []byte {
	t.Helper()

	var b bytes.Buffer
	require.NoError(t, png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 4, 2))))

	return b.Bytes()
}

func TestSetSavesABoardWhoseImageIsMissingAndSaysWhereItLooked(t *testing.T) {
	e := newEnv(t)

	out, errOut, err := e.run("nodes:\n  a: { title: A, image: img/gone.png }\n", "set", "x")
	require.NoError(t, err)
	assert.Contains(t, out, "?b=x")
	assert.Contains(t, errOut, "card a: image "+filepath.Join(e.dir, "img/gone.png")+": no such file")
	assert.Contains(t, e.board("x"), "img/gone.png")
}

func TestSetRefusesAURLAndAnImageOnATable(t *testing.T) {
	e := newEnv(t)

	assert.Contains(t, e.fail("nodes:\n  a: { title: A, image: \"https://x.example/a.png\" }\n", "set", "x"), "not a URL")
	assert.Contains(t, e.fail("nodes:\n  a: { title: A, fields: [id], image: a.png }\n", "set", "x"), "not a table")
}

func TestExportEmbedsTheBytesAndNoPathOrTraceOfABrokenImage(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, os.MkdirAll(e.dir, 0o755))

	data := picture(t)
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "private-name.png"), data, 0o644))

	_, _, err := e.run("nodes:\n  a: { title: A, image: private-name.png }\n  b: { title: B, image: gone-name.png }\n", "set", "x")
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), "x.html")

	_, errOut, err := e.run("", "export", "x", "-o", file)
	require.NoError(t, err)
	assert.Contains(t, errOut, "card b: image", "the owner hears of it")

	html, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(html), "data:image/png;base64,"+base64.StdEncoding.EncodeToString(data))
	assert.NotContains(t, string(html), "private-name")
	assert.NotContains(t, string(html), "gone-name")
	assert.NotContains(t, string(html), `"error":"missing"`)
	assert.NotContains(t, string(html), e.dir)
}

func TestExportStripsWhatThePictureCarries(t *testing.T) {
	e := newEnv(t)
	require.NoError(t, os.MkdirAll(e.dir, 0o755))

	data := picture(t)
	at := bytes.Index(data, []byte("IDAT")) - 4
	text := append([]byte("tEXt"), "Comment\x00export-secret"...)
	chunk := binary.BigEndian.AppendUint32(nil, uint32(len(text)-4))
	chunk = append(chunk, text...)
	chunk = binary.BigEndian.AppendUint32(chunk, crc32.ChecksumIEEE(text))
	raw := append(append(append([]byte{}, data[:at]...), chunk...), data[at:]...)
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "a.png"), raw, 0o644))

	_, _, err := e.run("nodes:\n  a: { title: A, image: a.png }\n", "set", "x")
	require.NoError(t, err)

	file := filepath.Join(t.TempDir(), "x.html")
	_, _, err = e.run("", "export", "x", "-o", file)
	require.NoError(t, err)

	html, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Contains(t, string(html), base64.StdEncoding.EncodeToString(data))
	assert.NotContains(t, string(html), base64.StdEncoding.EncodeToString(raw))
}
