package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/resources"
)

func TestCompleteArgsResourceNames(t *testing.T) {
	reg := resources.NewRegistry()
	cmd := &cobra.Command{}
	got, dir := completeArgs(reg)(cmd, nil, "ta")

	assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, dir)
	assert.Contains(t, got, "tables")
	for _, c := range got {
		assert.True(t, len(c) >= 2 && c[:2] == "ta", "only 'ta' prefixes: %q", c)
	}
}

func TestCompleteArgsSqlOffered(t *testing.T) {
	reg := resources.NewRegistry()
	got, _ := completeArgs(reg)(&cobra.Command{}, nil, "")
	assert.Contains(t, got, "sql")
}

func TestScopeArgLister(t *testing.T) {
	reg := resources.NewRegistry()
	want := []string{"catalog", "schema"}

	// First scope arg of tables ("catalog") is listed by the catalogs resource.
	def, scope, ok := scopeArgLister(reg, want, nil, 0)
	require.True(t, ok)
	assert.Equal(t, "catalogs", def.Name())
	assert.Empty(t, scope)

	// Second ("schema") is listed by schemas, scoped by the chosen catalog.
	def, scope, ok = scopeArgLister(reg, want, []string{"main"}, 1)
	require.True(t, ok)
	assert.Equal(t, "schemas", def.Name())
	assert.Equal(t, resource.Scope{"catalog": "main"}, scope)
}

func TestTabNames(t *testing.T) {
	reg := resources.NewRegistry()
	assert.Equal(t, []string{"details", "logs"}, tabNames(reg, []string{"apps"}))
	assert.Equal(t, []string{"details", "logs"}, tabNames(reg, []string{"app"}), "resolves aliases")
	assert.Nil(t, tabNames(reg, []string{"jobs"}), "resource without tabs")
	assert.Nil(t, tabNames(reg, nil))
	assert.Nil(t, tabNames(reg, []string{"nope"}))
}

func TestZipScope(t *testing.T) {
	assert.Equal(t, resource.Scope{"catalog": "main"}, zipScope([]string{"catalog", "schema"}, []string{"main"}))
	assert.Equal(t, resource.Scope{"catalog": "main", "schema": "silver"}, zipScope([]string{"catalog", "schema"}, []string{"main", "silver"}))
	assert.Empty(t, zipScope([]string{"catalog"}, nil))
}

func TestSingular(t *testing.T) {
	assert.Equal(t, "catalog", singular("catalogs"))
	assert.Equal(t, "job", singular("jobs"))
}

func TestRowNamerProjection(t *testing.T) {
	reg := resources.NewRegistry()

	// jobs address rows by their NAME cell, not the numeric Row.ID.
	jobs, _ := reg.Get("jobs")
	name := rowNamer(jobs)
	assert.Equal(t, "Nightly ETL", name(resource.Row{ID: "42", Cells: []string{"42", "Nightly ETL"}}))

	// tables have no RowNamer, so the ID (table name) is the candidate.
	tables, _ := reg.Get("tables")
	assert.Equal(t, "orders", rowNamer(tables)(resource.Row{ID: "orders"}))
}

func TestFilterPrefix(t *testing.T) {
	assert.Equal(t, []string{"abc", "abd"}, filterPrefix([]string{"abc", "abd", "xyz"}, "ab"))
	assert.Equal(t, []string{"abc", "xyz"}, filterPrefix([]string{"abc", "xyz"}, ""), "empty prefix keeps all")
}
