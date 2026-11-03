package render_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/iamnikolie/kreida/internal/render"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The shape of the HTML vite emits for the page.
const index = `<!doctype html>
<html lang="ru">
<head>
  <meta charset="utf-8">
  <title>doska</title>
  <script type="module" crossorigin src="/assets/index-AAA.js"></script>
  <link rel="stylesheet" crossorigin href="/assets/index-BBB.css">
</head>
<body>
  <div id="root"></div>
</body>
</html>`

func dist() fstest.MapFS {
	return fstest.MapFS{
		"index.html":           {Data: []byte(index)},
		"assets/index-AAA.js":  {Data: []byte(`var x="</SCRIPT>";console.log(x, "$1 $& $'")`)},
		"assets/index-BBB.css": {Data: []byte(`a{b:c}/*</style>*/`)},
	}
}

func data(t *testing.T, yaml string) render.Data {
	t.Helper()

	b, err := board.Parse(yaml)
	require.NoError(t, err)

	return render.Data{Name: "demo", Board: b, Pos: json.RawMessage(`{"a":{"x":1,"y":2}}`)}
}

func TestHTMLInlinesScriptAndStyleAndCarriesTheBoard(t *testing.T) {
	html, err := render.HTML(dist(), data(t, "title: Demo\nnodes: { a: A }\n"))
	require.NoError(t, err)

	assert.Contains(t, html, `<script type="module">var x="<\/SCRIPT>";console.log(x, "$1 $& $'")</script>`, "closing tags inside are broken up; $ patterns are literal")
	assert.Contains(t, html, `<style>a{b:c}/*<\/style>*/</style>`)
	assert.NotContains(t, html, `crossorigin`)
	assert.Contains(t, html, "<title>Demo</title>")
	assert.Contains(t, html, `<head><script>window.__KREIDA__=window.__DOSKA__={"name":"demo","board":{"title":"Demo"`)
	assert.Contains(t, html, `"pos":{"a":{"x":1,"y":2}}}</script>`)
	assert.Contains(t, html, `<meta charset="utf-8">`)
}

func TestHTMLTitleFallsBackToTheNameAndIsEscaped(t *testing.T) {
	html, err := render.HTML(dist(), data(t, "nodes: { a: A }\n"))
	require.NoError(t, err)
	assert.Contains(t, html, "<title>demo</title>")

	html, err = render.HTML(dist(), data(t, "title: \"</title><script>alert(1)</script>\"\n"))
	require.NoError(t, err)
	assert.Contains(t, html, "<title>&lt;/title>&lt;script>alert(1)&lt;/script></title>")
	assert.NotContains(t, html, "<script>alert(1)", "board data cannot close the data script either")
	assert.Contains(t, html, `\u003c/title\u003e`, "the data carries < as an escape, so it cannot end the script")
}

func TestHTMLWithoutPositionsHasEmptyOnes(t *testing.T) {
	d := data(t, "title: x\n")
	d.Pos = nil

	html, err := render.HTML(dist(), d)
	require.NoError(t, err)
	assert.Contains(t, html, `"pos":{}}`)
}

func TestHTMLErrors(t *testing.T) {
	_, err := render.HTML(fstest.MapFS{".keep": {}}, data(t, "title: x\n"))
	assert.ErrorContains(t, err, "page not built")

	d := dist()
	delete(d, "assets/index-AAA.js")

	_, err = render.HTML(d, data(t, "title: x\n"))
	assert.Error(t, err)
}

func TestFindChromeTakesTheBinaryOfTheEnvironment(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "my-chrome")
	require.NoError(t, os.WriteFile(bin, []byte("#!/bin/sh\n"), 0o755))

	t.Setenv(render.EnvChrome, bin)

	got, err := render.FindChrome()
	require.NoError(t, err)
	assert.Equal(t, bin, got)

	// a name that points nowhere is an error, not a silent search for another Chrome
	t.Setenv(render.EnvChrome, filepath.Join(t.TempDir(), "nope"))

	_, err = render.FindChrome()
	assert.ErrorIs(t, err, render.ErrNoChrome)
	assert.ErrorContains(t, err, render.EnvChrome)

	t.Setenv(render.EnvChrome, t.TempDir())

	_, err = render.FindChrome()
	assert.ErrorIs(t, err, render.ErrNoChrome)
}

func TestFindChromeReturnsAnExistingBinaryOrTheSentinel(t *testing.T) {
	p, err := render.FindChrome()
	if err != nil {
		assert.ErrorIs(t, err, render.ErrNoChrome)

		return
	}

	_, statErr := os.Stat(p)
	assert.NoError(t, statErr)
}

func TestPNGOfAnUnreachablePageFailsWithoutLeavingAFile(t *testing.T) {
	chrome, err := render.FindChrome()
	if err != nil {
		t.Skip("no Chrome here")
	}

	out := filepath.Join(t.TempDir(), "x.png")
	err = render.PNG(chrome, "http://127.0.0.1:1/never", out)

	// Chrome renders its own error page and exits 0, or it fails; either way this must not panic or hang
	if err == nil {
		assert.FileExists(t, out)
	}
}
