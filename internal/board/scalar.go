package board

import (
	"strconv"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"
)

// FormatScalar writes a string as a YAML scalar: plain when that reads back as the same string, quoted otherwise.
func FormatScalar(val string, inFlow bool) string { return formatScalar(val, 0, inFlow) }

// formatScalar is FormatScalar for a value that replaces one written in style: quoted stays quoted the same way.
func formatScalar(val string, style yaml.Style, inFlow bool) string {
	// single quotes cannot carry line breaks or control characters
	single := !strings.ContainsFunc(val, func(r rune) bool { return r != '\t' && unicode.IsControl(r) })
	quoted := "'" + strings.ReplaceAll(val, "'", "''") + "'"

	switch {
	case style&yaml.SingleQuotedStyle != 0 && single:
		return quoted
	case style&(yaml.SingleQuotedStyle|yaml.DoubleQuotedStyle) != 0:
	case plainOK(val, inFlow):
		return val
	case single && strings.Contains(val, `"`) && !strings.Contains(val, "'"):
		return quoted
	}

	// Go's escapes (\n, \t, \x01, \u2028, ...) are all valid in a YAML double-quoted scalar
	return strconv.Quote(val)
}

// plainOK reports whether val can be written unquoted as a map value (in a flow map when inFlow) and still read back
// as the same string.
func plainOK(val string, inFlow bool) bool {
	if val == "" || strings.ContainsAny(val, "\n\r") || strings.HasPrefix(val, ":") {
		return false
	}

	src := "k: " + val
	if inFlow {
		src = "{ k: " + val + " }"
	}

	doc, err := parseYAML(src)
	root := rootOf(doc)

	if err != nil || root == nil || root.Kind != yaml.MappingNode || len(root.Content) != 2 {
		return false
	}

	v := root.Content[1]

	return v.Kind == yaml.ScalarNode && v.Style == 0 && v.ShortTag() == "!!str" && v.Value == val
}
