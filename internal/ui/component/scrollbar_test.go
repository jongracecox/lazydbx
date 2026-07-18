package component

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// glyphs strips styling and returns the bar as one rune per visible line.
func glyphs(t *testing.T, s string) []string {
	t.Helper()
	lines := strings.Split(s, "\n")
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = ansi.Strip(l)
	}
	return out
}

func TestScrollbar(t *testing.T) {
	th := theme.Default()

	tests := []struct {
		name                   string
		height, total, visible int
		offset                 int
		want                   []string
	}{
		{"zero height", 0, 10, 4, 0, nil},
		{"fits renders blanks", 4, 3, 4, 0, []string{" ", " ", " ", " "}},
		{"top of overflow", 4, 8, 4, 0, []string{"█", "█", "░", "░"}},
		{"bottom of overflow", 4, 8, 4, 4, []string{"░", "░", "█", "█"}},
		{"middle of overflow", 6, 12, 3, 6, []string{"░", "░", "░", "█", "░", "░"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Scrollbar(th, tt.height, tt.total, tt.visible, tt.offset)
			if tt.height <= 0 {
				assert.Empty(t, got)
				return
			}
			g := glyphs(t, got)
			require.Len(t, g, tt.height, "one line per row")
			assert.Equal(t, tt.want, g)
		})
	}
}

func TestScrollbarThumbNeverEmpty(t *testing.T) {
	// Even with a huge content-to-window ratio, the thumb is at least one cell.
	g := glyphs(t, Scrollbar(theme.Default(), 5, 100000, 1, 0))
	assert.Contains(t, g, "█", "thumb is always visible")
}
