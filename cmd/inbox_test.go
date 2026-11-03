package cmd_test

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/iamnikolie/kreida/internal/notes"
	"github.com/iamnikolie/kreida/internal/paths"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	at1 = time.Date(2026, 10, 30, 9, 15, 0, 0, time.UTC)
	at2 = at1.Add(time.Hour)
	at3 = at1.Add(2 * time.Hour)
)

// when is how the inbox shows a time: local, to the minute.
func when(t time.Time) string { return t.Local().Format("2006-01-02 15:04") }

// note writes a local note the way the server does.
func (e *env) note(board, node, text string, at time.Time) {
	e.t.Helper()

	_, err := notes.Add(paths.Paths{Home: e.home, Dir: e.dir}, board, node, text, at)
	require.NoError(e.t, err)
}

func TestInboxPrintsOpenNotesOldestFirstOneBlockEach(t *testing.T) {
	e := newEnv(t)
	e.ok("title: Demo\nnodes: { a1: A, b2: B }\n", "set", "demo")

	assert.Equal(t, "no open notes\n", e.ok("", "inbox", "demo"))
	assert.Equal(t, "no open notes\n", e.ok("", "inbox"), "no notes anywhere")

	e.note("demo", "a1", "Rename this card", at1)
	e.note("demo", "", "Too wide:\nsplit it\n\nin two", at2)
	e.note("demo", "b2", "Done already", at3)
	require.NoError(t, notes.Resolve(paths.Paths{Home: e.home, Dir: e.dir}, "demo", "n3", at3))

	want := "demo n1 on a1 · you · " + when(at1) + "\n" +
		"  Rename this card\n" +
		"\n" +
		"demo n2 on board · you · " + when(at2) + "\n" +
		"  Too wide:\n" +
		"  split it\n" +
		"  \n" +
		"  in two\n" +
		"\n"

	assert.Equal(t, want, e.ok("", "inbox", "demo"))
	assert.Equal(t, want, e.ok("", "inbox"), "no name: every board")
	assert.NotContains(t, e.ok("", "inbox", "demo"), "Done already", "resolved ones stay out")
}

func TestInboxAllShowsResolvedOnesAndSaysSo(t *testing.T) {
	e := newEnv(t)
	e.ok("title: Demo\nnodes: { a1: A }\n", "set", "demo")
	e.note("demo", "a1", "First", at1)
	e.note("demo", "a1", "Second", at2)
	require.NoError(t, notes.Resolve(paths.Paths{Home: e.home, Dir: e.dir}, "demo", "n1", at3))

	assert.Equal(t, "demo n1 on a1 · you · "+when(at1)+" · resolved "+when(at3)+"\n  First\n\n"+
		"demo n2 on a1 · you · "+when(at2)+"\n  Second\n\n", e.ok("", "inbox", "demo", "--all"))

	e.ok("title: Empty\n", "set", "empty")
	assert.Equal(t, "no notes\n", e.ok("", "inbox", "empty", "--all"))
}

func TestInboxWithoutANameMergesBoardsByTime(t *testing.T) {
	e := newEnv(t)
	e.ok("title: A\nnodes: { x: X }\n", "set", "alpha")
	e.ok("title: B\nnodes: { y: Y }\n", "set", "beta")
	e.ok("title: C\n", "set", "quiet")

	e.note("beta", "y", "second in time", at2)
	e.note("alpha", "x", "third in time", at3)
	e.note("beta", "", "first in time", at1)

	out := e.ok("", "inbox")
	assert.Equal(t, []string{"beta n2", "beta n1", "alpha n1"}, headers(out))

	assert.NotContains(t, e.ok("", "inbox", "alpha"), "beta")
	assert.Equal(t, "no open notes\n", e.ok("", "inbox", "quiet"))
}

// headers lists "<board> <id>" of every block of an inbox listing.
func headers(out string) []string {
	var got []string

	for _, line := range strings.Split(out, "\n") {
		if line != "" && !strings.HasPrefix(line, " ") {
			f := strings.Fields(line)
			got = append(got, f[0]+" "+f[1])
		}
	}

	return got
}

func TestInboxJSONIsAnArrayOfNotesWithTheirBoard(t *testing.T) {
	e := newEnv(t)
	e.ok("title: Demo\nnodes: { a1: A }\n", "set", "demo")

	assert.Equal(t, "[]\n", e.ok("", "inbox", "demo", "--json"))
	assert.Equal(t, "[]\n", e.ok("", "inbox", "--json"))

	e.note("demo", "a1", "Use <b>bold</b> & 100% \"quotes\"", at1)
	e.note("demo", "", "Whole board", at2)

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(e.ok("", "inbox", "demo", "--json")), &got))
	require.Len(t, got, 2)
	assert.Equal(t, map[string]any{"id": "n1", "board": "demo", "node": "a1", "text": "Use <b>bold</b> & 100% \"quotes\"", "author": "", "created": "2026-10-30T09:15:00Z"}, got[0])
	assert.Equal(t, "", got[1]["node"])

	assert.Contains(t, e.ok("", "inbox", "--json"), "<b>bold</b> &", "no HTML escaping in what an agent reads")

	require.NoError(t, notes.Resolve(paths.Paths{Home: e.home, Dir: e.dir}, "demo", "n2", at3))
	assert.Len(t, jsonList(t, e.ok("", "inbox", "demo", "--json")), 1)

	all := jsonList(t, e.ok("", "inbox", "demo", "--json", "--all"))
	require.Len(t, all, 2)
	assert.Equal(t, "2026-10-30T11:15:00Z", all[1]["resolved"])
}

func jsonList(t *testing.T, out string) []map[string]any {
	t.Helper()

	var got []map[string]any
	require.NoError(t, json.Unmarshal([]byte(out), &got), out)

	return got
}

func TestInboxPrintsTextAsIsAndNeverAsAFormat(t *testing.T) {
	e := newEnv(t)
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "100%s done %d times\t# not a heading **not bold** {{x}}", at1)

	assert.Contains(t, e.ok("", "inbox", "demo"), "\n  100%s done %d times\t# not a heading **not bold** {{x}}\n")
}

func TestInboxNeedsAValidExistingBoard(t *testing.T) {
	e := newEnv(t)

	assert.Equal(t, "bad board name: bad name", e.fail("", "inbox", "bad name"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "inbox", "nope"))
	assert.Contains(t, e.fail("", "inbox", "a", "b"), "accepts at most 1 arg")
}

const comments = `[` +
	`{"id":"c_1","node":"a1","text":"Can this go up?","author":"Anna <anna@client.example>","created":"2026-10-30T10:15:00Z"},` +
	`{"id":"c_2","node":"","text":"Looks good\nthanks","author":"Boris","created":"2026-10-30T10:45:00Z"}]`

func TestInboxAddsTheCloudCommentsOfTheBoardOldestFirst(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")
	e.ok("title: Demo\nnodes: { a1: A }\n", "set", "demo")
	e.note("demo", "a1", "Mine, first", at1)
	e.note("demo", "", "Mine, last", at3)

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if method == "GET" && path == "/api/cli/boards/demo/comments" {
			_, _ = io.WriteString(w, comments)

			return true
		}

		return false
	}

	out := e.ok("", "inbox", "demo")
	assert.Equal(t, "demo n1 on a1 · you · "+when(at1)+"\n  Mine, first\n\n"+
		"demo c_1 on a1 · Anna <anna@client.example> · "+when(at2)+"\n  Can this go up?\n\n"+
		"demo c_2 on board · Boris · "+when(at2.Add(30*time.Minute))+"\n  Looks good\n  thanks\n\n"+
		"demo n2 on board · you · "+when(at3)+"\n  Mine, last\n\n", out)
	assert.Equal(t, []string{"GET /api/cli/boards/demo/comments "}, f.seen("GET /api/cli/boards"))

	got := jsonList(t, e.ok("", "inbox", "demo", "--json"))
	require.Len(t, got, 4)
	assert.Equal(t, []any{"n1", "c_1", "c_2", "n2"}, []any{got[0]["id"], got[1]["id"], got[2]["id"], got[3]["id"]})
	assert.Equal(t, "demo", got[1]["board"], "a comment of this board is told its board")
	assert.Equal(t, "Anna <anna@client.example>", got[1]["author"])
}

func TestInboxWithoutANameAsksTheCloudForEveryBoard(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo", "other")
	e.ok("title: Demo\nnodes: { a1: A }\n", "set", "demo")
	e.note("demo", "a1", "Local note", at3)

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if method == "GET" && path == "/api/cli/comments" {
			_, _ = io.WriteString(w, `[{"id":"c_7","board":"other","node":"z","text":"From a client","author":"Anna","created":"2026-10-30T09:15:00Z"}]`)

			return true
		}

		return false
	}

	out := e.ok("", "inbox")
	assert.Equal(t, []string{"other c_7", "demo n1"}, headers(out))
	assert.Equal(t, []string{"GET /api/cli/comments "}, f.seen("GET"))

	// a board that only exists on the cloud is still in the list; naming it is a local board's job
	assert.Contains(t, out, "other c_7 on z · Anna · "+when(at1))
}

func TestInboxAllAsksTheCloudForResolvedCommentsToo(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")
	e.ok("title: Demo\n", "set", "demo")

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		_, _ = io.WriteString(w, `[]`)

		return true
	}

	e.ok("", "inbox", "demo", "--all")
	e.ok("", "inbox", "--all")
	assert.Equal(t, []string{"GET /api/cli/boards/demo/comments?all=1 ", "GET /api/cli/comments?all=1 "}, f.seen("GET"))
}

func TestInboxWithoutALoginNeverCallsTheCloud(t *testing.T) {
	e, f := cloudEnv(t)
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "Local", at1)

	out, errOut, err := e.run("", "inbox", "demo")
	require.NoError(t, err)
	assert.Contains(t, out, "demo n1 on board")
	assert.Equal(t, "", errOut)
	assert.Empty(t, f.seen(""))
}

func TestInboxOfABoardThatIsNotPublishedHasNoCloudCommentsAndNoWarning(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f)
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "Local", at1)

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"status":"error","message":"not published"}`)

		return true
	}

	out, errOut, err := e.run("", "inbox", "demo")
	require.NoError(t, err)
	assert.Contains(t, out, "demo n1 on board")
	assert.Equal(t, "", errOut)
	assert.Len(t, f.seen("GET /api/cli/boards/demo/comments"), 1)
}

func TestInboxKeepsTheLocalNotesWhenTheCloudFails(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "Local", at1)

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(500)
		_, _ = io.WriteString(w, `{"status":"error","message":"boom"}`)

		return true
	}

	for _, args := range [][]string{{"inbox", "demo"}, {"inbox"}} {
		out, errOut, err := e.run("", args...)
		require.NoError(t, err, "a cloud failure is not the inbox's failure")
		assert.Equal(t, "demo n1 on board · you · "+when(at1)+"\n  Local\n\n", out)
		assert.Equal(t, "kreida: cloud comments: cloud: boom (HTTP 500)\n", errOut)
	}

	// a refused token says how to log in again, and stdout stays what a script can parse
	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"status":"error","message":"token revoked"}`)

		return true
	}

	out, errOut, err := e.run("", "inbox", "--json")
	require.NoError(t, err)
	assert.Len(t, jsonList(t, out), 1, "stdout stays clean JSON")
	assert.Equal(t, "kreida: cloud comments: cloud: token revoked (HTTP 401) (sign in again to your configured server)\n", errOut)
}

func TestInboxShowsCloudTextAsPlainText(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")
	e.ok("title: Demo\n", "set", "demo")

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		_, _ = io.WriteString(w, `[{"id":"c_1","node":"a\u001b[2J","text":"hi \u001b[31mred\u001b[0m 100%s\n\ndemo n9 on a1 · you · x","author":"Eve\nfake n9 on a1","created":"2026-10-30T09:15:00Z"}]`)

		return true
	}

	out := e.ok("", "inbox", "demo")
	assert.NotContains(t, out, "\x1b", "a terminal obeys no escape a stranger sent")
	assert.Equal(t, "demo c_1 on a[2J · Eve fake n9 on a1 · "+when(at1)+"\n  hi [31mred[0m 100%s\n  \n  demo n9 on a1 · you · x\n\n", out,
		"the author is one line, and every line of the text is indented: no fake block")
}

func TestResolveClosesLocalNotesAndSaysSo(t *testing.T) {
	e := newEnv(t)
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "one", at1)
	e.note("demo", "", "two", at2)
	e.note("demo", "", "three", at3)

	assert.Equal(t, "resolved n1\nresolved n3\n", e.ok("", "resolve", "demo", "n1", "n3"))
	assert.Equal(t, []string{"demo n2"}, headers(e.ok("", "inbox", "demo")))

	assert.Equal(t, "resolved n1\n", e.ok("", "resolve", "demo", "n1"), "again is fine")
	assert.Len(t, headers(e.ok("", "inbox", "demo", "--all")), 3, "resolved is not deleted")

	assert.Equal(t, "no note n9", e.fail("", "resolve", "demo", "n9"))
	assert.Equal(t, "no note x", e.fail("", "resolve", "demo", "x"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "resolve", "nope", "n1"))
	assert.Equal(t, "bad board name: (none)", e.fail("", "resolve"))
	assert.Equal(t, "no note id (kreida inbox demo)", e.fail("", "resolve", "demo"))
}

func TestResolveStopsAtTheFirstUnknownID(t *testing.T) {
	e := newEnv(t)
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "one", at1)
	e.note("demo", "", "two", at2)

	out, _, err := e.run("", "resolve", "demo", "n1", "n9", "n2")
	require.EqualError(t, err, "no note n9")
	assert.Equal(t, "resolved n1\n", out)
	assert.Equal(t, []string{"demo n2"}, headers(e.ok("", "inbox", "demo")))
}

func TestResolveSendsCloudCommentsToTheCloud(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "local", at1)

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		if method == "POST" {
			w.WriteHeader(204)

			return true
		}

		return false
	}

	assert.Equal(t, "resolved c_1\nresolved n1\nresolved c_2\n", e.ok("", "resolve", "demo", "c_1", "n1", "c_2"))
	assert.Equal(t, []string{
		"POST /api/cli/boards/demo/comments/c_1/resolve ",
		"POST /api/cli/boards/demo/comments/c_2/resolve ",
	}, f.seen("POST"))
	assert.Equal(t, "no open notes\n", e.ok("", "inbox", "demo"))
}

func TestResolveACloudCommentNeedsALoginBeforeAnythingChanges(t *testing.T) {
	e, f := cloudEnv(t)
	e.ok("title: Demo\n", "set", "demo")
	e.note("demo", "", "local", at1)

	assert.Equal(t, "requires login to your own configured compatible server", e.fail("", "resolve", "demo", "n1", "c_1"))
	assert.Equal(t, []string{"demo n1"}, headers(e.ok("", "inbox", "demo")), "n1 was not resolved on the way")
	assert.Empty(t, f.seen(""))
}

func TestResolveACloudCommentThatTheCloudRefusesIsAnError(t *testing.T) {
	e, f := cloudEnv(t)
	e.login(f, "demo")
	e.ok("title: Demo\n", "set", "demo")

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(404)
		_, _ = io.WriteString(w, `{"status":"error","message":"no comment"}`)

		return true
	}

	assert.Equal(t, "cloud: no comment (HTTP 404)", e.fail("", "resolve", "demo", "c_404"))

	f.answer = func(method, path string, w http.ResponseWriter) bool {
		w.WriteHeader(401)
		_, _ = io.WriteString(w, `{"status":"error","message":"token revoked"}`)

		return true
	}

	assert.Equal(t, "cloud: token revoked (HTTP 401) (sign in again to your configured server)", e.fail("", "resolve", "demo", "c_1"))
}
