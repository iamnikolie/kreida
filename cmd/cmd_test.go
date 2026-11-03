package cmd_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/iamnikolie/kreida/cmd"
	"github.com/iamnikolie/kreida/internal/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testDist = fstest.MapFS{
	"index.html": {Data: []byte(`<!doctype html>
<html><head>
  <title>kreida</title>
  <script type="module" crossorigin src="/assets/app.js"></script>
  <link rel="stylesheet" crossorigin href="/assets/app.css">
</head><body><div id="root"></div></body></html>`)},
	"assets/app.js":  {Data: []byte(`console.log("</script><b>")`)},
	"assets/app.css": {Data: []byte(`body{color:red}/* </style> */`)},
}

type env struct {
	t    *testing.T
	home string
	dir  string
	// thumb draws the thumbnails of published boards; nil is a stub, so no test starts Chrome.
	thumb cloud.Renderer
}

// stubThumb is the smallest PNG-looking bytes.
func stubThumb(context.Context, string) ([]byte, error) { return []byte("\x89PNG\r\n\x1a\nstub"), nil }

func (e *env) thumbs() cloud.Renderer {
	if e.thumb != nil {
		return e.thumb
	}

	return stubThumb
}

// newEnv points every run at a scratch home and a port nothing listens on; it never touches ~/.kreida or 7788.
func newEnv(t *testing.T) *env {
	t.Helper()

	home := t.TempDir()
	t.Setenv("KREIDA_HOME", home)
	t.Setenv("KREIDA_DIR", "")
	t.Setenv("KREIDA_PORT", "17999")
	t.Setenv("KREIDA_LAN", "")

	return &env{t: t, home: home, dir: filepath.Join(home, "boards")}
}

// run executes a command line and returns what it wrote to stdout and stderr, and its error.
func (e *env) run(stdin string, args ...string) (string, string, error) {
	e.t.Helper()

	var out, errOut bytes.Buffer

	app := &cmd.App{Out: &out, Err: &errOut, In: strings.NewReader(stdin), Dist: testDist, Thumb: e.thumbs()}
	root := cmd.NewRoot(app)
	root.SetArgs(args)

	err := root.Execute()

	return out.String(), errOut.String(), err
}

func (e *env) ok(stdin string, args ...string) string {
	e.t.Helper()

	out, _, err := e.run(stdin, args...)
	require.NoError(e.t, err)

	return out
}

func (e *env) fail(stdin string, args ...string) string {
	e.t.Helper()

	_, _, err := e.run(stdin, args...)
	require.Error(e.t, err)

	return err.Error()
}

func (e *env) board(name string) string {
	e.t.Helper()

	b, err := os.ReadFile(filepath.Join(e.dir, name+".yaml"))
	require.NoError(e.t, err)

	return string(b)
}

func TestHelpIsTheSameForNoArgsHelpAndFlags(t *testing.T) {
	e := newEnv(t)

	help := e.ok("")
	assert.True(t, strings.HasPrefix(help, "kreida — live boards from the shell (YAML → canvas)\n\n  kreida up [--lan]"), help)
	assert.Contains(t, help, "(http://127.0.0.1:17999)")
	assert.True(t, strings.HasSuffix(help, "\nboards: "+e.dir+"\n"), help)

	assert.Equal(t, help, e.ok("", "help"))
	assert.Equal(t, help, e.ok("", "-h"))
	assert.Equal(t, help, e.ok("", "--help"))

	for _, c := range []string{"up", "open", "ls", "new", "templates", "set", "cat", "path", "undo", "history", "export", "import", "png", "reset", "rm", "url", "share", "unshare", "shares", "live", "inbox", "resolve", "down", "skill"} {
		assert.Contains(t, help, "kreida "+c, c)
	}

	assert.NotContains(t, help, "kreida serve", "the server command is not advertised")
}

func TestUnknownCommandAndBadNames(t *testing.T) {
	e := newEnv(t)

	assert.Equal(t, "unknown command bogus (kreida help)", e.fail("", "bogus"))
	assert.Equal(t, "bad board name: (none)", e.fail("", "cat"))
	assert.Equal(t, "bad board name: bad name", e.fail("", "cat", "bad name"))
	assert.Equal(t, "bad board name: (none)", e.fail("", "new"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "cat", "nope"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "undo", "nope"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "export", "nope"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "live", "nope"))
	assert.Equal(t, "no board nope (kreida ls)", e.fail("", "share", "nope"))
}

func TestBadEnvironment(t *testing.T) {
	e := newEnv(t)
	t.Setenv("KREIDA_PORT", "seven")

	assert.Contains(t, e.fail("", "ls"), `bad KREIDA_PORT "seven"`)
}

func TestNewAndLs(t *testing.T) {
	e := newEnv(t)

	assert.Equal(t, "", e.ok("", "ls"))

	assert.Equal(t, filepath.Join(e.dir, "foo.yaml")+"\n", e.ok("", "new", "foo"))
	assert.Contains(t, e.board("foo"), "title: foo\n")

	assert.Equal(t, "board foo exists", e.fail("", "new", "foo"))
	assert.Equal(t, "no template nope (kreida templates)", e.fail("", "new", "x", "--template", "nope"))
	assert.NoFileExists(t, filepath.Join(e.dir, "x.yaml"))

	e.ok("", "new", "lanes1", "--template", "lanes", "Lanes:", "board")
	assert.Contains(t, e.board("lanes1"), `title: "Lanes: board"`)
	assert.Contains(t, e.board("lanes1"), "layout: lanes")

	e.ok("", "new", "ba", "--template", "before-after", "Привет")
	assert.Contains(t, e.board("ba"), "steps:")

	out := e.ok("", "ls")
	assert.Equal(t, "ba                       Привет\nfoo                      foo\nlanes1                   Lanes: board\n", out)

	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "broken.yaml"), []byte("nodes: { a: { colour: red } }\n"), 0o644))
	assert.Contains(t, e.ok("", "ls"), "broken                   (invalid)\n")
}

func TestEveryTemplateMakesAValidBoard(t *testing.T) {
	e := newEnv(t)

	for _, line := range strings.Split(strings.TrimSpace(e.ok("", "templates")), "\n") {
		name := strings.Fields(line)[0]
		e.ok("", "new", "t-"+name, "--template", name)
	}

	assert.Contains(t, e.ok("", "ls"), "t-mindmap")
}

func TestTemplatesList(t *testing.T) {
	e := newEnv(t)

	golden, err := os.ReadFile(filepath.Join("..", "internal", "templates", "testdata", "list.txt"))
	require.NoError(t, err)
	assert.Equal(t, string(golden), e.ok("", "templates"))
}

func TestSetFromStdinFileAndDash(t *testing.T) {
	e := newEnv(t)

	assert.Equal(t, "http://127.0.0.1:17999/?b=a\n", e.ok("title: A\nnodes: { a: A }", "set", "a"))
	assert.Equal(t, "title: A\nnodes: { a: A }\n", e.board("a"), "a newline is added")

	assert.Equal(t, "http://127.0.0.1:17999/?b=a\n", e.ok("title: A2\n", "set", "a", "-"))
	assert.Equal(t, "title: A2\n", e.board("a"))

	src := filepath.Join(t.TempDir(), "in.yaml")
	require.NoError(t, os.WriteFile(src, []byte("title: From file\n"), 0o644))
	e.ok("", "set", "a", src)
	assert.Equal(t, "title: From file\n", e.board("a"))

	assert.Contains(t, e.fail("", "set", "a", "/nonexistent/in.yaml"), "no such file")

	msg := e.fail("nodes: { a: { colour: red } }", "set", "a")
	assert.True(t, strings.HasPrefix(msg, "invalid board: node a: unknown key"), msg)
	assert.Equal(t, "title: From file\n", e.board("a"), "a rejected board leaves the old one")

	assert.Equal(t, "invalid board: board must be a YAML map", e.fail("", "set", "a"))
	assert.Equal(t, "bad board name: bad name", e.fail("title: x", "set", "bad name"))
}

func TestCatPathURLRmReset(t *testing.T) {
	e := newEnv(t)
	e.ok("title: T\n", "set", "a")

	assert.Equal(t, "title: T\n", e.ok("", "cat", "a"))
	assert.Equal(t, filepath.Join(e.dir, "a.yaml")+"\n", e.ok("", "path", "a"))
	assert.Equal(t, filepath.Join(e.dir, "ghost.yaml")+"\n", e.ok("", "path", "ghost"), "path does not need the board to exist")
	assert.Equal(t, "http://127.0.0.1:17999/?b=ghost\n", e.ok("", "url", "ghost"))

	pos := filepath.Join(e.dir, "a.pos.json")
	require.NoError(t, os.WriteFile(pos, []byte("{}"), 0o644))

	assert.Equal(t, "", e.ok("", "reset", "a"))
	assert.NoFileExists(t, pos)
	assert.Equal(t, "", e.ok("", "reset", "a"), "no positions to drop is fine")
	assert.Equal(t, "no board ghost (kreida ls)", e.fail("", "reset", "ghost"))

	notesFile := filepath.Join(e.dir, "a.notes.json")
	require.NoError(t, os.WriteFile(pos, []byte("{}"), 0o644))
	require.NoError(t, os.WriteFile(notesFile, []byte(`{"next":1,"notes":[]}`), 0o644))
	assert.Equal(t, "", e.ok("", "rm", "a"))
	assert.NoFileExists(t, filepath.Join(e.dir, "a.yaml"))
	assert.NoFileExists(t, pos)
	assert.NoFileExists(t, notesFile, "the notes go with the board")
	assert.Equal(t, "no board a (kreida ls)", e.fail("", "rm", "a"))
}

func TestUndoAndHistory(t *testing.T) {
	e := newEnv(t)
	e.ok("title: one\n", "set", "a")

	assert.Equal(t, "nothing to undo", e.fail("", "undo", "a"))

	// set keeps every text it writes as a version
	hist := regexp.MustCompile(`^\d{13}  \d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}Z\n$`)
	assert.Regexp(t, hist, e.ok("", "history", "a"))

	e.ok("title: two\n", "set", "a")
	assert.Equal(t, "http://127.0.0.1:17999/?b=a\n", e.ok("", "undo", "a"))
	assert.Equal(t, "title: one\n", e.board("a"))
	assert.Equal(t, "nothing to undo", e.fail("", "undo", "a"))
}

func TestSetDropsPositionsTheChangeDisplaces(t *testing.T) {
	e := newEnv(t)
	e.ok("groups: { g: G }\nnodes: { a: { group: g }, b: { group: g }, c: C }\n", "set", "a")
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "a.pos.json"), []byte(`{"a":{"x":1,"y":1},"c":{"x":2,"y":2}}`), 0o644))

	e.ok("groups: { g: G }\nnodes: { a: { group: g }, b: { group: g }, c: C, d: { group: g } }\n", "set", "a")

	pos, err := os.ReadFile(filepath.Join(e.dir, "a.pos.json"))
	require.NoError(t, err)
	assert.Equal(t, `{"c":{"x":2,"y":2}}`, string(pos), "g got a new member; c stays where it was dragged")
}

func TestImportAndExportMermaid(t *testing.T) {
	e := newEnv(t)

	src := "%% title: Imp\nflowchart LR\n  A[Start] --> B{Q}\n  B -->|yes| C[(DB)]\n"
	assert.Equal(t, "http://127.0.0.1:17999/?b=mm\n", e.ok(src, "import", "mm"))
	assert.Equal(t, "title: Imp\ndirection: LR\nnodes:\n  A: Start\n  B: Q\n  C: { title: DB, icon: database }\nedges:\n  - A -> B\n  - \"B -> C: yes\"\n", e.board("mm"))

	assert.Equal(t, "board mm exists (--force to replace; kreida undo restores)", e.fail(src, "import", "mm"))
	e.ok("flowchart TD\n  X --> Y\n", "import", "mm", "--force")
	assert.Contains(t, e.board("mm"), "X -> Y")

	assert.Contains(t, e.fail("pie\n \"a\": 1", "import", "pie"), "only flowchart")
	assert.NoFileExists(t, filepath.Join(e.dir, "pie.yaml"))

	e.ok(src, "import", "again", "-")
	text := e.ok("", "export", "again", "--mermaid")
	assert.True(t, strings.HasPrefix(text, "%% title: Imp\nflowchart LR\n"), text)
	assert.True(t, strings.HasSuffix(text, "\n"))

	out := filepath.Join(t.TempDir(), "out.mmd")
	assert.Equal(t, out+"\n", e.ok("", "export", "again", "--mermaid", "-o", out))

	written, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, text, string(written))
}

func TestExportHTMLInlinesThePageAndTheBoard(t *testing.T) {
	e := newEnv(t)
	e.ok("title: A <b> board\nnodes: { a: A }\n", "set", "a")
	require.NoError(t, os.WriteFile(filepath.Join(e.dir, "a.pos.json"), []byte(`{"a":{"x":1,"y":2}}`), 0o644))

	out := filepath.Join(t.TempDir(), "a.html")
	assert.Equal(t, out+"\n", e.ok("", "export", "a", "-o", out))

	b, err := os.ReadFile(out)
	require.NoError(t, err)

	html := string(b)
	assert.Contains(t, html, `<script type="module">console.log("<\/script><b>")</script>`, "the script is inlined and cannot close its tag early")
	assert.Contains(t, html, `<style>body{color:red}/* <\/style> */</style>`)
	assert.NotContains(t, html, `src="/assets`)
	assert.Contains(t, html, `<title>A &lt;b> board</title>`)
	assert.Contains(t, html, `window.__KREIDA__=window.__DOSKA__={"name":"a","board":{"title":"A \u003cb\u003e board"`)
	assert.Contains(t, html, `"pos":{"a":{"x":1,"y":2}}`)
	assert.True(t, strings.HasPrefix(html, "<!doctype html>\n<html><head><script>window."), "the data comes first in <head>")

	// default file name: <name>.html in the current directory
	wd, _ := os.Getwd()
	require.NoError(t, os.Chdir(t.TempDir()))
	t.Cleanup(func() { _ = os.Chdir(wd) })

	got := e.ok("", "export", "a")
	assert.True(t, strings.HasSuffix(got, "a.html\n"), got)
}

func TestExportHTMLWithoutAPageSaysSo(t *testing.T) {
	e := newEnv(t)
	e.ok("title: A\n", "set", "a")

	var out bytes.Buffer

	app := &cmd.App{Out: &out, Err: &out, In: strings.NewReader(""), Dist: fstest.MapFS{".keep": {}}}
	root := cmd.NewRoot(app)
	root.SetArgs([]string{"export", "a", "-o", filepath.Join(t.TempDir(), "a.html")})

	err := root.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "page not built")
}

func TestShareLinksWithoutAServer(t *testing.T) {
	e := newEnv(t)
	e.ok("title: A\n", "set", "a")

	assert.Equal(t, "", e.ok("", "shares"))
	assert.Equal(t, "not shared", strings.TrimSpace(e.ok("", "unshare", "a")))
}

func TestLiveCommands(t *testing.T) {
	e := newEnv(t)
	e.ok("nodes:\n  a: { title: A, live: { cmd: \"echo 42\" } }\n  bb: { title: B, live: { url: \"http://127.0.0.1:1/x\", every: 120 } }\n", "set", "lv")

	assert.Equal(t, "no live cards\n", func() string { e.ok("title: x\n", "set", "plain"); return e.ok("", "live", "plain") }())

	assert.Equal(t, "BLOCKED  a              every  30s  cmd  echo 42\nBLOCKED  bb             every 120s  url  http://127.0.0.1:1/x\n", e.ok("", "live", "lv"))
	assert.Equal(t, "a              BLOCKED\nbb             BLOCKED\n", e.ok("", "live", "lv", "--run"))

	assert.Equal(t, "allowed  a: cmd:echo 42\n", e.ok("", "live", "lv", "--allow", "a"))
	assert.Equal(t, "allowed  a              every  30s  cmd  echo 42\nBLOCKED  bb             every 120s  url  http://127.0.0.1:1/x\n", e.ok("", "live", "lv"))
	assert.Equal(t, "allowed  bb: url:http://127.0.0.1:1/x\n", e.ok("", "live", "lv", "--allow"))
	assert.Equal(t, "nothing new to allow\n", e.ok("", "live", "lv", "--allow"))

	run := e.ok("", "live", "lv", "--run")
	assert.True(t, strings.HasPrefix(run, "a              42\nbb             error: "), run)

	assert.Equal(t, "revoked 2\n", e.ok("", "live", "lv", "--revoke"))
	assert.Equal(t, "revoked 0\n", e.ok("", "live", "lv", "--revoke"))

	info, err := os.Stat(filepath.Join(e.home, "live-allow.json"))
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
}

func TestSkillPrintsTheEmbeddedReference(t *testing.T) {
	e := newEnv(t)

	text := e.ok("", "skill")
	assert.True(t, strings.HasPrefix(text, "---\nname: kreida\n"), text[:40])
	assert.Contains(t, text, "## Board format (YAML)")
	assert.Contains(t, text, "kreida inbox workflow", "the inbox is in the command block")
	assert.Contains(t, text, "kreida resolve workflow")
	assert.Contains(t, text, "Notes from the human (inbox)")
	assert.NotContains(t, strings.ToLower(text), "doska")

	file, err := os.ReadFile("skill.md")
	require.NoError(t, err)
	assert.Equal(t, string(file), text)
}

func TestSubcommandHelpIsCobras(t *testing.T) {
	e := newEnv(t)

	out := e.ok("", "export", "--help")
	assert.Contains(t, out, "--mermaid")
	assert.Contains(t, out, "-o, --output")
}
