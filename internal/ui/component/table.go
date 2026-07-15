// Package component holds the dumb, reusable widgets composed by the app
// shell and views. Components never do I/O and never import the SDK, the
// engine, or resource defs' packages — they render what they're given.
package component

import (
	btable "charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
)

// wideCutoff is the terminal width at which Wide columns become visible.
const wideCutoff = 110

// Table wraps the bubbles table with resource-aware column sizing and
// cursor preservation across data refreshes.
type Table struct {
	tbl    btable.Model
	rows   []resource.Row
	cols   []resource.Column
	width  int
	height int
}

// NewTable builds a themed table.
func NewTable(th theme.Theme) Table {
	styles := btable.DefaultStyles()
	styles.Header = styles.Header.Foreground(th.Accent).Bold(true)
	styles.Selected = lipgloss.NewStyle().Reverse(true).Bold(true)

	tbl := btable.New(btable.WithFocused(true), btable.WithStyles(styles))
	return Table{tbl: tbl}
}

// SetSize resizes the table region. Reflow only happens on actual size
// changes — this is called every render.
func (t *Table) SetSize(width, height int) {
	if width == t.width && height == t.height {
		return
	}
	t.width, t.height = width, height
	t.tbl.SetWidth(width)
	t.tbl.SetHeight(height)
	t.reflow()
}

// SetData replaces columns and rows, keeping the cursor on the same row ID
// across refreshes so background polls don't yank the selection.
func (t *Table) SetData(cols []resource.Column, rows []resource.Row) {
	selectedID := t.SelectedID()
	t.cols, t.rows = cols, rows
	t.reflow()

	if selectedID != "" {
		for i, r := range rows {
			if r.ID == selectedID {
				t.tbl.SetCursor(i)
				return
			}
		}
	}
}

// reflow recomputes visible columns and widths for the current size.
func (t *Table) reflow() {
	if len(t.cols) == 0 {
		return
	}
	visible := make([]resource.Column, 0, len(t.cols))
	visIdx := make([]int, 0, len(t.cols))
	for i, c := range t.cols {
		if c.Wide && t.width < wideCutoff {
			continue
		}
		visible = append(visible, c)
		visIdx = append(visIdx, i)
	}

	// Fixed columns take their width; flex columns (Width 0) share the rest.
	const pad = 2 // bubbles table cell padding
	remaining := t.width
	flexCount := 0
	for _, c := range visible {
		if c.Width > 0 {
			remaining -= c.Width + pad
		} else {
			flexCount++
		}
	}
	flexWidth := 20
	if flexCount > 0 {
		flexWidth = max(10, remaining/flexCount-pad)
	}

	bcols := make([]btable.Column, len(visible))
	for i, c := range visible {
		w := c.Width
		if w == 0 {
			w = flexWidth
		}
		bcols[i] = btable.Column{Title: c.Title, Width: w}
	}
	brows := make([]btable.Row, len(t.rows))
	for i, r := range t.rows {
		cells := make([]string, len(visIdx))
		for j, src := range visIdx {
			if src < len(r.Cells) {
				cells[j] = r.Cells[src]
			}
		}
		brows[i] = cells
	}
	// Clearing rows while the column count changes avoids ragged data, but
	// it also clamps the bubbles cursor to -1 — so capture and restore it.
	cur := t.tbl.Cursor()
	t.tbl.SetRows(nil)
	t.tbl.SetColumns(bcols)
	t.tbl.SetRows(brows)
	if len(brows) > 0 {
		t.tbl.SetCursor(min(max(cur, 0), len(brows)-1))
	}
}

// Update forwards navigation keys to the underlying table.
func (t Table) Update(msg tea.Msg) (Table, tea.Cmd) {
	var cmd tea.Cmd
	t.tbl, cmd = t.tbl.Update(msg)
	return t, cmd
}

// View renders the table.
func (t Table) View() string { return t.tbl.View() }

// SelectedRow returns the row under the cursor.
func (t Table) SelectedRow() (resource.Row, bool) {
	i := t.tbl.Cursor()
	if i < 0 || i >= len(t.rows) {
		return resource.Row{}, false
	}
	return t.rows[i], true
}

// SelectedID returns the ID of the row under the cursor, or "".
func (t Table) SelectedID() string {
	if r, ok := t.SelectedRow(); ok {
		return r.ID
	}
	return ""
}

// Len is the number of rows currently displayed.
func (t Table) Len() int { return len(t.rows) }
