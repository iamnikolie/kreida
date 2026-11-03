// Package board reads a board file (YAML) into the normalized board the page draws, and edits board files in place.
//
// The page never parses YAML: the server hands it the JSON shapes defined here (web/src/shared/api/contract.ts is the
// other side of that contract).
package board

// Board is a normalized board.
type Board struct {
	Title     string       `json:"title"`
	Sub       string       `json:"sub"`
	Direction string       `json:"direction"`
	Layout    string       `json:"layout"`
	Style     string       `json:"style"`
	Legend    []LegendItem `json:"legend"`
	Groups    []Group      `json:"groups"`
	Nodes     []Node       `json:"nodes"`
	Edges     []Edge       `json:"edges"`
	Steps     []Step       `json:"steps"`
	// Matrix is the 2×2 of `layout: matrix`: its axes, the hot corner and the quadrant labels.
	Matrix    *Matrix    `json:"matrix,omitempty"`
	Questions []Question `json:"questions,omitempty"`
	Terms     []Term     `json:"terms,omitempty"`
	// Lang is the language of the board's words (ru, uk, en): the page labels its own bits on the canvas in it.
	Lang string `json:"lang,omitempty"`
}

// Matrix holds a 2×2's axes. Corners are "<x>-<y>": "low-high" is low on x, high on y.
type Matrix struct {
	X         Axis              `json:"x"`
	Y         Axis              `json:"y"`
	Hot       string            `json:"hot,omitempty"`
	Quadrants map[string]string `json:"quadrants"`
}

// Axis names an axis of a matrix and its two ends.
type Axis struct {
	Label string `json:"label"`
	Low   string `json:"low"`
	High  string `json:"high"`
}

// Question is an open question of the board, optionally about one card or group.
type Question struct {
	Text string `json:"text"`
	Done bool   `json:"done,omitempty"`
	On   string `json:"on,omitempty"`
}

// Term is a glossary entry: cards that mention the term or one of its aliases explain it on hover.
type Term struct {
	Term string   `json:"term"`
	Text string   `json:"text"`
	Aka  []string `json:"aka"`
}

// LegendItem names what a kind (colour role) means.
type LegendItem struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
}

// Group is a container of cards.
type Group struct {
	ID        string  `json:"id"`
	Label     string  `json:"label"`
	Kind      *string `json:"kind"`
	Direction *string `json:"direction"`
	Parent    *string `json:"parent"`
	// Collapsed starts the group folded into one tile on the page (the reader can unfold it).
	Collapsed bool `json:"collapsed,omitempty"`
}

// Node is a card; with Fields it is a table row set.
type Node struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Text   string   `json:"text"`
	Kind   string   `json:"kind"`
	Group  *string  `json:"group"`
	Icon   *string  `json:"icon"`
	Shape  *string  `json:"shape"`
	Width  *float64 `json:"width"`
	Step   *float64 `json:"step"`
	Fields []Field  `json:"fields"`
	Live   *Live    `json:"live"`
	// Cols and Rows make the card a table of text cells (a comparison, a scored decision).
	Cols []string `json:"cols,omitempty"`
	Rows [][]Cell `json:"rows,omitempty"`
	// X and Y place the card in a quadrant of `layout: matrix`: "low" or "high".
	X string `json:"x,omitempty"`
	Y string `json:"y,omitempty"`
}

// Cell is a table cell; a score tints it (the table's lowest red, its highest green).
type Cell struct {
	Text  string   `json:"text"`
	Score *float64 `json:"score,omitempty"`
}

// Field is a table row of a card: a name, an optional foreign key and a note.
type Field struct {
	Name   string  `json:"name"`
	Ref    *string `json:"ref"`
	Dashed bool    `json:"dashed"`
	Note   string  `json:"note"`
}

// Live is a value a card shows and refreshes: a shell command or an http(s) URL.
type Live struct {
	Source string  `json:"source"`
	Every  float64 `json:"every"`
	Path   *string `json:"path"`
	Label  string  `json:"label"`
}

// Edge is an arrow. FromPort and ToPort name the table rows a foreign-key edge joins.
type Edge struct {
	ID       string `json:"id"`
	From     string `json:"from"`
	To       string `json:"to"`
	Label    string `json:"label"`
	Dashed   bool   `json:"dashed,omitempty"`
	Hidden   bool   `json:"hidden,omitempty"`
	Thick    bool   `json:"thick,omitempty"`
	Animated bool   `json:"animated,omitempty"`
	FromPort string `json:"fromPort,omitempty"`
	ToPort   string `json:"toPort,omitempty"`
}

// Step is one presentation step: what it shows and hides.
type Step struct {
	Title string   `json:"title"`
	Show  []string `json:"show"`
	Hide  []string `json:"hide"`
}

// Error is a board that parses as YAML but breaks a rule of the format. Its text is shown to the user as is.
type Error struct{ Msg string }

func (e *Error) Error() string { return e.Msg }
