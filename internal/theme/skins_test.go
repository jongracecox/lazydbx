package theme

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForProfile(t *testing.T) {
	def := Default()

	tests := []struct {
		name    string
		profile string
		skins   map[string]string
		want    interface{} // expected Accent color
		desc    string
	}{
		{
			name:    "plain profile keeps default accent",
			profile: "dev",
			want:    def.Accent,
			desc:    "no prod smell, no skin match",
		},
		{
			name:    "prod substring gets red",
			profile: "my-prod-workspace",
			want:    accents["red"],
			desc:    "anything smelling of prod turns red",
		},
		{
			name:    "explicit glob wins over prod heuristic",
			profile: "prod-eu",
			skins:   map[string]string{"prod-*": "blue"},
			want:    accents["blue"],
			desc:    "config glob overrides the built-in prod=red rule",
		},
		{
			name:    "glob is case-insensitive",
			profile: "PROD-EU",
			skins:   map[string]string{"prod-*": "green"},
			want:    accents["green"],
		},
		{
			name:    "unknown color name falls through to default",
			profile: "staging",
			skins:   map[string]string{"staging": "chartreuse"},
			want:    def.Accent,
			desc:    "a typo in config must never break startup",
		},
		{
			name:    "unknown color on a prod profile does not re-trigger red",
			profile: "prod-eu",
			skins:   map[string]string{"prod-*": "chartreuse"},
			want:    def.Accent,
			desc:    "an explicit (if bad) match short-circuits the heuristic",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ForProfile(tt.profile, tt.skins)
			assert.Equal(t, tt.want, got.Accent, tt.desc)
			// Accent must be threaded into the derived styles too.
			assert.Equal(t, got.Accent, withAccent(def, got.Accent).Accent)
		})
	}
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
