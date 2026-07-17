package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/theme"
)

func keyPress(code rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: code}
}

func TestPickerSelectsProfile(t *testing.T) {
	profiles := []dbx.Profile{
		{Name: "alpha", Host: "https://a.cloud.databricks.com"},
		{Name: "beta", Host: "https://b.cloud.databricks.com"},
	}
	p := NewPicker(theme.Default(), profiles)
	p.Render(100, 20) // size the table so a row is selectable

	v, cmd := p.Update(keyPress(tea.KeyEnter))
	require.NotNil(t, cmd, "enter must produce a selection command")

	msg := cmd()
	sel, ok := msg.(ProfileSelectedMsg)
	require.True(t, ok, "expected ProfileSelectedMsg, got %T", msg)
	assert.Equal(t, "alpha", sel.Profile.Name)
	assert.Same(t, p, v)
}

func TestPickerOpensColorPicker(t *testing.T) {
	profiles := []dbx.Profile{{Name: "alpha", Host: "https://a.cloud.databricks.com"}}
	p := NewPicker(theme.Default(), profiles)
	p.Render(100, 20)

	_, cmd := p.Update(keyPress('c'))
	require.NotNil(t, cmd, "c must open the color picker for the selected profile")
	msg, ok := cmd().(OpenColorPickerMsg)
	require.True(t, ok, "expected OpenColorPickerMsg, got %T", cmd())
	assert.Equal(t, "alpha", msg.Profile)
}

func TestPickerMovesCursor(t *testing.T) {
	profiles := []dbx.Profile{
		{Name: "alpha", Host: "https://a.cloud.databricks.com"},
		{Name: "beta", Host: "https://b.cloud.databricks.com"},
	}
	p := NewPicker(theme.Default(), profiles)
	p.Render(100, 20)

	_, _ = p.Update(keyPress('j'))
	_, cmd := p.Update(keyPress(tea.KeyEnter))
	require.NotNil(t, cmd)
	sel := cmd().(ProfileSelectedMsg)
	assert.Equal(t, "beta", sel.Profile.Name)
}
