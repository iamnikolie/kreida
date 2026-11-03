// Package render produces the two artifacts made from a board outside the live page: a standalone HTML file
// (`kreida export`) and a PNG screenshot taken by headless Chrome (`kreida png`).
package render

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"regexp"
	"strings"

	"github.com/iamnikolie/kreida/internal/board"
)

var (
	scriptTag = regexp.MustCompile(`<script type="module" crossorigin src="/([^"]+)"></script>`)
	styleTag  = regexp.MustCompile(`<link rel="stylesheet" crossorigin href="/([^"]+)">`)
	titleTag  = regexp.MustCompile(`<title>[^<]*</title>`)
	closeTag  = regexp.MustCompile(`(?i)</(script|style)`)
)

// Data is what the standalone page finds in window.__KREIDA__ (and window.__DOSKA__, the name the page reads today):
// the board, its card positions and its name, as the API would have served them.
type Data struct {
	Name  string          `json:"name"`
	Board *board.Board    `json:"board"`
	Pos   json.RawMessage `json:"pos"`
}

// HTML returns one HTML file holding the page with its script and styles inlined and the board in the data
// variable, so it opens from a file or any static host with no server.
func HTML(dist fs.FS, d Data) (string, error) {
	index, err := fs.ReadFile(dist, "index.html")
	if err != nil {
		return "", fmt.Errorf("render.HTML: page not built (run make build): %w", err)
	}

	html := string(index)

	html, err = inline(html, dist, scriptTag, func(js string) string { return `<script type="module">` + js + `</script>` })
	if err != nil {
		return "", err
	}

	html, err = inline(html, dist, styleTag, func(css string) string { return `<style>` + css + `</style>` })
	if err != nil {
		return "", err
	}

	if len(d.Pos) == 0 {
		d.Pos = json.RawMessage("{}")
	}

	data, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("render.HTML: %w", err)
	}

	script := "window.__KREIDA__=window.__DOSKA__=" + strings.ReplaceAll(string(data), "<", `\u003c`)

	html = strings.Replace(html, "<head>", "<head><script>"+script+"</script>", 1)

	title := d.Board.Title
	if title == "" {
		title = d.Name
	}

	loc := titleTag.FindStringIndex(html)
	if loc != nil {
		html = html[:loc[0]] + "<title>" + strings.ReplaceAll(title, "<", "&lt;") + "</title>" + html[loc[1]:]
	}

	return html, nil
}

// inline replaces the first tag matching re with wrap(content of the file it points to).
func inline(html string, dist fs.FS, re *regexp.Regexp, wrap func(string) string) (string, error) {
	m := re.FindStringSubmatchIndex(html)
	if m == nil {
		return html, nil
	}

	src, err := fs.ReadFile(dist, html[m[2]:m[3]])
	if err != nil {
		return "", fmt.Errorf("render.inline: %w", err)
	}

	body := closeTag.ReplaceAllStringFunc(string(src), func(s string) string { return `<\/` + s[2:] })

	return html[:m[0]] + wrap(body) + html[m[1]:], nil
}
