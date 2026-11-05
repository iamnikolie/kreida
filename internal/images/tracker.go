package images

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/iamnikolie/kreida/internal/board"
)

// PollEvery is how often the image files of the boards are looked at. They may live anywhere on the machine, so a
// watch on the boards directory does not see them.
const PollEvery = 2 * time.Second

// Tracker notices that an image file a board names changed while the board file itself did not. It looks at sizes and
// times only and reads no image. It answers one question, and whoever asks sends the board down the road a change of
// its file takes, so a changed image gets no road of its own.
type Tracker struct {
	seen map[string]*tracked
}

type tracked struct {
	mtime time.Time
	size  int64
	paths []string
	print string
}

// NewTracker returns a Tracker that knows no board yet.
func NewTracker() *Tracker { return &Tracker{seen: map[string]*tracked{}} }

// Changed says whether an image of the board changed since the last call. The first look at a board, and a look at a
// board whose own file changed, are never a change: that file's change has its own event.
func (t *Tracker) Changed(name, file, dir string) bool {
	st, err := os.Stat(file)
	if err != nil {
		delete(t.seen, name)

		return false
	}

	cur := t.seen[name]
	if cur == nil || !cur.mtime.Equal(st.ModTime()) || cur.size != st.Size() {
		cur = &tracked{mtime: st.ModTime(), size: st.Size(), paths: pathsOf(file, dir)}
		cur.print = fingerprint(cur.paths)
		t.seen[name] = cur

		return false
	}

	if len(cur.paths) == 0 {
		return false
	}

	now := fingerprint(cur.paths)
	if now == cur.print {
		return false
	}

	cur.print = now

	return true
}

// pathsOf lists the image files a board names. A board that does not parse names none.
func pathsOf(file, dir string) []string {
	text, err := os.ReadFile(file)
	if err != nil {
		return nil
	}

	b, err := board.Parse(string(text))
	if err != nil {
		return nil
	}

	var out []string

	for _, n := range b.Nodes {
		if n.ImagePath != "" {
			out = append(out, Resolve(dir, n.ImagePath))
		}
	}

	return out
}

func fingerprint(paths []string) string {
	var b strings.Builder

	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%q %d %d\n", p, st.Size(), st.ModTime().UnixNano())
		} else {
			fmt.Fprintf(&b, "%q -\n", p)
		}
	}

	return b.String()
}
