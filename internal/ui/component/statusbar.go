package component

import (
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// FlashLevel classifies a status bar flash. It lives here (not in view) so
// both packages can use it without an import cycle.
type FlashLevel int

// Flash levels.
const (
	FlashInfo FlashLevel = iota
	FlashWarn
	FlashError
)

// flashDuration is how long a flash message stays visible.
const flashDuration = 5 * time.Second

// StatusBar renders the bottom line: breadcrumbs on the left, transient
// flash or view status on the right.
type StatusBar struct {
	level FlashLevel
	text  string
	until time.Time
}

// Flash shows a message until it expires.
func (s *StatusBar) Flash(level FlashLevel, text string, now time.Time) {
	s.level = level
	s.text = text
	s.until = now.Add(flashDuration)
}

// Render draws the bar. left is the breadcrumb trail; right is the top
// view's status (freshness, counts) which a live flash overrides.
func (s StatusBar) Render(th theme.Theme, width int, left, right string, now time.Time) string {
	if now.Before(s.until) {
		style := th.Subtle
		switch s.level {
		case FlashWarn:
			style = th.Warning
		case FlashError:
			style = th.Error
		case FlashInfo:
		}
		right = style.Render(s.text)
	}

	// Reserve the final column. This is the last line of the frame, so its
	// last cell is the terminal's bottom-right corner; painting it leaves the
	// cursor in the pending-wrap state, and terminals that mishandle that
	// wrap the line and scroll the whole frame up by one. Bubble Tea only
	// repaints that cell when the line's content shifts — which is exactly
	// what the freshness timer does when it grows a digit ("9s" → "10s"), so
	// the corruption looks like the timer breaking the layout. One blank
	// column costs nothing and makes the bar wrap-proof.
	avail := max(1, width-1)

	gap := avail - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	line := left + strings.Repeat(" ", gap) + right
	return lipgloss.NewStyle().MaxWidth(avail).Render(line)
}

// Breadcrumbs renders the navigation trail like `<catalogs> <main>` with the
// active segment accented.
func Breadcrumbs(th theme.Theme, titles []string) string {
	parts := make([]string, len(titles))
	for i, t := range titles {
		seg := "<" + t + ">"
		if i == len(titles)-1 {
			parts[i] = th.KeyHint.Render(seg)
		} else {
			parts[i] = th.Subtle.Render(seg)
		}
	}
	return strings.Join(parts, " ")
}
