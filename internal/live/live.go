// Package live computes the values live cards show. A board names a shell command or a URL; it runs only when that
// exact source is on the allow-list for that board (<home>/live-allow.json), which only `kreida live <board> --allow`
// writes. An edited command no longer matches and is blocked again.
package live

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/history"
	"github.com/iamnikolie/kreida/internal/paths"
)

const (
	// DefaultTimeout bounds one command run or one request.
	DefaultTimeout = 10 * time.Second
	// MaxValue is the longest value kept, in characters.
	MaxValue = 200
	maxBody  = 16 << 20
)

// Source is the live source of one card and whether the board's allow-list covers it.
type Source struct {
	Node    string
	Source  string // "cmd:<command>" or "url:<url>"
	Every   float64
	Path    *string
	Label   string
	Allowed bool
}

// Value is what a card shows, in the shape of the page's LiveVal. Share responses leave Source empty.
type Value struct {
	Blocked bool    `json:"blocked,omitempty"`
	Source  string  `json:"source,omitempty"`
	Value   *string `json:"value,omitempty"`
	Error   *string `json:"error,omitempty"`
	At      string  `json:"at,omitempty"`
	Every   float64 `json:"every,omitempty"`
}

// ValueSet is the values of a board's live cards by card id.
type ValueSet map[string]Value

// WithoutSource returns a copy that no longer says which command or URL produced each value: clients of a share
// link see values, never the sources behind them.
func (vs ValueSet) WithoutSource() ValueSet {
	out := ValueSet{}

	for id, v := range vs {
		v.Source = ""
		out[id] = v
	}

	return out
}

// Runner reads live sources from boards and runs the allowed ones, sharing results between callers.
type Runner struct {
	Paths   paths.Paths
	Timeout time.Duration // 0 means DefaultTimeout
	Client  *http.Client  // nil means a client with Timeout
	Now     func() time.Time

	mu    sync.Mutex
	cache map[string]*entry
}

type entry struct {
	at   time.Time
	done chan struct{}
	res  result
}

type result struct {
	value *string
	err   *string
}

// New returns a Runner for a Paths root.
func New(p paths.Paths) *Runner { return &Runner{Paths: p} }

func (r *Runner) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}

	return time.Now()
}

func (r *Runner) timeout() time.Duration {
	if r.Timeout > 0 {
		return r.Timeout
	}

	return DefaultTimeout
}

// allowEntry is one approved source in <home>/live-allow.json.
type allowEntry struct {
	Board  string `json:"board"`
	Source string `json:"source"`
	At     string `json:"at"`
}

func (r *Runner) allowList() []allowEntry {
	var l []allowEntry

	if data, err := os.ReadFile(r.Paths.AllowFile()); err == nil {
		_ = json.Unmarshal(data, &l) // an unreadable list allows nothing
	}

	return l
}

func (r *Runner) saveAllowList(l []allowEntry) error {
	if l == nil {
		l = []allowEntry{}
	}

	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return fmt.Errorf("live.saveAllowList: %w", err)
	}

	if err := os.MkdirAll(r.Paths.Home, 0o755); err != nil {
		return fmt.Errorf("live.saveAllowList: %w", err)
	}

	if err := os.WriteFile(r.Paths.AllowFile(), append(data, '\n'), 0o600); err != nil {
		return fmt.Errorf("live.saveAllowList: %w", err)
	}

	return nil
}

func allowed(l []allowEntry, name, source string) bool {
	for _, a := range l {
		if a.Board == name && a.Source == source {
			return true
		}
	}

	return false
}

// Sources lists the live cards of a board. The board must parse.
func (r *Runner) Sources(name string) ([]Source, error) {
	text, err := os.ReadFile(r.Paths.BoardFile(name))
	if err != nil {
		return nil, fmt.Errorf("live.Sources: %w", err)
	}

	b, err := board.Parse(string(text))
	if err != nil {
		return nil, err
	}

	l := r.allowList()

	var out []Source

	for _, n := range b.Nodes {
		if n.Live == nil {
			continue
		}

		out = append(out, Source{Node: n.ID, Source: n.Live.Source, Every: n.Live.Every, Path: n.Live.Path,
			Label: n.Live.Label, Allowed: allowed(l, name, n.Live.Source)})
	}

	return out, nil
}

// Allow puts the board's sources (only node's, when node is set) on the allow-list and returns the ones added.
func (r *Runner) Allow(name, node string) ([]Source, error) {
	srcs, err := r.Sources(name)
	if err != nil {
		return nil, err
	}

	l := r.allowList()
	added := []Source{}

	for _, s := range srcs {
		if s.Allowed || (node != "" && s.Node != node) {
			continue
		}

		l = append(l, allowEntry{Board: name, Source: s.Source, At: history.ISO(r.now())})
		added = append(added, s)
	}

	return added, r.saveAllowList(l)
}

// Revoke drops every allow-list entry of the board and returns how many there were.
func (r *Runner) Revoke(name string) (int, error) {
	l := r.allowList()

	var keep []allowEntry

	for _, a := range l {
		if a.Board != name {
			keep = append(keep, a)
		}
	}

	return len(l) - len(keep), r.saveAllowList(keep)
}

// clip is the first line of the trimmed text, at most MaxValue characters.
func clip(s string) string {
	first, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	if r := []rune(first); len(r) > MaxValue {
		return string(r[:MaxValue])
	}

	return first
}

func strp(s string) *string { return &s }

// pick follows a dotted path (data.count) into a parsed JSON value; false when the path leads nowhere.
func pick(v any, path string) (any, bool) {
	if path == "" {
		return v, true
	}

	for _, k := range strings.Split(path, ".") {
		ok := false

		switch x := v.(type) {
		case map[string]any:
			v, ok = x[k]
		case []any:
			if i, err := strconv.Atoi(k); err == nil && i >= 0 && i < len(x) {
				v, ok = x[i], true
			}
		}

		if !ok {
			return nil, false
		}
	}

	return v, true
}

func (r *Runner) get(url, path string) result {
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: r.timeout()}
	}

	resp, err := client.Get(url)
	if err != nil {
		return result{err: strp(clip(err.Error()))}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return result{err: strp(clip(err.Error()))}
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return result{err: strp(fmt.Sprintf("HTTP %d", resp.StatusCode))}
	}

	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber() // a number shows as the server wrote it

	var parsed any
	if dec.Decode(&parsed) != nil || dec.More() {
		return result{value: strp(clip(string(body)))}
	}

	picked, found := pick(parsed, path)
	if s, ok := picked.(string); ok {
		return result{value: strp(clip(s))}
	}

	if !found {
		return result{value: strp("")}
	}

	j, _ := json.Marshal(picked) // an object shows with its keys sorted

	return result{value: strp(clip(string(j)))}
}

func (r *Runner) run(s Source) result {
	if cmd, ok := strings.CutPrefix(s.Source, "cmd:"); ok {
		return r.exec(cmd)
	}

	path := ""
	if s.Path != nil {
		path = *s.Path
	}

	return r.get(strings.TrimPrefix(s.Source, "url:"), path)
}

// Values runs the board's allowed sources (a blocked one is reported, not run). A value is fresh for the card's
// `every` seconds, and callers that ask meanwhile share one run.
func (r *Runner) Values(name string) (ValueSet, error) {
	srcs, err := r.Sources(name)
	if err != nil {
		return ValueSet{}, err
	}

	vals := make([]Value, len(srcs))

	var wg sync.WaitGroup

	for i, s := range srcs {
		if !s.Allowed {
			vals[i] = Value{Blocked: true, Source: s.Source}

			continue
		}

		wg.Add(1)

		go func() {
			defer wg.Done()

			e := r.entryFor(name, s)
			<-e.done
			vals[i] = Value{Value: e.res.value, Error: e.res.err, At: history.ISO(e.at), Every: s.Every, Source: s.Source}
		}()
	}

	wg.Wait()

	out := ValueSet{}
	for i, s := range srcs {
		out[s.Node] = vals[i]
	}

	return out, nil
}

func (r *Runner) entryFor(name string, s Source) *entry {
	key := name + "\x00" + s.Node + "\x00" + s.Source

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.cache == nil {
		r.cache = map[string]*entry{}
	}

	e := r.cache[key]
	if e != nil && r.now().Sub(e.at) <= time.Duration(s.Every*float64(time.Second)) {
		return e
	}

	e = &entry{at: r.now(), done: make(chan struct{})}
	r.cache[key] = e

	go func() {
		e.res = r.run(s)
		close(e.done)
	}()

	return e
}
