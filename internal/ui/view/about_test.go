package view

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/version"
)

func TestAboutRenderShowsMetadata(t *testing.T) {
	out := NewAbout(theme.Default()).Render(80, 24)
	for _, want := range []string{version.Version, projectURL, "MIT", "Jon Grace-Cox"} {
		assert.Contains(t, out, want)
	}
}

func TestAboutClosesOnKeys(t *testing.T) {
	cases := map[string]tea.KeyPressMsg{
		"esc": {Code: tea.KeyEscape},
		"a":   {Code: 'a', Text: "a"},
		"q":   {Code: 'q', Text: "q"},
	}
	for name, msg := range cases {
		a := NewAbout(theme.Default())
		_, cmd := a.Update(msg)
		require.NotNil(t, cmd, "key %q should emit a command", name)
		_, ok := cmd().(PopMsg)
		assert.True(t, ok, "key %q should pop", name)
	}
}
