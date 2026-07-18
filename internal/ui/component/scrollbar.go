package component

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// Scrollbar characters: a solid thumb over a light-shaded track.
const (
	scrollbarThumb = "█"
	scrollbarTrack = "░"
)

// Scrollbar renders a vertical shaded scrollbar `height` rows tall, indicating
// which slice of `total` content lines is visible given a window of `visible`
// lines scrolled to `offset`.
//
// It is a pure formatter over scroll metrics — pass a viewport's
// TotalLineCount / VisibleLineCount / YOffset — so any viewport-backed view
// can drop it in. When everything fits (total <= visible) it renders a column
// of blanks the same height, so layout stays stable without drawing a needless
// bar. The returned string is `height` lines tall and one cell wide.
func Scrollbar(th theme.Theme, height, total, visible, offset int) string {
	if height <= 0 {
		return ""
	}

	lines := make([]string, height)
	if total <= visible || total <= 0 || visible <= 0 {
		for i := range lines {
			lines[i] = " "
		}
		return strings.Join(lines, "\n")
	}

	// Thumb length is proportional to the visible fraction (at least one cell);
	// its position is proportional to how far we've scrolled.
	thumb := min(height, max(1, height*visible/total))
	maxOff := total - visible
	pos := 0
	if maxOff > 0 {
		pos = (height - thumb) * offset / maxOff
	}
	pos = min(max(0, pos), height-thumb)

	track := th.Subtle.Render(scrollbarTrack)
	bar := lipgloss.NewStyle().Foreground(th.Accent).Render(scrollbarThumb)
	for i := range height {
		if i >= pos && i < pos+thumb {
			lines[i] = bar
		} else {
			lines[i] = track
		}
	}
	return strings.Join(lines, "\n")
}
