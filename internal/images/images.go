// Package images turns the `image:` paths of a board into checked bytes. A board's YAML can name any path on the
// machine, and what is read here can end up in an HTML export or on a published board, so this is the one place that
// reads such a file: only a regular file, only a known image format judged by its content, only up to a size. The
// path stays on this machine: what goes out is the hash, the type, the pixel size and the bytes.
package images

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"

	"github.com/iamnikolie/kreida/internal/board"
)

const (
	// MaxFile is the largest image file, in bytes.
	MaxFile = 5 << 20
	// MaxBoard is the most image bytes on one board, in bytes. Cards past it, in file order, count as too big.
	MaxBoard = 25 << 20
	// MaxPixels is the largest picture a browser is asked to unpack: a small file can hold a huge one.
	MaxPixels = 40_000_000
)

// The reasons a card has no picture. They go in board.Image.Error, so they never name a path.
const (
	Missing     = "missing"
	Unreadable  = "unreadable"
	Unsupported = "unsupported"
	TooBig      = "too-big"
)

var shaRe = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidSHA reports whether s has the form of an image's hash.
func ValidSHA(s string) bool { return shaRe.MatchString(s) }

// File is a checked image: what a card says about it and its bytes.
type File struct {
	board.Image
	Bytes []byte
}

// DataURL is the file as one `data:` address, for a page that has no server to ask.
func (f File) DataURL() string {
	return "data:" + f.Type + ";base64," + base64.StdEncoding.EncodeToString(f.Bytes)
}

// reasonError is a refused file. The message may name the path: it is for the owner's terminal only.
type reasonError struct {
	reason string
	msg    string
}

func (e *reasonError) Error() string { return e.msg }

func refuse(reason, format string, args ...any) error {
	return &reasonError{reason: reason, msg: fmt.Sprintf(format, args...)}
}

// reasonOf is the reason code of an error from Load.
func reasonOf(err error) string {
	var r *reasonError
	if errors.As(err, &r) {
		return r.reason
	}

	return Unreadable
}

// Resolve makes an `image:` value a path on this machine. A relative path is relative to the directory of the board
// file (dir), never to where a command runs: the server runs from anywhere, and a board keeps working when its
// directory moves with its images. `~/` is the user's home.
func Resolve(dir, raw string) string {
	if rest, ok := strings.CutPrefix(raw, "~/"); ok {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, rest)
		}
	}

	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}

	return filepath.Join(dir, raw)
}

// Load reads and checks one image. Links are followed, and what they lead to is checked like any file: a link to a
// text file is refused by its content, a link to a device or a pipe before it is read at all.
func Load(path string) (File, error) {
	st, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, refuse(Missing, "%s: no such file", path)
		}

		return File{}, refuse(Unreadable, "%s: cannot be read", path)
	}

	if !st.Mode().IsRegular() {
		return File{}, refuse(Unsupported, "%s: not a regular file", path)
	}

	if st.Size() > MaxFile {
		return File{}, refuse(TooBig, "%s: over the %d MiB limit", path, MaxFile>>20)
	}

	// non-blocking: the path may have become a pipe since the Stat, and opening a pipe would wait for a writer
	f, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return File{}, refuse(Unreadable, "%s: cannot be read", path)
	}

	defer f.Close()

	if st, err = f.Stat(); err != nil || !st.Mode().IsRegular() {
		return File{}, refuse(Unsupported, "%s: not a regular file", path)
	}

	data, err := io.ReadAll(io.LimitReader(f, MaxFile+1))
	if err != nil {
		return File{}, refuse(Unreadable, "%s: cannot be read", path)
	}

	if len(data) > MaxFile {
		return File{}, refuse(TooBig, "%s: over the %d MiB limit", path, MaxFile>>20)
	}

	typ, w, h, err := sniff(data)
	if err != nil {
		return File{}, refuse(Unsupported, "%s: not a PNG, JPEG, GIF or WebP image", path)
	}

	if int64(w)*int64(h) > MaxPixels {
		return File{}, refuse(TooBig, "%s: %dx%d is over the %d MP limit", path, w, h, MaxPixels/1_000_000)
	}

	sum := sha256.Sum256(data)

	return File{Image: board.Image{SHA256: hex.EncodeToString(sum[:]), Type: typ, W: w, H: h}, Bytes: data}, nil
}

// sniff names the format from the file's first bytes, whatever the file is called, and reads the picture's size from
// its header. A file that only starts like an image fails here. SVG is not on the list: it is text that can run
// scripts.
func sniff(data []byte) (typ string, w, h int, err error) {
	var cfg image.Config

	r := bytes.NewReader(data)

	switch {
	case bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		typ = "image/png"
		cfg, err = png.DecodeConfig(r)
	case bytes.HasPrefix(data, []byte("\xff\xd8\xff")):
		typ = "image/jpeg"
		cfg, err = jpeg.DecodeConfig(r)
	case bytes.HasPrefix(data, []byte("GIF87a")), bytes.HasPrefix(data, []byte("GIF89a")):
		typ = "image/gif"
		cfg, err = gif.DecodeConfig(r)
	case len(data) >= 12 && string(data[:4]) == "RIFF" && string(data[8:12]) == "WEBP":
		typ = "image/webp"
		cfg, err = webpConfig(data)
	default:
		return "", 0, 0, errors.New("unknown format")
	}

	if err != nil {
		return "", 0, 0, err
	}

	if cfg.Width <= 0 || cfg.Height <= 0 {
		return "", 0, 0, errors.New("empty picture")
	}

	return typ, cfg.Width, cfg.Height, nil
}

// webpConfig reads the size from the first chunk of a WebP file: VP8 (lossy), VP8L (lossless) or VP8X (extended).
func webpConfig(d []byte) (image.Config, error) {
	bad := errors.New("bad WebP header")

	if len(d) < 30 {
		return image.Config{}, bad
	}

	var w, h int

	switch string(d[12:16]) {
	case "VP8 ":
		if d[23] != 0x9d || d[24] != 0x01 || d[25] != 0x2a {
			return image.Config{}, bad
		}

		w = int(binary.LittleEndian.Uint16(d[26:]) & 0x3fff)
		h = int(binary.LittleEndian.Uint16(d[28:]) & 0x3fff)
	case "VP8L":
		if d[20] != 0x2f {
			return image.Config{}, bad
		}

		bits := binary.LittleEndian.Uint32(d[21:])
		w = int(bits&0x3fff) + 1
		h = int(bits>>14&0x3fff) + 1
	case "VP8X":
		w = (int(d[24]) | int(d[25])<<8 | int(d[26])<<16) + 1
		h = (int(d[27]) | int(d[28])<<8 | int(d[29])<<16) + 1
	default:
		return image.Config{}, bad
	}

	return image.Config{Width: w, Height: h}, nil
}

// Problem is a card whose picture cannot be shown. It names the path, so it is for the owner's terminal and log only.
type Problem struct {
	Node string
	Err  error
}

func (p Problem) String() string { return fmt.Sprintf("card %s: image %s", p.Node, p.Err) }

// Set is what a board's images came to.
type Set struct {
	// Files are the checked images of the board by hash: the only bytes that may leave with this board.
	Files map[string]File
	// Problems are the cards whose picture cannot be shown, in file order.
	Problems []Problem
}

// DataURLs is the set as the map of a standalone page: hash to `data:` address. Nil when there are none.
func (s Set) DataURLs() map[string]string {
	if len(s.Files) == 0 {
		return nil
	}

	out := make(map[string]string, len(s.Files))
	for sha, f := range s.Files {
		out[sha] = f.DataURL()
	}

	return out
}

// Attach reads the images a board names, with relative paths against dir, and fills in every such card's Image: the
// picture, or the reason there is none. A broken image never fails the board: it may be there tomorrow, or on the
// machine the board moves to. Attach is for the owner's own page; what a client gets goes through Public.
func Attach(b *board.Board, dir string) Set {
	set := Set{Files: map[string]File{}}
	seen := map[string]File{}
	total := 0

	for i := range b.Nodes {
		n := &b.Nodes[i]
		if n.ImagePath == "" {
			continue
		}

		path := Resolve(dir, n.ImagePath)

		f, ok := seen[path]
		if !ok {
			var err error

			f, err = Load(path)
			if err != nil {
				set.Problems = append(set.Problems, Problem{Node: n.ID, Err: err})
				n.Image = &board.Image{Error: reasonOf(err)}

				continue
			}

			seen[path] = f
		}

		if _, have := set.Files[f.SHA256]; !have {
			if total+len(f.Bytes) > MaxBoard {
				err := refuse(TooBig, "%s: the board is over its %d MiB of images", path, MaxBoard>>20)
				set.Problems = append(set.Problems, Problem{Node: n.ID, Err: err})
				n.Image = &board.Image{Error: TooBig}

				continue
			}

			total += len(f.Bytes)
			set.Files[f.SHA256] = f
		}

		img := f.Image
		n.Image = &img
	}

	return set
}

// Public is Attach for what leaves the owner's own page: an export, a share link, a published board. A card with a
// broken image simply has no picture: the reader gets a clean card, and the owner hears of it from Problems.
func Public(b *board.Board, dir string) Set {
	set := Attach(b, dir)

	for i := range b.Nodes {
		if img := b.Nodes[i].Image; img != nil && img.Error != "" {
			b.Nodes[i].Image = nil
		}
	}

	return set
}
