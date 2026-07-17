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
