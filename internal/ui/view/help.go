package view

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
)

// Help is the `?` view: global keys plus the keys of the view beneath it.
type Help struct {
	th       theme.Theme
	sections []HelpSection
}

// HelpSection groups bindings (or plain lines) under a heading.
type HelpSection struct {
	Title    string
	Bindings []key.Binding
	// Lines are rendered as plain indented text — used for the resource
	// catalog, which isn't key-bound.
	Lines []string
}

// NewHelp builds the help view.
func NewHelp(th theme.Theme, sections []HelpSection) *Help {
	return &Help{th: th, sections: sections}
}

// Init implements View.
func (h *Help) Init() tea.Cmd { return nil }

// Close implements View.
func (h *Help) Close() {}

// Title implements View.
func (h *Help) Title() string { return "help" }

// Hints implements View.
func (h *Help) Hints() []key.Binding { return nil }

// Update pops on esc or ?.
func (h *Help) Update(msg tea.Msg) (View, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "esc", "?", "q":
			return h, func() tea.Msg { return PopMsg{} }
		}
	}
	return h, nil
}

// Render implements View.
func (h *Help) Render(width, _ int) string {
	var b strings.Builder
	for i, section := range h.sections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(h.th.Title.Render(section.Title) + "\n")
		var keyWidth int
		for _, bind := range section.Bindings {
			keyWidth = max(keyWidth, len(bind.Help().Key))
		}
		for _, bind := range section.Bindings {
			pad := strings.Repeat(" ", keyWidth-len(bind.Help().Key))
			b.WriteString("  " + h.th.KeyHint.Render("<"+bind.Help().Key+">") + pad + "  " +
				h.th.KeyLabel.Render(bind.Help().Desc) + "\n")
		}
		for _, line := range section.Lines {
			b.WriteString("  " + h.th.KeyLabel.Render(line) + "\n")
		}
	}
	out := b.String()
	if width > 0 {
		lines := strings.Split(out, "\n")
		for i, l := range lines {
			if len(l) > width {
				lines[i] = l[:width]
			}
		}
		out = strings.Join(lines, "\n")
	}
	return out
}
