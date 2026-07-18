package view

import (
	"strings"

	"charm.land/bubbles/v2/key"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// RowDetail shows a single result row as a scrollable COLUMN / VALUE list.
// Large or multi-line values wrap across lines so the whole row is readable —
// a companion to the necessarily truncated grid in SQLView.
type RowDetail struct {
	th     theme.Theme
	title  string
	names  []string
	values []string

	vp      viewport.Model
	ready   bool
	lastW   int
	lastH   int
	showBar bool
}

// NewRowDetail builds a detail view for one row: names and values are parallel
// slices (column i's value is values[i]).
func NewRowDetail(th theme.Theme, title string, names, values []string) *RowDetail {
	return &RowDetail{th: th, title: title, names: names, values: values}
}

// Init implements View.
func (d *RowDetail) Init() tea.Cmd { return nil }

// Update implements View: esc pops back to the grid, everything else scrolls.
func (d *RowDetail) Update(msg tea.Msg) (View, tea.Cmd) {
	if km, ok := msg.(tea.KeyPressMsg); ok && km.String() == "esc" {
		return d, func() tea.Msg { return PopMsg{} }
	}
	var cmd tea.Cmd
	d.vp, cmd = d.vp.Update(msg)
	return d, cmd
}

// Render implements View. It reserves a right-hand column for a vertical
// scrollbar whenever the content overflows the height, and lays out (wraps)
// the pairs only when the size changes so scroll position is preserved.
func (d *RowDetail) Render(width, height int) string {
	if !d.ready {
		d.vp = viewport.New(viewport.WithWidth(width), viewport.WithHeight(height))
		d.ready = true
		d.lastW, d.lastH = -1, -1
	}

	if width != d.lastW || height != d.lastH {
		d.lastW, d.lastH = width, height
		d.vp.SetHeight(height)
		// Lay out at full width to learn whether it overflows the height.
		d.vp.SetWidth(width)
		d.vp.SetContent(d.content(width))
		d.showBar = width > 1 && d.vp.TotalLineCount() > height
		if d.showBar {
			contentW := width - 1
			d.vp.SetWidth(contentW)
			d.vp.SetContent(d.content(contentW))
		}
	}

	body := d.vp.View()
	if d.showBar {
		bar := component.Scrollbar(d.th, height, d.vp.TotalLineCount(), height, d.vp.YOffset())
		body = lipgloss.JoinHorizontal(lipgloss.Top, body, bar)
	}
	return body
}

// content lays out the name/value pairs. The name column is sized to the
// widest name (capped so wide values still get room); values are wrapped to
// the remaining width, and their own newlines are preserved.
func (d *RowDetail) content(width int) string {
	const gap = 2

	nameW := 0
	for _, n := range d.names {
		if w := ansi.StringWidth(n); w > nameW {
			nameW = w
		}
	}
	if maxName := max(8, width/3); nameW > maxName {
		nameW = maxName
	}

	valW := max(1, width-nameW-gap)
	nameStyle := d.th.Title.Foreground(d.th.Accent)
	valWrap := lipgloss.NewStyle().Width(valW)

	var b strings.Builder
	for i, name := range d.names {
		val := ""
		if i < len(d.values) {
			val = d.values[i]
		}
		lines := strings.Split(valWrap.Render(val), "\n")
		nm := ansi.Truncate(name, nameW, "…")
		for j, ln := range lines {
			if j == 0 {
				b.WriteString(nameStyle.Render(pad(nm, nameW)))
			} else {
				b.WriteString(strings.Repeat(" ", nameW))
			}
			b.WriteString(strings.Repeat(" ", gap))
			b.WriteString(ln)
			b.WriteByte('\n')
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// Title implements View.
func (d *RowDetail) Title() string { return d.title }

// Hints implements View.
func (d *RowDetail) Hints() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "scroll")),
		key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

// Close implements View.
func (d *RowDetail) Close() {}
