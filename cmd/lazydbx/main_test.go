package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resources"
)

func TestValidateLaunch(t *testing.T) {
	reg := resources.NewRegistry()

	tests := []struct {
		name    string
		launch  string
		wantErr string // substring; "" means no error
	}{
		{name: "empty (no positional args)", launch: ""},
		{name: "whitespace only", launch: "   "},
		{name: "top-level resource", launch: "jobs"},
		{name: "scoped resource", launch: "tables main silver"},
		{name: "dotted scope sugar", launch: "tables main.silver"},
		{name: "trailing filter", launch: "jobs /etl"},
		{name: "bare sql", launch: "sql"},
		{name: "sql with query", launch: "sql SELECT 1"},
		{name: "unknown resource", launch: "jbos", wantErr: `unknown resource "jbos"`},
		{name: "too few args", launch: "tables main", wantErr: "requires"},
		{name: "too many args", launch: "jobs extra", wantErr: "at most"},
		{name: "apps list", launch: "apps"},
		{name: "apps name normalized to filter", launch: normalizeLaunch(reg, "apps my-app")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateLaunch(reg, tt.launch)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestNormalizeLaunch(t *testing.T) {
	reg := resources.NewRegistry()
	tests := []struct {
		name, in, want string
	}{
		{"apps name → filter", "apps my-app", "apps /my-app"},
		{"app alias → filter", "app my-app", "app /my-app"},
		{"apps alone unchanged", "apps", "apps"},
		{"apps explicit filter unchanged", "apps /foo", "apps /foo"},
		{"other resources unchanged", "jobs etl", "jobs etl"},
		{"scoped resource unchanged", "tables main silver", "tables main silver"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, normalizeLaunch(reg, tt.in))
		})
	}
}
