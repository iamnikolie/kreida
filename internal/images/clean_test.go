package images_test

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"path/filepath"
	"testing"

	"github.com/iamnikolie/kreida/internal/images"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func load(t *testing.T, data []byte) images.File {
	t.Helper()

	f, err := images.Load(write(t, filepath.Join(t.TempDir(), "file"), data))
	require.NoError(t, err)

	return f
}

func clean(t *testing.T, data []byte) images.File {
	t.Helper()

	c, err := images.Clean(load(t, data))
	require.NoError(t, err)

	return c
}

func TestAJPEGLosesGPSAndEveryOtherTag(t *testing.T) {
	raw := taggedJPEG(t, 16, 8, 1, "jpeg-secret")
	require.Contains(t, string(raw), "jpeg-secret-exif")

	c := clean(t, raw)
	assert.NotContains(t, string(c.Bytes), "jpeg-secret")
	assert.NotContains(t, string(c.Bytes), "Exif")
	assert.NotContains(t, string(c.Bytes), "8BIM")
	assert.NotContains(t, string(c.Bytes), "\x00\x00\x00\x32\x00\x00\x00\x01", "the latitude's 50/1")
	assert.Equal(t, "image/jpeg", c.Type)
	assert.Equal(t, 16, c.W)
	assert.Equal(t, 8, c.H)

	_, err := jpeg.Decode(bytes.NewReader(c.Bytes))
	require.NoError(t, err)

	// an upright file keeps its pixels: only segments go
	assert.True(t, bytes.HasSuffix(c.Bytes, []byte{0xff, 0xd9}))
	assert.Less(t, len(c.Bytes), len(raw))
}

func TestAPNGLosesItsTextChunks(t *testing.T) {
	raw := taggedPNG(t, 4, 2, 1, "png-secret")

	c := clean(t, raw)
	assert.NotContains(t, string(c.Bytes), "png-secret")

	for _, chunk := range []string{"tEXt", "zTXt", "iTXt", "eXIf", "tIME"} {
		assert.NotContains(t, string(c.Bytes), chunk)
	}

	img, err := png.Decode(bytes.NewReader(c.Bytes))
	require.NoError(t, err)
	assert.Equal(t, image.Rect(0, 0, 4, 2), img.Bounds())
}

func TestAGIFLosesCommentsAndXMPAndKeepsItsLoop(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	frame := func() *image.Paletted { return image.NewPaletted(image.Rect(0, 0, 3, 2), pal) }

	var plain bytes.Buffer
	require.NoError(t, gif.EncodeAll(&plain, &gif.GIF{Image: []*image.Paletted{frame(), frame()}, Delay: []int{10, 20}, LoopCount: 0}))

	d := plain.Bytes()
	raw := append([]byte{}, d[:len(d)-1]...)
	raw = append(raw, 0x21, 0xfe, 10)
	raw = append(raw, "gif-secret"...)
	raw = append(raw, 0, 0x21, 0xff, 11)
	raw = append(raw, "XMP DataXMP"...)
	raw = append(raw, 14)
	raw = append(raw, "gif-secret-xmp"...)
	raw = append(raw, 0, 0x3b)
	raw = append(raw, "gif-secret-trailer"...)

	c := clean(t, raw)
	assert.NotContains(t, string(c.Bytes), "gif-secret")
	assert.NotContains(t, string(c.Bytes), "XMP")

	g, err := gif.DecodeAll(bytes.NewReader(c.Bytes))
	require.NoError(t, err)
	assert.Len(t, g.Image, 2)
	assert.Equal(t, []int{10, 20}, g.Delay)
	assert.Equal(t, 0, g.LoopCount)
}

// webp is an extended WebP of 12×7 with ICC, the chunks given and a lossless bitstream.
func webp(extra ...[]byte) []byte {
	riff := func(id string, data []byte) []byte {
		out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
		out = append(out, data...)
		if len(data)%2 == 1 {
			out = append(out, 0)
		}

		return out
	}

	head := []byte{0x20 | 0x08 | 0x04, 0, 0, 0, 11, 0, 0, 6, 0, 0}
	body := riff("VP8X", head)
	body = append(body, riff("ICCP", []byte("colour"))...)

	for _, e := range extra {
		body = append(body, e...)
	}

	body = append(body, riff("VP8L", []byte{0x2f, 11, 0x80, 1, 0, 0, 0})...)
	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body)))...)

	return append(append(out, "WEBP"...), body...)
}

func riff(id, data string) []byte {
	out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
	out = append(out, data...)
	if len(data)%2 == 1 {
		out = append(out, 0)
	}

	return out
}

func TestAWebPLosesItsEXIFAndXMPChunksAndFlags(t *testing.T) {
	raw := append(webp(riff("EXIF", "Exif\x00\x00"+string(exifTIFF(1, "webp-secret-exif"))), riff("XMP ", "webp-secret-xmp")), "webp-secret-tail"...)

	c := clean(t, raw)
	assert.NotContains(t, string(c.Bytes), "webp-secret")
	assert.NotContains(t, string(c.Bytes), "EXIF")
	assert.Contains(t, string(c.Bytes), "ICCP", "the colour profile stays")
	assert.Equal(t, byte(0x20), c.Bytes[20], "the EXIF and XMP flags are off")
	assert.Equal(t, len(c.Bytes)-8, int(binary.LittleEndian.Uint32(c.Bytes[4:])))
	assert.Equal(t, 12, c.W)
	assert.Equal(t, 7, c.H)

	_, err := images.Clean(load(t, webp(riff("EXIF", string(exifTIFF(6, "x"))))))
	require.Error(t, err, "a WebP its EXIF turns cannot be turned here")
	assert.Contains(t, err.Error(), "save the picture upright")
}

func TestThePixelsAreTurnedBeforeTheOrientationGoes(t *testing.T) {
	// orientation 6: the picture is seen turned 90° clockwise, so the left half (red) comes out on top
	p := clean(t, taggedPNG(t, 4, 2, 6, "s"))
	assert.Equal(t, 2, p.W)
	assert.Equal(t, 4, p.H)

	img, err := png.Decode(bytes.NewReader(p.Bytes))
	require.NoError(t, err)
	assert.Equal(t, color.NRGBA{255, 0, 0, 255}, color.NRGBAModel.Convert(img.At(0, 0)))
	assert.Equal(t, color.NRGBA{255, 0, 0, 255}, color.NRGBAModel.Convert(img.At(1, 1)))
	assert.Equal(t, color.NRGBA{0, 0, 255, 255}, color.NRGBAModel.Convert(img.At(0, 3)))

	j := clean(t, taggedJPEG(t, 32, 16, 6, "s"))
	assert.Equal(t, 16, j.W)
	assert.Equal(t, 32, j.H)
	assert.NotContains(t, string(j.Bytes), "Exif")

	jimg, err := jpeg.Decode(bytes.NewReader(j.Bytes))
	require.NoError(t, err)

	top := color.RGBAModel.Convert(jimg.At(8, 4)).(color.RGBA)
	bottom := color.RGBAModel.Convert(jimg.At(8, 28)).(color.RGBA)
	assert.Greater(t, top.R, top.B, "red on top")
	assert.Greater(t, bottom.B, bottom.R, "blue below")

	// 8: turned the other way, red at the bottom
	e := clean(t, taggedPNG(t, 4, 2, 8, "s"))
	eimg, err := png.Decode(bytes.NewReader(e.Bytes))
	require.NoError(t, err)
	assert.Equal(t, color.NRGBA{0, 0, 255, 255}, color.NRGBAModel.Convert(eimg.At(0, 0)))
	assert.Equal(t, color.NRGBA{255, 0, 0, 255}, color.NRGBAModel.Convert(eimg.At(0, 3)))
}

func TestTheSameFileAlwaysComesOutTheSame(t *testing.T) {
	for _, raw := range [][]byte{taggedJPEG(t, 32, 16, 6, "s"), taggedJPEG(t, 32, 16, 1, "s"), taggedPNG(t, 4, 2, 6, "s")} {
		a := clean(t, raw)
		b := clean(t, append([]byte{}, raw...))
		assert.Equal(t, a.SHA256, b.SHA256)
		assert.Equal(t, a.Bytes, b.Bytes)
	}
}

func TestAFileThatCannotBeTakenApartIsRefusedNotSentRaw(t *testing.T) {
	dir := t.TempDir()

	jpg := taggedJPEG(t, 16, 8, 1, "raw-secret")
	cut := jpg[:bytes.LastIndex(jpg, []byte{0xff, 0xd9})]
	write(t, filepath.Join(dir, "cut.jpg"), cut)

	badCRC := taggedPNG(t, 4, 2, 1, "raw-secret")
	badCRC[bytes.Index(badCRC, []byte("tEXt"))+6] ^= 1
	write(t, filepath.Join(dir, "crc.png"), badCRC)

	b := parse(t, "nodes:\n  a: {title: A, image: cut.jpg}\n  b: {title: B, image: crc.png}\n")
	set := images.Public(b, dir)

	assert.Empty(t, set.Files)
	assert.Nil(t, b.Nodes[0].Image)
	assert.Nil(t, b.Nodes[1].Image)
	require.Len(t, set.Problems, 2)
	assert.Contains(t, set.Problems[0].String(), "cannot strip its metadata")

	// the owner's page refuses it too, and says so
	b = parse(t, "nodes:\n  a: {title: A, image: cut.jpg}\n")
	images.Attach(b, dir)
	assert.Equal(t, images.Unsupported, b.Nodes[0].Image.Error)
}

func TestTheOwnersPageShowsTheFileAndThePublicGetsItStripped(t *testing.T) {
	dir := t.TempDir()
	raw := taggedJPEG(t, 16, 8, 1, "own-secret")
	write(t, filepath.Join(dir, "a.jpg"), raw)

	own := parse(t, "nodes:\n  a: {title: A, image: a.jpg}\n")
	ownSet := images.Attach(own, dir)
	assert.Equal(t, raw, ownSet.Files[own.Nodes[0].Image.SHA256].Bytes)

	pub := parse(t, "nodes:\n  a: {title: A, image: a.jpg}\n")
	pubSet := images.Public(pub, dir)
	assert.NotEqual(t, own.Nodes[0].Image.SHA256, pub.Nodes[0].Image.SHA256)
	assert.NotContains(t, string(pubSet.Files[pub.Nodes[0].Image.SHA256].Bytes), "own-secret")
}

func TestTheOwnersCardIsSizedLikeTheUprightPicture(t *testing.T) {
	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.jpg"), taggedJPEG(t, 32, 16, 6, "s"))

	b := parse(t, "nodes:\n  a: {title: A, image: a.jpg}\n")
	images.Attach(b, dir)
	assert.Equal(t, 16, b.Nodes[0].Image.W)
	assert.Equal(t, 32, b.Nodes[0].Image.H)
}
