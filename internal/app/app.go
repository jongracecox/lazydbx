// Package app hosts the root Bubble Tea model. In Phase 0 it renders a splash
// screen; later phases add the view stack, command bar, and overlays.
package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/version"
)

const logo = `
 ██▓    ▄▄▄      ▒███████▒▓██   ██▓▓█████▄  ▄▄▄▄   ▒██   ██▒
▓██▒   ▒████▄    ▒ ▒ ▒ ▄▀░ ▒██  ██▒▒██▀ ██▌▓█████▄ ▒▒ █ █ ▒░
▒██░   ▒██  ▀█▄  ░ ▒ ▄▀▒░   ▒██ ██░░██   █▌▒██▒ ▄██░░  █   ░
▒██░   ░██▄▄▄▄██   ▄▀▒   ░  ░ ▐██▓░░▓█▄   ▌▒██░█▀    ░ █ █ ▒
░██████▒▓█   ▓██▒▒███████▒  ░ ██▒▓░░▒████▓ ░▓█  ▀█▓▒██▒ ▒██▒
░ ▒░▓  ░▒▒   ▓▒█░░▒▒ ▓░▒░▒   ██▒▒▒  ▒▒▓  ▒ ░▒▓███▀▒▒▒ ░ ░▓ ░
░ ░ ▒  ░ ▒   ▒▒ ░░░▒ ▒ ░ ▒ ▓██ ░▒░  ░ ▒  ▒ ▒░▒   ░ ░░   ░▒ ░
`

// Model is the root application model.
type Model struct {
	cfg    config.Config
	theme  theme.Theme
	width  int
	height int
}

// New builds the root model.
func New(cfg config.Config) Model {
	return Model{cfg: cfg, theme: theme.Default()}
}

// Init implements tea.Model.
func (m Model) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil
	case tea.KeyPressMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}
	}
	return m, nil
}

// View implements tea.Model.
func (m Model) View() tea.View {
	t := m.theme

	lines := []string{
		t.Logo.Render(strings.Trim(logo, "\n")),
		"",
		t.Title.Render("a lazier way to Databricks"),
		t.Subtle.Render(version.String()),
		"",
		t.KeyHint.Render("q") + t.KeyLabel.Render(" quit"),
	}
	if m.cfg.ReadOnly {
		lines = append(lines, "", t.Warning.Render("read-only mode"))
	}
	content := strings.Join(lines, "\n")

	if m.width > 0 && m.height > 0 {
		content = lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, content)
	}

	v := tea.NewView(content)
	v.AltScreen = true
	return v
}
