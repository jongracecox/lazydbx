package view

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/jongracecox/lazydbx/internal/theme"
	"github.com/jongracecox/lazydbx/internal/ui/component"
	"github.com/jongracecox/lazydbx/internal/version"
)

// projectURL is the canonical home of the project, shown on the About page.
const projectURL = "https://github.com/jongracecox/lazydbx"

// About is the `a` view: a centred splash with the logo, build metadata,
// project URL and copyright.
type About struct {
	th theme.Theme
}

// NewAbout builds the about view.
func NewAbout(th theme.Theme) *About { return &About{th: th} }

// Init implements View.
func (a *About) Init() tea.Cmd { return nil }

// Close implements View.
func (a *About) Close() {}

// Title implements View.
func (a *About) Title() string { return "about" }

// Hints implements View.
func (a *About) Hints() []key.Binding { return nil }

// Update pops on esc, a or q.
func (a *About) Update(msg tea.Msg) (View, tea.Cmd) {
	if kmsg, ok := msg.(tea.KeyPressMsg); ok {
		switch kmsg.String() {
		case "esc", "a", "q":
			return a, func() tea.Msg { return PopMsg{} }
		}
	}
	return a, nil
}

// Render centres the logo and build info in the available space.
func (a *About) Render(width, height int) string {
	logo := a.th.Logo.Render(component.Banner)

	rows := []struct{ label, value string }{
		{"version", version.Version},
		{"commit", version.Commit},
		{"built", version.Date},
		{"project", projectURL},
		{"license", "MIT"},
	}
	var labelW, rowW int
	for _, r := range rows {
		labelW = max(labelW, len(r.label))
	}
	for _, r := range rows {
		rowW = max(rowW, labelW+2+len(r.value))
	}
	value := lipgloss.NewStyle().Foreground(a.th.Accent)
	metaLines := make([]string, len(rows))
	for i, r := range rows {
		labelPad := strings.Repeat(" ", labelW-len(r.label))
		tailPad := strings.Repeat(" ", rowW-(labelW+2+len(r.value)))
		metaLines[i] = a.th.Subtle.Render(r.label+labelPad) + "  " + value.Render(r.value) + tailPad
	}
	meta := strings.Join(metaLines, "\n")
	copyright := a.th.Subtle.Render("© 2026 Jon Grace-Cox")

	block := lipgloss.JoinVertical(lipgloss.Center, logo, "", meta, copyright)
	if width <= 0 || height <= 0 {
		return block
	}
	return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, block)
}
