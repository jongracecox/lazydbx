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
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, DecodeEscapes(tt.in))
		})
	}
}
