package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resources"
)

func TestValidateLaunch(t *testing.T) {
	reg := resources.NewRegistry()

	tests := []struct {
		name    string
		launch  string // space-split into args; use quotes-free tokens
		tab     string
		wantErr string // substring; "" means no error
	}{
		{name: "empty (no positional args)", launch: ""},
		{name: "top-level resource", launch: "jobs"},
		{name: "scoped resource", launch: "tables main silver"},
		{name: "dotted scope sugar", launch: "tables main.silver"},
		{name: "trailing filter", launch: "jobs /etl"},
		{name: "bare sql", launch: "sql"},
		{name: "sql with query", launch: "sql SELECT 1"},
		{name: "unknown resource", launch: "jbos", wantErr: `unknown resource "jbos"`},
		{name: "too few args", launch: "tables main", wantErr: "requires"},
		{name: "apps list", launch: "apps"},

		// Positional item selectors (formerly errors / only /filter).
		{name: "apps item name", launch: "apps my-app"},
		{name: "jobs item name", launch: "jobs nightly-etl"},
		{name: "tables item name", launch: "tables main silver orders"},
		{name: "tables dotted plus item", launch: "tables main.silver orders"},
		{name: "two items is an error", launch: "jobs a b", wantErr: "at most one item"},

		// --tab selection.
		{name: "tab on apps item", launch: "apps my-app", tab: "logs"},
		{name: "tab case-insensitive", launch: "apps my-app", tab: "LOGS"},
		{name: "tab on tables item", launch: "tables main silver orders", tab: "data"},
		{name: "tab without item", launch: "apps", tab: "logs", wantErr: "naming a single apps"},
		{name: "tab on unsupported resource", launch: "jobs nightly-etl", tab: "logs", wantErr: "no tabs"},
		{name: "unknown tab name", launch: "apps my-app", tab: "bogus", wantErr: "unknown tab"},
		{name: "tab without launch", launch: "", tab: "logs", wantErr: "requires launching"},
		{name: "tab with sql", launch: "sql SELECT 1", tab: "logs", wantErr: "requires launching"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLaunch(reg, splitArgs(tt.launch), tt.tab)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

// splitArgs tokenizes a test launch string like a shell would (whitespace
// only — no quoting), mirroring how cobra hands args to RunE.
func splitArgs(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
