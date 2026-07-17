// Package theme defines the lipgloss styles used across the UI. All colors
// live here; views and components never hardcode colors.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

// Theme is the set of styles the app renders with. The accent is app-wide and
// constant; per-profile highlighting (see skins.go) colors only the header
// name/host, leaving the rest of the UI on the default accent.
type Theme struct {
	// Accent is the app-wide highlight color.
	Accent color.Color

	Logo     lipgloss.Style
	Title    lipgloss.Style
	Subtle   lipgloss.Style
	Error    lipgloss.Style
	Success  lipgloss.Style
	Warning  lipgloss.Style
	KeyHint  lipgloss.Style
	KeyLabel lipgloss.Style
}

// Default returns the built-in skin.
func Default() Theme {
	accent := lipgloss.Color("#FF6F00") // lakeside orange
	subtle := lipgloss.Color("245")

	return Theme{
		Accent:   accent,
		Logo:     lipgloss.NewStyle().Foreground(accent).Bold(true),
		Title:    lipgloss.NewStyle().Bold(true),
		Subtle:   lipgloss.NewStyle().Foreground(subtle),
		Error:    lipgloss.NewStyle().Foreground(lipgloss.Color("196")),
		Success:  lipgloss.NewStyle().Foreground(lipgloss.Color("42")),
		Warning:  lipgloss.NewStyle().Foreground(lipgloss.Color("214")),
		KeyHint:  lipgloss.NewStyle().Foreground(accent).Bold(true),
		KeyLabel: lipgloss.NewStyle().Foreground(subtle),
	}
}
