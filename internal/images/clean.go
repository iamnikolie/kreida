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

// Clean returns an image as it may leave this machine: its pixels, decoded whole and written again by Go's encoders
// (JPEG at quality 92, PNG and GIF losslessly, a WebP as a PNG or a JPEG), and its colour, as a profile rebuilt from
// its numbers (see cleanICC). No byte of the original stream goes along, so no EXIF, XMP, IPTC, comment, text chunk or
// unused byte anywhere in the file can. A photo that stands upright through its EXIF orientation is turned in its
// pixels first, so it does not fall over when the orientation goes. The result says what it is: its own hash, type and
// pixel size. It fails closed: a file it cannot take apart, or whose pixels do not decode, is refused, and its bytes
// never go anywhere. The same file always comes out as the same bytes.
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
		out, orient, err = cleanWebP(f.Bytes, f.W, f.H)
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

	// a WebP goes out as a PNG or a JPEG: there is no WebP encoder here
	sameType := typ == f.Type || f.Type == "image/webp" && (typ == "image/png" || typ == "image/jpeg")

	if err != nil || !sameType || w != wantW || h != wantH {
		return File{}, refuse(Unsupported, "cannot strip its metadata: the result does not read as the same picture")
	}

	if len(out) > MaxFile {
		return File{}, refuse(TooBig, "over the %d MiB limit once stripped", MaxFile>>20)
	}

	sum := sha256.Sum256(out)

	return File{Image: board.Image{SHA256: hex.EncodeToString(sum[:]), Type: typ, W: w, H: h}, Bytes: out}, nil
}

var (
	errPixels = errors.New("its pixels do not decode")
	errFit    = errors.New("its colour profile does not fit its pixels")
	errCMYK   = errors.New("a CMYK picture with a colour profile; save it as RGB")
)

// Every picture leaves as the encoder writes it from its decoded pixels: no byte of the original stream goes along, so
// nothing a decoder skips (a trailer in a scan, a chunk's tail) can ride out with it. Only its colour is carried over,
// as a profile rebuilt by cleanICC and, for a PNG, its sRGB, gamma and chromaticity chunks.

// encodeJPEG writes the pixels as a JPEG with the profile, if any, after SOI.
func encodeJPEG(img image.Image, profile []byte, space string) ([]byte, error) {
	if profile != nil {
		_, gray := img.(*image.Gray)
		if gray != (space == "GRAY") {
			return nil, errFit
		}
	}

	// a JPEG has no alpha: the NRGBA bytes of an opaque picture are its RGBA bytes, which the encoder takes fastest
	if n, ok := img.(*image.NRGBA); ok {
		img = &image.RGBA{Pix: n.Pix, Stride: n.Stride, Rect: n.Rect}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, err
	}

	out := buf.Bytes()
	if profile == nil {
		return out, nil
	}

	// a rebuilt profile is a few kilobytes: one APP2 segment
	if len(profile) > 65535-2-len(iccID)-2 {
		return nil, errProfile
	}

	return slices.Concat(out[:2], segment(0xe2, slices.Concat(iccID, []byte{1, 1}, profile)), out[2:]), nil
}

// encodePNG writes the pixels as a PNG with the colour chunks and the profile, if any, right after IHDR.
func encodePNG(img image.Image, colour, profile []byte, space string) ([]byte, error) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}

	out := buf.Bytes()

	if profile != nil {
		want := "RGB "
		if ct := out[25]; ct == 0 || ct == 4 {
			want = "GRAY"
		}

		if space != want {
			return nil, errFit
		}

		var z bytes.Buffer

		zw, _ := zlib.NewWriterLevel(&z, zlib.BestCompression)
		_, _ = zw.Write(profile)
		_ = zw.Close()

		colour = slices.Concat(colour, pngChunk("iCCP", append([]byte("ICC Profile\x00\x00"), z.Bytes()...)))
	}

	// signature 8, IHDR 25: the colour goes before any palette or data
	return slices.Concat(out[:33], colour, out[33:]), nil
}

func segment(marker byte, body []byte) []byte {
	return append([]byte{0xff, marker, byte((len(body) + 2) >> 8), byte(len(body) + 2)}, body...)
}

// ----- JPEG

var (
	exifID = []byte("Exif\x00\x00")
	iccID  = []byte("ICC_PROFILE\x00")
)

// cleanJPEG decodes a JPEG, turns it upright when its EXIF says so and writes it again at quality 92, with its colour
// profile rebuilt.
func cleanJPEG(d []byte) ([]byte, int, error) {
	orient, icc, err := readJPEG(d)
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

	src, err := jpeg.Decode(bytes.NewReader(d))
	if err != nil {
		return nil, 0, errPixels
	}

	if _, cmyk := src.(*image.CMYK); cmyk && profile != nil {
		return nil, 0, errCMYK
	}

	out, err := encodeJPEG(upright(src, orient), profile, space)

	return out, orient, err
}

// readJPEG walks a JPEG's segments up to its end marker and answers its EXIF orientation and its ICC profile in one
// piece. A marker it does not know, a segment that runs past the end or a profile with a missing piece fails it.
func readJPEG(d []byte) (int, []byte, error) {
	if len(d) < 4 || d[0] != 0xff || d[1] != 0xd8 {
		return 0, nil, errors.New("not a JPEG")
	}

	orient, exif := 1, false
	icc, iccCount := map[int][]byte{}, 0

	for i := 2; ; {
		if i >= len(d) || d[i] != 0xff {
			return 0, nil, errors.New("JPEG has no marker where one belongs, or ends before its end")
		}

		for i < len(d) && d[i] == 0xff {
			i++
		}

		if i >= len(d) {
			return 0, nil, errors.New("JPEG ends before its end marker")
		}

		m := d[i]
		i++

		switch {
		case m == 0xd9:
			profile, err := joinICC(icc, iccCount)

			return orient, profile, err
		case m >= 0xd0 && m <= 0xd7 || m == 0x01:
			continue
		}

		if i+2 > len(d) {
			return 0, nil, errors.New("JPEG ends inside a segment")
		}

		n := int(binary.BigEndian.Uint16(d[i:]))
		if n < 2 || i+n > len(d) {
			return 0, nil, errors.New("JPEG segment runs past the end")
		}

		body := d[i+2 : i+n]

		switch {
		case m == 0xe1 && bytes.HasPrefix(body, exifID):
			if !exif {
				exif, orient = true, orientation(body[len(exifID):])
			}
		case m == 0xe2 && bytes.HasPrefix(body, iccID):
			rest := body[len(iccID):]
			if len(rest) < 2 || rest[0] == 0 || rest[1] == 0 || iccCount != 0 && int(rest[1]) != iccCount || icc[int(rest[0])] != nil {
				return 0, nil, errors.New("JPEG colour profile segments do not add up")
			}

			iccCount = int(rest[1])
			icc[int(rest[0])] = rest[2:]
		case m >= 0xe0 && m <= 0xef, m == 0xfe:
		case m == 0xda:
			i += n

			for i+1 < len(d) && (d[i] != 0xff || d[i+1] == 0 || d[i+1] >= 0xd0 && d[i+1] <= 0xd7) {
				if d[i] == 0xff {
					i++
				}

				i++
			}

			continue
		case m == 0xdb, m == 0xc4, m == 0xdd, m == 0xdc, m == 0xcc, m >= 0xc0 && m <= 0xcf && m != 0xc8:
		default:
			return 0, nil, fmt.Errorf("JPEG marker %#x is not one kreida knows", m)
		}

		i += n
	}
}

// joinICC puts a profile split over APP2 segments back together, in their order; every piece must be there.
func joinICC(parts map[int][]byte, count int) ([]byte, error) {
	if count == 0 {
		return nil, nil
	}

	if len(parts) != count {
		return nil, errors.New("JPEG colour profile segments do not add up")
	}

	var out []byte

	for i := 1; i <= count; i++ {
		p, ok := parts[i]
		if !ok || len(out)+len(p) > MaxFile {
			return nil, errors.New("JPEG colour profile segments do not add up")
		}

		out = append(out, p...)
	}

	return out, nil
}

// ----- PNG

// cleanPNG decodes a PNG and writes its pixels again, losslessly. Its sRGB, gamma and chromaticities come along, and
// its ICC profile rebuilt. An animated PNG becomes its default image. A picture its EXIF turns is turned.
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

	var (
		profile []byte
		space   string
	)

	if iccp != nil {
		if profile, space, err = iccpProfile(iccp); err != nil {
			return nil, 0, err
		}
	}

	out, err := encodePNG(upright(src, orient), colour, profile, space)

	return out, orient, err
}

const pngSignature = "\x89PNG\r\n\x1a\n"

func pngChunk(typ string, data []byte) []byte {
	out := binary.BigEndian.AppendUint32(nil, uint32(len(data)))
	out = append(out, typ...)
	out = append(out, data...)

	return binary.BigEndian.AppendUint32(out, crc32.ChecksumIEEE(out[4:]))
}

// iccpProfile reads the profile out of an iCCP chunk (a name, the method, the zlib stream) and rebuilds it. The name
// does not come along.
func iccpProfile(data []byte) ([]byte, string, error) {
	name := bytes.IndexByte(data, 0)
	if name < 1 || name > 79 || len(data) < name+2 || data[name+1] != 0 {
		return nil, "", errProfile
	}

	zr, err := zlib.NewReader(bytes.NewReader(data[name+2:]))
	if err != nil {
		return nil, "", errProfile
	}

	raw, err := io.ReadAll(io.LimitReader(zr, MaxFile+1))
	if err != nil || len(raw) > MaxFile {
		return nil, "", errProfile
	}

	return cleanICC(raw)
}

// ----- GIF

// maxFramePixels bounds the pixels of all a GIF's frames together, so decoding one cannot take the machine's memory.
const maxFramePixels = 4 * MaxPixels

// cleanGIF decodes every frame of a GIF and writes them again, losslessly, with their palettes, timing, disposal and
// the loop count: no comment, plain text or application block survives.
func cleanGIF(d []byte) ([]byte, error) {
	if err := gifFrames(d); err != nil {
		return nil, err
	}

	g, err := gif.DecodeAll(bytes.NewReader(d))
	if err != nil {
		return nil, errPixels
	}

	var buf bytes.Buffer
	if err := gif.EncodeAll(&buf, g); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// gifFrames walks a GIF's blocks and checks that all its frames together stay within maxFramePixels.
func gifFrames(d []byte) error {
	if len(d) < 13 {
		return errors.New("GIF too short")
	}

	i := 13
	if d[10]&0x80 != 0 {
		i += 3 << (d[10]&7 + 1)
	}

	pixels := 0

	for {
		if i >= len(d) {
			return errors.New("GIF ends before its trailer")
		}

		switch d[i] {
		case 0x3b:
			return nil
		case 0x21:
			end, err := subBlocks(d, i+2)
			if err != nil {
				return err
			}

			i = end
		case 0x2c:
			if i+10 > len(d) {
				return errors.New("GIF ends inside a frame")
			}

			pixels += int(binary.LittleEndian.Uint16(d[i+5:])) * int(binary.LittleEndian.Uint16(d[i+7:]))
			if pixels > maxFramePixels {
				return errors.New("GIF frames hold too many pixels")
			}

			j := i + 10
			if d[i+9]&0x80 != 0 {
				j += 3 << (d[i+9]&7 + 1)
			}

			end, err := subBlocks(d, j+1) // after the LZW code size
			if err != nil {
				return err
			}

			i = end
		default:
			return fmt.Errorf("GIF block %#x is not one kreida knows", d[i])
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

// cleanWebP decodes a still WebP and writes it again as a PNG (lossless or with alpha) or a JPEG (lossy), with its
// colour profile rebuilt; there is no WebP encoder here. Before any decoding, the frame inside must be the size the
// file says it is (w×h): a small header over a huge frame is refused. An animated WebP is refused.
func cleanWebP(d []byte, w, h int) ([]byte, int, error) {
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

	var (
		icc, picture []byte
		lossless     = chunks[0].id == "VP8L"
		alpha        bool
		orient       = 1
		exif         bool
	)

	switch chunks[0].id {
	case "VP8 ", "VP8L":
		picture = chunks[0].raw
	case "VP8X":
		if len(chunks[0].data) != 10 {
			return nil, 0, errors.New("WebP VP8X chunk has the wrong size")
		}

		if chunks[0].data[0]&0x02 != 0 {
			return nil, 0, errors.New("an animated WebP; kreida shows still WebP pictures only")
		}

		pictures := 0

		for _, c := range chunks[1:] {
			switch c.id {
			case "ICCP":
				if icc != nil {
					return nil, 0, errors.New("WebP has two colour profiles")
				}

				icc = c.data
			case "ALPH":
				alpha = true
			case "VP8 ", "VP8L":
				picture, lossless = c.raw, c.id == "VP8L"
				pictures++
			case "ANIM", "ANMF":
				return nil, 0, errors.New("an animated WebP; kreida shows still WebP pictures only")
			case "EXIF":
				if !exif {
					exif, orient = true, orientation(bytes.TrimPrefix(c.data, exifID))
				}
			}
		}

		if pictures != 1 {
			return nil, 0, errors.New("WebP must hold one picture")
		}
	default:
		return nil, 0, fmt.Errorf("WebP starts with chunk %q", chunks[0].id)
	}

	// the frame's own header, read as a WebP of its own, before anything is decoded
	alone := slices.Concat([]byte("RIFF"), binary.LittleEndian.AppendUint32(nil, uint32(4+len(picture))), []byte("WEBP"), picture)

	if _, fw, fh, err := sniff(alone); err != nil || fw != w || fh != h {
		return nil, 0, errors.New("the WebP frame is not the size its header says")
	}

	var (
		profile []byte
		space   string
	)

	if icc != nil {
		if profile, space, err = cleanICC(icc); err != nil {
			return nil, 0, err
		}
	}

	src, err := webp.Decode(bytes.NewReader(d))
	if err != nil || src.Bounds().Dx() != w || src.Bounds().Dy() != h {
		return nil, 0, errPixels
	}

	up := upright(src, orient)

	var out []byte
	if lossless || alpha {
		out, err = encodePNG(up, nil, profile, space)
	} else {
		out, err = encodeJPEG(up, profile, space)
	}

	return out, orient, err
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

// upright draws a picture the way its EXIF orientation says it is to be seen. An upright picture is returned as it is;
// a grey one stays grey; anything else becomes NRGBA.
func upright(src image.Image, orient int) image.Image {
	if orient <= 1 || orient > 8 {
		return src
	}

	b := src.Bounds()
	w, h := b.Dx(), b.Dy()

	dw, dh := w, h
	if orient >= 5 {
		dw, dh = h, w
	}

	var (
		from, to []byte
		fromAt   func(x, y int) int
		toAt     func(x, y int) int
		px       int
		dst      image.Image
	)

	if g, ok := src.(*image.Gray); ok {
		d := image.NewGray(image.Rect(0, 0, dw, dh))
		from, fromAt = g.Pix, func(x, y int) int { return g.PixOffset(b.Min.X+x, b.Min.Y+y) }
		to, toAt, px, dst = d.Pix, d.PixOffset, 1, d
	} else {
		flat := image.NewNRGBA(image.Rect(0, 0, w, h))
		draw.Draw(flat, flat.Bounds(), src, b.Min, draw.Src)

		d := image.NewNRGBA(image.Rect(0, 0, dw, dh))
		from, fromAt = flat.Pix, flat.PixOffset
		to, toAt, px, dst = d.Pix, d.PixOffset, 4, d
	}

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

			s, t := fromAt(x, y), toAt(dx, dy)
			copy(to[t:t+px], from[s:s+px])
		}
	}

	return dst
}
