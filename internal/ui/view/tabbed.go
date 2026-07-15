package view

import (
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// Tab pairs a name with the view it shows.
type Tab struct {
	Name string
	View View
}

// Tabbed hosts sibling views of one subject (e.g. a table's columns / data /
// details) behind a tab bar. `[` and `]` switch tabs; every other message
// routes to the active tab, except non-key messages, which broadcast so
// background tabs keep receiving their data.
type Tabbed struct {
	th     theme.Theme
	title  string
	tabs   []Tab
	active int
}

// NewTabbed builds the container; the first tab starts active.
func NewTabbed(th theme.Theme, title string, tabs []Tab) *Tabbed {
	return &Tabbed{th: th, title: title, tabs: tabs}
}

// Init starts every tab so all of them prefetch — switching feels instant.
func (t *Tabbed) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for _, tab := range t.tabs {
		if cmd := tab.View.Init(); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return tea.Batch(cmds...)
}

// Close closes every tab.
func (t *Tabbed) Close() {
	for _, tab := range t.tabs {
		tab.View.Close()
	}
}

// Title implements View.
func (t *Tabbed) Title() string { return t.title }

// CapturesKeys defers to the active tab (e.g. the SQL editor's textarea).
func (t *Tabbed) CapturesKeys() bool {
	if v, ok := t.tabs[t.active].View.(interface{ CapturesKeys() bool }); ok {
		return v.CapturesKeys()
	}
	return false
}

// Hints prepends the tab-switch keys to the active tab's hints.
func (t *Tabbed) Hints() []key.Binding {
	hints := []key.Binding{
		key.NewBinding(key.WithKeys("["), key.WithHelp("[/]", "switch tab")),
	}
	return append(hints, t.tabs[t.active].View.Hints()...)
}

// Status delegates to the active tab when it reports status.
func (t *Tabbed) Status(now time.Time) string {
	if s, ok := t.tabs[t.active].View.(StatusProvider); ok {
		return s.Status(now)
	}
	return ""
}

// Update switches tabs on [/] (unless the active tab captures keys) and
// otherwise routes: key messages to the active tab, everything else to all
// tabs so inactive ones stay live.
func (t *Tabbed) Update(msg tea.Msg) (View, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		if !t.CapturesKeys() {
			switch kmsg.String() {
			case "]":
				t.active = (t.active + 1) % len(t.tabs)
				return t, nil
			case "[":
				t.active = (t.active + len(t.tabs) - 1) % len(t.tabs)
				return t, nil
			}
		}
		v, cmd := t.tabs[t.active].View.Update(msg)
		t.tabs[t.active].View = v
		return t, cmd
	}

	cmds := make([]tea.Cmd, 0, len(t.tabs))
	for i := range t.tabs {
		v, cmd := t.tabs[i].View.Update(msg)
		t.tabs[i].View = v
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	return t, tea.Batch(cmds...)
}

// Render draws the tab bar plus the active tab's body.
func (t *Tabbed) Render(width, height int) string {
	var parts []string
	for i, tab := range t.tabs {
		label := " " + tab.Name + " "
		if i == t.active {
			parts = append(parts, t.th.KeyHint.Reverse(true).Render(label))
		} else {
			parts = append(parts, t.th.Subtle.Render(label))
		}
	}
	bar := lipgloss.NewStyle().MaxWidth(width).Render(strings.Join(parts, t.th.Subtle.Render("│")))
	return bar + "\n" + t.tabs[t.active].View.Render(width, height-1)
}
