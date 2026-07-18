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

// TabCycler is implemented by a tab's view when it has internal focus stops
// (e.g. the SQL editor / results split) that should participate in the global
// tab cycle instead of colliding with it. Tabbed consults it on tab/shift+tab:
// AdvanceFocus moves the internal focus one step in the given direction and
// returns true when it did so; a false return means the view is already at its
// boundary, so Tabbed advances to the adjacent tab. EnterFocus then places the
// arriving view at its entry boundary (forward → first stop, backward → last).
// This is the sanctioned way to resolve a tab-key conflict — see CLAUDE.md.
type TabCycler interface {
	AdvanceFocus(forward bool) bool
	EnterFocus(forward bool)
}

// Tabbed hosts sibling views of one subject (e.g. a table's columns / data /
// details) behind a tab bar. tab/shift+tab step through the tabs and any
// internal focus stops a tab exposes via TabCycler (so a tab-using pane like
// the SQL editor no longer fights the container for the key); `[`/`]` jump
// whole tabs, skipping internal stops. Every other key routes to the active
// tab, and non-key messages broadcast so background tabs keep receiving data.
type Tabbed struct {
	th     theme.Theme
	title  string
	tabs   []Tab
	active int
}

// NewTabbed builds the container. active selects the initially shown tab
// (0-based); out-of-range values clamp to the first tab, so callers can pass a
// resolved index without bounds-checking.
func NewTabbed(th theme.Theme, title string, tabs []Tab, active int) *Tabbed {
	if active < 0 || active >= len(tabs) {
		active = 0
	}
	return &Tabbed{th: th, title: title, tabs: tabs, active: active}
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

// Hints prepends the tab-switch keys to the active tab's hints. The active tab
// may bind `tab` itself for its internal focus movement (e.g. SQLView's
// editor/results split) — that key is folded into this unified cycle, so its
// own `tab` hint is dropped to avoid showing two `tab` entries.
func (t *Tabbed) Hints() []key.Binding {
	hints := []key.Binding{
		key.NewBinding(key.WithKeys("tab", "shift+tab"), key.WithHelp("tab", "switch tab")),
	}
	for _, h := range t.tabs[t.active].View.Hints() {
		if bindsTab(h) {
			continue
		}
		hints = append(hints, h)
	}
	return hints
}

// bindsTab reports whether a binding is triggered by the `tab` key.
func bindsTab(b key.Binding) bool {
	for _, k := range b.Keys() {
		if k == "tab" {
			return true
		}
	}
	return false
}

// Status delegates to the active tab when it reports status.
func (t *Tabbed) Status(now time.Time) string {
	if s, ok := t.tabs[t.active].View.(StatusProvider); ok {
		return s.Status(now)
	}
	return ""
}

// Update cycles focus on tab/shift+tab, and otherwise routes: key messages to
// the active tab, everything else to all tabs so inactive ones stay live.
func (t *Tabbed) Update(msg tea.Msg) (View, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "tab":
			t.cycle(true)
			return t, nil
		case "shift+tab":
			t.cycle(false)
			return t, nil
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

// cycle advances one step in direction across the unified tab/focus loop: it
// first asks the active tab (when it is a TabCycler) to move its internal
// focus; only when the tab has no internal stops or is already at its boundary
// does it switch to the adjacent tab, landing the arriving tab at its entry
// boundary.
func (t *Tabbed) cycle(forward bool) {
	if c, ok := t.tabs[t.active].View.(TabCycler); ok && c.AdvanceFocus(forward) {
		return
	}
	t.switchTab(forward)
	if c, ok := t.tabs[t.active].View.(TabCycler); ok {
		c.EnterFocus(forward)
	}
}

// switchTab moves the active index one tab in direction, wrapping around.
func (t *Tabbed) switchTab(forward bool) {
	if forward {
		t.active = (t.active + 1) % len(t.tabs)
	} else {
		t.active = (t.active + len(t.tabs) - 1) % len(t.tabs)
	}
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
