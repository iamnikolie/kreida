package board

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// value parses YAML text into plain values.
func value(t *testing.T, text string) any {
	t.Helper()

	doc, err := parseYAML(text)
	require.NoError(t, err, "must parse:\n%s", text)

	v, err := decode(doc)
	require.NoError(t, err)

	return v
}

type patchOp struct {
	Doc   string  `json:"doc"`
	Op    string  `json:"op"`
	ID    string  `json:"id"`
	Title *string `json:"title"`
	Text  *string `json:"text"`
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

func read(t *testing.T, path string) string {
	t.Helper()

	b, err := os.ReadFile(path)
	require.NoError(t, err)

	return string(b)
}

func lines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")

	return strings.Split(s, "\n")
}

func counts(ls []string) map[string]int {
	m := map[string]int{}
	for _, l := range ls {
		m[l]++
	}

	return m
}

func runOp(text string, op patchOp) (string, bool, error) {
	if op.Op == "patchNode" {
		return edit(text, planPatchNode(op.ID, op.Title, op.Text))
	}

	return edit(text, planSetTop(op.Key, op.Value))
}

// TestGoldenEdits replays patchNode/setTop calls on the documents under testdata/patch/docs. The text need not match
// the expected output byte for byte; the document must read back the same, and every line of the input that the
// expected output keeps must survive.
func TestGoldenEdits(t *testing.T) {
	dir := filepath.Join("testdata", "patch")
	files, err := filepath.Glob(filepath.Join(dir, "*.op.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)

	var fellBack []string

	for _, f := range files {
		name := strings.TrimSuffix(filepath.Base(f), ".op.json")

		t.Run(name, func(t *testing.T) {
			var op patchOp
			require.NoError(t, json.Unmarshal([]byte(read(t, f)), &op))

			in := read(t, filepath.Join(dir, "docs", op.Doc+".yaml"))
			out, spliced, err := runOp(in, op)

			if errPath := filepath.Join(dir, name+".err.json"); fileExists(errPath) {
				var want struct{ Message string }
				require.NoError(t, json.Unmarshal([]byte(read(t, errPath)), &want))
				require.Error(t, err)
				assert.Equal(t, want.Message, err.Error())

				return
			}

			require.NoError(t, err)

			if !spliced {
				fellBack = append(fellBack, name)
			}

			want := read(t, filepath.Join(dir, name+".out.yaml"))

			assert.Equal(t, value(t, want), value(t, out), "document differs:\n--- got\n%s\n--- want\n%s", out, want)

			if _, err := Parse(out); err != nil {
				t.Fatalf("edit left an invalid board: %v", err)
			}

			// everything the input and the expected output share verbatim must survive: comments, blank lines
			inC, wantC, gotC := counts(lines(in)), counts(lines(want)), counts(lines(out))
			for l, n := range inC {
				keep := min(n, wantC[l])
				assert.GreaterOrEqual(t, gotC[l], keep, "lost line %q\n--- ours\n%s", l, out)
			}
		})
	}

	sort.Strings(fellBack)
	// block scalars and tagged scalars are left to the re-encoder; every other golden is spliced into the original text
	assert.Equal(t, []string{"patch-literal", "patch-tagged"}, fellBack)
}

func fileExists(p string) bool {
	_, err := os.Stat(p)

	return err == nil
}

func sp(s string) *string { return &s }

// TestSplicePreservesEverythingElse: a splice changes only the touched lines.
func TestSplicePreservesEverythingElse(t *testing.T) {
	in := read(t, filepath.Join("testdata", "patch", "docs", "doc1.yaml"))

	cases := []struct {
		name  string
		title *string
		text  *string
		id    string
		want  string // the one line (or two) that must differ from the input
	}{
		{"flow map value", sp("New"), nil, "a", "  a: { title: New, text: one, kind: src }   # inline comment"},
		{"quoted scalar keeps its quotes", sp("B"), sp("T"), "b", `  b: "B | T"`},
		{"single quotes are kept", sp("x"), nil, "i", `  i: 'x | quoted'`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, spliced, err := edit(in, planPatchNode(c.id, c.title, c.text))
			require.NoError(t, err)
			require.True(t, spliced)

			inL, outL := lines(in), lines(out)
			require.Equal(t, len(inL), len(outL))

			var diff []string

			for i := range inL {
				if inL[i] != outL[i] {
					diff = append(diff, outL[i])
				}
			}

			assert.Equal(t, []string{c.want}, diff)
		})
	}
}

func TestSpliceAddsAndDeletesPairs(t *testing.T) {
	in := "# c\nnodes:\n  a:\n    title: A   # keep\n    kind: src\n\n  b: B\n"

	out, spliced, err := edit(in, planPatchNode("a", nil, sp("added")))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, "# c\nnodes:\n  a:\n    title: A   # keep\n    kind: src\n    text: added\n\n  b: B\n", out)

	out, spliced, err = edit(out, planPatchNode("a", nil, sp("")))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, in, out, "removing the text again restores the original text")

	flow := "nodes:\n  a: { title: A, text: x, kind: src }\n"
	out, spliced, err = edit(flow, planPatchNode("a", nil, sp("")))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, "nodes:\n  a: { title: A, kind: src }\n", out)

	out, _, err = edit(flow, planPatchNode("a", sp("Z"), sp("y")))
	require.NoError(t, err)
	assert.Equal(t, "nodes:\n  a: { title: Z, text: y, kind: src }\n", out)

	out, _, err = edit("nodes:\n  a: { kind: src }\n", planPatchNode("a", sp("T"), sp("x")))
	require.NoError(t, err)
	assert.Equal(t, "nodes:\n  a: { kind: src, title: T, text: x }\n", out)
}

func TestSetTopRoundTrip(t *testing.T) {
	in := "# header\ntitle: T\n\nnodes:\n  a: A   # card\nedges:\n  - a -> b\n# end\n"

	out, spliced, err := edit(in, planSetTop("style", sp("sketch")))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, "# header\ntitle: T\n\nnodes:\n  a: A   # card\nedges:\n  - a -> b\nstyle: sketch\n# end\n", out)

	back, spliced, err := edit(out, planSetTop("style", nil))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, in, back, "dropping the style again restores the original text")

	again, _, err := edit(out, planSetTop("style", sp("sketch")))
	require.NoError(t, err)
	assert.Equal(t, out, again)
}

func TestEditKeepsCRLF(t *testing.T) {
	in := "title: T\r\nnodes:\r\n  a: { title: A }\r\n  b: B\r\n"

	out, spliced, err := edit(in, planPatchNode("b", sp("B2"), nil))
	require.NoError(t, err)
	assert.True(t, spliced)
	assert.Equal(t, "title: T\r\nnodes:\r\n  a: { title: A }\r\n  b: B2\r\n", out)

	out, _, err = edit(in, planSetTop("style", sp("sketch")))
	require.NoError(t, err)
	assert.Equal(t, in+"style: sketch\r\n", out)
}

func TestEditErrors(t *testing.T) {
	_, _, err := edit("nodes:\n  a: A\n", planPatchNode("zz", sp("x"), nil))
	assert.EqualError(t, err, "node zz is not under nodes:")

	_, _, err = edit("nodes:\n  - id: a\n", planPatchNode("zz", sp("x"), nil))
	assert.EqualError(t, err, "node zz is not under nodes:")

	_, _, err = edit("nodes: [", planSetTop("style", sp("sketch")))
	assert.Error(t, err)

	_, _, err = edit("just text\n", planSetTop("style", sp("sketch")))
	assert.EqualError(t, err, "board.SetTop: the board is not a YAML map")
}

func TestFormatScalar(t *testing.T) {
	cases := []struct {
		val    string
		style  yaml.Style
		inFlow bool
		want   string
	}{
		{"Alpha", 0, false, "Alpha"},
		{"Title: with colon", 0, true, `"Title: with colon"`},
		{"a, b", 0, false, "a, b"},
		{"a, b", 0, true, `"a, b"`},
		{"# hash", 0, false, `"# hash"`},
		{"say \"hi\"", 0, false, `say "hi"`},
		{"\"hi\" there", 0, false, `'"hi" there'`},
		{"it's", 0, false, "it's"},
		{`both "d" and 'q'`, 0, false, `both "d" and 'q'`},
		{"123", 0, false, `"123"`},
		{"true", 0, false, `"true"`},
		{"null", 0, false, `"null"`},
		{"~", 0, false, `"~"`},
		{" padded ", 0, false, `" padded "`},
		{"", 0, false, `""`},
		{"line\nbreak", 0, false, `"line\nbreak"`},
		{"Привет, мир", 0, false, "Привет, мир"},
		{"→ дальше", 0, false, "→ дальше"},
		{"yes", 0, false, "yes"},
		{"- x", 0, false, `"- x"`},
		{":::src Bold", 0, true, `":::src Bold"`},
		{"a:b", 0, false, "a:b"},
		{"B | T", yaml.DoubleQuotedStyle, false, `"B | T"`},
		{"it's", yaml.SingleQuotedStyle, false, `'it''s'`},
		{"a\nb", yaml.SingleQuotedStyle, false, `"a\nb"`},
	}

	for _, c := range cases {
		assert.Equal(t, c.want, formatScalar(c.val, c.style, c.inFlow), "%q", c.val)
	}
}
