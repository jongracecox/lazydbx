package theme

import (
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/stretchr/testify/assert"
)

func TestHighlightColor(t *testing.T) {
	tests := []struct {
		name    string
		profile string
		skins   map[string]string
		want    interface{} // expected color, nil when none
		ok      bool
		desc    string
	}{
		{
			name:    "no skin match, no highlight",
			profile: "dev",
			ok:      false,
			desc:    "there is no auto-detection; unconfigured profiles stay plain",
		},
		{
			name:    "prod substring is NOT auto-detected",
			profile: "my-prod-workspace",
			ok:      false,
			desc:    "the old prod=red heuristic is gone",
		},
		{
			name:    "explicit glob resolves its color",
			profile: "prod-eu",
			skins:   map[string]string{"prod-*": "blue"},
			want:    accents["blue"],
			ok:      true,
		},
		{
			name:    "glob is case-insensitive",
			profile: "PROD-EU",
			skins:   map[string]string{"prod-*": "green"},
			want:    accents["green"],
			ok:      true,
		},
		{
			name:    "exact name (as the picker writes) matches",
			profile: "prod",
			skins:   map[string]string{"prod": "red"},
			want:    accents["red"],
			ok:      true,
		},
		{
			name:    "unknown color name yields no highlight",
			profile: "staging",
			skins:   map[string]string{"staging": "chartreuse"},
			ok:      false,
			desc:    "a typo in config must never break rendering",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := HighlightColor(tt.profile, tt.skins)
			assert.Equal(t, tt.ok, ok, tt.desc)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}

func TestContrast(t *testing.T) {
	// Light accents get black text; dark ones get white.
	assert.Equal(t, lipgloss.Color("#000000"), Contrast(accents["yellow"]), "dark text on a light accent")
	assert.Equal(t, lipgloss.Color("#FFFFFF"), Contrast(accents["red"]), "light text on a dark accent")
	assert.Equal(t, lipgloss.Color("#FFFFFF"), Contrast(accents["blue"]))
}

func TestAccentNamesAndColor(t *testing.T) {
	names := AccentNames()
	assert.Len(t, names, len(accents))
	assert.IsIncreasing(t, names, "names are returned in a stable, sorted order")
	for _, name := range names {
		c, ok := AccentColor(name)
		assert.True(t, ok, "listed name %q must resolve", name)
		assert.Equal(t, accents[name], c)
	}
	// Case-insensitive; unknown names report not-found.
	c, ok := AccentColor("RED")
	assert.True(t, ok)
	assert.Equal(t, accents["red"], c)
	_, ok = AccentColor("chartreuse")
	assert.False(t, ok)
}

func TestMatchSkin(t *testing.T) {
	skins := map[string]string{
		"prod-*":  "red",
		"exact":   "blue",
		"team-??": "green",
	}

	tests := []struct {
		profile string
		want    string
		ok      bool
	}{
		{"prod-eu", "red", true},
		{"PROD-US", "red", true},
		{"exact", "blue", true},
		{"team-01", "green", true},
		{"team-001", "", false}, // ?? matches exactly two chars
		{"nomatch", "", false},
	}

	for _, tt := range tests {
		got, ok := matchSkin(tt.profile, skins)
		assert.Equal(t, tt.ok, ok, "match %q", tt.profile)
		if tt.ok {
			assert.Equal(t, tt.want, got, "color for %q", tt.profile)
		}
	}
}
