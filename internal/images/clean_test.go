package images_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/iamnikolie/kreida/internal/images"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/webp"
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

func fixture(t *testing.T, name string) []byte {
	t.Helper()

	d, err := os.ReadFile(filepath.Join("testdata", name))
	require.NoError(t, err)

	return d
}

func TestAWebPLosesItsEXIFAndXMPAndKeepsItsPictureAlphaAndColour(t *testing.T) {
	raw := append(fixture(t, "meta.webp"), "webp-secret-tail"...)
	require.Contains(t, string(raw), "secret-webp-exif")
	require.Contains(t, string(raw), "secret-icc-desc")

	c := clean(t, raw)
	assert.NotContains(t, string(c.Bytes), "secret")
	assert.NotContains(t, string(c.Bytes), "EXIF")
	assert.NotContains(t, string(c.Bytes), "XMP")
	assert.Contains(t, string(c.Bytes), "ICCP", "the colour profile stays, cleaned")
	assert.Contains(t, string(c.Bytes), "ALPH")
	assert.Equal(t, byte(0x30), c.Bytes[20], "only the ICC and alpha flags are on")
	assert.Equal(t, len(c.Bytes)-8, int(binary.LittleEndian.Uint32(c.Bytes[4:])))
	assert.Equal(t, 12, c.W)
	assert.Equal(t, 7, c.H)

	img, err := webp.Decode(bytes.NewReader(c.Bytes))
	require.NoError(t, err)

	_, _, _, a := img.At(9, 3).RGBA()
	assert.Less(t, a, uint32(0xffff), "the half-transparent half kept its alpha")

	plain := clean(t, fixture(t, "plain.webp"))
	assert.Equal(t, fixture(t, "plain.webp"), plain.Bytes, "a simple WebP has nothing to lose")
}

func TestAWebPThatMovesOrThatEXIFTurnsIsRefused(t *testing.T) {
	_, err := images.Clean(load(t, fixture(t, "turned.webp")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save the picture upright")

	_, err = images.Clean(load(t, fixture(t, "anim.webp")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "animated WebP")
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

// ----- review findings

func TestPixelsThatDoNotDecodeAreRefused(t *testing.T) {
	// a PNG whose IDAT is text with a right checksum: the header reads, the picture does not
	p := taggedPNG(t, 4, 2, 1, "s")
	at := bytes.Index(p, []byte("IDAT")) - 4
	n := int(binary.BigEndian.Uint32(p[at:]))
	bad := slices.Concat(p[:at], pngChunk("IDAT", []byte("private-text-not-zlib")), p[at+12+n:])

	_, err := images.Clean(load(t, bad))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pixels do not decode")

	// a WebP whose lossless bitstream is cut short
	w := fixture(t, "plain.webp")
	cut := append([]byte{}, w[:len(w)-8]...)
	binary.LittleEndian.PutUint32(cut[4:], uint32(len(cut)-8))
	binary.LittleEndian.PutUint32(cut[16:], uint32(len(cut)-20))

	_, err = images.Clean(load(t, cut))
	require.Error(t, err)

	// a GIF frame whose LZW data is not LZW: the frame follows its timing block, its data the code size and length
	g := gifWith(t)
	frame := bytes.Index(g, []byte{0x21, 0xf9, 4}) + 8
	require.Equal(t, byte(0x2c), g[frame])

	data := frame + 10
	if g[frame+9]&0x80 != 0 {
		data += 3 << (g[frame+9]&7 + 1)
	}

	broken := append([]byte{}, g...)
	for i := range int(broken[data+1]) {
		broken[data+2+i] = 0xff
	}

	_, err = images.Clean(load(t, broken))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pixels do not decode")
}

func gifWith(t *testing.T) []byte {
	t.Helper()

	pal := color.Palette{color.Black, color.White}

	var b bytes.Buffer
	frame := func() *image.Paletted { return image.NewPaletted(image.Rect(0, 0, 3, 2), pal) }

	// two frames: Go writes the loop block only for an animation
	require.NoError(t, gif.EncodeAll(&b, &gif.GIF{Image: []*image.Paletted{frame(), frame()}, Delay: []int{10, 10}, LoopCount: 0}))

	return b.Bytes()
}

func TestKeptBlocksKeepOnlyTheirStandardFields(t *testing.T) {
	// Adobe APP14 is 12 bytes; whatever follows them goes
	jpg := taggedJPEG(t, 16, 8, 1, "s")
	withAdobe := slices.Concat(jpg[:2], segment(0xee, []byte("Adobe\x00\x64\x00\x00\x00\x00\x01private-adobe-trailer")), jpg[2:])
	c := clean(t, withAdobe)
	assert.NotContains(t, string(c.Bytes), "private-adobe")
	assert.Contains(t, string(c.Bytes), "Adobe\x00\x64\x00\x00\x00\x00\x01")

	// the loop block keeps its count; the timing block its four bytes
	g := gifWith(t)
	loop := bytes.Index(g, []byte("NETSCAPE2.0")) + 11 + 4 // its terminator
	require.Equal(t, byte(0), g[loop])
	g = slices.Concat(g[:loop], []byte{18}, []byte("private-loop-secret"[:18]), g[loop:])

	gce := bytes.Index(g, []byte{0x21, 0xf9, 4}) + 7 // its terminator
	require.Equal(t, byte(0), g[gce])
	g = slices.Concat(g[:gce], []byte{5}, []byte("gce-x"), g[gce:])

	c = clean(t, g)
	assert.NotContains(t, string(c.Bytes), "private-loop")
	assert.NotContains(t, string(c.Bytes), "gce-x")

	out, err := gif.DecodeAll(bytes.NewReader(c.Bytes))
	require.NoError(t, err)
	assert.Equal(t, 0, out.LoopCount)
	assert.Equal(t, []int{10, 10}, out.Delay)
}

// iccOf finds the profile a cleaned JPEG or PNG carries.
func iccOf(t *testing.T, f images.File) []byte {
	t.Helper()

	switch f.Type {
	case "image/jpeg":
		at := bytes.Index(f.Bytes, []byte("ICC_PROFILE\x00"))
		require.GreaterOrEqual(t, at, 0, "no profile")

		n := int(binary.BigEndian.Uint16(f.Bytes[at-2:]))

		return f.Bytes[at+14 : at-2+n]
	default:
		at := bytes.Index(f.Bytes, []byte("iCCP"))
		require.GreaterOrEqual(t, at, 0, "no profile")

		n := int(binary.BigEndian.Uint32(f.Bytes[at-4:]))
		data := f.Bytes[at+4 : at+4+n]
		name := bytes.IndexByte(data, 0)
		assert.Equal(t, "ICC Profile", string(data[:name]), "the profile's own name does not come along")

		zr, err := zlib.NewReader(bytes.NewReader(data[name+2:]))
		require.NoError(t, err)

		out, err := io.ReadAll(zr)
		require.NoError(t, err)

		return out
	}
}

func withJPEGProfile(jpg, profile []byte) []byte {
	return slices.Concat(jpg[:2], segment(0xe2, slices.Concat([]byte("ICC_PROFILE\x00\x01\x01"), profile)), jpg[2:])
}

func withPNGProfile(t *testing.T, p, profile []byte, name string) []byte {
	t.Helper()

	var z bytes.Buffer

	zw := zlib.NewWriter(&z)
	_, _ = zw.Write(profile)
	require.NoError(t, zw.Close())

	at := bytes.Index(p, []byte("IDAT")) - 4

	return slices.Concat(p[:at], pngChunk("gAMA", []byte{0, 0, 0xb1, 0x8f}), pngChunk("iCCP", slices.Concat([]byte(name+"\x00\x00"), z.Bytes())), p[at:])
}

// assertCleanProfile checks a cleaned profile against the fixture it came from: no words, the colour tags whole.
func assertCleanProfile(t *testing.T, profile []byte) {
	t.Helper()

	assert.NotContains(t, string(profile), "secret")
	assert.NotContains(t, string(profile), "SCRT")
	assert.NotContains(t, string(profile), "MODL")
	assert.Equal(t, len(profile), int(binary.BigEndian.Uint32(profile)))
	assert.Equal(t, "acsp", string(profile[36:40]))
	assert.Contains(t, string(profile), "curv\x00\x00\x00\x00\x00\x00\x00\x01\x02\x33", "the tone curve stays")
	assert.Contains(t, string(profile), "rXYZ")
	assert.Contains(t, string(profile), "desc")
	assert.NotContains(t, string(profile), "dmdd")
	assert.NotContains(t, string(profile), "priv")
}

func TestAColourProfileLosesItsWordsAndKeepsItsColour(t *testing.T) {
	rgb := fixture(t, "rgb.icc")

	j := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), rgb))
	assertCleanProfile(t, iccOf(t, j))

	p := clean(t, withPNGProfile(t, taggedPNG(t, 4, 2, 1, "s"), rgb, "Private-person-machine-profile"))
	assert.NotContains(t, string(p.Bytes), "Private-person")
	assertCleanProfile(t, iccOf(t, p))
	assert.Contains(t, string(p.Bytes), "gAMA")
}

func TestASystemProfileKeepsEveryColourTag(t *testing.T) {
	sys, err := os.ReadFile("/System/Library/ColorSync/Profiles/Display P3.icc")
	if err != nil {
		t.Skip("no Display P3 profile on this machine")
	}

	j := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), sys))
	out := iccOf(t, j)

	n := func(p []byte) int { return int(binary.BigEndian.Uint32(p[128:])) }
	assert.Equal(t, n(sys), n(out), "all of its tags are colour or desc/cprt")
	assert.Equal(t, sys[16:20], out[16:20])
	assert.Equal(t, sys[64:80], out[64:80], "rendering intent and illuminant")
}

func TestATurnedPictureKeepsItsColour(t *testing.T) {
	rgb := fixture(t, "rgb.icc")

	j := clean(t, withJPEGProfile(taggedJPEG(t, 32, 16, 6, "s"), rgb))
	assert.Equal(t, 16, j.W)
	assertCleanProfile(t, iccOf(t, j))

	_, err := jpeg.Decode(bytes.NewReader(j.Bytes))
	require.NoError(t, err)

	p := clean(t, withPNGProfile(t, taggedPNG(t, 4, 2, 6, "s"), rgb, "x"))
	assert.Equal(t, 2, p.W)
	assertCleanProfile(t, iccOf(t, p))
	assert.Contains(t, string(p.Bytes), "gAMA")

	// a grey profile cannot follow a picture that turning makes colour
	_, err = images.Clean(load(t, withJPEGProfile(taggedJPEG(t, 32, 16, 6, "s"), fixture(t, "gray.icc"))))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save the picture upright")
}

func TestABrokenProfileIsRefused(t *testing.T) {
	rgb := fixture(t, "rgb.icc")
	bad := append([]byte{}, rgb...)
	binary.BigEndian.PutUint32(bad[128+4+4:], 1<<30) // the first tag points far past the end

	_, err := images.Clean(load(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), bad)))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "colour profile")

	missing := slices.Concat(taggedJPEG(t, 16, 8, 1, "s")[:2], segment(0xe2, slices.Concat([]byte("ICC_PROFILE\x00\x02\x02"), rgb)), taggedJPEG(t, 16, 8, 1, "s")[2:])
	_, err = images.Clean(load(t, missing))
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "do not add up"))
}
