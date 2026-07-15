package dbx

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// DecodeEscapes turns literal escape sequences (as often pasted into
// Databricks comments from Python sources) into their characters:
// \uXXXX and \UXXXXXXXX become runes ("\U0001F511 keys" → "🔑 keys"),
// and \n, \t, \r become real whitespace so multi-line comments render as
// multi-line text. Invalid sequences pass through unchanged.
func DecodeEscapes(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			switch s[i+1] {
			case 'n':
				b.WriteByte('\n')
				i += 2
				continue
			case 't':
				b.WriteByte('\t')
				i += 2
				continue
			case 'r':
				i += 2 // discard: \r\n collapses to \n, lone \r vanishes
				continue
			case 'u', 'U':
				width := 4
				if s[i+1] == 'U' {
					width = 8
				}
				if i+2+width <= len(s) {
					if v, err := strconv.ParseUint(s[i+2:i+2+width], 16, 32); err == nil && utf8.ValidRune(rune(v)) {
						b.WriteRune(rune(v))
						i += 2 + width
						continue
					}
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}

// OneLine flattens a (possibly multi-line) string for table-cell display:
// all whitespace runs, including newlines, collapse to single spaces.
func OneLine(s string) string {
	if !strings.ContainsAny(s, "\n\t\r") {
		return s
	}
	return strings.Join(strings.Fields(s), " ")
}
