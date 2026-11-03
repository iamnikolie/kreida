package server_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/iamnikolie/kreida/internal/server"
	"github.com/iamnikolie/kreida/internal/share"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDist = fstest.MapFS{
	"index.html":    {Data: []byte("<!doctype html><title>kreida</title><div id=root></div>")},
	"assets/app.js": {Data: []byte("console.log(1)")},
	"assets/f.woff": {Data: []byte("font")},
}

// stubThumb is the renderer of the harness: the smallest PNG-looking bytes.
func stubThumb(context.Context, string) ([]byte, error) { return []byte("\x89PNG\r\n\x1a\nstub"), nil }

type harness struct {
	t      *testing.T
	srv    *server.Server
	ts     *httptest.Server
	paths  paths.Paths
	shares share.Store
	port   int
}

// start runs a server on a random port over a fresh temporary home. It never touches ~/.kreida or port 7788.
func start(t *testing.T, opts ...func(*server.Config)) *harness {
	t.Helper()

	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}
	require.NoError(t, p.Ensure())

	ts := httptest.NewUnstartedServer(nil)
	port := ts.Listener.Addr().(*net.TCPAddr).Port

	// no test starts Chrome: a login with published boards would draw their thumbnails
	cfg := server.Config{Paths: p, Port: port, Dist: testDist, Thumb: stubThumb}
	for _, opt := range opts {
		opt(&cfg)
	}

	srv, err := server.New(cfg)
	require.NoError(t, err)

	ts.Config.Handler = srv.Handler()
	ts.Start()

	t.Cleanup(func() {
		srv.Close()
		ts.CloseClientConnections()
		ts.Close()
	})

	return &harness{t: t, srv: srv, ts: ts, paths: p, shares: share.Store{Paths: p}, port: port}
}

func (h *harness) put(name, yaml string) {
	h.t.Helper()
	require.NoError(h.t, os.WriteFile(h.paths.BoardFile(name), []byte(yaml), 0o644))
}

type reply struct {
	status int
	header http.Header
	body   string
}

func (r reply) json(t *testing.T) map[string]any {
	t.Helper()

	var m map[string]any
	require.NoError(t, json.Unmarshal([]byte(r.body), &m), r.body)

	return m
}

// do sends a request with the headers a page on this server would send.
func (h *harness) do(method, path, body string, hdr ...string) reply {
	h.t.Helper()

	req, err := http.NewRequest(method, h.ts.URL+path, strings.NewReader(body))
	require.NoError(h.t, err)

	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i] == "Host" {
			req.Host = hdr[i+1]
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}

	resp, err := h.ts.Client().Do(req)
	require.NoError(h.t, err)

	defer resp.Body.Close()

	b, err := io.ReadAll(resp.Body)
	require.NoError(h.t, err)

	return reply{resp.StatusCode, resp.Header, string(b)}
}

func (h *harness) get(path string, hdr ...string) reply { return h.do("GET", path, "", hdr...) }

// ----- editor API

func TestHealth(t *testing.T) {
	h := start(t)

	r := h.get("/api/health")
	assert.Equal(t, 200, r.status)
	assert.Equal(t, "application/json", r.header.Get("Content-Type"))

	m := r.json(t)
	assert.Equal(t, true, m["ok"])
	assert.Equal(t, h.paths.Dir, m["dir"])
	assert.Equal(t, false, m["lan"])
	assert.EqualValues(t, os.Getpid(), m["pid"])
}

func TestEditorAPIBoardsLifecycle(t *testing.T) {
	h := start(t)

	assert.JSONEq(t, `[]`, h.get("/api/boards").body)

	// create from the default template, titled with the board name
	r := h.do("POST", "/api/boards/new1", "")
	assert.Equal(t, 200, r.status)
	assert.JSONEq(t, `{"ok":true}`, r.body)

	r = h.do("POST", "/api/boards/new1", "")
	assert.Equal(t, 409, r.status)
	assert.JSONEq(t, `{"error":"board new1 exists"}`, r.body)

	h.put("two", "title: Second\nnodes: { a: A }\n")

	// Consecutive writes can have the same mtime. Set distinct times to test recency order.
	older := time.Unix(1700000000, 0)
	newer := older.Add(time.Minute)
	require.NoError(t, os.Chtimes(h.paths.BoardFile("new1"), older, older))
	require.NoError(t, os.Chtimes(h.paths.BoardFile("two"), newer, newer))

	list := h.get("/api/boards")
	assert.Equal(t, 200, list.status)

	var items []map[string]any
	require.NoError(t, json.Unmarshal([]byte(list.body), &items))
	require.Len(t, items, 2)
	assert.Equal(t, "two", items[0]["name"], "most recently changed first")
	assert.Equal(t, "Second", items[0]["title"])
	assert.Equal(t, "new1", items[1]["name"])
	assert.Equal(t, "new1", items[1]["title"])
	assert.IsType(t, float64(0), items[0]["mtime"])

	r = h.get("/api/boards/two")
	assert.Equal(t, 200, r.status)

	m := r.json(t)
	assert.Equal(t, "two", m["name"])
	assert.Equal(t, "Second", m["board"].(map[string]any)["title"])
	assert.Equal(t, map[string]any{}, m["pos"])

	r = h.get("/api/boards/nope")
	assert.Equal(t, 404, r.status)
	assert.JSONEq(t, `{"error":"no board nope"}`, r.body)

	r = h.get("/api/boards/bad%20name")
	assert.Equal(t, 400, r.status)
	assert.JSONEq(t, `{"error":"bad name"}`, r.body)

	r = h.do("DELETE", "/api/boards/two", "")
	assert.Equal(t, 405, r.status)
	assert.JSONEq(t, `{"error":"method not allowed"}`, r.body)
}

func TestABoardThatDoesNotParseIsAReplyWithAnError(t *testing.T) {
	h := start(t)
	h.put("broken", "nodes: { a: { colour: red } }\n")

	r := h.get("/api/boards/broken")
	assert.Equal(t, 200, r.status)
	assert.JSONEq(t, `{"name":"broken","error":"node a: unknown key \"colour\" — quote values that contain commas or colons"}`, r.body)

	list := h.get("/api/boards")
	assert.Contains(t, list.body, `"title":""`, "an unparseable board is still listed")
}

func TestPositionsRoundTrip(t *testing.T) {
	h := start(t)
	h.put("b", "nodes: { a: A }\n")

	r := h.do("PUT", "/api/boards/b/pos", `{"a":{"x":10,"y":20.5}}`)
	assert.Equal(t, 200, r.status)
	assert.JSONEq(t, `{"ok":true}`, r.body)

	raw, err := os.ReadFile(h.paths.PosFile("b"))
	require.NoError(t, err)
	assert.Equal(t, `{"a":{"x":10,"y":20.5}}`, string(raw), "the body is stored as sent")

	assert.JSONEq(t, `{"a":{"x":10,"y":20.5}}`, mustField(t, h.get("/api/boards/b"), "pos"))

	r = h.do("DELETE", "/api/boards/b/pos", "")
	assert.Equal(t, 200, r.status)
	assert.NoFileExists(t, h.paths.PosFile("b"))

	r = h.do("DELETE", "/api/boards/b/pos", "")
	assert.Equal(t, 200, r.status, "deleting positions that are not there is fine")

	require.NoError(t, os.WriteFile(h.paths.PosFile("b"), []byte("not json"), 0o644))
	assert.JSONEq(t, `{}`, mustField(t, h.get("/api/boards/b"), "pos"), "unreadable positions read as none")
}

func mustField(t *testing.T, r reply, key string) string {
	t.Helper()

	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(r.body), &m))

	return string(m[key])
}

func TestEditingACardInPlace(t *testing.T) {
	h := start(t)
	h.put("b", "# keep me\ntitle: T\n\nnodes:\n  a: { title: Alpha, text: one, kind: src }   # inline\n  b: Bee | two\n")

	r := h.do("PATCH", "/api/boards/b/node/a", `{"title":"New","text":"second"}`)
	assert.Equal(t, 200, r.status)
	assert.JSONEq(t, `{"ok":true}`, r.body)

	got, _ := os.ReadFile(h.paths.BoardFile("b"))
	assert.Equal(t, "# keep me\ntitle: T\n\nnodes:\n  a: { title: New, text: second, kind: src }   # inline\n  b: Bee | two\n", string(got))

	r = h.do("PATCH", "/api/boards/b/node/b", `{"text":""}`)
	assert.Equal(t, 200, r.status)

	got, _ = os.ReadFile(h.paths.BoardFile("b"))
	assert.Contains(t, string(got), "  b: Bee\n")

	r = h.do("PATCH", "/api/boards/b/node/zz", `{"title":"x"}`)
	assert.Equal(t, 400, r.status)
	assert.JSONEq(t, `{"error":"node zz is not under nodes:"}`, r.body)

	r = h.do("PATCH", "/api/boards/b/node/a", `{not json`)
	assert.Equal(t, 400, r.status)
	assert.Contains(t, r.json(t)["error"], "invalid character")

	r = h.do("PATCH", "/api/boards/b/node", `{}`)
	assert.Equal(t, 405, r.status, "a card id is required")
}

func TestEditingAnEncodedCardID(t *testing.T) {
	h := start(t)
	h.put("b", "nodes:\n  \"a b/c\": Old\n")

	r := h.do("PATCH", "/api/boards/b/node/a%20b%2Fc", `{"title":"New"}`)
	assert.Equal(t, 200, r.status, r.body)

	got, _ := os.ReadFile(h.paths.BoardFile("b"))
	assert.Equal(t, "nodes:\n  \"a b/c\": New\n", string(got))
}

func TestStyleSwitch(t *testing.T) {
	h := start(t)
	h.put("b", "# top\ntitle: T\nnodes: { a: A }\n")

	r := h.do("PUT", "/api/boards/b/style", `{"style":"sketch"}`)
	assert.Equal(t, 200, r.status)

	got, _ := os.ReadFile(h.paths.BoardFile("b"))
	assert.Equal(t, "# top\ntitle: T\nnodes: { a: A }\nstyle: sketch\n", string(got))
	assert.Equal(t, "sketch", boardField(t, h.get("/api/boards/b"), "style"))

	r = h.do("PUT", "/api/boards/b/style", `{"style":"clean"}`)
	assert.Equal(t, 200, r.status)

	got, _ = os.ReadFile(h.paths.BoardFile("b"))
	assert.Equal(t, "# top\ntitle: T\nnodes: { a: A }\n", string(got), "anything but sketch removes the key")

	r = h.do("PUT", "/api/boards/b/style", `[1]`)
	assert.Equal(t, 500, r.status)
	assert.Contains(t, r.json(t)["error"], "cannot unmarshal")

	r = h.do("PUT", "/api/boards/b/style", `nope`)
	assert.Equal(t, 500, r.status)
}

func boardField(t *testing.T, r reply, key string) any {
	t.Helper()

	return r.json(t)["board"].(map[string]any)[key]
}

func TestUndoAndHistory(t *testing.T) {
	h := start(t)

	// the watcher snapshots a board 60 ms after it changes
	h.put("b", "title: one\n")
	waitFor(t, func() bool { return historyLen(t, h, "b") == 1 })

	h.put("b", "title: two\n")
	waitFor(t, func() bool { return historyLen(t, h, "b") == 2 })

	r := h.get("/api/boards/b/history")
	assert.Equal(t, 200, r.status)

	var versions []map[string]string
	require.NoError(t, json.Unmarshal([]byte(r.body), &versions))
	assert.Regexp(t, `^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z$`, versions[0]["time"])
	assert.Regexp(t, `^\d{13}$`, versions[0]["id"])

	r = h.do("POST", "/api/boards/b/undo", "")
	assert.Equal(t, 200, r.status)
	assert.JSONEq(t, `{"ok":true}`, r.body)

	got, _ := os.ReadFile(h.paths.BoardFile("b"))
	assert.Equal(t, "title: one\n", string(got))

	waitFor(t, func() bool { return historyLen(t, h, "b") == 1 })

	r = h.do("POST", "/api/boards/b/undo", "")
	assert.Equal(t, 409, r.status)
	assert.JSONEq(t, `{"error":"nothing to undo"}`, r.body)

	assert.Equal(t, 405, h.do("GET", "/api/boards/b/undo", "").status)
}

func TestAChangedFileDropsDisplacedPositions(t *testing.T) {
	h := start(t)

	h.put("b", "nodes: { a: A, b: B }\n")
	waitFor(t, func() bool { return historyLen(t, h, "b") == 1 })
	require.Equal(t, 200, h.do("PUT", "/api/boards/b/pos", `{"a":{"x":1,"y":2},"b":{"x":3,"y":4}}`).status)

	// b got a neighbour
	h.put("b", "nodes: { a: A, b: B }\nedges: [ b -> c ]\n")
	waitFor(t, func() bool { return mustField(t, h.get("/api/boards/b"), "pos") == `{"a":{"x":1,"y":2}}` })
}

func historyLen(t *testing.T, h *harness, name string) int {
	t.Helper()

	var v []any
	require.NoError(t, json.Unmarshal([]byte(h.get("/api/boards/"+name+"/history").body), &v))

	return len(v)
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("condition not reached in time")
}

// ----- notes for the agent

func TestNotesRoundTrip(t *testing.T) {
	h := start(t)
	h.put("b", "nodes: { a: A, b: B }\ngroups: { g: Group }\n")

	r := h.get("/api/boards/b/notes")
	assert.Equal(t, 200, r.status)
	assert.Equal(t, "application/json", r.header.Get("Content-Type"))
	assert.JSONEq(t, `[]`, r.body, "an empty list, not null")

	r = h.do("POST", "/api/boards/b/notes", `{"node":"a","text":"  Rename this card\n"}`)
	assert.Equal(t, 201, r.status)

	m := r.json(t)
	assert.Equal(t, "n1", m["id"])
	assert.Equal(t, "a", m["node"])
	assert.Equal(t, "Rename this card", m["text"], "stored cleaned")
	assert.Equal(t, "", m["author"], "a local note has no author")
	assert.Regexp(t, `^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`, m["created"])
	assert.NotContains(t, m, "resolved")
	assert.NotContains(t, m, "board")

	assert.Equal(t, 201, h.do("POST", "/api/boards/b/notes", `{"node":"g","text":"A group is fine"}`).status)
	assert.Equal(t, 201, h.do("POST", "/api/boards/b/notes", `{"text":"The whole board"}`).status)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(h.get("/api/boards/b/notes").body), &got))
	require.Len(t, got, 3)
	assert.Equal(t, []any{"n1", "n2", "n3"}, []any{got[0]["id"], got[1]["id"], got[2]["id"]}, "oldest first")
	assert.Equal(t, "", got[2]["node"])

	// the file is next to the board and is not part of what the page reads
	assert.FileExists(t, h.paths.NotesFile("b"))
	assert.NotContains(t, h.get("/api/boards/b").body, "Rename this card")

	require.NoError(t, notesResolve(h, "b", "n2"))
	assert.NotContains(t, h.get("/api/boards/b/notes").body, "A group is fine", "only open notes")

	assert.Equal(t, 404, h.get("/api/boards/nope/notes").status)
	assert.JSONEq(t, `{"error":"no board nope"}`, h.do("POST", "/api/boards/nope/notes", `{"text":"x"}`).body)
	assert.Equal(t, 400, h.get("/api/boards/bad%20name/notes").status)

	r = h.do("DELETE", "/api/boards/b/notes", "")
	assert.Equal(t, 405, r.status)
	assert.JSONEq(t, `{"error":"method not allowed"}`, r.body)
}

func TestNotesRefuseWhatIsNotANote(t *testing.T) {
	h := start(t)
	h.put("b", "nodes: { a: A }\n")

	for body, want := range map[string]string{
		`{"node":"a","text":""}`:        "note is empty",
		`{"node":"a","text":"  \n\t "}`: "note is empty",
		`{"node":"a"}`:                  "note is empty",
		`{"node":"a","text":"` + strings.Repeat("x", 2001) + `"}`: "note is longer than 2000 characters",
		`{"node":"ghost","text":"hi"}`:                            "no card or group ghost on this board",
		`not json`:                                                "invalid character 'o' in literal null (expecting 'u')",
	} {
		r := h.do("POST", "/api/boards/b/notes", body)
		assert.Equal(t, 400, r.status, body)
		assert.JSONEq(t, `{"error":"`+want+`"}`, r.body, body)
	}

	assert.NoFileExists(t, h.paths.NotesFile("b"), "a refused note writes nothing")

	// a board that does not parse takes notes on the whole board only: no card can be checked
	h.put("broken", "nodes: { a: { colour: red } }\n")
	assert.Equal(t, 400, h.do("POST", "/api/boards/broken/notes", `{"node":"a","text":"hi"}`).status)
	assert.Equal(t, 201, h.do("POST", "/api/boards/broken/notes", `{"text":"hi"}`).status)
}

func TestNotesAreNotReachableFromAShareLink(t *testing.T) {
	h := start(t)
	h.put("pub", "nodes: { a: A }\n")

	tok, err := h.shares.Share("pub")
	require.NoError(t, err)

	require.Equal(t, 201, h.do("POST", "/api/boards/pub/notes", `{"node":"a","text":"private thought"}`).status)

	for _, r := range []reply{
		h.get("/api/share/" + tok + "/notes"),
		h.do("POST", "/api/share/"+tok+"/notes", `{"text":"from the client"}`),
		h.do("DELETE", "/api/share/"+tok+"/notes", ""),
		h.get("/api/share/" + tok),
	} {
		assert.NotContains(t, r.body, "private thought")
	}

	assert.Equal(t, 405, h.get("/api/share/"+tok+"/notes").status)
	assert.Equal(t, 405, h.do("POST", "/api/share/"+tok+"/notes", `{"text":"from the client"}`).status)

	got, err := notes.List(h.paths, "pub", false)
	require.NoError(t, err)
	assert.Len(t, got, 1, "the client wrote nothing")
}

func TestANoteIsAnEventButNotAVersionOfTheBoard(t *testing.T) {
	h := start(t)
	h.put("b", "nodes: { a: A }\n")
	waitFor(t, func() bool { return historyLen(t, h, "b") == 1 })

	s := h.stream("/api/events")
	require.Equal(t, ": hi", s.next(t, 2*time.Second))
	drain(t, s)

	require.Equal(t, 201, h.do("POST", "/api/boards/b/notes", `{"node":"a","text":"hi"}`).status)
	assert.Equal(t, `data: {"name":"b"}`, s.next(t, 3*time.Second), "the page refetches the notes of b")
	assert.Equal(t, "", s.next(t, 400*time.Millisecond), "one event for the note: the temp file is not a board")

	// the same by hand, as `kreida resolve` does from another process
	require.NoError(t, notes.Resolve(h.paths, "b", "n1", time.Now()))
	assert.Equal(t, `data: {"name":"b"}`, s.next(t, 3*time.Second))
	assert.Equal(t, "", s.next(t, 400*time.Millisecond))

	assert.Equal(t, 1, historyLen(t, h, "b"), "no snapshot for a note")

	// a board change together with a note is still one event and a snapshot
	h.put("b", "nodes: { a: A2 }\n")
	require.NoError(t, notes.Resolve(h.paths, "b", "n1", time.Now()))
	_, err := notes.Add(h.paths, "b", "", "again", time.Now())
	require.NoError(t, err)
	assert.Equal(t, `data: {"name":"b"}`, s.next(t, 3*time.Second))
	waitFor(t, func() bool { return historyLen(t, h, "b") == 2 })
}

func TestNoteEventsAreFilteredToTheirBoardOnShareStreams(t *testing.T) {
	h := start(t)
	h.put("pub", "title: Pub\n")
	h.put("secret", "nodes: { a: A }\n")

	tok, _ := h.shares.Share("pub")
	s := h.stream("/api/share/" + tok + "/events")
	require.Equal(t, ": hi", s.next(t, 2*time.Second))
	drain(t, s)

	require.Equal(t, 201, h.do("POST", "/api/boards/secret/notes", `{"text":"hi"}`).status)
	assert.Equal(t, "", s.next(t, 500*time.Millisecond), "another board's note is not announced")
}

func notesResolve(h *harness, board, id string) error {
	return notes.Resolve(h.paths, board, id, time.Now())
}

// ----- share links

func TestShareLinkServesOnlyItsBoardReadOnly(t *testing.T) {
	h := start(t)
	h.put("pub", "title: Pub\nnodes: { a: A }\n")
	h.put("secret", "title: Secret\nnodes: { s: S }\n")

	tok, err := h.shares.Share("pub")
	require.NoError(t, err)

	r := h.get("/api/share/" + tok)
	assert.Equal(t, 200, r.status)

	m := r.json(t)
	assert.Equal(t, "pub", m["name"])
	assert.Equal(t, "Pub", m["board"].(map[string]any)["title"])
	assert.NotContains(t, r.body, "Secret")

	assert.Equal(t, 405, h.do("PUT", "/api/share/"+tok, "").status)
	assert.Equal(t, 405, h.do("POST", "/api/share/"+tok, "{}").status)
	assert.Equal(t, 405, h.do("DELETE", "/api/share/"+tok, "").status)
	assert.Equal(t, 405, h.get("/api/share/"+tok+"/undo").status, "no sub-route but events and live")
	assert.JSONEq(t, `{"error":"read-only"}`, h.do("PUT", "/api/share/"+tok, "").body)

	r = h.get("/api/share/wrongwrongwrongwrong")
	assert.Equal(t, 404, r.status)
	assert.JSONEq(t, `{"error":"link expired"}`, r.body)

	assert.Equal(t, 404, h.get("/api/share/"+tok+"x").status)
	assert.Equal(t, 404, h.get("/api/share/").status)

	_, err = h.shares.Unshare("pub")
	require.NoError(t, err)
	assert.Equal(t, 404, h.get("/api/share/"+tok).status)
}

// The page of a share link asks for its board with ?lang=<page language> (kreida cloud picks the owner's name by it);
// the local server has no use for it and answers as it always did.
func TestShareLinkIgnoresTheLangQuery(t *testing.T) {
	h := start(t)
	h.put("pub", "title: Pub\nnodes: { a: A }\n")

	tok, err := h.shares.Share("pub")
	require.NoError(t, err)

	plain := h.get("/api/share/" + tok)
	require.Equal(t, 200, plain.status)

	for _, lang := range []string{"ru", "uk", "en", "xx"} {
		r := h.get("/api/share/" + tok + "?lang=" + lang)
		assert.Equal(t, 200, r.status, lang)
		assert.JSONEq(t, plain.body, r.body, lang)
	}
}

func TestShareLinkOfADeletedBoardExpires(t *testing.T) {
	h := start(t)
	h.put("pub", "title: Pub\n")

	tok, err := h.shares.Share("pub")
	require.NoError(t, err)
	require.NoError(t, os.Remove(h.paths.BoardFile("pub")))

	assert.Equal(t, 404, h.get("/api/share/"+tok).status)
}

func TestShareLinkWithABrokenBoardReportsTheError(t *testing.T) {
	h := start(t)
	h.put("pub", "nodes: { a: { colour: red } }\n")

	tok, _ := h.shares.Share("pub")
	r := h.get("/api/share/" + tok)
	assert.Equal(t, 200, r.status)
	assert.Contains(t, r.json(t)["error"], "unknown key")
}

func TestShareLiveValuesNeverShowTheSource(t *testing.T) {
	h := start(t)
	h.put("pub", "nodes:\n  ok: { live: { cmd: \"echo 42\" } }\n  no: { live: { cmd: \"echo hidden-command\" } }\n")

	tok, _ := h.shares.Share("pub")

	// the editor sees what is blocked and why
	r := h.get("/api/boards/pub/live")
	assert.Equal(t, 200, r.status)
	assert.JSONEq(t, `{"ok":{"blocked":true,"source":"cmd:echo 42"},"no":{"blocked":true,"source":"cmd:echo hidden-command"}}`, r.body)

	// allow one source by writing the allow-list the way `kreida live --allow` does
	allow := fmt.Sprintf("[\n  {\n    \"board\": \"pub\",\n    \"source\": \"cmd:echo 42\",\n    \"at\": \"2026-01-01T00:00:00.000Z\"\n  }\n]\n")
	require.NoError(t, os.WriteFile(h.paths.AllowFile(), []byte(allow), 0o600))

	r = h.get("/api/boards/pub/live")
	m := r.json(t)
	ok := m["ok"].(map[string]any)
	assert.Equal(t, "42", ok["value"])
	assert.Equal(t, "cmd:echo 42", ok["source"])
	assert.EqualValues(t, 30, ok["every"])
	assert.NotEmpty(t, ok["at"])

	// the client of a share link sees values and states, never the commands
	r = h.get("/api/share/" + tok + "/live")
	assert.Equal(t, 200, r.status)
	assert.NotContains(t, r.body, "echo")
	assert.NotContains(t, r.body, "source")

	m = r.json(t)
	assert.Equal(t, "42", m["ok"].(map[string]any)["value"])
	assert.Equal(t, map[string]any{"blocked": true}, m["no"])

	assert.Equal(t, 405, h.do("POST", "/api/share/"+tok+"/live", "").status)
}

func TestLiveOfABoardThatDoesNotParseIs500(t *testing.T) {
	h := start(t)
	h.put("b", "nodes:\n  a: { live: { cmd: x, url: \"http://x\" } }\n")

	r := h.get("/api/boards/b/live")
	assert.Equal(t, 500, r.status)
	assert.Contains(t, r.json(t)["error"], "exactly one")
}

// ----- events

type sse struct {
	resp  *http.Response
	lines chan string
}

func (h *harness) stream(path string) *sse {
	h.t.Helper()

	req, _ := http.NewRequest("GET", h.ts.URL+path, nil)
	resp, err := h.ts.Client().Do(req)
	require.NoError(h.t, err)

	h.t.Cleanup(func() { resp.Body.Close() })

	s := &sse{resp: resp, lines: make(chan string, 256)}

	go func() { // one reader per stream, so a timed-out wait never steals the next line
		r := bufio.NewReader(resp.Body)

		for {
			line, err := r.ReadString('\n')
			if err != nil {
				close(s.lines)

				return
			}

			if line = strings.TrimSpace(line); line != "" {
				s.lines <- line
			}
		}
	}()

	return s
}

// next returns the next non-blank line, or "" on timeout or when the stream ends.
func (s *sse) next(t *testing.T, wait time.Duration) string {
	t.Helper()

	select {
	case l := <-s.lines:
		return l
	case <-time.After(wait):
		return ""
	}
}

func TestEditorEventsAnnounceChangedBoards(t *testing.T) {
	h := start(t)
	h.put("b", "title: one\n")

	s := h.stream("/api/events")
	assert.Equal(t, 200, s.resp.StatusCode)
	assert.Equal(t, "text/event-stream", s.resp.Header.Get("Content-Type"))
	assert.Equal(t, "no-cache", s.resp.Header.Get("Cache-Control"))
	assert.Equal(t, ": hi", s.next(t, 2*time.Second))

	drain(t, s)

	h.put("b", "title: two\n")
	assert.Equal(t, `data: {"name":"b"}`, s.next(t, 3*time.Second))

	// a position write is a change of that board too
	require.NoError(t, os.WriteFile(h.paths.PosFile("b"), []byte(`{}`), 0o644))
	assert.Equal(t, `data: {"name":"b"}`, s.next(t, 3*time.Second))

	h.put("other", "title: x\n")
	assert.Equal(t, `data: {"name":"other"}`, s.next(t, 3*time.Second))
}

func TestABurstOfWritesIsOneEvent(t *testing.T) {
	h := start(t)
	h.put("b", "title: 0\n")

	s := h.stream("/api/events")
	require.Equal(t, ": hi", s.next(t, 2*time.Second))
	drain(t, s)

	for i := 1; i <= 5; i++ {
		h.put("b", fmt.Sprintf("title: %d\n", i))
		time.Sleep(5 * time.Millisecond)
	}

	assert.Equal(t, `data: {"name":"b"}`, s.next(t, 3*time.Second))
	assert.Equal(t, "", s.next(t, 400*time.Millisecond), "debounced into one event")
}

// drain discards events caused by the setup of a test.
func drain(t *testing.T, s *sse) {
	t.Helper()

	for s.next(t, 300*time.Millisecond) != "" {
	}
}

func TestShareEventsAreFilteredToTheirBoard(t *testing.T) {
	h := start(t)
	h.put("pub", "title: Pub\n")
	h.put("secret", "title: Secret\n")

	tok, _ := h.shares.Share("pub")
	s := h.stream("/api/share/" + tok + "/events")
	assert.Equal(t, ": hi", s.next(t, 2*time.Second))
	drain(t, s)

	h.put("secret", "title: Secret 2\n")
	assert.Equal(t, "", s.next(t, 500*time.Millisecond), "another board's change is not announced")

	h.put("pub", "title: Pub 2\n")
	assert.Equal(t, `data: {"name":"pub"}`, s.next(t, 3*time.Second))

	assert.Equal(t, 404, h.get("/api/share/nopenopenopenope/events").status)
}

// ----- security

func TestForeignHostIsRefusedOnTheEditorAPI(t *testing.T) {
	h := start(t)
	h.put("b", "title: B\n")

	for _, host := range []string{"evil.example", "evil.example:" + fmt.Sprint(h.port), "127.0.0.1", "127.0.0.1:1", "localhost", "192.168.42.5:" + fmt.Sprint(h.port)} {
		for _, path := range []string{"/api/boards", "/api/boards/b", "/api/health", "/api/events", "/", "/api/boards/b/live"} {
			r := h.get(path, "Host", host)
			assert.Equal(t, 403, r.status, "Host %q on %s", host, path)
			assert.JSONEq(t, `{"error":"only share links are reachable from other machines"}`, r.body)
		}
	}

	for _, host := range []string{fmt.Sprintf("127.0.0.1:%d", h.port), fmt.Sprintf("localhost:%d", h.port)} {
		assert.Equal(t, 200, h.get("/api/boards", "Host", host).status, host)
	}
}

func TestCrossOriginRequestsAreRefused(t *testing.T) {
	h := start(t)
	h.put("b", "title: B\nnodes:\n  a: A\n")

	self := fmt.Sprintf("http://127.0.0.1:%d", h.port)

	for _, origin := range []string{"http://evil.example", "https://evil.example", "null", "http://127.0.0.1:1", "http://localhost", "http://127.0.0.1:" + fmt.Sprint(h.port) + ".evil.example"} {
		r := h.do("POST", "/api/boards/csrf", "", "Origin", origin)
		assert.Equal(t, 403, r.status, "Origin %q", origin)
		assert.NoFileExists(t, h.paths.BoardFile("csrf"))

		r = h.do("PATCH", "/api/boards/b/node/a", `{"title":"pwned"}`, "Origin", origin)
		assert.Equal(t, 403, r.status, "Origin %q", origin)

		r = h.do("PUT", "/api/boards/b/pos", `{}`, "Origin", origin)
		assert.Equal(t, 403, r.status)
	}

	got, _ := os.ReadFile(h.paths.BoardFile("b"))
	assert.Equal(t, "title: B\nnodes:\n  a: A\n", string(got))

	// the page's own origin, with or without a scheme match, is fine
	for _, origin := range []string{self, fmt.Sprintf("http://localhost:%d", h.port), fmt.Sprintf("https://127.0.0.1:%d", h.port)} {
		r := h.do("PUT", "/api/boards/b/pos", `{}`, "Origin", origin)
		assert.Equal(t, 200, r.status, "Origin %q", origin)
	}

	assert.Equal(t, 200, h.do("PUT", "/api/boards/b/pos", `{}`).status, "no Origin header: curl and the CLI")
}

func TestSecFetchSiteCrossSiteIsRefused(t *testing.T) {
	h := start(t)

	assert.Equal(t, 403, h.get("/api/boards", "Sec-Fetch-Site", "cross-site").status)
	assert.Equal(t, 200, h.get("/api/boards", "Sec-Fetch-Site", "same-origin").status)
	assert.Equal(t, 200, h.get("/api/boards", "Sec-Fetch-Site", "none").status)
	assert.Equal(t, 200, h.get("/api/boards", "Sec-Fetch-Site", "same-site").status)
}

func TestNonLoopbackPeersReachOnlyShareLinksAndAssets(t *testing.T) {
	h := start(t)
	h.put("pub", "title: Pub\nnodes: { a: A }\n")
	h.put("secret", "title: Secret\n")

	tok, err := h.shares.Share("pub")
	require.NoError(t, err)

	handler := h.srv.Handler()
	lan := fmt.Sprintf("192.168.42.5:%d", h.port)

	call := func(method, path, remote string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "http://"+lan+path, nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		return rec
	}

	for _, remote := range []string{"192.168.42.9:5555", "10.0.0.2:1", "[2001:db8::1]:80", "127.0.0.2:1", "8.8.8.8:1"} {
		// reachable: the share link, its API, the page assets
		assert.Equal(t, 200, call("GET", "/s/"+tok, remote).Code, remote)
		assert.Equal(t, 200, call("GET", "/assets/app.js", remote).Code)
		r := call("GET", "/api/share/"+tok, remote)
		assert.Equal(t, 200, r.Code)
		assert.Contains(t, r.Body.String(), "Pub")
		assert.Equal(t, 405, call("PUT", "/api/share/"+tok, remote).Code)

		// not reachable: everything else
		for _, path := range []string{"/", "/api/boards", "/api/boards/secret", "/api/events", "/api/health", "/index.html", "/anything", "/api/boards/secret/live", "/api/boards/secret/notes"} {
			r := call("GET", path, remote)
			assert.Equal(t, 403, r.Code, "%s from %s", path, remote)
			assert.NotContains(t, r.Body.String(), "Secret")
		}

		assert.Equal(t, 403, call("POST", "/api/boards/x", remote).Code)
		assert.Equal(t, 403, call("PATCH", "/api/boards/pub/node/a", remote).Code)
	}

	// the same requests from a loopback peer with this server's own Host
	req := httptest.NewRequest("GET", fmt.Sprintf("http://127.0.0.1:%d/api/boards", h.port), nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, 200, rec.Code)

	for _, remote := range []string{"[::1]:5555", "[::ffff:127.0.0.1]:5555"} {
		req := httptest.NewRequest("GET", fmt.Sprintf("http://[::1]:%d/api/boards", h.port), nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		assert.Equal(t, 200, rec.Code, remote)
	}

	// a loopback peer with a foreign Host (DNS rebinding) is refused
	req = httptest.NewRequest("GET", fmt.Sprintf("http://rebind.example:%d/api/boards", h.port), nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	assert.Equal(t, 403, rec.Code)
}

// ----- the page

func TestStaticPage(t *testing.T) {
	h := start(t)

	r := h.get("/")
	assert.Equal(t, 200, r.status)
	assert.Equal(t, "text/html", r.header.Get("Content-Type"))
	assert.Equal(t, "no-cache", r.header.Get("Cache-Control"))
	assert.Contains(t, r.body, "<div id=root>")

	r = h.get("/assets/app.js")
	assert.Equal(t, 200, r.status)
	assert.Equal(t, "text/javascript", r.header.Get("Content-Type"))
	assert.Equal(t, "max-age=31536000, immutable", r.header.Get("Cache-Control"))
	assert.Equal(t, "console.log(1)", r.body)

	assert.Equal(t, "font/woff", h.get("/assets/f.woff").header.Get("Content-Type"))

	// any other path is the page itself, which finds its board in the URL
	for _, p := range []string{"/s/sometoken", "/?b=x", "/nope/nope", "/assets/missing.js", "/../../etc/passwd", "/assets/../index.html"} {
		r = h.get(p)
		assert.Equal(t, 200, r.status, p)
		assert.Contains(t, r.body, "<div id=root>", p)
	}

	assert.Equal(t, 404, h.get("/api/share/x").status, "an unknown share link is not the page")
}

func TestServerWithoutAPageSaysSo(t *testing.T) {
	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}

	srv, err := server.New(server.Config{Paths: p, Port: 1})
	require.NoError(t, err)
	t.Cleanup(srv.Close)

	req := httptest.NewRequest("GET", "http://127.0.0.1:1/", nil)
	req.RemoteAddr = "127.0.0.1:5555"
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	assert.Equal(t, 500, rec.Code)
	assert.Contains(t, rec.Body.String(), "page not built")
}

// ----- serving

func TestServeOnARandomPortAndShutDownWithOpenStreams(t *testing.T) {
	home := t.TempDir()
	p := paths.Paths{Home: home, Dir: filepath.Join(home, "boards")}

	srv, err := server.New(server.Config{Paths: p, Dist: testDist})
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)

	go func() { done <- srv.Serve(ctx, ln) }()

	base := fmt.Sprintf("http://127.0.0.1:%d", port)

	waitFor(t, func() bool {
		resp, err := http.Get(base + "/api/health")
		if err != nil {
			return false
		}

		resp.Body.Close()

		return resp.StatusCode == 200
	})

	// the Host check follows the listener's real port
	resp, err := http.Get(base + "/api/boards")
	require.NoError(t, err)
	resp.Body.Close()
	assert.Equal(t, 200, resp.StatusCode)

	stream, err := http.Get(base + "/api/events")
	require.NoError(t, err)

	defer stream.Body.Close()

	cancel()

	select {
	case err := <-done:
		assert.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("shutdown did not finish while an event stream was open")
	}
}
