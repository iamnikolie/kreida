package images

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"sync"

	"github.com/iamnikolie/kreida/internal/board"
)

// jpegQuality is the quality of a JPEG that had to be drawn again to stand upright.
const jpegQuality = 92

// cacheBytes bounds the memory of the cleaned images kept for the next request.
const cacheBytes = 64 << 20

// Clean returns an image as it may leave this machine: no EXIF, XMP, IPTC, comments, text chunks or bytes after the
// end of the picture, whatever the format. Only what draws the picture is kept, and a colour profile. A photo that
// stands upright through its EXIF orientation is turned in its pixels first, so it does not fall over when the
// orientation goes. The result says what it is: its own hash, type and pixel size. It fails closed: a file it cannot
// take apart is refused, and its bytes never go anywhere. The same file always comes out as the same bytes.
func Clean(f File) (File, error) {
	cache.Lock()
	r, ok := cache.m[f.SHA256]
	cache.Unlock()

	if ok {
		return r.f, r.err
	}

	c, err := clean(f)

	cache.Lock()
	defer cache.Unlock()

	if cache.size+len(c.Bytes) > cacheBytes {
		cache.m, cache.size = map[string]cleaned{}, 0
	}

	cache.m[f.SHA256] = cleaned{c, err}
	cache.size += len(c.Bytes)

	return c, err
}

type cleaned struct {
	f   File
	err error
}

var cache = struct {
	sync.Mutex
	m    map[string]cleaned
	size int
}{m: map[string]cleaned{}}

func clean(f File) (File, error) {
	var (
		out    []byte
		orient = 1
		err    error
	)

	switch f.Type {
	case "image/jpeg":
		out, orient, err = stripJPEG(f.Bytes)
	case "image/png":
		out, orient, err = stripPNG(f.Bytes)
	case "image/gif":
		out, err = stripGIF(f.Bytes)
	case "image/webp":
		out, orient, err = stripWebP(f.Bytes)
	default:
		err = errors.New("unknown type")
	}

	if err == nil && orient != 1 {
		switch f.Type {
		case "image/jpeg":
			out, err = turnJPEG(out, orient)
		case "image/png":
			out, err = turnPNG(out, orient)
		default:
			err = errors.New("its EXIF turns it, which kreida cannot do for a WebP; save the picture upright")
		}
	}

	if err != nil {
		return File{}, refuse(Unsupported, "cannot strip its metadata: %s", err)
	}

	typ, w, h, err := sniff(out)

	wantW, wantH := f.W, f.H
	if orient >= 5 {
		wantW, wantH = f.H, f.W
	}

	if err != nil || typ != f.Type || w != wantW || h != wantH {
		return File{}, refuse(Unsupported, "cannot strip its metadata: the result does not read as the same picture")
	}

	if len(out) > MaxFile {
		return File{}, refuse(TooBig, "over the %d MiB limit once stripped", MaxFile>>20)
	}

	sum := sha256.Sum256(out)

	return File{Image: board.Image{SHA256: hex.EncodeToString(sum[:]), Type: typ, W: w, H: h}, Bytes: out}, nil
}

// ----- JPEG

var (
	exifID = []byte("Exif\x00\x00")
	iccID  = []byte("ICC_PROFILE\x00")
)

// stripJPEG keeps the segments that draw the picture (tables, frame, scans), the ICC profile and Adobe's colour
// transform, and drops every other APPn (EXIF, XMP, IPTC, thumbnails), comments and whatever follows the end marker.
// It also reads the EXIF orientation. A marker it does not know fails it.
func stripJPEG(d []byte) ([]byte, int, error) {
	if len(d) < 4 || d[0] != 0xff || d[1] != 0xd8 {
		return nil, 0, errors.New("not a JPEG")
	}

	out := []byte{0xff, 0xd8}
	orient, exif := 1, false

	for i := 2; ; {
		if i >= len(d) || d[i] != 0xff {
			return nil, 0, errors.New("JPEG has no marker where one belongs, or ends before its end")
		}

		for i < len(d) && d[i] == 0xff {
			i++
		}

		if i >= len(d) {
			return nil, 0, errors.New("JPEG ends before its end marker")
		}

		m := d[i]
		i++

		switch {
		case m == 0xd9:
			return append(out, 0xff, 0xd9), orient, nil
		case m >= 0xd0 && m <= 0xd7 || m == 0x01:
			out = append(out, 0xff, m)

			continue
		}

		if i+2 > len(d) {
			return nil, 0, errors.New("JPEG ends inside a segment")
		}

		n := int(binary.BigEndian.Uint16(d[i:]))
		if n < 2 || i+n > len(d) {
			return nil, 0, errors.New("JPEG segment runs past the end")
		}

		seg, body := d[i:i+n], d[i+2:i+n]
		keep := false

		switch {
		case m == 0xe1 && bytes.HasPrefix(body, exifID):
			if !exif {
				exif, orient = true, orientation(body[len(exifID):])
			}
		case m == 0xe2 && bytes.HasPrefix(body, iccID), m == 0xee && bytes.HasPrefix(body, []byte("Adobe")):
			keep = true
		case m >= 0xe0 && m <= 0xef, m == 0xfe:
		case m == 0xda:
			out = append(out, 0xff, m)
			out = append(out, seg...)
			i += n

			j := i
			for j+1 < len(d) && (d[j] != 0xff || d[j+1] == 0 || d[j+1] >= 0xd0 && d[j+1] <= 0xd7) {
				if d[j] == 0xff {
					j++
				}

				j++
			}

			out = append(out, d[i:j]...)
			i = j

			continue
		case m == 0xdb, m == 0xc4, m == 0xdd, m == 0xdc, m == 0xcc, m >= 0xc0 && m <= 0xcf && m != 0xc8:
			keep = true
		default:
			return nil, 0, fmt.Errorf("JPEG marker %#x is not one kreida knows", m)
		}

		if keep {
			out = append(out, 0xff, m)
			out = append(out, seg...)
		}

		i += n
	}
}

func turnJPEG(d []byte, orient int) ([]byte, error) {
	src, err := jpeg.Decode(bytes.NewReader(d))
	if err != nil {
		return nil, err
	}

	up := upright(src, orient)

	var out bytes.Buffer

	// a JPEG has no alpha: every pixel is opaque, so its NRGBA bytes are its RGBA bytes
	if err := jpeg.Encode(&out, &image.RGBA{Pix: up.Pix, Stride: up.Stride, Rect: up.Rect}, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// ----- PNG

// pngKeep are the chunks that draw a PNG (APNG frames too) and its colour: no text, no eXIf, no time, nothing unknown.
var pngKeep = map[string]bool{
	"IHDR": true, "PLTE": true, "IDAT": true, "IEND": true, "tRNS": true, "gAMA": true, "cHRM": true, "sRGB": true,
	"iCCP": true, "sBIT": true, "bKGD": true, "pHYs": true, "acTL": true, "fcTL": true, "fdAT": true,
}

func stripPNG(d []byte) ([]byte, int, error) {
	if !bytes.HasPrefix(d, []byte(pngSignature)) {
		return nil, 0, errors.New("not a PNG")
	}

	out := []byte(pngSignature)
	orient, exif, anim := 1, false, false

	for i := len(pngSignature); ; {
		if i+12 > len(d) {
			return nil, 0, errors.New("PNG ends before IEND")
		}

		n := int(binary.BigEndian.Uint32(d[i:]))
		if n > len(d)-i-12 {
			return nil, 0, errors.New("PNG chunk runs past the end")
		}

		chunk := d[i : i+12+n]
		typ := string(chunk[4:8])

		if crc32.ChecksumIEEE(chunk[4:8+n]) != binary.BigEndian.Uint32(chunk[8+n:]) {
			return nil, 0, fmt.Errorf("PNG chunk %q has a bad checksum", typ)
		}

		if i == len(pngSignature) && typ != "IHDR" {
			return nil, 0, errors.New("PNG does not start with IHDR")
		}

		switch typ {
		case "eXIf":
			if !exif {
				exif, orient = true, orientation(chunk[8:8+n])
			}
		case "acTL":
			anim = true
		}

		if pngKeep[typ] {
			out = append(out, chunk...)
		}

		i += 12 + n

		if typ == "IEND" {
			break
		}
	}

	if orient != 1 && anim {
		return nil, 0, errors.New("an animated PNG that its EXIF turns; save it upright")
	}

	return out, orient, nil
}

const pngSignature = "\x89PNG\r\n\x1a\n"

func turnPNG(d []byte, orient int) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(d))
	if err != nil {
		return nil, err
	}

	var out bytes.Buffer
	if err := png.Encode(&out, upright(src, orient)); err != nil {
		return nil, err
	}

	return out.Bytes(), nil
}

// ----- GIF

// stripGIF keeps the screen, the colour tables, the frames with their timing and the loop count; comments, plain text
// and every other application block (XMP among them) go, and so does anything after the trailer.
func stripGIF(d []byte) ([]byte, error) {
	if len(d) < 13 {
		return nil, errors.New("GIF too short")
	}

	i := 13
	if d[10]&0x80 != 0 {
		i += 3 << (d[10]&7 + 1)
	}

	if i > len(d) {
		return nil, errors.New("GIF ends inside its colour table")
	}

	out := append([]byte{}, d[:i]...)

	for {
		if i >= len(d) {
			return nil, errors.New("GIF ends before its trailer")
		}

		switch d[i] {
		case 0x3b:
			return append(out, 0x3b), nil
		case 0x21:
			if i+2 > len(d) {
				return nil, errors.New("GIF ends inside an extension")
			}

			end, err := subBlocks(d, i+2)
			if err != nil {
				return nil, err
			}

			label := d[i+1]
			loop := label == 0xff && i+14 <= end && d[i+2] == 11 &&
				(string(d[i+3:i+14]) == "NETSCAPE2.0" || string(d[i+3:i+14]) == "ANIMEXTS1.0")

			if label == 0xf9 || loop {
				out = append(out, d[i:end]...)
			}

			i = end
		case 0x2c:
			if i+10 > len(d) {
				return nil, errors.New("GIF ends inside a frame")
			}

			j := i + 10
			if d[i+9]&0x80 != 0 {
				j += 3 << (d[i+9]&7 + 1)
			}

			end, err := subBlocks(d, j+1) // after the LZW code size
			if err != nil {
				return nil, err
			}

			out = append(out, d[i:end]...)
			i = end
		default:
			return nil, fmt.Errorf("GIF block %#x is not one kreida knows", d[i])
		}
	}
}

// subBlocks finds the end of a run of GIF data sub-blocks that starts at i.
func subBlocks(d []byte, i int) (int, error) {
	for {
		if i >= len(d) {
			return 0, errors.New("GIF ends inside a block")
		}

		n := int(d[i])
		i += 1 + n

		if n == 0 {
			return i, nil
		}
	}
}

// ----- WebP

type riffChunk struct {
	id        string
	data, raw []byte
}

func riffChunks(d []byte) ([]riffChunk, error) {
	var out []riffChunk

	for i := 0; i < len(d); {
		if i+8 > len(d) {
			return nil, errors.New("WebP ends inside a chunk header")
		}

		n := int(binary.LittleEndian.Uint32(d[i+4:]))
		if n > len(d)-i-8 {
			return nil, errors.New("WebP chunk runs past the end")
		}

		end := i + 8 + n + n&1
		if end > len(d) {
			return nil, errors.New("WebP chunk misses its padding")
		}

		out = append(out, riffChunk{id: string(d[i : i+4]), data: d[i+8 : i+8+n], raw: d[i:end]})
		i = end
	}

	return out, nil
}

func riffBytes(id string, data []byte) []byte {
	out := append([]byte(id), binary.LittleEndian.AppendUint32(nil, uint32(len(data)))...)
	out = append(out, data...)

	if len(data)%2 == 1 {
		out = append(out, 0)
	}

	return out
}

// stripWebP keeps the bitstreams, alpha, animation and the ICC profile, inside animation frames too, and drops the
// EXIF and XMP chunks with their flags, unknown chunks and anything after the RIFF's end.
func stripWebP(d []byte) ([]byte, int, error) {
	if len(d) < 12 || string(d[:4]) != "RIFF" || string(d[8:12]) != "WEBP" {
		return nil, 0, errors.New("not a WebP")
	}

	size := int(binary.LittleEndian.Uint32(d[4:]))
	if size < 4 || size > len(d)-8 {
		return nil, 0, errors.New("WebP is shorter than it says")
	}

	chunks, err := riffChunks(d[12 : 8+size])
	if err != nil {
		return nil, 0, err
	}

	if len(chunks) == 0 {
		return nil, 0, errors.New("WebP has no picture")
	}

	var body []byte

	orient, exif := 1, false

	switch chunks[0].id {
	case "VP8 ", "VP8L":
		body = chunks[0].raw
	case "VP8X":
		for _, c := range chunks {
			switch c.id {
			case "VP8X":
				if len(c.data) < 10 {
					return nil, 0, errors.New("WebP VP8X chunk too short")
				}

				head := append([]byte{}, c.data...)
				head[0] &^= 0x08 | 0x04 // the EXIF and XMP flags

				body = append(body, riffBytes("VP8X", head)...)
			case "ICCP", "ANIM", "ALPH", "VP8 ", "VP8L":
				body = append(body, c.raw...)
			case "ANMF":
				if len(c.data) < 16 {
					return nil, 0, errors.New("WebP frame too short")
				}

				subs, err := riffChunks(c.data[16:])
				if err != nil {
					return nil, 0, err
				}

				frame := append([]byte{}, c.data[:16]...)

				for _, s := range subs {
					if s.id == "ALPH" || s.id == "VP8 " || s.id == "VP8L" {
						frame = append(frame, s.raw...)
					}
				}

				body = append(body, riffBytes("ANMF", frame)...)
			case "EXIF":
				if !exif {
					exif, orient = true, orientation(bytes.TrimPrefix(c.data, exifID))
				}
			}
		}
	default:
		return nil, 0, fmt.Errorf("WebP starts with chunk %q", chunks[0].id)
	}

	out := append([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body)))...)
	out = append(out, "WEBP"...)

	return append(out, body...), orient, nil
}

// ----- orientation

// orientation reads the Orientation tag (1–8) from the first directory of an EXIF (TIFF) block. A block it cannot read
// counts as upright, as it does for a browser.
func orientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}

	var bo binary.ByteOrder

	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}

	if bo.Uint16(t[2:]) != 42 {
		return 1
	}

	off := int(bo.Uint32(t[4:]))
	if off < 8 || off > len(t)-2 {
		return 1
	}

	for k := range int(bo.Uint16(t[off:])) {
		e := off + 2 + 12*k
		if e+12 > len(t) {
			break
		}

		if bo.Uint16(t[e:]) != 0x0112 {
			continue
		}

		if v := int(bo.Uint16(t[e+8:])); bo.Uint16(t[e+2:]) == 3 && v >= 1 && v <= 8 {
			return v
		}

		return 1
	}

	return 1
}

// upright draws a picture the way its EXIF orientation says it is to be seen.
func upright(src image.Image, orient int) *image.NRGBA {
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	flat := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Src)

	if orient <= 1 || orient > 8 {
		return flat
	}

	dw, dh := w, h
	if orient >= 5 {
		dw, dh = h, w
	}

	dst := image.NewNRGBA(image.Rect(0, 0, dw, dh))

	for y := range h {
		for x := range w {
			var dx, dy int

			switch orient {
			case 2:
				dx, dy = w-1-x, y
			case 3:
				dx, dy = w-1-x, h-1-y
			case 4:
				dx, dy = x, h-1-y
			case 5:
				dx, dy = y, x
			case 6:
				dx, dy = h-1-y, x
			case 7:
				dx, dy = h-1-y, w-1-x
			case 8:
				dx, dy = y, w-1-x
			}

			s, t := flat.PixOffset(x, y), dst.PixOffset(dx, dy)
			copy(dst.Pix[t:t+4], flat.Pix[s:s+4])
		}
	}

	return dst
}
