package board_test

import (
	"encoding/json"
	"testing"

	"github.com/iamnikolie/kreida/internal/board"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func node(t *testing.T, b *board.Board, id string) board.Node {
	t.Helper()

	for _, n := range b.Nodes {
		if n.ID == id {
			return n
		}
	}

	t.Fatalf("no node %s", id)

	return board.Node{}
}

func score(f float64) *float64 { return &f }

func TestTableCardColsAndRows(t *testing.T) {
	b, err := board.Parse(`nodes:
  opts:
    title: Варианты
    cols: [Вариант, Цена, Скорость]
    rows:
      - [n8n, "$20", { text: быстро, score: 3 }]
      - [Zapier, { text: "$70", score: -1 }]
`)
	require.NoError(t, err)

	n := node(t, b, "opts")
	assert.Equal(t, []string{"Вариант", "Цена", "Скорость"}, n.Cols)
	assert.Equal(t, [][]board.Cell{
		{{Text: "n8n"}, {Text: "$20"}, {Text: "быстро", Score: score(3)}},
		{{Text: "Zapier"}, {Text: "$70", Score: score(-1)}, {Text: ""}}, // a short row is padded
	}, n.Rows)
}

func TestTableCardRowsWithoutCols(t *testing.T) {
	b, err := board.Parse("nodes:\n  t: { rows: [[a, b], [c]] }\n")
	require.NoError(t, err)

	n := node(t, b, "t")
	assert.Nil(t, n.Cols)
	assert.Equal(t, [][]board.Cell{{{Text: "a"}, {Text: "b"}}, {{Text: "c"}, {Text: ""}}}, n.Rows)
}

func TestTableCardErrors(t *testing.T) {
	for yaml, msg := range map[string]string{
		"nodes:\n  t: { cols: [a], rows: [[x, y]] }\n":          "row 1 has 2 cells, cols has 1",
		"nodes:\n  t: { fields: [id], rows: [[x]] }\n":          "fields or cols/rows, not both",
		"nodes:\n  t: { rows: [x] }\n":                          "row 1 must be a list of cells",
		"nodes:\n  t: { rows: [[{ text: a, score: lots }]] }\n": "score must be a number",
		"nodes:\n  t: { rows: [[{ text: a, color: red }]] }\n":  "unknown key \"color\"",
		"nodes:\n  t: { cols: a }\n":                            "cols must be a list",
	} {
		_, err := board.Parse(yaml)
		assert.ErrorContains(t, err, msg, yaml)
	}
}

func TestTableFieldsStayOffJSONWhenUnused(t *testing.T) {
	b, err := board.Parse("nodes:\n  a: A\n")
	require.NoError(t, err)

	data, err := json.Marshal(b)
	require.NoError(t, err)

	for _, k := range []string{`"cols"`, `"rows"`, `"x"`, `"y"`, `"questions"`, `"terms"`, `"matrix"`} {
		assert.NotContains(t, string(data), k)
	}
}

func TestMatrixLayout(t *testing.T) {
	b, err := board.Parse(`title: Что делать первым
layout: matrix
x: { label: Усилия, low: мало, high: много }
y: { label: Эффект, low: слабый, high: сильный }
hot: low-high
quadrants: { low-high: Быстрые победы, high-low: Не трогать }
nodes:
  a: { title: Шаблоны ответов, x: low, y: high }
  b: { title: Своя CRM, x: high, y: high }
  c: Ещё не решили
`)
	require.NoError(t, err)

	assert.Equal(t, "matrix", b.Layout)
	require.NotNil(t, b.Matrix)
	assert.Equal(t, board.Axis{Label: "Усилия", Low: "мало", High: "много"}, b.Matrix.X)
	assert.Equal(t, board.Axis{Label: "Эффект", Low: "слабый", High: "сильный"}, b.Matrix.Y)
	assert.Equal(t, "low-high", b.Matrix.Hot)
	assert.Equal(t, map[string]string{"low-high": "Быстрые победы", "high-low": "Не трогать"}, b.Matrix.Quadrants)

	assert.Equal(t, "low", node(t, b, "a").X)
	assert.Equal(t, "high", node(t, b, "a").Y)
	assert.Empty(t, node(t, b, "c").X) // no quadrant: drawn under the matrix
}

func TestMatrixDefaultsAndErrors(t *testing.T) {
	b, err := board.Parse("layout: matrix\nnodes:\n  a: { x: high, y: low }\n")
	require.NoError(t, err)
	require.NotNil(t, b.Matrix)
	assert.Equal(t, board.Axis{Low: "low", High: "high"}, b.Matrix.X)
	assert.Equal(t, map[string]string{}, b.Matrix.Quadrants)

	// x/y outside a matrix are kept, and do nothing
	b, err = board.Parse("nodes:\n  a: { x: high, y: low }\n")
	require.NoError(t, err)
	assert.Nil(t, b.Matrix)

	for yaml, msg := range map[string]string{
		"layout: matrix\nnodes:\n  a: { x: middle, y: low }\n": "node a: x must be low or high",
		"layout: matrix\nhot: top-left\n":                      "hot must be one of low-low, low-high, high-low, high-high",
		"layout: matrix\nquadrants: { left: a }\n":             "quadrants: \"left\" must be one of low-low",
		"layout: matrix\nx: { label: a, mid: b }\n":            "x: unknown key \"mid\"",
	} {
		_, err := board.Parse(yaml)
		assert.ErrorContains(t, err, msg, yaml)
	}
}

func TestRadialLayout(t *testing.T) {
	b, err := board.Parse("layout: radial\nnodes: { a: A }\n")
	require.NoError(t, err)
	assert.Equal(t, "radial", b.Layout)

	b, err = board.Parse("layout: spiral\n")
	require.NoError(t, err)
	assert.Equal(t, "auto", b.Layout)
}

func TestQuestions(t *testing.T) {
	b, err := board.Parse(`nodes:
  budget: Бюджет
groups:
  g: Группа
questions:
  - Кто подписывает договор?
  - { text: Сколько в месяц, on: budget }
  - { text: Доступ к базе, done: true, on: g }
`)
	require.NoError(t, err)
	assert.Equal(t, []board.Question{
		{Text: "Кто подписывает договор?"},
		{Text: "Сколько в месяц", On: "budget"},
		{Text: "Доступ к базе", Done: true, On: "g"},
	}, b.Questions)

	for yaml, msg := range map[string]string{
		"questions:\n  - { text: a, on: nowhere }\n": "question 1: unknown node or group \"nowhere\"",
		"questions:\n  - { text: a, who: b }\n":      "question 1: unknown key \"who\"",
		"questions:\n  - { done: true }\n":           "question 1 needs text",
		"questions: a\n":                             "questions must be a list",
	} {
		_, err := board.Parse(yaml)
		assert.ErrorContains(t, err, msg, yaml)
	}
}

func TestTerms(t *testing.T) {
	b, err := board.Parse(`terms:
  CRM: база клиентов и сделок
  webhook: { text: "адрес, на который сервис сам шлёт событие", aka: [вебхук, вебхуки] }
`)
	require.NoError(t, err)
	assert.Equal(t, []board.Term{
		{Term: "CRM", Text: "база клиентов и сделок", Aka: []string{}},
		{Term: "webhook", Text: "адрес, на который сервис сам шлёт событие", Aka: []string{"вебхук", "вебхуки"}},
	}, b.Terms)

	for yaml, msg := range map[string]string{
		"terms: [a, b]\n":                      "terms must be a map (term: definition)",
		"terms:\n  CRM: { text: a, see: b }\n": "term CRM: unknown key \"see\"",
		"terms:\n  CRM: \"\"\n":                "term CRM needs a definition",
	} {
		_, err := board.Parse(yaml)
		assert.ErrorContains(t, err, msg, yaml)
	}
}

func TestMatrixCellChangeDisplacesCard(t *testing.T) {
	prev, err := board.Parse("layout: matrix\nnodes:\n  a: { x: low, y: low }\n  b: { x: high, y: high }\n")
	require.NoError(t, err)
	next, err := board.Parse("layout: matrix\nnodes:\n  a: { x: high, y: low }\n  b: { x: high, y: high }\n")
	require.NoError(t, err)

	assert.Equal(t, map[string]bool{"a": true}, board.Displaced(prev, next))
}

func TestGroupCollapsed(t *testing.T) {
	b, err := board.Parse("groups:\n  a: { label: A, collapsed: true }\n  b: B\n")
	require.NoError(t, err)
	assert.True(t, b.Groups[0].Collapsed)
	assert.False(t, b.Groups[1].Collapsed)

	data, err := json.Marshal(b.Groups[1])
	require.NoError(t, err)
	assert.NotContains(t, string(data), "collapsed")
}

func TestBoardLang(t *testing.T) {
	for yaml, want := range map[string]string{"lang: uk\n": "uk", "lang: EN\n": "en", "title: x\n": "", "lang: de\n": ""} {
		b, err := board.Parse(yaml)
		require.NoError(t, err)
		assert.Equal(t, want, b.Lang, yaml)
	}
}
