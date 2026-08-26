package resource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
)

// stubDef is a minimal Def for framework tests.
type stubDef struct {
	name    string
	aliases []string
	args    []string
}

func (d stubDef) Name() string                    { return d.name }
func (d stubDef) Aliases() []string               { return d.aliases }
func (d stubDef) Args() []string                  { return d.args }
func (d stubDef) Columns() []Column               { return nil }
func (d stubDef) PollInterval() time.Duration     { return time.Second }
func (d stubDef) Child() string                   { return "" }
func (d stubDef) ChildScope(p Scope, _ Row) Scope { return p }
func (d stubDef) Actions() []Action               { return nil }

func (d stubDef) List(context.Context, *dbx.Clients, Scope) ([]Row, error) {
	return nil, nil
}

func (d stubDef) Describe(context.Context, *dbx.Clients, Scope, Row) (any, error) {
	return nil, nil
}

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	r := NewRegistry()
	r.MustRegister(stubDef{name: "catalogs", aliases: []string{"catalog", "cat"}})
	r.MustRegister(stubDef{name: "tables", aliases: []string{"table"}, args: []string{"catalog", "schema"}})
	return r
}

func TestRegistryGet(t *testing.T) {
	r := newTestRegistry(t)

	for _, name := range []string{"catalogs", "catalog", "cat"} {
		d, ok := r.Get(name)
		require.True(t, ok, name)
		assert.Equal(t, "catalogs", d.Name())
	}

	_, ok := r.Get("nope")
	assert.False(t, ok)
}

func TestRegistryCollisionsPanic(t *testing.T) {
	tests := []struct {
		name string
		def  Def
	}{
		{"duplicate name", stubDef{name: "catalogs"}},
		{"name collides with alias", stubDef{name: "cat"}},
		{"alias collides with name", stubDef{name: "fresh", aliases: []string{"tables"}}},
		{"alias collides with alias", stubDef{name: "fresh", aliases: []string{"cat"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRegistry(t)
			assert.Panics(t, func() { r.MustRegister(tt.def) })
		})
	}
}

func TestRegistryParse(t *testing.T) {
	r := newTestRegistry(t)

	tests := []struct {
		name    string
		input   string
		wantDef string
		want    Scope
		filter  string
		item    string
		wantErr string
	}{
		{name: "unscoped", input: "catalogs", wantDef: "catalogs", want: Scope{}},
		{name: "alias", input: "cat", wantDef: "catalogs", want: Scope{}},
		{name: "positional args", input: "tables main silver", wantDef: "tables", want: Scope{"catalog": "main", "schema": "silver"}},
		{name: "dotted sugar", input: "tables main.silver", wantDef: "tables", want: Scope{"catalog": "main", "schema": "silver"}},
		{name: "trailing filter", input: "tables main silver /events", wantDef: "tables", want: Scope{"catalog": "main", "schema": "silver"}, filter: "events"},
		{name: "filter on unscoped", input: "catalogs /prod", wantDef: "catalogs", want: Scope{}, filter: "prod"},
		{name: "item on unscoped", input: "catalogs prod", wantDef: "catalogs", want: Scope{}, item: "prod"},
		{name: "item after scope", input: "tables main silver orders", wantDef: "tables", want: Scope{"catalog": "main", "schema": "silver"}, item: "orders"},
		{name: "dotted scope plus item", input: "tables main.silver orders", wantDef: "tables", want: Scope{"catalog": "main", "schema": "silver"}, item: "orders"},
		{name: "item and filter", input: "tables main silver orders /x", wantDef: "tables", want: Scope{"catalog": "main", "schema": "silver"}, item: "orders", filter: "x"},
		{name: "missing args", input: "tables main", wantErr: "requires schema"},
		{name: "two items unscoped", input: "catalogs a b", wantErr: "at most one item"},
		{name: "two items scoped", input: "tables main silver a b", wantErr: "and at most one item"},
		{name: "unknown resource", input: "bogus", wantErr: `unknown resource "bogus"`},
		{name: "empty", input: "  ", wantErr: "empty command"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd, err := r.Parse(tt.input)
			if tt.wantErr != "" {
				require.ErrorContains(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.wantDef, cmd.Def.Name())
			assert.Equal(t, tt.want, cmd.Scope)
			assert.Equal(t, tt.filter, cmd.Filter)
			assert.Equal(t, tt.item, cmd.Item)
		})
	}
}

func TestRegistryComplete(t *testing.T) {
	r := newTestRegistry(t)
	assert.Equal(t, []string{"cat", "catalog", "catalogs"}, r.Complete("ca"))
	assert.Empty(t, r.Complete("zz"))
}

func TestScope(t *testing.T) {
	base := Scope{"catalog": "main"}
	child := base.Merge("schema", "silver")

	assert.Equal(t, Scope{"catalog": "main"}, base, "Merge must not mutate the parent")
	assert.Equal(t, Scope{"catalog": "main", "schema": "silver"}, child)
	assert.Equal(t, "catalog=main,schema=silver", child.Hash())
	assert.Empty(t, Scope{}.Hash())
}

func TestBuildRows(t *testing.T) {
	type item struct{ name, owner string }
	specs := []ColSpec[item]{
		{Column: Column{Title: "NAME"}, Extract: func(i item) string { return i.name }},
		{Column: Column{Title: "OWNER"}, Extract: func(i item) string { return i.owner }},
	}

	rows := BuildRows(
		[]item{{"main", "jon"}, {"dev", "sam"}},
		func(i item) string { return i.name },
		specs,
	)

	require.Len(t, rows, 2)
	assert.Equal(t, "main", rows[0].ID)
	assert.Equal(t, []string{"main", "jon"}, rows[0].Cells)
	assert.Equal(t, item{"main", "jon"}, rows[0].Data)
	assert.Equal(t, []Column{{Title: "NAME"}, {Title: "OWNER"}}, Cols(specs))
}

func TestRowMatchesFilter(t *testing.T) {
	row := Row{Cells: []string{"Main", "jon@example.com"}}

	assert.True(t, row.MatchesFilter(""))
	assert.True(t, row.MatchesFilter("main"))
	assert.True(t, row.MatchesFilter("EXAMPLE"))
	assert.False(t, row.MatchesFilter("prod"))
}

func TestScopeAncestors(t *testing.T) {
	r := newTestRegistry(t)
	r.MustRegister(stubDef{name: "schemas", args: []string{"catalog"}})
	tables, _ := r.Get("tables")

	got := r.ScopeAncestors(tables, Scope{"catalog": "main", "schema": "silver"})
	require.Len(t, got, 2)
	assert.Equal(t, "catalogs", got[0].Def.Name())
	assert.Equal(t, Scope{}, got[0].Scope)
	assert.Equal(t, "main", got[0].Select, "the catalog list lands on the chosen catalog")
	assert.Equal(t, "schemas", got[1].Def.Name())
	assert.Equal(t, Scope{"catalog": "main"}, got[1].Scope, "the schema list is scoped by the chosen catalog")
	assert.Equal(t, "silver", got[1].Select)

	// An unscoped resource has no levels above it.
	catalogs, _ := r.Get("catalogs")
	assert.Empty(t, r.ScopeAncestors(catalogs, Scope{}))
}

func TestScopeAncestorsSkipsMissingListers(t *testing.T) {
	// No "schemas" def registered, so that level is simply omitted.
	r := newTestRegistry(t)
	tables, _ := r.Get("tables")

	got := r.ScopeAncestors(tables, Scope{"catalog": "main", "schema": "silver"})
	require.Len(t, got, 1)
	assert.Equal(t, "catalogs", got[0].Def.Name())
}

func TestScopeLister(t *testing.T) {
	r := newTestRegistry(t)
	tables, _ := r.Get("tables")
	want := tables.Args()

	def, ok := r.ScopeLister(want, 0)
	require.True(t, ok)
	assert.Equal(t, "catalogs", def.Name())

	_, ok = r.ScopeLister(want, 1) // no schemas def registered
	assert.False(t, ok)
	_, ok = r.ScopeLister(want, 2) // out of range
	assert.False(t, ok)
}
