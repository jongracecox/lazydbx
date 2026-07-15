package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func tabKey(s string) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: rune(s[0]), Text: s}
}

func newTestTabbed(t *testing.T) *Tabbed {
	t.Helper()
	th := theme.Default()
	return NewTabbed(th, "events", []Tab{
		{Name: "one", View: NewDescribe(th, "one", map[string]string{"payload": "body-one"})},
		{Name: "two", View: NewDescribe(th, "two", map[string]string{"payload": "body-two"})},
		{Name: "three", View: NewDescribe(th, "three", map[string]string{"payload": "body-three"})},
	})
}

func TestTabbedSwitching(t *testing.T) {
	tb := newTestTabbed(t)
	assert.Contains(t, tb.Render(80, 20), "body-one", "first tab active")

	v, _ := tb.Update(tabKey("]"))
	tb = v.(*Tabbed)
	assert.Contains(t, tb.Render(80, 20), "body-two")

	v, _ = tb.Update(tabKey("["))
	tb = v.(*Tabbed)
	assert.Contains(t, tb.Render(80, 20), "body-one")

	// Wraps around backwards.
	v, _ = tb.Update(tabKey("["))
	tb = v.(*Tabbed)
	assert.Contains(t, tb.Render(80, 20), "body-three")
}

func TestTabbedEscPopsWholeView(t *testing.T) {
	tb := newTestTabbed(t)
	v, cmd := tb.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, isPop := cmd().(PopMsg)
	assert.True(t, isPop, "esc from a tab pops the whole tabbed view")
	assert.Same(t, tb, v)
}

func TestTabbedTitleAndHints(t *testing.T) {
	tb := newTestTabbed(t)
	assert.Equal(t, "events", tb.Title())
	require.NotEmpty(t, tb.Hints())
	assert.Equal(t, "[/]", tb.Hints()[0].Help().Key)
}
