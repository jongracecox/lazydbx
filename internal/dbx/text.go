package dbx

import (
	"strconv"
	"strings"
	"unicode/utf8"
)

// DecodeEscapes turns literal \uXXXX and \UXXXXXXXX sequences (as often
// pasted into Databricks comments from Python sources) into their runes, so
// "\U0001F511 keys" renders as "🔑 keys" in the terminal. Invalid sequences
// pass through unchanged.
func DecodeEscapes(s string) string {
	if !strings.Contains(s, `\u`) && !strings.Contains(s, `\U`) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] == '\\' && i+1 < len(s) {
			var width int
			switch s[i+1] {
			case 'u':
				width = 4
			case 'U':
				width = 8
			}
			if width > 0 && i+2+width <= len(s) {
				if v, err := strconv.ParseUint(s[i+2:i+2+width], 16, 32); err == nil && utf8.ValidRune(rune(v)) {
					b.WriteRune(rune(v))
					i += 2 + width
					continue
				}
			}
		}
		b.WriteByte(s[i])
		i++
	}
	return b.String()
}
