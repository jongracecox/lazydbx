package component

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
)

func testTable(t *testing.T) Table {
	t.Helper()
	tbl := NewTable(theme.Default())
	tbl.SetSize(80, 20)
	tbl.SetData(
		[]resource.Column{{Title: "NAME"}, {Title: "UPDATED", Width: 10}},
		[]resource.Row{
			{ID: "beta", Cells: []string{"beta", "3h"}},
			{ID: "alpha", Cells: []string{"alpha", "13d"}},
			{ID: "gamma", Cells: []string{"gamma", "just now"}},
		},
	)
	return tbl
}

func press(tbl Table, keys ...string) Table {
	for _, k := range keys {
		msg := tea.KeyPressMsg{}
		switch k {
		case "enter":
			msg.Code = tea.KeyEnter
		case "esc":
			msg.Code = tea.KeyEscape
		case "left":
			msg.Code = tea.KeyLeft
		case "right":
			msg.Code = tea.KeyRight
		case "space":
			msg.Code = tea.KeySpace
		default:
			msg.Code = rune(k[0])
			msg.Text = k
		}
		tbl, _ = tbl.Update(msg)
	}
	return tbl
}

func ids(tbl Table) []string {
	out := make([]string, 0, tbl.Len())
	for _, r := range tbl.rows {
		out = append(out, r.ID)
	}
	return out
}

func TestSortModeSelectConfirm(t *testing.T) {
	tbl := testTable(t)
	assert.Equal(t, []string{"beta", "alpha", "gamma"}, ids(tbl), "unsorted keeps supplied order")

	tbl = press(tbl, "s")
	assert.True(t, tbl.InSortMode())

	tbl = press(tbl, "s") // select NAME → ascending
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, ids(tbl), "live-applied")

	tbl = press(tbl, "s") // same column again → descending
	assert.Equal(t, []string{"gamma", "beta", "alpha"}, ids(tbl))

	tbl = press(tbl, "enter")
	assert.False(t, tbl.InSortMode())
	assert.Equal(t, []string{"gamma", "beta", "alpha"}, ids(tbl), "confirm keeps the sort")
}

func TestSortModeEnterOnNewColumnAppliesAndConfirms(t *testing.T) {
	tbl := press(testTable(t), "s", "right", "enter") // pick UPDATED, confirm
	assert.False(t, tbl.InSortMode())
	assert.Equal(t, []string{"gamma", "beta", "alpha"}, ids(tbl), "ages sort by duration, not lexically")
}

func TestSortModeEscReverts(t *testing.T) {
	tbl := press(testTable(t), "s", "s") // sort by NAME asc
	tbl = press(tbl, "enter")
	require.Equal(t, []string{"alpha", "beta", "gamma"}, ids(tbl))

	tbl = press(tbl, "s", "right", "s") // start re-sorting by UPDATED
	require.Equal(t, []string{"gamma", "beta", "alpha"}, ids(tbl))
	tbl = press(tbl, "esc")
	assert.False(t, tbl.InSortMode())
	assert.Equal(t, []string{"alpha", "beta", "gamma"}, ids(tbl), "esc restores prior sort")
}

func TestSortSurvivesRefresh(t *testing.T) {
	tbl := press(testTable(t), "s", "s", "enter") // NAME ascending
	tbl.SetData(
		[]resource.Column{{Title: "NAME"}, {Title: "UPDATED", Width: 10}},
		[]resource.Row{
			{ID: "zeta", Cells: []string{"zeta", "1m"}},
			{ID: "alpha", Cells: []string{"alpha", "13d"}},
		},
	)
	assert.Equal(t, []string{"alpha", "zeta"}, ids(tbl), "sort re-applies to fresh poll data")
}

func TestCellLess(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"alpha", "Beta", true},
		{"10", "9", false},
		{"2", "10", true},
		{"just now", "5m", true},
		{"5m", "3h", true},
		{"3h", "13d", true},
		{"anything", "", true},
		{"", "anything", false},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, cellLess(tt.a, tt.b), "%q < %q", tt.a, tt.b)
	}
}
