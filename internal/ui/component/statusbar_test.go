package component

import (
	"testing"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func TestStatusBarRendersLeftAndRight(t *testing.T) {
	th := theme.Default()
	now := time.Now()
	out := StatusBar{}.Render(th, 80, "left-crumbs", "right-status", now)

	assert.Contains(t, out, "left-crumbs")
	assert.Contains(t, out, "right-status")
	assert.LessOrEqual(t, lipgloss.Width(out), 80)
}

func TestStatusBarFlashOverridesRight(t *testing.T) {
	th := theme.Default()
	base := time.Now()

	var s StatusBar
	s.Flash(FlashError, "boom", base)

	// While active, the flash replaces the supplied right status.
	live := s.Render(th, 80, "crumbs", "freshness", base.Add(time.Second))
	assert.Contains(t, live, "boom")
	assert.NotContains(t, live, "freshness", "an active flash hides the view status")

	// After it expires, the right status returns.
	expired := s.Render(th, 80, "crumbs", "freshness", base.Add(flashDuration+time.Second))
	assert.Contains(t, expired, "freshness")
	assert.NotContains(t, expired, "boom")
}

func TestStatusBarFlashLevels(t *testing.T) {
	th := theme.Default()
	base := time.Now()
	at := base.Add(time.Second)

	for _, level := range []FlashLevel{FlashInfo, FlashWarn, FlashError} {
		var s StatusBar
		s.Flash(level, "msg", base)
		assert.Contains(t, s.Render(th, 80, "l", "r", at), "msg")
	}
}

func TestStatusBarClampsWhenTooNarrow(t *testing.T) {
	th := theme.Default()
	// left+right already exceed width: still renders with a single-space gap
	// and no negative repeat panic.
	out := StatusBar{}.Render(th, 10, "leftleftleft", "rightright", time.Now())
	assert.NotEmpty(t, out)
	assert.NotContains(t, out, "\n", "status bar stays one line")
}

func TestBreadcrumbs(t *testing.T) {
	th := theme.Default()

	out := Breadcrumbs(th, []string{"catalogs", "main", "silver"})
	assert.Contains(t, out, "<catalogs>")
	assert.Contains(t, out, "<main>")
	assert.Contains(t, out, "<silver>")

	// The active (last) segment is accented differently from the rest.
	active := th.KeyHint.Render("<silver>")
	assert.Contains(t, out, active, "trailing crumb uses the accent style")

	assert.Empty(t, Breadcrumbs(th, nil))
}

// TestStatusBarLeavesLastColumnBlank pins the wrap fix: the bar is the last
// line of the frame, so writing its final cell parks the cursor in the
// terminal's pending-wrap state and terminals that mishandle that scroll the
// whole frame. The freshness timer exposed it — Bubble Tea only repaints that
// cell when the line shifts, which is what "9s ago" → "10s ago" does.
func TestStatusBarLeavesLastColumnBlank(t *testing.T) {
	th := theme.Default()
	now := time.Now()
	const width = 40

	for _, right := range []string{
		"", "3/3  ⟳ 9s ago", "3/3  ⟳ 10s ago", "3/3  ⟳ 16m40s ago",
		"a right status long enough to need clamping at this width",
	} {
		out := StatusBar{}.Render(th, width, "<profiles> <catalogs>", right, now)
		assert.Less(t, lipgloss.Width(out), width,
			"never paint the final column (right=%q)", right)
	}

	// Degenerate widths must not produce an empty or negative-width render.
	for _, w := range []int{0, 1, 2} {
		assert.NotPanics(t, func() { StatusBar{}.Render(th, w, "l", "r", now) })
	}
}
