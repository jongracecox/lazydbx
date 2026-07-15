package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

type fakeSchemasDAO struct {
	ListFn func(ctx context.Context, catalog string) ([]dbx.Schema, error)
}

func (f fakeSchemasDAO) List(ctx context.Context, catalog string) ([]dbx.Schema, error) {
	return f.ListFn(ctx, catalog)
}

type fakeTablesDAO struct {
	ListFn func(ctx context.Context, catalog, schema string) ([]dbx.Table, error)
	GetFn  func(ctx context.Context, catalog, schema, table string) (dbx.TableDetail, error)
}

func (f fakeTablesDAO) List(ctx context.Context, catalog, schema string) ([]dbx.Table, error) {
	return f.ListFn(ctx, catalog, schema)
}

func (f fakeTablesDAO) Get(ctx context.Context, catalog, schema, table string) (dbx.TableDetail, error) {
	return f.GetFn(ctx, catalog, schema, table)
}

func TestSchemasDefList(t *testing.T) {
	var gotCatalog string
	c := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{
		Schemas: fakeSchemasDAO{ListFn: func(_ context.Context, cat string) ([]dbx.Schema, error) {
			gotCatalog = cat
			return []dbx.Schema{{Name: "silver", Owner: "jon"}}, nil
		}},
	})

	rows, err := SchemasDef{}.List(context.Background(), c, resource.Scope{"catalog": "main"})
	require.NoError(t, err)
	assert.Equal(t, "main", gotCatalog, "catalog comes from scope")
	require.Len(t, rows, 1)
	assert.Equal(t, "silver", rows[0].ID)
}

func TestSchemasDefDrillDown(t *testing.T) {
	d := SchemasDef{}
	assert.Equal(t, []string{"catalog"}, d.Args())
	assert.Equal(t, "tables", d.Child())
	assert.Equal(t,
		resource.Scope{"catalog": "main", "schema": "silver"},
		d.ChildScope(resource.Scope{"catalog": "main"}, resource.Row{ID: "silver"}))
}

func TestTablesDefListAndDescribe(t *testing.T) {
	var gotFull []string
	c := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{
		Tables: fakeTablesDAO{
			ListFn: func(_ context.Context, cat, sch string) ([]dbx.Table, error) {
				return []dbx.Table{{Name: "events", Type: "MANAGED", Format: "DELTA"}}, nil
			},
			GetFn: func(_ context.Context, cat, sch, tbl string) (dbx.TableDetail, error) {
				gotFull = []string{cat, sch, tbl}
				return dbx.TableDetail{
					Table:   dbx.Table{Name: tbl},
					Columns: []dbx.TableColumn{{Name: "id", Type: "bigint", Position: 0}},
				}, nil
			},
		},
	})
	scope := resource.Scope{"catalog": "main", "schema": "silver"}

	rows, err := TablesDef{}.List(context.Background(), c, scope)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "events", rows[0].ID)

	detail, err := TablesDef{}.Describe(context.Background(), c, scope, rows[0])
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "silver", "events"}, gotFull, "describe re-derives the full path from scope")
	assert.Len(t, detail.(dbx.TableDetail).Columns, 1)

	assert.Equal(t, "columns", TablesDef{}.Child())
	assert.Equal(t,
		resource.Scope{"catalog": "main", "schema": "silver", "table": "events"},
		TablesDef{}.ChildScope(scope, rows[0]))
}

func TestColumnsDefList(t *testing.T) {
	c := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{
		Tables: fakeTablesDAO{
			GetFn: func(_ context.Context, cat, sch, tbl string) (dbx.TableDetail, error) {
				return dbx.TableDetail{Columns: []dbx.TableColumn{
					{Name: "id", Type: "bigint", Nullable: false, Position: 0},
					{Name: "ts", Type: "timestamp", Nullable: true, Position: 1},
				}}, nil
			},
		},
	})
	scope := resource.Scope{"catalog": "main", "schema": "silver", "table": "events"}

	rows, err := ColumnsDef{}.List(context.Background(), c, scope)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"0", "id", "bigint", "no", ""}, rows[0].Cells)
	assert.Equal(t, []string{"1", "ts", "timestamp", "yes", ""}, rows[1].Cells)
	assert.Empty(t, ColumnsDef{}.Child(), "columns is the leaf")
}

func TestFullDrillDownRegistered(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"catalogs", "schemas", "tables", "columns"} {
		_, ok := reg.Get(name)
		assert.True(t, ok, name)
	}
	// The chain is wired: catalogs → schemas → tables → columns → leaf.
	assert.Equal(t, "schemas", CatalogsDef{}.Child())

	cmd, err := reg.Parse("tables main.silver /ev")
	require.NoError(t, err)
	assert.Equal(t, resource.Scope{"catalog": "main", "schema": "silver"}, cmd.Scope)
	assert.Equal(t, "ev", cmd.Filter)
}
