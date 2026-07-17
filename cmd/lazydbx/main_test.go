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
