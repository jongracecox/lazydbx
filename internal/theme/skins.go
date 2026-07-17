package theme

import (
	"image/color"
	"path"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
)

// accents are the named colors a user can pick to highlight a profile's
// name/host in the header — usable both in config `skins:` globs and the
// in-app color picker (`c` on the profile screen).
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

// AccentNames returns the accent color names in a stable order — the choices
// offered by the color picker.
func AccentNames() []string {
	names := make([]string, 0, len(accents))
	for name := range accents {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// AccentColor returns the color for a named accent (case-insensitive), or
// false if name is not a known accent.
func AccentColor(name string) (color.Color, bool) {
	c, ok := accents[strings.ToLower(name)]
	return c, ok
}

// Contrast returns a foreground color (black or white) that stays legible when
// drawn on top of bg, chosen by perceived luminance. Used for the header's
// highlight chip so the profile name reads on any accent (e.g. dark on yellow,
// light on red).
func Contrast(bg color.Color) color.Color {
	r, g, b, _ := bg.RGBA() // 16-bit channels
	lum := (0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(b>>8)) / 255
	if lum > 0.55 {
		return lipgloss.Color("#000000")
	}
	return lipgloss.Color("#FFFFFF")
}

// HighlightColor resolves the highlight color a user configured for a profile
// via skins globs (e.g. "PROD-*": red). It returns false when no glob matches
// or the mapped name is not a known color — a typo just leaves the profile
// with the default (uncoloured) treatment rather than breaking rendering.
func HighlightColor(profile string, skins map[string]string) (color.Color, bool) {
	name, ok := matchSkin(profile, skins)
	if !ok {
		return nil, false
	}
	return AccentColor(name)
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
