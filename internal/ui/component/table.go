// Package component holds the dumb, reusable widgets composed by the app
// shell and views. Components never do I/O and never import the SDK, the
// engine, or resource defs' packages — they render what they're given.
package component

import (
	"sort"

	btable "charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
)

// wideCutoff is the terminal width at which Wide columns become visible.
const wideCutoff = 110

// Table wraps the bubbles table with resource-aware column sizing, cursor
// preservation across data refreshes, and interactive column sorting:
// `s` enters sort mode, ←/→ pick a column, `s`/space select it (selecting
// the sorted column again reverses direction — applied live), enter
// confirms, esc reverts.
type Table struct {
	th       theme.Theme
	tbl      btable.Model
	baseRows []resource.Row // as supplied (post-filter), original order
	rows     []resource.Row // baseRows through the active sort
	cols     []resource.Column
	visIdx   []int // visible column -> t.cols index
	width    int
	height   int

	// cellStyler, when set, classifies each cell by its original column index
	// and raw value; the class maps to a theme style applied at render time
	// only (raw Row.Cells stay unstyled so filter/sort operate on plain text).
	cellStyler func(col int, value string) resource.CellClass

	sortCol int // index into t.cols; -1 = no sort
	sortAsc bool

	sortMode  bool
	highlight int // visible-column index under the picker
	prevCol   int // sort state to restore on esc
	prevAsc   bool
}

// NewTable builds a themed table.
func NewTable(th theme.Theme) Table {
	styles := btable.DefaultStyles()
	styles.Header = styles.Header.Foreground(th.Accent).Bold(true)
	styles.Selected = lipgloss.NewStyle().Reverse(true).Bold(true)

	tbl := btable.New(btable.WithFocused(true), btable.WithStyles(styles))
	return Table{th: th, tbl: tbl, sortCol: -1}
}

// SetCellStyler installs a semantic classifier for cell values; nil disables
// styling. fn receives the ORIGINAL column index (into Columns()) and the raw
// value, and returns a CellClass mapped to a theme style at render time. The
// underlying Row.Cells are never mutated.
func (t *Table) SetCellStyler(fn func(col int, value string) resource.CellClass) {
	t.cellStyler = fn
	t.reflow()
}

// classStyle maps a CellClass to its theme style. The bool is false for
// CellDefault (and any unknown class), meaning "render the value unstyled".
func (t *Table) classStyle(c resource.CellClass) (lipgloss.Style, bool) {
	switch c {
	case resource.CellGood:
		return t.th.Success, true
	case resource.CellBad:
		return t.th.Error, true
	case resource.CellWarn:
		return t.th.Warning, true
	case resource.CellRunning:
		return t.th.KeyHint, true
	case resource.CellDefault:
		return lipgloss.Style{}, false
	}
	return lipgloss.Style{}, false
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
// across refreshes so background polls don't yank the selection. The active
// sort is re-applied to fresh data.
func (t *Table) SetData(cols []resource.Column, rows []resource.Row) {
	selectedID := t.SelectedID()
	t.cols, t.baseRows = cols, rows
	t.applySort()
	t.reflow()

	if selectedID != "" {
		for i, r := range t.rows {
			if r.ID == selectedID {
				t.tbl.SetCursor(i)
				return
			}
		}
	}
}

// InSortMode reports whether the column picker is active; while true the
// owning view should route all key input to the table.
func (t Table) InSortMode() bool { return t.sortMode }

// applySort orders rows by the sort column; no sort keeps supplied order.
func (t *Table) applySort() {
	if t.sortCol < 0 || t.sortCol >= len(t.cols) {
		t.rows = t.baseRows
		return
	}
	t.rows = make([]resource.Row, len(t.baseRows))
	copy(t.rows, t.baseRows)
	col, asc := t.sortCol, t.sortAsc
	sort.SliceStable(t.rows, func(i, j int) bool {
		var a, b string
		if col < len(t.rows[i].Cells) {
			a = t.rows[i].Cells[col]
		}
		if col < len(t.rows[j].Cells) {
			b = t.rows[j].Cells[col]
		}
		if asc {
			return cellLess(a, b)
		}
		return cellLess(b, a)
	})
}

// reflow recomputes visible columns and widths for the current size.
func (t *Table) reflow() {
	if len(t.cols) == 0 {
		return
	}
	visible := make([]resource.Column, 0, len(t.cols))
	t.visIdx = t.visIdx[:0]
	for i, c := range t.cols {
		if c.Wide && t.width < wideCutoff {
			continue
		}
		visible = append(visible, c)
		t.visIdx = append(t.visIdx, i)
	}
	if len(t.visIdx) > 0 && t.highlight >= len(t.visIdx) {
		t.highlight = len(t.visIdx) - 1
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
		bcols[i] = btable.Column{Title: t.decorateTitle(c.Title, t.visIdx[i], i), Width: w}
	}
	brows := make([]btable.Row, len(t.rows))
	for i, r := range t.rows {
		cells := make([]string, len(t.visIdx))
		for j, src := range t.visIdx {
			var val string
			if src < len(r.Cells) {
				val = r.Cells[src]
			}
			// Style at render time only; src is the original column index.
			if t.cellStyler != nil {
				if st, ok := t.classStyle(t.cellStyler(src, val)); ok {
					val = st.Render(val)
				}
			}
			cells[j] = val
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

// decorateTitle marks the sorted column with a direction arrow and, in sort
// mode, the picker position with a pointer.
func (t *Table) decorateTitle(title string, colIdx, visPos int) string {
	if t.sortCol == colIdx {
		if t.sortAsc {
			title += " ↑"
		} else {
			title += " ↓"
		}
	}
	if t.sortMode && visPos == t.highlight {
		title = "▶ " + title
	}
	return title
}

// Update forwards navigation keys to the underlying table and drives sort
// mode.
func (t Table) Update(msg tea.Msg) (Table, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if t.sortMode {
			t.handleSortKey(kmsg.String())
			return t, nil
		}
		if kmsg.String() == "s" && len(t.visIdx) > 0 {
			t.sortMode = true
			t.prevCol, t.prevAsc = t.sortCol, t.sortAsc
			t.highlight = 0
			for i, src := range t.visIdx {
				if src == t.sortCol {
					t.highlight = i
				}
			}
			t.reflow()
			return t, nil
		}
	}
	var cmd tea.Cmd
	t.tbl, cmd = t.tbl.Update(msg)
	return t, cmd
}

func (t *Table) handleSortKey(key string) {
	switch key {
	case "left", "h":
		if t.highlight > 0 {
			t.highlight--
		}
	case "right", "l":
		if t.highlight < len(t.visIdx)-1 {
			t.highlight++
		}
	case "s", "space":
		t.selectHighlighted()
	case "enter":
		// Confirm: apply if the highlight isn't the active sort yet, then
		// leave sort mode keeping the result.
		if t.sortCol != t.visIdx[t.highlight] {
			t.selectHighlighted()
		}
		t.sortMode = false
	case "esc":
		t.sortCol, t.sortAsc = t.prevCol, t.prevAsc
		t.sortMode = false
		t.applySort()
	default:
		return
	}
	t.reflow()
}

// selectHighlighted sorts by the highlighted column; selecting the already
// sorted column reverses direction. Applied live so the effect is visible
// before confirming.
func (t *Table) selectHighlighted() {
	col := t.visIdx[t.highlight]
	if t.sortCol == col {
		t.sortAsc = !t.sortAsc
	} else {
		t.sortCol, t.sortAsc = col, true
	}
	t.applySort()
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
