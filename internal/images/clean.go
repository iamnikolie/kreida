package images

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"hash/crc32"
	"image"
	"image/draw"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"slices"
	"sync"

	"github.com/iamnikolie/kreida/internal/board"
	"golang.org/x/image/webp"
)

// jpegQuality is the quality of a JPEG that had to be drawn again to stand upright.
const jpegQuality = 92

// cacheBytes bounds the memory of the cleaned images kept for the next request.
const cacheBytes = 64 << 20

// Clean returns an image as it may leave this machine: no EXIF, XMP, IPTC, comments, text chunks or bytes after the
// end of the picture, whatever the format. Only what draws the picture is kept, and its colour, with the words taken
// out of a colour profile (see cleanICC). Every picture is decoded whole before it may go. A photo that stands upright
// through its EXIF orientation is turned in its pixels first, so it does not fall over when the orientation goes. The
// result says what it is: its own hash, type and pixel size. It fails closed: a file it cannot take apart, or whose
// pixels do not decode, is refused, and its bytes never go anywhere. The same file always comes out as the same bytes.
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
		out, orient, err = cleanJPEG(f.Bytes)
	case "image/png":
		out, orient, err = cleanPNG(f.Bytes)
	case "image/gif":
		out, err = cleanGIF(f.Bytes)
	case "image/webp":
		out, err = cleanWebP(f.Bytes)
	default:
		err = errors.New("unknown type")
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

var errPixels = errors.New("its pixels do not decode")

// errTurned is a picture that EXIF turns and whose colour cannot follow it into the turned pixels.
var errTurned = errors.New("its EXIF turns it and its colour profile cannot follow; save the picture upright")

// ----- JPEG

var (
	exifID  = []byte("Exif\x00\x00")
	iccID   = []byte("ICC_PROFILE\x00")
	adobeID = []byte("Adobe")
)

// iccChunk is the most profile bytes one APP2 segment holds: 65535 less the length, the name and the two counters.
const iccChunk = 65535 - 2 - len("ICC_PROFILE\x00") - 2

// cleanJPEG strips a JPEG (see stripJPEG), decodes it whole to make sure it is a picture, turns it upright when its
// EXIF says so, and puts its colour profile back, cleaned (see cleanICC). The pixels of an upright JPEG are not
// touched: its scans go out as they came.
func cleanJPEG(d []byte) ([]byte, int, error) {
	out, orient, icc, err := stripJPEG(d)
	if err != nil {
		return nil, 0, err
	}

	var profile []byte

	space := ""

	if icc != nil {
		if profile, space, err = cleanICC(icc); err != nil {
			return nil, 0, err
		}
	}

	src, err := jpeg.Decode(bytes.NewReader(out))
	if err != nil {
		return nil, 0, errPixels
	}

	if orient != 1 {
		if _, gray := src.(*image.Gray); profile != nil && (space != "RGB " || gray) {
			return nil, 0, errTurned
		}

		if _, cmyk := src.(*image.CMYK); cmyk {
			return nil, 0, errTurned
		}

		up := upright(src, orient)

		var buf bytes.Buffer

		// a JPEG has no alpha: every pixel is opaque, so its NRGBA bytes are its RGBA bytes
		if err := jpeg.Encode(&buf, &image.RGBA{Pix: up.Pix, Stride: up.Stride, Rect: up.Rect}, &jpeg.Options{Quality: jpegQuality}); err != nil {
			return nil, 0, err
		}

		out = buf.Bytes()
	}

	if profile == nil {
		return out, orient, nil
	}

	// the profile goes right after SOI, in as many APP2 segments as it takes
	count := (len(profile) + iccChunk - 1) / iccChunk
	if count > 255 {
		return nil, 0, errors.New("colour profile too large")
	}

	with := []byte{0xff, 0xd8}

	for i := range count {
		part := profile[i*iccChunk : min((i+1)*iccChunk, len(profile))]
		with = append(with, segment(0xe2, slices.Concat(iccID, []byte{byte(i + 1), byte(count)}, part))...)
	}

	return append(with, out[2:]...), orient, nil
}

func segment(marker byte, body []byte) []byte {
	return append([]byte{0xff, marker, byte((len(body) + 2) >> 8), byte(len(body) + 2)}, body...)
}

// stripJPEG keeps the segments that draw the picture (tables, frame, scans) and Adobe's colour transform in its
// standard 12 bytes, and drops every other APPn (EXIF, XMP, IPTC, thumbnails, the ICC profile, which comes back
// cleaned), comments and whatever follows the end marker. It answers the EXIF orientation and the ICC profile as one
// piece. A marker it does not know fails it.
func stripJPEG(d []byte) ([]byte, int, []byte, error) {
	if len(d) < 4 || d[0] != 0xff || d[1] != 0xd8 {
		return nil, 0, nil, errors.New("not a JPEG")
	}

	out := []byte{0xff, 0xd8}
	orient, exif := 1, false
	icc, iccCount := map[int][]byte{}, 0

	for i := 2; ; {
		if i >= len(d) || d[i] != 0xff {
			return nil, 0, nil, errors.New("JPEG has no marker where one belongs, or ends before its end")
		}

		for i < len(d) && d[i] == 0xff {
			i++
		}

		if i >= len(d) {
			return nil, 0, nil, errors.New("JPEG ends before its end marker")
		}

		m := d[i]
		i++

		switch {
		case m == 0xd9:
			profile, err := joinICC(icc, iccCount)
			if err != nil {
				return nil, 0, nil, err
			}

			return append(out, 0xff, 0xd9), orient, profile, nil
		case m >= 0xd0 && m <= 0xd7 || m == 0x01:
			out = append(out, 0xff, m)

			continue
		}

		if i+2 > len(d) {
			return nil, 0, nil, errors.New("JPEG ends inside a segment")
		}

		n := int(binary.BigEndian.Uint16(d[i:]))
		if n < 2 || i+n > len(d) {
			return nil, 0, nil, errors.New("JPEG segment runs past the end")
		}

		seg, body := d[i:i+n], d[i+2:i+n]

		switch {
		case m == 0xe1 && bytes.HasPrefix(body, exifID):
			if !exif {
				exif, orient = true, orientation(body[len(exifID):])
			}
		case m == 0xe2 && bytes.HasPrefix(body, iccID):
			rest := body[len(iccID):]
			if len(rest) < 2 || rest[0] == 0 || rest[1] == 0 || iccCount != 0 && int(rest[1]) != iccCount || icc[int(rest[0])] != nil {
				return nil, 0, nil, errors.New("JPEG colour profile segments do not add up")
			}

			iccCount = int(rest[1])
			icc[int(rest[0])] = rest[2:]
		case m == 0xee && bytes.HasPrefix(body, adobeID):
			// "Adobe", version, two flag words and the transform: nothing past them
			if len(body) < 12 {
				return nil, 0, nil, errors.New("JPEG Adobe segment too short")
			}

			out = append(out, segment(0xee, body[:12])...)
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
			out = append(out, 0xff, m)
			out = append(out, seg...)
		default:
			return nil, 0, nil, fmt.Errorf("JPEG marker %#x is not one kreida knows", m)
		}

		i += n
	}
}

// joinICC puts a profile split over APP2 segments back together, in their order; every piece must be there.
func joinICC(parts map[int][]byte, count int) ([]byte, error) {
	if count == 0 {
		return nil, nil
	}

	var out []byte

	for i := 1; i <= count; i++ {
		p, ok := parts[i]
		if !ok || len(parts) != count {
			return nil, errors.New("JPEG colour profile segments do not add up")
		}

		out = append(out, p...)
	}

	return out, nil
}

// ----- PNG

// cleanPNG reads a PNG whole and writes its pixels again: no chunk the encoder does not write survives, and a broken
// pixel stream fails the decode. Only the colour of the original comes along: sRGB, gamma, chromaticities and a
// cleaned ICC profile. An animated PNG becomes its default image. A picture its EXIF turns is turned in its pixels.
func cleanPNG(d []byte) ([]byte, int, error) {
	if !bytes.HasPrefix(d, []byte(pngSignature)) {
		return nil, 0, errors.New("not a PNG")
	}

	orient, exif := 1, false

	var (
		colour []byte
		iccp   []byte
	)

	for i := len(pngSignature); ; {
		if i+12 > len(d) {
			return nil, 0, errors.New("PNG ends before IEND")
		}

		n := int(binary.BigEndian.Uint32(d[i:]))
		if n > len(d)-i-12 {
			return nil, 0, errors.New("PNG chunk runs past the end")
		}

		chunk := d[i : i+12+n]
		typ, data := string(chunk[4:8]), chunk[8:8+n]

		if crc32.ChecksumIEEE(chunk[4:8+n]) != binary.BigEndian.Uint32(chunk[8+n:]) {
			return nil, 0, fmt.Errorf("PNG chunk %q has a bad checksum", typ)
		}

		if i == len(pngSignature) && typ != "IHDR" {
			return nil, 0, errors.New("PNG does not start with IHDR")
		}

		switch {
		case typ == "eXIf":
			if !exif {
				exif, orient = true, orientation(data)
			}
		case typ == "gAMA" && n == 4, typ == "cHRM" && n == 32, typ == "sRGB" && n == 1 && data[0] <= 3:
			colour = append(colour, chunk...)
		case typ == "gAMA", typ == "cHRM", typ == "sRGB":
			return nil, 0, fmt.Errorf("PNG chunk %q has the wrong size", typ)
		case typ == "iCCP" && iccp == nil:
			iccp = data
		}

		i += 12 + n

		if typ == "IEND" {
			break
		}
	}

	src, err := png.Decode(bytes.NewReader(d))
	if err != nil {
		return nil, 0, errPixels
	}

	if orient != 1 {
		src = upright(src, orient)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, src); err != nil {
		return nil, 0, err
	}

	out := buf.Bytes()

	if iccp != nil {
		profile, space, err := iccpProfile(iccp)
		if err != nil {
			return nil, 0, err
		}

		want := "RGB "
		if ct := out[25]; ct == 0 || ct == 4 {
			want = "GRAY"
		}

		if space != want {
			if orient != 1 {
				return nil, 0, errTurned
			}

			return nil, 0, errors.New("its colour profile does not fit its pixels")
		}

		var z bytes.Buffer

		zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
		_, _ = zw.Write(profile)
		_ = zw.Close()

		colour = append(colour, pngChunk("iCCP", append([]byte("ICC Profile\x00\x00"), z.Bytes()...))...)
	}

	// the colour goes right after IHDR (signature 8, IHDR 25), before any palette or data
	return slices.Concat(out[:33], colour, out[33:]), orient, nil
}

const pngSignature = "\x89PNG\r\n\x1a\n"

func pngChunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)

	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[4:]))
}

// iccpProfile reads the profile out of an iCCP chunk (a name, the method, the zlib stream) and cleans it. The name
// does not come along.
func iccpProfile(data []byte) ([]byte, string, error) {
	name := bytes.IndexByte(data, 0)
	if name < 1 || name > 79 || len(data) < name+2 || data[name+1] != 0 {
		return nil, "", errors.New("PNG colour profile unreadable")
	}

	zr, err := zlib.NewReader(bytes.NewReader(data[name+2:]))
	if err != nil {
		return nil, "", errors.New("PNG colour profile unreadable")
	}

	raw, err := io.ReadAll(io.LimitReader(zr, MaxFile+1))
	if err != nil || len(raw) > MaxFile {
		return nil, "", errors.New("PNG colour profile unreadable")
	}

	return cleanICC(raw)
}

// ----- GIF

// maxFramePixels bounds the pixels of all a GIF's frames together, so checking one cannot take the machine's memory.
const maxFramePixels = 4 * MaxPixels

// cleanGIF keeps the screen, the colour tables, the frames, each frame's timing and the loop count, the last two
// written in their standard form; comments, plain text and every other application block (XMP among them) go, and so
// does anything after the trailer. The result is decoded whole, every frame.
func cleanGIF(d []byte) ([]byte, error) {
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
	pixels := 0

	for done := false; !done; {
		if i >= len(d) {
			return nil, errors.New("GIF ends before its trailer")
		}

		switch d[i] {
		case 0x3b:
			out = append(out, 0x3b)
			done = true
		case 0x21:
			if i+3 > len(d) {
				return nil, errors.New("GIF ends inside an extension")
			}

			end, err := subBlocks(d, i+2)
			if err != nil {
				return nil, err
			}

			first := d[i+3 : min(i+3+int(d[i+2]), end)]

			switch label := d[i+1]; {
			case label == 0xf9:
				// timing, disposal and transparency in four bytes; the reserved bits cleared
				if len(first) != 4 {
					return nil, errors.New("GIF frame timing block has the wrong size")
				}

				out = append(out, 0x21, 0xf9, 4, first[0]&0x1f, first[1], first[2], first[3], 0)
			case label == 0xff && (string(first) == "NETSCAPE2.0" || string(first) == "ANIMEXTS1.0"):
				// the loop count, if that is what the block says
				j := i + 3 + len(first)
				if j+4 <= end && d[j] == 3 && d[j+1] == 1 {
					out = append(out, 0x21, 0xff, 11)
					out = append(out, "NETSCAPE2.0"...)
					out = append(out, 3, 1, d[j+2], d[j+3], 0)
				}
			}

			i = end
		case 0x2c:
			if i+10 > len(d) {
				return nil, errors.New("GIF ends inside a frame")
			}

			pixels += int(binary.LittleEndian.Uint16(d[i+5:])) * int(binary.LittleEndian.Uint16(d[i+7:]))
			if pixels > maxFramePixels {
				return nil, errors.New("GIF frames hold too many pixels")
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

	if _, err := gif.DecodeAll(bytes.NewReader(out)); err != nil {
		return nil, errPixels
	}

	return out, nil
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

// cleanWebP keeps one still picture: its bitstream, its alpha and a cleaned ICC profile, under a header that names
// only those. EXIF, XMP and unknown chunks go, and so does anything after the RIFF's end. An animated WebP, and one
// its EXIF turns, are refused: there is no WebP encoder here to put them right. The result is decoded whole.
func cleanWebP(d []byte) ([]byte, error) {
	if len(d) < 12 || string(d[:4]) != "RIFF" || string(d[8:12]) != "WEBP" {
		return nil, errors.New("not a WebP")
	}

	size := int(binary.LittleEndian.Uint32(d[4:]))
	if size < 4 || size > len(d)-8 {
		return nil, errors.New("WebP is shorter than it says")
	}

	chunks, err := riffChunks(d[12 : 8+size])
	if err != nil {
		return nil, err
	}

	if len(chunks) == 0 {
		return nil, errors.New("WebP has no picture")
	}

	var body []byte

	switch chunks[0].id {
	case "VP8 ", "VP8L":
		body = chunks[0].raw
	case "VP8X":
		head := chunks[0].data
		if len(head) != 10 {
			return nil, errors.New("WebP VP8X chunk has the wrong size")
		}

		if head[0]&0x02 != 0 {
			return nil, errors.New("an animated WebP; kreida shows still WebP pictures only")
		}

		var icc, alpha, picture []byte

		orient, exif, pictures := 1, false, 0

		for _, c := range chunks[1:] {
			switch c.id {
			case "ICCP":
				if icc != nil {
					return nil, errors.New("WebP has two colour profiles")
				}

				profile, space, err := cleanICC(c.data)
				if err != nil {
					return nil, err
				}

				if space != "RGB " {
					return nil, errors.New("its colour profile does not fit its pixels")
				}

				icc = riffBytes("ICCP", profile)
			case "ALPH":
				alpha = c.raw
			case "VP8 ", "VP8L":
				picture = c.raw
				pictures++
			case "ANIM", "ANMF":
				return nil, errors.New("an animated WebP; kreida shows still WebP pictures only")
			case "EXIF":
				if !exif {
					exif, orient = true, orientation(bytes.TrimPrefix(c.data, exifID))
				}
			}
		}

		if pictures != 1 {
			return nil, errors.New("WebP must hold one picture")
		}

		if orient != 1 {
			return nil, errors.New("its EXIF turns it, which kreida cannot do for a WebP; save the picture upright")
		}

		flags := head[0] & 0x10 // alpha
		if icc != nil {
			flags |= 0x20
		}

		body = riffBytes("VP8X", append([]byte{flags, 0, 0, 0}, head[4:]...))
		body = slices.Concat(body, icc, alpha, picture)
	default:
		return nil, fmt.Errorf("WebP starts with chunk %q", chunks[0].id)
	}

	out := slices.Concat([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(body))), []byte("WEBP"), body)

	if _, err := webp.Decode(bytes.NewReader(out)); err != nil {
		return nil, errPixels
	}

	return out, nil
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
