package resources

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
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

func TestTablesDefQueryAction(t *testing.T) {
	scope := resource.Scope{"catalog": "main", "schema": "silver"}
	actions := TablesDef{}.Actions()
	require.Len(t, actions, 1)

	c := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test", Host: "https://ws"}, dbx.DAOs{})
	query := actions[0]
	assert.Equal(t, "x", query.Key)
	assert.True(t, query.NeedsRow)
	msg := query.Run(context.Background(), c, scope, resource.Row{ID: "events"})
	open, ok := msg.(view.OpenSQLMsg)
	require.True(t, ok)
	assert.Equal(t, "SELECT * FROM `main`.`silver`.`events` LIMIT 200", open.Query)
	assert.False(t, open.Execute, "query opens the editor without executing")
	assert.Equal(t, "https://ws/explore/data/main/silver/events?activeTab=sampleData", open.Web.URL,
		"`o` on the preview opens the table's sample data")
}

func TestTablesDefEnterOpensTabbedView(t *testing.T) {
	var gotFull []string
	c := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{
		Tables: fakeTablesDAO{
			GetFn: func(_ context.Context, cat, sch, tbl string) (dbx.TableDetail, error) {
				gotFull = []string{cat, sch, tbl}
				return dbx.TableDetail{
					Table:      dbx.Table{Name: tbl},
					Columns:    []dbx.TableColumn{{Name: "id"}, {Name: "ts"}},
					Properties: map[string]string{"pipeline.drop_and_recreate": "true"},
				}, nil
			},
		},
	})
	scope := resource.Scope{"catalog": "main", "schema": "silver"}

	msg := TablesDef{}.EnterMsg(c, scope, resource.Row{ID: "events"})
	open, ok := msg.(view.OpenTabsMsg)
	require.True(t, ok)
	assert.Equal(t, "events", open.Title)
	require.Len(t, open.Tabs, 4)

	assert.Equal(t, "columns", open.Tabs[0].Name)
	assert.Equal(t, resource.Scope{"catalog": "main", "schema": "silver", "table": "events"}, open.Tabs[0].Browse.Scope)
	assert.Equal(t, "data", open.Tabs[1].Name)
	assert.Equal(t, "SELECT * FROM `main`.`silver`.`events` LIMIT 200", open.Tabs[1].SQL.Query)
	assert.True(t, open.Tabs[1].SQL.Execute)

	assert.Equal(t, "details", open.Tabs[2].Name)
	detail, err := open.Tabs[2].Detail(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"main", "silver", "events"}, gotFull, "detail fetch binds the full path")
	summary, ok := detail.(tableSummary)
	require.True(t, ok, "the details tab shows a summary, not the raw TableDetail")
	assert.Equal(t, "events", summary.Name)
	assert.Equal(t, "main", summary.Catalog)
	assert.Equal(t, "silver", summary.Schema)
	assert.Equal(t, 2, summary.Columns, "columns are counted here and listed on their own tab")
	assert.Equal(t, 1, summary.Properties, "properties are counted here and listed on their own tab")

	assert.Equal(t, "properties", open.Tabs[3].Name)
	nodes, err := open.Tabs[3].Tree.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, nodes, 1)
	assert.Equal(t, "pipeline", nodes[0].Label, "dotted keys nest: pipeline → drop_and_recreate")
	require.Len(t, nodes[0].Children, 1)
	assert.Equal(t, "drop_and_recreate", nodes[0].Children[0].Label)
	assert.Equal(t, "true", nodes[0].Children[0].Value)

	assert.Equal(t, []string{"columns", "data", "details", "properties"}, TablesDef{}.Tabs(),
		"Tabber must match the tabs EnterMsg builds, so --tab validates")
}

func TestPropTree(t *testing.T) {
	nodes := propTree(map[string]string{
		"spark.sql.statistics.colStats.id.avgLen": "8",
		"pipeline.drop_and_recreate":              "true",
		"delta.minReaderVersion":                  "3",
		"delta.feature.appendOnly":                "supported",
		"bare":                                    "yes",
	})

	// Top-level branches sort alphabetically, so the generated spark.* block
	// (one entry per column per statistic) always lands last.
	labels := make([]string, len(nodes))
	for i, n := range nodes {
		labels[i] = n.Label
	}
	assert.Equal(t, []string{"bare", "delta", "pipeline", "spark"}, labels)

	assert.Equal(t, "yes", nodes[0].Value, "an undotted key is a root leaf")
	assert.Empty(t, nodes[0].Children)

	delta := nodes[1]
	assert.Empty(t, delta.Value, "a pure branch carries no value of its own")
	require.Len(t, delta.Children, 2)
	assert.Equal(t, "feature", delta.Children[0].Label)
	assert.Equal(t, "appendOnly", delta.Children[0].Children[0].Label)
	assert.Equal(t, "supported", delta.Children[0].Children[0].Value)
	assert.Equal(t, "minReaderVersion", delta.Children[1].Label)
	assert.Equal(t, "3", delta.Children[1].Value)
}

func TestPropTreeEmpty(t *testing.T) {
	assert.Empty(t, propTree(nil))
}

func TestPropTreeAnnotatesEpochTimestamps(t *testing.T) {
	nodes := propTree(map[string]string{
		"delta.lastCommitTimestamp": "1785273330000",
		"delta.lastUpdateVersion":   "6215",
	})
	delta := nodes[0]
	require.Len(t, delta.Children, 2)

	want := time.UnixMilli(1785273330000).Local().Format(epochTimeFormat)
	assert.Equal(t, "lastCommitTimestamp", delta.Children[0].Label)
	assert.Equal(t, "1785273330000", delta.Children[0].Value, "the raw value is kept")
	assert.Equal(t, want, delta.Children[0].Note, "and glossed with the decoded time")

	assert.Equal(t, "lastUpdateVersion", delta.Children[1].Label)
	assert.Empty(t, delta.Children[1].Note, "a version number is not a timestamp")
}

func TestEpochNote(t *testing.T) {
	// Reference instant, in each precision the decoder accepts.
	ref := time.UnixMilli(1785273330123)
	want := ref.Local().Format(epochTimeFormat)

	cases := []struct {
		name, label, value, want string
	}{
		{"millis", "createdAt", "1785273330123", want},
		{"seconds", "created_at", "1785273330", ref.Truncate(time.Second).Local().Format(epochTimeFormat)},
		{"micros", "updatedAt", "1785273330123456", want},
		{"nanos", "deleted_at", "1785273330123456789", want},
		{"name says timestamp", "lastCommitTimestamp", "1785273330123", want},
		{"name says time", "loadTime", "1785273330123", want},
		{"name says date", "snapshotDate", "1785273330123", want},
		{"name says modified", "lastModified", "1785273330123", want},

		{"unrelated name", "minReaderVersion", "1785273330123", ""},
		{"format is not an _at suffix", "format", "1785273330123", ""},
		{"non-numeric value", "createdAt", "2026-07-28", ""},
		{"quoted column name", "deltaFileStatistics", "`load_date`", ""},
		{"too few digits", "createdAt", "6215", ""},
		{"11 digits is no precision", "createdAt", "17852733301", ""},
		{"out of range low", "createdAt", "0000000001000", ""},
		{"out of range high", "createdAt", "9999999999999", ""},
		{"zero", "createdAt", "0000000000", ""},
		{"empty", "createdAt", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, epochNote(tc.label, tc.value))
		})
	}
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
