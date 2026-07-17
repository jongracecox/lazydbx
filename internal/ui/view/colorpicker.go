package view

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// noColor is the picker entry that clears a profile's highlight.
const noColor = "none"

// ColorPicker sets the highlight color for a profile's name + host in the
// header. Selecting a color emits ProfileColorSelectedMsg; "none" clears it.
type ColorPicker struct {
	th      theme.Theme
	profile string
	options []string // noColor first, then theme.AccentNames()
	cursor  int
}

// NewColorPicker builds the picker for profile, pre-selecting current (the
// configured color name, "" for none).
func NewColorPicker(th theme.Theme, profile, current string) *ColorPicker {
	options := append([]string{noColor}, theme.AccentNames()...)
	cursor := 0
	for i, o := range options {
		if o == current {
			cursor = i
			break
		}
	}
	return &ColorPicker{th: th, profile: profile, options: options, cursor: cursor}
}

// Init implements View.
func (c *ColorPicker) Init() tea.Cmd { return nil }

// Close implements View.
func (c *ColorPicker) Close() {}

// Title implements View.
func (c *ColorPicker) Title() string { return "color" }

// Hints implements View.
func (c *ColorPicker) Hints() []key.Binding {
	return []key.Binding{
		key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "apply")),
		key.NewBinding(key.WithKeys("j"), key.WithHelp("j/k", "move")),
	}
}

// Update handles navigation and selection.
func (c *ColorPicker) Update(msg tea.Msg) (View, tea.Cmd) {
	kmsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		return c, nil
	}
	switch kmsg.String() {
	case "up", "k":
		if c.cursor > 0 {
			c.cursor--
		}
	case "down", "j":
		if c.cursor < len(c.options)-1 {
			c.cursor++
		}
	case "enter":
		choice := c.options[c.cursor]
		if choice == noColor {
			choice = ""
		}
		profile := c.profile
		return c, func() tea.Msg { return ProfileColorSelectedMsg{Profile: profile, Color: choice} }
	case "esc":
		return c, func() tea.Msg { return PopMsg{} }
	}
	return c, nil
}

// Render draws the swatch list.
func (c *ColorPicker) Render(width, height int) string {
	var b strings.Builder
	b.WriteString(c.th.Subtle.Render("highlight for "+c.profile) + "\n\n")
	for i, name := range c.options {
		cursor := "  "
		if i == c.cursor {
			cursor = c.th.KeyHint.Render("▸ ")
		}
		var label string
		switch col, ok := theme.AccentColor(name); {
		case name == noColor:
			label = c.th.Subtle.Render("none (default)")
		case ok:
			// Preview the color exactly as the header chip will render it.
			style := lipgloss.NewStyle().Background(col).Foreground(theme.Contrast(col)).Bold(true).Padding(0, 1)
			label = style.Render(name)
		default:
			label = name
		}
		b.WriteString(cursor + label + "\n")
	}
	return b.String()
}
