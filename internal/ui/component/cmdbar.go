package component

import (
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// EventKind classifies what an input-bar Update produced.
type EventKind int

// Input bar event kinds.
const (
	EventNone EventKind = iota
	EventChanged
	EventSubmit
	EventCancel
)

// Event is the result of feeding a message to an input bar.
type Event struct {
	Kind  EventKind
	Value string
}

// CmdBar is the `:` command prompt with prefix autocomplete.
type CmdBar struct {
	input    textinput.Model
	complete func(prefix string) []string
	matches  []string
	sel      int
}

// NewCmdBar builds the command bar; complete supplies candidate names for
// the first word (usually Registry.Complete).
func NewCmdBar(complete func(string) []string) CmdBar {
	ti := textinput.New()
	ti.Prompt = ":"
	ti.Placeholder = "resource [args] [/filter]"
	return CmdBar{input: ti, complete: complete}
}

// Open resets and focuses the bar.
func (c CmdBar) Open() (CmdBar, tea.Cmd) {
	c.input.SetValue("")
	c.matches = nil
	c.sel = 0
	return c, c.input.Focus()
}

// Update processes one message while the bar is open.
func (c CmdBar) Update(msg tea.Msg) (CmdBar, Event, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "esc", "ctrl+c":
			c.input.Blur()
			return c, Event{Kind: EventCancel}, nil
		case "enter":
			value := strings.TrimSpace(c.input.Value())
			// A bare Enter adopts the highlighted completion.
			if value != "" && !strings.Contains(value, " ") && len(c.matches) > 0 && value != c.matches[c.sel] {
				value = c.matches[c.sel]
			}
			c.input.Blur()
			return c, Event{Kind: EventSubmit, Value: value}, nil
		case "tab":
			if len(c.matches) > 0 {
				c.sel = (c.sel + 1) % len(c.matches)
				c.input.SetValue(c.matches[c.sel])
				c.input.CursorEnd()
			}
			return c, Event{Kind: EventNone}, nil
		}
	}

	var cmd tea.Cmd
	c.input, cmd = c.input.Update(msg)
	c.refreshMatches()
	return c, Event{Kind: EventNone}, cmd
}

func (c *CmdBar) refreshMatches() {
	value := c.input.Value()
	if value == "" || strings.Contains(value, " ") {
		c.matches = nil
		c.sel = 0
		return
	}
	c.matches = c.complete(value)
	if c.sel >= len(c.matches) {
		c.sel = 0
	}
}

// View renders the prompt plus a one-line suggestion strip.
func (c CmdBar) View(th theme.Theme, width int) string {
	line := c.input.View()

	var suggestions string
	if len(c.matches) > 0 {
		parts := make([]string, 0, len(c.matches))
		for i, m := range c.matches {
			if i == c.sel {
				parts = append(parts, th.KeyHint.Render(m))
			} else {
				parts = append(parts, th.Subtle.Render(m))
			}
		}
		suggestions = strings.Join(parts, "  ")
	}
	return clampWidth(th, width, line) + "\n" + clampWidth(th, width, suggestions)
}

// FilterBar is the `/` live filter prompt.
type FilterBar struct {
	input textinput.Model
	prev  string // value to restore on cancel
}

// NewFilterBar builds the filter bar.
func NewFilterBar() FilterBar {
	ti := textinput.New()
	ti.Prompt = "/"
	return FilterBar{input: ti}
}

// Open focuses the bar seeded with the current filter.
func (f FilterBar) Open(current string) (FilterBar, tea.Cmd) {
	f.prev = current
	f.input.SetValue(current)
	f.input.CursorEnd()
	return f, f.input.Focus()
}

// Update processes one message; EventChanged fires on every edit so the view
// can filter live.
func (f FilterBar) Update(msg tea.Msg) (FilterBar, Event, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "esc", "ctrl+c":
			f.input.Blur()
			return f, Event{Kind: EventCancel, Value: f.prev}, nil
		case "enter":
			f.input.Blur()
			return f, Event{Kind: EventSubmit, Value: f.input.Value()}, nil
		}
	}
	before := f.input.Value()
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	if v := f.input.Value(); v != before {
		return f, Event{Kind: EventChanged, Value: v}, cmd
	}
	return f, Event{Kind: EventNone}, cmd
}

// View renders the filter prompt.
func (f FilterBar) View(th theme.Theme, width int) string {
	return clampWidth(th, width, f.input.View())
}

func clampWidth(_ theme.Theme, width int, s string) string {
	if width <= 0 {
		return s
	}
	return lipgloss.NewStyle().MaxWidth(width).Render(s)
}
