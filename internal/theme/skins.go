package theme

import (
	"image/color"
	"path"
	"strings"

	"charm.land/lipgloss/v2"
)

// Named accent colors usable in config `skins:` mappings.
var accents = map[string]color.Color{
	"orange":  lipgloss.Color("#FF6F00"),
	"red":     lipgloss.Color("#E53935"),
	"green":   lipgloss.Color("#43A047"),
	"blue":    lipgloss.Color("#1E88E5"),
	"purple":  lipgloss.Color("#8E24AA"),
	"cyan":    lipgloss.Color("#00ACC1"),
	"magenta": lipgloss.Color("#D81B60"),
	"yellow":  lipgloss.Color("#FDD835"),
}

// ForProfile resolves the theme for a profile. Explicit config globs win
// (e.g. "PROD-*": red); otherwise anything smelling like production gets the
// red treatment k9s users love, and everything else keeps the default.
func ForProfile(profile string, skins map[string]string) Theme {
	th := Default()

	if name, ok := matchSkin(profile, skins); ok {
		if accent, known := accents[strings.ToLower(name)]; known {
			return withAccent(th, accent)
		}
		// Unknown color names fall through to the default; a typo in
		// config should never break startup.
		return th
	}

	if strings.Contains(strings.ToLower(profile), "prod") {
		return withAccent(th, accents["red"])
	}
	return th
}

func matchSkin(profile string, skins map[string]string) (string, bool) {
	lower := strings.ToLower(profile)
	for pattern, color := range skins {
		if ok, err := path.Match(strings.ToLower(pattern), lower); err == nil && ok {
			return color, true
		}
	}
	return "", false
}

func withAccent(th Theme, accent color.Color) Theme {
	th.Accent = accent
	th.Logo = th.Logo.Foreground(accent)
	th.KeyHint = th.KeyHint.Foreground(accent)
	return th
}
