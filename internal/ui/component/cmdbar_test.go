package component

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// typeText feeds each rune of s to the bar as a key press.
func typeCmd(c CmdBar, s string) CmdBar {
	for _, r := range s {
		c, _, _ = c.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
	return c
}

func keyMsg(code rune, text string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code, Text: text}
}

// allResources filters the fixed resource set by prefix, mirroring how the
// real registry completer behaves.
func allResources(prefix string) []string {
	out := []string{}
	for _, r := range []string{"apps", "catalogs", "jobs"} {
		if strings.HasPrefix(r, prefix) {
			out = append(out, r)
		}
	}
	return out
}

func TestCmdBarOpenListsEverything(t *testing.T) {
	c := NewCmdBar(allResources)
	c, cmd := c.Open()
	assert.NotNil(t, cmd, "Open focuses the input")
	assert.Equal(t, []string{"apps", "catalogs", "jobs"}, c.matches, "empty prompt lists all")
}

func TestCmdBarTabCyclesCompletions(t *testing.T) {
	c := NewCmdBar(allResources)
	c, _ = c.Open()

	tab := keyMsg(tea.KeyTab, "")
	tab.Code = tea.KeyTab

	c, ev, _ := c.Update(tab)
	assert.Equal(t, EventNone, ev.Kind)
	assert.Equal(t, "catalogs", c.input.Value(), "tab adopts the next completion")

	c, _, _ = c.Update(tab)
	assert.Equal(t, "jobs", c.input.Value())

	c, _, _ = c.Update(tab)
	assert.Equal(t, "apps", c.input.Value(), "tab wraps around")
}

func TestCmdBarEnterAdoptsHighlightedCompletion(t *testing.T) {
	c := NewCmdBar(allResources)
	c, _ = c.Open()
	c = typeCmd(c, "c") // matches "catalogs", highlighted

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, ev, _ := c.Update(enter)
	require.Equal(t, EventSubmit, ev.Kind)
	assert.Equal(t, "catalogs", ev.Value, "bare single-word Enter completes to the highlight")
}

func TestCmdBarEnterKeepsMultiWordValue(t *testing.T) {
	c := NewCmdBar(allResources)
	c, _ = c.Open()
	c = typeCmd(c, "tables main.silver")

	enter := tea.KeyPressMsg{Code: tea.KeyEnter}
	_, ev, _ := c.Update(enter)
	require.Equal(t, EventSubmit, ev.Kind)
	assert.Equal(t, "tables main.silver", ev.Value, "a value with args is submitted verbatim")
}

func TestCmdBarEscCancels(t *testing.T) {
	c := NewCmdBar(allResources)
	c, _ = c.Open()
	_, ev, _ := c.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, EventCancel, ev.Kind)
}

func TestCmdBarNoCompletionsAfterSpace(t *testing.T) {
	c := NewCmdBar(allResources)
	c, _ = c.Open()
	c = typeCmd(c, "jobs ")
	assert.Nil(t, c.matches, "completion only applies to the first word")
}

func TestCmdBarView(t *testing.T) {
	th := theme.Default()
	c := NewCmdBar(allResources)
	c, _ = c.Open()
	out := c.View(th, 80)
	assert.Contains(t, out, "apps")
	assert.Contains(t, out, "catalogs")
}

func TestFilterBarLifecycle(t *testing.T) {
	f := NewFilterBar()
	f, cmd := f.Open("seed")
	assert.NotNil(t, cmd)
	assert.Equal(t, "seed", f.input.Value(), "opens seeded with the current filter")

	// Editing fires EventChanged live.
	f, ev, _ := f.Update(keyMsg('x', "x"))
	assert.Equal(t, EventChanged, ev.Kind)
	assert.Equal(t, "seedx", ev.Value)

	// Enter submits.
	_, ev, _ = f.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	assert.Equal(t, EventSubmit, ev.Kind)
	assert.Equal(t, "seedx", ev.Value)
}

func TestFilterBarEscRestoresPrev(t *testing.T) {
	f := NewFilterBar()
	f, _ = f.Open("original")
	f = typeCmd2(f, "zzz")

	_, ev, _ := f.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Equal(t, EventCancel, ev.Kind)
	assert.Equal(t, "original", ev.Value, "cancel restores the pre-edit filter")
}

func TestFilterBarView(t *testing.T) {
	th := theme.Default()
	f := NewFilterBar()
	f, _ = f.Open("q")
	assert.NotEmpty(t, f.View(th, 80))
	// Width <= 0 returns the raw string without clamping.
	assert.NotEmpty(t, f.View(th, 0))
}

func typeCmd2(f FilterBar, s string) FilterBar {
	for _, r := range s {
		f, _, _ = f.Update(keyMsg(r, string(r)))
	}
	return f
}
