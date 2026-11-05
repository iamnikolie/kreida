package images_test

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
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

func TestAWebPGoesOutAsAPNGOrAJPEGWithoutItsTags(t *testing.T) {
	raw := append(fixture(t, "meta.webp"), "webp-secret-tail"...)
	require.Contains(t, string(raw), "secret-webp-exif")
	require.Contains(t, string(raw), "secret-icc-desc")

	// lossy with alpha: a PNG, so the alpha stays
	c := clean(t, raw)
	assert.Equal(t, "image/png", c.Type)
	assert.NotContains(t, string(c.Bytes), "secret")
	assertOurProfile(t, iccOf(t, c), "sRGB")
	assert.Equal(t, 12, c.W)
	assert.Equal(t, 7, c.H)

	img, err := png.Decode(bytes.NewReader(c.Bytes))
	require.NoError(t, err)

	_, _, _, a := img.At(9, 3).RGBA()
	assert.Less(t, a, uint32(0xffff), "the half-transparent half kept its alpha")

	// lossless: a PNG with the very same pixels
	plain := clean(t, fixture(t, "plain.webp"))
	assert.Equal(t, "image/png", plain.Type)

	want, err := webp.Decode(bytes.NewReader(fixture(t, "plain.webp")))
	require.NoError(t, err)

	got, err := png.Decode(bytes.NewReader(plain.Bytes))
	require.NoError(t, err)
	assert.Equal(t, color.NRGBAModel.Convert(want.At(3, 3)), color.NRGBAModel.Convert(got.At(3, 3)))

	// lossy and opaque: a JPEG
	opaque := slices.Concat([]byte("RIFF\x00\x00\x00\x00WEBP"), vp8Chunk(t))
	binary.LittleEndian.PutUint32(opaque[4:], uint32(len(opaque)-8))
	assert.Equal(t, "image/jpeg", clean(t, opaque).Type)
}

// vp8Chunk is the lossy bitstream chunk of meta.webp: an opaque lossy WebP once it stands alone.
func vp8Chunk(t *testing.T) []byte {
	t.Helper()

	d := fixture(t, "meta.webp")
	at := bytes.Index(d, []byte("VP8 "))
	n := int(binary.LittleEndian.Uint32(d[at+4:]))

	return d[at : at+8+n+n&1]
}

func TestAWebPThatEXIFTurnsIsTurnedAndAnAnimatedOneRefused(t *testing.T) {
	c := clean(t, fixture(t, "turned.webp"))
	assert.Equal(t, 7, c.W)
	assert.Equal(t, 12, c.H)
	assert.NotContains(t, string(c.Bytes), "secret")

	_, err := images.Clean(load(t, fixture(t, "anim.webp")))
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

	// the loop block keeps its count; the timing block its four bytes
	g := gifWith(t)
	loop := bytes.Index(g, []byte("NETSCAPE2.0")) + 11 + 4 // its terminator
	require.Equal(t, byte(0), g[loop])
	g = slices.Concat(g[:loop], []byte{18}, []byte("private-loop-secret"[:18]), g[loop:])

	c = clean(t, g)
	assert.NotContains(t, string(c.Bytes), "private-loop")

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

// assertOurProfile checks that a picture carries kreida's own copy of a standard profile, and nothing of the one it had.
func assertOurProfile(t *testing.T, profile []byte, name string) {
	t.Helper()

	assert.NotContains(t, string(profile), "secret")
	assert.NotContains(t, string(profile), "SCRT")
	assert.NotContains(t, string(profile), "MODL")
	assert.Contains(t, string(profile), name)
	assert.Contains(t, string(profile), "No copyright, use freely")
	assert.Equal(t, len(profile), int(binary.BigEndian.Uint32(profile)))
	assert.Equal(t, "acsp", string(profile[36:40]))
}

func TestAKnownProfileIsReplacedByKreidasOwn(t *testing.T) {
	rgb := fixture(t, "rgb.icc")

	j := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), rgb))
	assertOurProfile(t, iccOf(t, j), "sRGB")
	assert.Empty(t, j.Note)

	p := clean(t, withPNGProfile(t, taggedPNG(t, 4, 2, 1, "s"), rgb, "Private-person-machine-profile"))
	assert.NotContains(t, string(p.Bytes), "Private-person")
	assert.NotContains(t, string(p.Bytes), "gAMA")
	assertOurProfile(t, iccOf(t, p), "sRGB")

	// whatever the sRGB profile said, the copy that goes out is the same
	assert.Equal(t, iccOf(t, j), iccOf(t, p))
}

func TestAnyOtherProfileIsDroppedAndTheOwnerTold(t *testing.T) {
	for name, profile := range map[string][]byte{
		"unknown curve": fixture(t, "odd.icc"),
		"grey":          fixture(t, "gray.icc"),
		"unreadable":    []byte("not a profile, private-unreadable"),
	} {
		c := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), profile))
		assert.NotContains(t, string(c.Bytes), "ICC_PROFILE", name)
		assert.NotContains(t, string(c.Bytes), "secret", name)
		assert.NotContains(t, string(c.Bytes), "private", name)
		assert.Contains(t, c.Note, "colours may shift", name)
	}

	dir := t.TempDir()
	write(t, filepath.Join(dir, "a.jpg"), withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), fixture(t, "odd.icc")))

	b := parse(t, "nodes:\n  a: {title: A, image: a.jpg}\n")
	set := images.Public(b, dir)
	require.NotNil(t, b.Nodes[0].Image, "the picture still goes")
	require.Len(t, set.Notes, 1)
	assert.Contains(t, set.Notes[0].String(), "a.jpg")

	// a picture without a profile has nothing to say
	assert.Empty(t, clean(t, taggedJPEG(t, 16, 8, 1, "s")).Note)
}

func TestASystemProfileIsRecognised(t *testing.T) {
	sys, err := os.ReadFile("/System/Library/ColorSync/Profiles/Display P3.icc")
	if err != nil {
		t.Skip("no Display P3 profile on this machine")
	}

	j := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), sys))
	out := iccOf(t, j)
	assertOurProfile(t, out, "Display P3")
	assert.NotContains(t, string(out), "Apple")
}

func TestATurnedPictureKeepsItsColour(t *testing.T) {
	rgb := fixture(t, "rgb.icc")

	j := clean(t, withJPEGProfile(taggedJPEG(t, 32, 16, 6, "s"), rgb))
	assert.Equal(t, 16, j.W)
	assertOurProfile(t, iccOf(t, j), "sRGB")

	_, err := jpeg.Decode(bytes.NewReader(j.Bytes))
	require.NoError(t, err)

	p := clean(t, withPNGProfile(t, taggedPNG(t, 4, 2, 6, "s"), rgb, "x"))
	assert.Equal(t, 2, p.W)
	assertOurProfile(t, iccOf(t, p), "sRGB")

	// a grey picture stays grey when turned
	var gray bytes.Buffer
	require.NoError(t, jpeg.Encode(&gray, image.NewGray(image.Rect(0, 0, 32, 16)), nil))

	exif := segment(0xe1, append([]byte("Exif\x00\x00"), exifTIFF(6, "s")...))
	g := clean(t, withJPEGProfile(slices.Concat(gray.Bytes()[:2], exif, gray.Bytes()[2:]), fixture(t, "gray.icc")))
	assert.Equal(t, 16, g.W)

	gimg, err := jpeg.Decode(bytes.NewReader(g.Bytes))
	require.NoError(t, err)
	assert.IsType(t, &image.Gray{}, gimg)
}

func TestAProfileInPiecesThatDoNotAddUpIsDropped(t *testing.T) {
	rgb := fixture(t, "rgb.icc")
	jpg := taggedJPEG(t, 16, 8, 1, "s")

	missing := slices.Concat(jpg[:2], segment(0xe2, slices.Concat([]byte("ICC_PROFILE\x00\x02\x02"), rgb)), jpg[2:])
	c := clean(t, missing)
	assert.NotContains(t, string(c.Bytes), "ICC_PROFILE")
	assert.Contains(t, c.Note, "colours may shift")
}

func TestUnusedPaletteEntriesDoNotRideOut(t *testing.T) {
	// a palette whose unused colours spell text
	pal := color.Palette{color.NRGBA{0, 0, 0, 255}, color.NRGBA{255, 255, 255, 255}}
	for _, ch := range []byte("private-palette-text") {
		pal = append(pal, color.NRGBA{ch, ch, ch, 255})
	}

	img := image.NewPaletted(image.Rect(0, 0, 4, 2), pal)
	img.Pix[3] = 1

	var p, g bytes.Buffer
	require.NoError(t, png.Encode(&p, img))
	require.NoError(t, gif.EncodeAll(&g, &gif.GIF{Image: []*image.Paletted{img, img}, Delay: []int{5, 5}}))

	spelled := func(b []byte) bool { return bytes.Contains(b, []byte("ppprrriii")) }
	require.True(t, spelled(p.Bytes()))
	require.True(t, spelled(g.Bytes()))

	for _, raw := range [][]byte{p.Bytes(), g.Bytes()} {
		c := clean(t, raw)
		assert.False(t, spelled(c.Bytes), c.Type)
	}

	out, err := gif.DecodeAll(bytes.NewReader(clean(t, g.Bytes()).Bytes))
	require.NoError(t, err)
	assert.Len(t, out.Image, 2)
	assert.Len(t, out.Image[0].Palette, 2)
}

// ----- second review

func TestAWebPWhoseFrameIsNotItsHeadersSizeIsRefusedBeforeDecoding(t *testing.T) {
	// 54 bytes: a 1×1 canvas over a lossless frame of 6401×6250
	tiny, err := hex.DecodeString("524946462e00000057454250565038580a000000000000000000000000005650384c100000002f00591a068888080000000000000000")
	require.NoError(t, err)

	f := load(t, tiny)
	require.Equal(t, 1, f.W)

	var before, after runtime.MemStats

	runtime.GC()
	runtime.ReadMemStats(&before)

	_, err = images.Clean(f)

	runtime.ReadMemStats(&after)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not the size its header says")
	assert.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(8<<20), "nothing of the frame's size was allocated")

	// committed plain.webp (12×7) under a 1×1 VP8X
	inner := fixture(t, "plain.webp")[12:]
	wrapped := slices.Concat([]byte("RIFF\x00\x00\x00\x00WEBPVP8X\x0a\x00\x00\x00"), make([]byte, 10), inner)
	binary.LittleEndian.PutUint32(wrapped[4:], uint32(len(wrapped)-8))

	_, err = images.Clean(load(t, wrapped))
	require.Error(t, err)
}

// profileWith is rgb.icc with its rTRC pointing at body, appended at its end.
func profileWith(t *testing.T, body []byte) []byte {
	t.Helper()

	p := append([]byte{}, fixture(t, "rgb.icc")...)
	off := len(p)
	p = append(p, body...)
	binary.BigEndian.PutUint32(p, uint32(len(p)))

	for i := range int(binary.BigEndian.Uint32(p[128:])) {
		e := 128 + 4 + 12*i
		if string(p[e:e+4]) == "rTRC" {
			binary.BigEndian.PutUint32(p[e+4:], uint32(off))
			binary.BigEndian.PutUint32(p[e+8:], uint32(len(body)))
		}
	}

	return p
}

func TestNothingOfAForeignProfileGoesOut(t *testing.T) {
	points := []byte("curv\x00\x00\x00\x00\x00\x00\x00\x0a")
	points = append(points, "private-curve-text!!"...)

	for name, body := range map[string][]byte{
		"text after a curve":    []byte("curv\x00\x00\x00\x00\x00\x00\x00\x01\x02\x33private-profile-owner@example.test"),
		"text in reserved":      []byte("curvUSER\x00\x00\x00\x01\x02\x33\x00\x00"),
		"text after no points":  []byte("curv\x00\x00\x00\x00\x00\x00\x00\x00private-zero-count"),
		"text after parameters": []byte("paraSECR\x00\x00PRIV\x00\x02\x33\x33private-para"),
		"text as curve points":  points,
	} {
		c := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), profileWith(t, body)))

		for _, word := range []string{"private", "USER", "SECR", "PRIV", "secret"} {
			assert.NotContains(t, string(c.Bytes), word, name)
		}
	}

	// sRGB with a tag twice cannot be read: dropped
	twice := append([]byte{}, fixture(t, "rgb.icc")...)
	copy(twice[128+4+12*3:], "wtpt")

	c := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), twice))
	assert.NotContains(t, string(c.Bytes), "ICC_PROFILE")
}

func TestOverlappingTagsCannotBlowAProfileUp(t *testing.T) {
	// a hundred tags; all but the matrix point at one 128 KiB curve, each a little shorter than the last
	const n, size = 100, 128 << 10

	table := 128 + 4 + 12*n
	xyz, curve := table, table+20
	p := make([]byte, curve+size)
	binary.BigEndian.PutUint32(p, uint32(len(p)))
	p[8] = 2
	copy(p[12:], "mntrRGB XYZ ")
	copy(p[36:], "acsp")
	binary.BigEndian.PutUint32(p[128:], n)
	copy(p[xyz:], "XYZ \x00\x00\x00\x00\x00\x00\xf6\xd6\x00\x01\x00\x00\x00\x00\xd3\x2d")
	copy(p[curve:], "curv")
	binary.BigEndian.PutUint32(p[curve+8:], 4096)

	for i := range n {
		e := 128 + 4 + 12*i

		switch {
		case i < 4:
			copy(p[e:], []string{"wtpt", "rXYZ", "gXYZ", "bXYZ"}[i])
			binary.BigEndian.PutUint32(p[e+4:], uint32(xyz))
			binary.BigEndian.PutUint32(p[e+8:], 20)
		default:
			sig := fmt.Sprintf("t%03d", i)
			if i < 7 {
				sig = []string{"rTRC", "gTRC", "bTRC"}[i-4]
			}

			copy(p[e:], sig)
			binary.BigEndian.PutUint32(p[e+4:], uint32(curve))
			binary.BigEndian.PutUint32(p[e+8:], uint32(size-4*i))
		}
	}

	c := clean(t, withPNGProfile(t, taggedPNG(t, 4, 2, 1, "s"), p, "x"))
	assert.Less(t, len(c.Bytes), 4<<10, "nothing of the profile is copied, however it is laid out")
	assert.NotContains(t, string(c.Bytes), "iCCP")
}

func TestUnusedBytesInsideAStreamDoNotRideOut(t *testing.T) {
	jpg := taggedJPEG(t, 16, 8, 1, "s")
	end := bytes.LastIndex(jpg, []byte{0xff, 0xd9})
	scan := slices.Concat(jpg[:end], []byte("private-unused-scan-owner@example.test"), jpg[end:])

	_, err := jpeg.Decode(bytes.NewReader(scan))
	require.NoError(t, err, "the decoder skips it")
	assert.NotContains(t, string(clean(t, scan).Bytes), "private-unused")

	w := fixture(t, "plain.webp")
	n := int(binary.LittleEndian.Uint32(w[16:]))
	extra := "private-unused-webp-owner@example.test"
	chunk := slices.Concat([]byte("VP8L"), binary.LittleEndian.AppendUint32(nil, uint32(n+len(extra))), w[20:20+n], []byte(extra))
	if len(chunk)%2 == 1 {
		chunk = append(chunk, 0)
	}

	webpIn := slices.Concat([]byte("RIFF\x00\x00\x00\x00WEBP"), chunk)
	binary.LittleEndian.PutUint32(webpIn[4:], uint32(len(webpIn)-8))

	_, err = webp.Decode(bytes.NewReader(webpIn))
	require.NoError(t, err, "the decoder skips it")
	assert.NotContains(t, string(clean(t, webpIn).Bytes), "private-unused")
}

// ----- third review

func TestAGIFKeepsItsLookWithOnlyTheColoursItUses(t *testing.T) {
	// 4×2 frames, all index 0, the rest of each palette spelling text; a canvas larger than the frames, offsets,
	// disposal, timing and a background colour on the screen's palette
	spell := func(s string) color.Palette {
		pal := color.Palette{color.NRGBA{10, 20, 30, 255}}
		for i := 0; i+2 < len(s); i += 3 {
			pal = append(pal, color.NRGBA{s[i], s[i+1], s[i+2], 255})
		}

		return pal
	}

	screen := spell("private-global-palette-text!")
	screen[0] = color.NRGBA{200, 100, 50, 255}

	first := image.NewPaletted(image.Rect(0, 0, 4, 2), spell("private-local-one-text!!"))
	second := image.NewPaletted(image.Rect(3, 4, 7, 6), spell("private-local-two-text!!"))
	second.Pix[5] = 1

	var raw bytes.Buffer
	require.NoError(t, gif.EncodeAll(&raw, &gif.GIF{
		Image: []*image.Paletted{first, second}, Delay: []int{7, 9}, LoopCount: 2,
		Disposal: []byte{gif.DisposalBackground, gif.DisposalPrevious},
		Config:   image.Config{ColorModel: screen, Width: 10, Height: 8}, BackgroundIndex: 0,
	}))
	require.Contains(t, raw.String(), "private")

	c := clean(t, raw.Bytes())
	assert.NotContains(t, string(c.Bytes), "private")
	assert.NotContains(t, string(c.Bytes), "text")
	assert.Equal(t, 10, c.W)
	assert.Equal(t, 8, c.H)

	in, err := gif.DecodeAll(bytes.NewReader(raw.Bytes()))
	require.NoError(t, err)

	out, err := gif.DecodeAll(bytes.NewReader(c.Bytes))
	require.NoError(t, err)

	assert.Equal(t, in.Config.Width, out.Config.Width)
	assert.Equal(t, in.Config.Height, out.Config.Height)
	assert.Equal(t, in.Delay, out.Delay)
	assert.Equal(t, in.Disposal, out.Disposal)
	assert.Equal(t, in.LoopCount, out.LoopCount)
	assert.Equal(t, color.NRGBAModel.Convert(screen[0]), color.NRGBAModel.Convert(out.Config.ColorModel.(color.Palette)[out.BackgroundIndex]), "the background colour")

	require.Len(t, out.Image, 2)

	for i := range in.Image {
		assert.Equal(t, in.Image[i].Bounds(), out.Image[i].Bounds(), "frame %d offset", i)

		b := in.Image[i].Bounds()
		for y := b.Min.Y; y < b.Max.Y; y++ {
			for x := b.Min.X; x < b.Max.X; x++ {
				assert.Equal(t, color.NRGBAModel.Convert(in.Image[i].At(x, y)), color.NRGBAModel.Convert(out.Image[i].At(x, y)), "frame %d at %d,%d", i, x, y)
			}
		}
	}

	// a GIF table holds at least two colours: a one-colour frame is padded with black, which says nothing
	assert.Len(t, out.Image[0].Palette, 2)
	assert.Equal(t, color.NRGBAModel.Convert(color.Black), color.NRGBAModel.Convert(out.Image[0].Palette[1]))
	assert.Len(t, out.Image[1].Palette, 2)
}

func TestAPNGWithAPaletteKeepsItsLookWithOnlyTheColoursItUses(t *testing.T) {
	pal := color.Palette{color.NRGBA{10, 20, 30, 255}, color.NRGBA{40, 50, 60, 128}}
	for _, ch := range []byte("private-unused-entries") {
		pal = append(pal, color.NRGBA{ch, ch, ch, 255})
	}

	img := image.NewPaletted(image.Rect(0, 0, 4, 2), pal)
	img.Pix[6] = 1

	var raw bytes.Buffer
	require.NoError(t, png.Encode(&raw, img))
	require.Contains(t, raw.String(), "ppprrriii")

	c := clean(t, raw.Bytes())
	assert.NotContains(t, string(c.Bytes), "ppprrriii")

	out, err := png.Decode(bytes.NewReader(c.Bytes))
	require.NoError(t, err)
	require.IsType(t, &image.Paletted{}, out)
	assert.Len(t, out.(*image.Paletted).Palette, 2)

	for y := range 2 {
		for x := range 4 {
			assert.Equal(t, color.NRGBAModel.Convert(img.At(x, y)), color.NRGBAModel.Convert(out.At(x, y)))
		}
	}
}

// ----- fourth review

// withTag adds a tag to a profile: one more entry in its table, the body at its end.
func withTag(p []byte, sig string, body []byte) []byte {
	n := int(binary.BigEndian.Uint32(p[128:]))
	table := 128 + 4 + 12*n

	out := slices.Concat(p[:table], make([]byte, 12), p[table:])
	for i := range n {
		e := 128 + 4 + 12*i
		binary.BigEndian.PutUint32(out[e+4:], binary.BigEndian.Uint32(out[e+4:])+12)
	}

	for len(out)%4 != 0 {
		out = append(out, 0)
	}

	copy(out[table:], sig)
	binary.BigEndian.PutUint32(out[table+4:], uint32(len(out)))
	binary.BigEndian.PutUint32(out[table+8:], uint32(len(body)))
	binary.BigEndian.PutUint32(out[128:], uint32(n+1))

	out = append(out, body...)
	binary.BigEndian.PutUint32(out, uint32(len(out)))

	return out
}

// lut8 is a valid three-channel mft1 transform: identity matrix and tables, two grid points, every grid point black.
func lut8() []byte {
	b := slices.Concat([]byte("mft1\x00\x00\x00\x00"), []byte{3, 3, 2, 0})
	for i := range 9 {
		v := uint32(0)
		if i%4 == 0 {
			v = 1 << 16
		}

		b = binary.BigEndian.AppendUint32(b, v)
	}

	ramp := make([]byte, 256)
	for i := range ramp {
		ramp[i] = byte(i)
	}

	for range 3 {
		b = append(b, ramp...)
	}

	b = append(b, make([]byte, 8*3)...)

	for range 3 {
		b = append(b, ramp...)
	}

	return b
}

func TestAProfileWhoseColourIsNotItsMatrixIsNotTakenForAStandard(t *testing.T) {
	rgb := fixture(t, "rgb.icc")
	require.Equal(t, 1608, len(lut8()))

	header := func(at int, b string) []byte {
		p := append([]byte{}, rgb...)
		copy(p[at:], b)

		return p
	}

	for name, profile := range map[string][]byte{
		// the matrix and curves of sRGB, with a table that turns red black: a CMM uses the table
		"A2B0 table":         withTag(rgb, "A2B0", lut8()),
		"B2A0 table":         withTag(rgb, "B2A0", lut8()),
		"D2B0 multi-process": withTag(rgb, "D2B0", []byte("mpet\x00\x00\x00\x00\x00\x03\x00\x03\x00\x00\x00\x00")),
		"code points":        withTag(rgb, "cicp", []byte("cicp\x00\x00\x00\x00\x09\x10\x00\x01")),
		"not a display":      header(12, "scnr"),
		"a later version":    header(8, "\x05"),
		"another illuminant": header(68, "\x00\x00\xf3\x51\x00\x01\x00\x00\x00\x01\x16\xcc"),
	} {
		c := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), profile))
		assert.NotContains(t, string(c.Bytes), "ICC_PROFILE", name)
		assert.Contains(t, c.Note, "colours may shift", name)
	}

	// the same profile with only names and private tags added is still sRGB
	plain := clean(t, withJPEGProfile(taggedJPEG(t, 16, 8, 1, "s"), withTag(rgb, "zzzz", []byte("zzzz\x00\x00\x00\x00private"))))
	assertOurProfile(t, iccOf(t, plain), "sRGB")
	assert.Empty(t, plain.Note)
}

func TestAGIFWhoseFirstFrameIsSmallerThanItsCanvasIsKept(t *testing.T) {
	pal := color.Palette{color.Black, color.White}
	first, second := image.NewPaletted(image.Rect(0, 0, 2, 2), pal), image.NewPaletted(image.Rect(2, 2, 4, 4), pal)
	second.Pix[1] = 1

	in := &gif.GIF{
		Image: []*image.Paletted{first, second}, Delay: []int{5, 10}, LoopCount: 0,
		Disposal: []byte{gif.DisposalNone, gif.DisposalBackground},
		Config:   image.Config{Width: 4, Height: 4, ColorModel: pal},
	}

	var raw bytes.Buffer
	require.NoError(t, gif.EncodeAll(&raw, in))

	c := clean(t, raw.Bytes())
	assert.Equal(t, 4, c.W)
	assert.Equal(t, 4, c.H)

	out, err := gif.DecodeAll(bytes.NewReader(c.Bytes))
	require.NoError(t, err)
	assert.Equal(t, 4, out.Config.Width)
	assert.Equal(t, 4, out.Config.Height)
	assert.Equal(t, in.Delay, out.Delay)
	assert.Equal(t, in.Disposal, out.Disposal)
	assert.Equal(t, in.LoopCount, out.LoopCount)
	require.Len(t, out.Image, 2)

	for i, f := range in.Image {
		assert.Equal(t, f.Bounds(), out.Image[i].Bounds(), "frame %d offset", i)

		for y := f.Rect.Min.Y; y < f.Rect.Max.Y; y++ {
			for x := f.Rect.Min.X; x < f.Rect.Max.X; x++ {
				assert.Equal(t, color.NRGBAModel.Convert(f.At(x, y)), color.NRGBAModel.Convert(out.Image[i].At(x, y)), "frame %d at %d,%d", i, x, y)
			}
		}
	}
}
