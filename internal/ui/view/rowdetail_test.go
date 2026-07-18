package view

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/theme"
)

func TestRowDetailShowsMultilineValue(t *testing.T) {
	body := "line one\nline two\nline three"
	rd := NewRowDetail(theme.Default(), "row 1", []string{"id", "body"}, []string{"1", body})

	out := rd.Render(80, 24)
	assert.Contains(t, out, "body", "column name is shown")
	for _, want := range []string{"line one", "line two", "line three"} {
		assert.Contains(t, out, want, "each line of a multi-line value is shown")
	}
}

func TestRowDetailWrapsLongValue(t *testing.T) {
	long := strings.Repeat("word ", 60) // ~300 cols, must wrap into a narrow view
	rd := NewRowDetail(theme.Default(), "row 1", []string{"note"}, []string{long})

	out := rd.Render(40, 24)
	for _, line := range strings.Split(out, "\n") {
		assert.LessOrEqual(t, ansi.StringWidth(line), 40, "no rendered line exceeds the view width")
	}
}

func TestRowDetailScrollbar(t *testing.T) {
	// Many columns into a short view → the content overflows and the bar shows.
	names := make([]string, 40)
	values := make([]string, 40)
	for i := range names {
		names[i] = "col"
		values[i] = "v"
	}
	rd := NewRowDetail(theme.Default(), "row 1", names, values)
	assert.Contains(t, rd.Render(60, 6), "█", "scrollbar shows when the row overflows the view")

	// A couple of short fields into a tall view → no scrollbar.
	small := NewRowDetail(theme.Default(), "row 1", []string{"id", "name"}, []string{"1", "a"})
	assert.NotContains(t, small.Render(60, 24), "█", "no scrollbar when everything fits")
}

func TestRowDetailEscPops(t *testing.T) {
	rd := NewRowDetail(theme.Default(), "row 1", []string{"id"}, []string{"1"})
	_, cmd := rd.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, ok := cmd().(PopMsg)
	assert.True(t, ok, "esc pops back to the grid")
}
