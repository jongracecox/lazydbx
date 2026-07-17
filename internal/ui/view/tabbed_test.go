package view

import (
	"testing"

	"charm.land/bubbles/v2/key"
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
	}, 0)
}

func TestTabbedInitialActive(t *testing.T) {
	th := theme.Default()
	tabs := []Tab{
		{Name: "one", View: NewDescribe(th, "one", map[string]string{"payload": "body-one"})},
		{Name: "two", View: NewDescribe(th, "two", map[string]string{"payload": "body-two"})},
	}
	assert.Contains(t, NewTabbed(th, "t", tabs, 1).Render(80, 20), "body-two", "opens the requested tab")
	assert.Contains(t, NewTabbed(th, "t", tabs, 9).Render(80, 20), "body-one", "out-of-range clamps to first")
	assert.Contains(t, NewTabbed(th, "t", tabs, -1).Render(80, 20), "body-one", "negative clamps to first")
}

// cyclerView is a fake tab body with two internal focus stops (0 and 1) that
// participate in the tab cycle via TabCycler, standing in for the SQL view's
// editor/results split.
type cyclerView struct {
	body  string
	focus int
}

func (c *cyclerView) Init() tea.Cmd                  { return nil }
func (c *cyclerView) Update(tea.Msg) (View, tea.Cmd) { return c, nil }
func (c *cyclerView) Render(_, _ int) string         { return c.body }
func (c *cyclerView) Title() string                  { return c.body }
func (c *cyclerView) Hints() []key.Binding           { return nil }
func (c *cyclerView) Close()                         {}

func (c *cyclerView) AdvanceFocus(forward bool) bool {
	if forward && c.focus == 0 {
		c.focus = 1
		return true
	}
	if !forward && c.focus == 1 {
		c.focus = 0
		return true
	}
	return false
}

func (c *cyclerView) EnterFocus(forward bool) {
	if forward {
		c.focus = 0
	} else {
		c.focus = 1
	}
}

func TestTabbedCycleThroughInternalStops(t *testing.T) {
	th := theme.Default()
	mid := &cyclerView{body: "mid"}
	tb := NewTabbed(th, "t", []Tab{
		{Name: "one", View: NewDescribe(th, "one", map[string]string{"payload": "body-one"})},
		{Name: "mid", View: mid},
		{Name: "three", View: NewDescribe(th, "three", map[string]string{"payload": "body-three"})},
	}, 0)

	tab := func() { v, _ := tb.Update(tabKey("tab")); tb = v.(*Tabbed) }
	shiftTab := func() { v, _ := tb.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}); tb = v.(*Tabbed) }

	// Forward: one -> mid(stop 0) -> mid(stop 1) -> three.
	tab()
	assert.Equal(t, 1, tb.active)
	assert.Equal(t, 0, mid.focus, "entering mid forward lands on the first stop")
	tab()
	assert.Equal(t, 1, tb.active, "second stop stays on the same tab")
	assert.Equal(t, 1, mid.focus)
	tab()
	assert.Equal(t, 2, tb.active, "boundary reached, advance to the next tab")

	// Backward: three -> mid(stop 1) -> mid(stop 0) -> one.
	shiftTab()
	assert.Equal(t, 1, tb.active)
	assert.Equal(t, 1, mid.focus, "entering mid backward lands on the last stop")
	shiftTab()
	assert.Equal(t, 1, tb.active)
	assert.Equal(t, 0, mid.focus)
	shiftTab()
	assert.Equal(t, 0, tb.active, "boundary reached, retreat to the previous tab")
}

func TestTabbedBracketsSkipInternalStops(t *testing.T) {
	th := theme.Default()
	mid := &cyclerView{body: "mid", focus: 1}
	tb := NewTabbed(th, "t", []Tab{
		{Name: "one", View: NewDescribe(th, "one", map[string]string{"payload": "body-one"})},
		{Name: "mid", View: mid},
	}, 0)

	// `]` jumps to the whole tab without touching its internal focus.
	v, _ := tb.Update(tabKey("]"))
	tb = v.(*Tabbed)
	assert.Equal(t, 1, tb.active)
	assert.Equal(t, 1, mid.focus, "brackets don't reset internal focus")
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
