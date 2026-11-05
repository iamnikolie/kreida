package images_test

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"

	"github.com/stretchr/testify/require"
)

// exifTIFF is an EXIF block with an orientation, a description holding `secret` and a GPS directory: N 50° 27' 12.34".
func exifTIFF(orient uint16, secret string) []byte {
	le := binary.LittleEndian
	desc := append([]byte(secret), 0)
	descOff := 8 + 2 + 3*12 + 4
	gpsOff := descOff + len(desc) + len(desc)%2
	ratOff := gpsOff + 2 + 2*12 + 4

	var b bytes.Buffer

	w := func(v any) { _ = binary.Write(&b, le, v) }
	entry := func(tag, typ uint16, count, value uint32) { w(tag); w(typ); w(count); w(value) }

	b.WriteString("II")
	w(uint16(42))
	w(uint32(8))
	w(uint16(3))
	entry(0x010e, 2, uint32(len(desc)), uint32(descOff))
	entry(0x0112, 3, 1, uint32(orient))
	entry(0x8825, 4, 1, uint32(gpsOff))
	w(uint32(0))
	b.Write(desc)

	for b.Len() < gpsOff {
		b.WriteByte(0)
	}

	w(uint16(2))
	entry(0x0001, 2, 2, uint32('N'))
	entry(0x0002, 5, 3, uint32(ratOff))
	w(uint32(0))

	for _, r := range []uint32{50, 1, 27, 1, 1234, 100} {
		w(r)
	}

	return b.Bytes()
}

// halves is a w×h picture, red on its left half and blue on its right.
func halves(w, h int) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))

	for y := range h {
		for x := range w {
			c := color.NRGBA{255, 0, 0, 255}
			if x >= w/2 {
				c = color.NRGBA{0, 0, 255, 255}
			}

			img.SetNRGBA(x, y, c)
		}
	}

	return img
}

func segment(marker byte, body []byte) []byte {
	return append([]byte{0xff, marker, byte((len(body) + 2) >> 8), byte(len(body) + 2)}, body...)
}

// taggedJPEG is a JPEG of `halves` carrying EXIF with GPS, XMP, IPTC, a comment and bytes after its end, each with secret.
func taggedJPEG(t *testing.T, w, h int, orient uint16, secret string) []byte {
	t.Helper()

	var plain bytes.Buffer
	require.NoError(t, jpeg.Encode(&plain, halves(w, h), &jpeg.Options{Quality: 100}))

	d := plain.Bytes()
	out := append([]byte{}, d[:2]...)
	out = append(out, segment(0xe1, append([]byte("Exif\x00\x00"), exifTIFF(orient, secret+"-exif")...))...)
	out = append(out, segment(0xe1, []byte("http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta>"+secret+"-xmp</x:xmpmeta>"))...)
	out = append(out, segment(0xed, []byte("Photoshop 3.0\x008BIM\x04\x04\x00\x00\x00\x00\x00\x10\x1c\x02\x78\x00\x0b"+secret+"-iptc"))...)
	out = append(out, segment(0xfe, []byte(secret+"-comment"))...)
	out = append(out, d[2:]...)

	return append(out, secret+"-trailer"...)
}

func pngChunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)

	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(append([]byte(typ), data...)))
}

// taggedPNG is a PNG of `halves` with text chunks, an eXIf with GPS and a time, each with secret, and bytes after IEND.
func taggedPNG(t *testing.T, w, h int, orient uint16, secret string) []byte {
	t.Helper()

	var plain bytes.Buffer
	require.NoError(t, png.Encode(&plain, halves(w, h)))

	d := plain.Bytes()
	at := bytes.Index(d, []byte("IDAT")) - 4

	out := append([]byte{}, d[:at]...)
	out = append(out, pngChunk("tEXt", []byte("Comment\x00"+secret+"-text"))...)
	out = append(out, pngChunk("zTXt", []byte("Author\x00\x00"+secret))...)
	out = append(out, pngChunk("iTXt", []byte("XML:com.adobe.xmp\x00\x00\x00\x00\x00"+secret+"-xmp"))...)
	out = append(out, pngChunk("eXIf", exifTIFF(orient, secret+"-exif"))...)
	out = append(out, pngChunk("tIME", []byte{0x07, 0xea, 11, 5, 12, 0, 0})...)
	out = append(out, d[at:]...)

	return append(out, secret+"-trailer"...)
}
