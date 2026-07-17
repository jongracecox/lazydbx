package component

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func hint(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

func TestHeaderShowsContextAndBadges(t *testing.T) {
	th := theme.Default()
	out := Header(th, 80, 20, "catalogs", []string{"readonly", "stale"}, nil)

	assert.Contains(t, out, "catalogs", "context is shown")
	assert.Contains(t, out, "[readonly]", "badges are bracketed")
	assert.Contains(t, out, "[stale]")
}

func TestHeaderBannerThreshold(t *testing.T) {
	th := theme.Default()

	// Below threshold: compact name chip stands in for the banner.
	narrow := Header(th, 80, 20, "ctx", nil, nil)
	assert.Contains(t, narrow, "lazydbx", "compact chip carries identity without the banner")
	assert.NotContains(t, narrow, "▄▄", "no banner art when the terminal is small")

	// Above threshold: the ASCII banner appears.
	wide := Header(th, 120, 30, "ctx", nil, nil)
	assert.Contains(t, wide, "▄▄", "banner art shown when there is room")
}

func TestHeaderRendersHints(t *testing.T) {
	th := theme.Default()
	hints := []key.Binding{hint("q", "quit"), hint("r", "refresh")}
	out := Header(th, 80, 20, "ctx", nil, hints)

	assert.Contains(t, out, "quit")
	assert.Contains(t, out, "refresh")
	assert.Contains(t, out, "<q>")
}

func TestRenderHintsSkipsDisabledAndEmpty(t *testing.T) {
	th := theme.Default()

	disabled := hint("x", "hidden")
	disabled.SetEnabled(false)
	noHelp := key.NewBinding(key.WithKeys("z")) // no WithHelp → empty Key

	out := renderHints(th, 80, []key.Binding{hint("q", "quit"), disabled, noHelp})
	assert.Contains(t, out, "quit")
	assert.NotContains(t, out, "hidden", "disabled bindings are not shown")
}

func TestRenderHintsHasFixedHeight(t *testing.T) {
	th := theme.Default()

	// No visible hints still occupies the reserved rows so the body doesn't jump.
	empty := renderHints(th, 80, nil)
	assert.Equal(t, hintRows-1, strings.Count(empty, "\n"),
		"empty hint block still reserves its vertical space")

	// A single hint is padded up to the same height.
	one := renderHints(th, 80, []key.Binding{hint("q", "quit")})
	assert.Equal(t, hintRows-1, strings.Count(one, "\n"))
}

func TestRenderHintsColumnizes(t *testing.T) {
	th := theme.Default()
	// More than hintRows bindings must spill into a second column, i.e. the
	// first column's last row and the second column's first row coexist.
	binds := make([]key.Binding, 0, hintRows+2)
	for i := 0; i < hintRows+2; i++ {
		binds = append(binds, hint(string(rune('a'+i)), "desc"+string(rune('a'+i))))
	}
	out := renderHints(th, 200, binds)
	// The (hintRows+1)-th binding starts the second column, so it appears on
	// the first rendered row alongside the first binding.
	firstRow := strings.Split(out, "\n")[0]
	assert.Contains(t, firstRow, "desca", "column 1 first entry")
	assert.Contains(t, firstRow, "desc"+string(rune('a'+hintRows)), "column 2 first entry")
}

func TestInterleave(t *testing.T) {
	assert.Equal(t, []string{"a", "-", "b", "-", "c"}, interleave([]string{"a", "b", "c"}, "-"))
	assert.Equal(t, []string{"a"}, interleave([]string{"a"}, "-"))
	assert.Empty(t, interleave(nil, "-"))
}

func TestRenderBannerCarriesVersionTag(t *testing.T) {
	th := theme.Default()
	out := renderBanner(th)
	assert.Contains(t, out, "lazydbx", "banner tucks the app name in")
	// The banner keeps its shape (same line count as the raw art).
	require.Equal(t, strings.Count(Banner, "\n"), strings.Count(out, "\n"))
}

func TestHeaderClampsToWidth(t *testing.T) {
	th := theme.Default()
	out := Header(th, 40, 20, "a-very-long-context-name-that-exceeds", nil, nil)
	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, lipgloss.Width(line), 40, "no line exceeds the terminal width")
	}
}
