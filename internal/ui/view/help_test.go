package view

import (
	"strings"
	"testing"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func bind(k, desc string) key.Binding {
	return key.NewBinding(key.WithKeys(k), key.WithHelp(k, desc))
}

func testHelp() *Help {
	return NewHelp(theme.Default(), []HelpSection{
		{Title: "Global", Bindings: []key.Binding{bind("q", "quit"), bind("?", "help")}},
		{Title: "Resources", Lines: []string{"catalogs", "jobs"}},
	})
}

func TestHelpRenderShowsSectionsBindingsAndLines(t *testing.T) {
	out := testHelp().Render(80, 40)

	assert.Contains(t, out, "Global")
	assert.Contains(t, out, "<q>")
	assert.Contains(t, out, "quit")
	assert.Contains(t, out, "Resources")
	assert.Contains(t, out, "catalogs", "plain Lines are rendered")
	assert.Contains(t, out, "jobs")
}

func TestHelpRenderClampsToWidth(t *testing.T) {
	h := NewHelp(theme.Default(), []HelpSection{
		{Title: "S", Lines: []string{strings.Repeat("x", 200)}},
	})
	out := h.Render(20, 40)
	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, len(line), 20)
	}
}

func TestHelpRenderZeroWidthUnclamped(t *testing.T) {
	// width == 0 means "don't clamp" — long lines survive intact.
	long := strings.Repeat("y", 100)
	h := NewHelp(theme.Default(), []HelpSection{{Title: "S", Lines: []string{long}}})
	assert.Contains(t, h.Render(0, 40), long)
}

func TestHelpUpdatePopsOnDismissKeys(t *testing.T) {
	for _, code := range []struct {
		name string
		msg  tea.KeyPressMsg
	}{
		{"esc", tea.KeyPressMsg{Code: tea.KeyEscape}},
		{"?", tea.KeyPressMsg{Code: '?', Text: "?"}},
		{"q", tea.KeyPressMsg{Code: 'q', Text: "q"}},
	} {
		t.Run(code.name, func(t *testing.T) {
			_, cmd := testHelp().Update(code.msg)
			require.NotNil(t, cmd, "dismiss key returns a command")
			_, ok := cmd().(PopMsg)
			assert.True(t, ok, "dismiss emits PopMsg")
		})
	}
}

func TestHelpUpdateIgnoresOtherKeys(t *testing.T) {
	_, cmd := testHelp().Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.Nil(t, cmd, "unrelated keys do nothing")
}

func TestHelpViewInterface(t *testing.T) {
	h := testHelp()
	assert.Equal(t, "help", h.Title())
	assert.Nil(t, h.Hints())
	assert.Nil(t, h.Init())
	h.Close() // must not panic
}
