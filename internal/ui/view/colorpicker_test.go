package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func TestColorPickerPreselectsCurrent(t *testing.T) {
	c := NewColorPicker(theme.Default(), "prod", "red")
	assert.Equal(t, "red", c.options[c.cursor], "cursor starts on the configured color")

	none := NewColorPicker(theme.Default(), "prod", "")
	assert.Equal(t, noColor, none.options[none.cursor], "no color → cursor on 'none'")
}

func TestColorPickerSelectsColor(t *testing.T) {
	c := NewColorPicker(theme.Default(), "prod", "")
	// Move off "none" onto the first real color and apply.
	_, _ = c.Update(keyPress('j'))
	_, cmd := c.Update(keyPress(tea.KeyEnter))
	require.NotNil(t, cmd)
	msg, ok := cmd().(ProfileColorSelectedMsg)
	require.True(t, ok, "expected ProfileColorSelectedMsg, got %T", cmd())
	assert.Equal(t, "prod", msg.Profile)
	assert.Equal(t, theme.AccentNames()[0], msg.Color, "applies the highlighted color")
}

func TestColorPickerNoneClears(t *testing.T) {
	c := NewColorPicker(theme.Default(), "prod", "red")
	// Jump the cursor to "none" (top) and apply.
	for range c.options {
		_, _ = c.Update(keyPress('k'))
	}
	_, cmd := c.Update(keyPress(tea.KeyEnter))
	require.NotNil(t, cmd)
	msg := cmd().(ProfileColorSelectedMsg)
	assert.Empty(t, msg.Color, "'none' clears the highlight")
}

func TestColorPickerEscPops(t *testing.T) {
	c := NewColorPicker(theme.Default(), "prod", "")
	_, cmd := c.Update(keyPress(tea.KeyEscape))
	require.NotNil(t, cmd)
	_, ok := cmd().(PopMsg)
	assert.True(t, ok)
}
