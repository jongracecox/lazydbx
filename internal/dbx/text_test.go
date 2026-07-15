package dbx

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecodeEscapes(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"long escape", `\U0001F511 keys`, "🔑 keys"},
		{"short escape", `café`, "café"},
		{"mixed", `\U0001F4CA data → gold`, "📊 data → gold"},
		{"plain text untouched", "no escapes here", "no escapes here"},
		{"invalid hex passes through", `\Uzzzzzzzz`, `\Uzzzzzzzz`},
		{"truncated escape passes through", `\u00`, `\u00`},
		{"invalid rune passes through", `\UFFFFFFFF`, `\UFFFFFFFF`},
		{"newline escapes", `line1\nline2`, "line1\nline2"},
		{"tab escape", `a\tb`, "a\tb"},
		{"crlf collapses", `a\r\nb`, "a\nb"},
		{
			"multi-line markdown comment",
			`# \U0001F947 Gold Accounts\n\n## Details\n\n • **Table Type**: \U0001F4D0 Dimension Table`,
			"# 🥇 Gold Accounts\n\n## Details\n\n • **Table Type**: 📐 Dimension Table",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DecodeEscapes(tt.in))
		})
	}
}

func TestOneLine(t *testing.T) {
	assert.Equal(t, "plain stays", OneLine("plain stays"))
	assert.Equal(t, "# 🥇 Gold Accounts ## Details", OneLine("# 🥇 Gold Accounts\n\n## Details"))
	assert.Equal(t, "a b c", OneLine("a\tb\r\nc"))
	assert.Empty(t, OneLine("\n\n"))
}
