package resources

import (
	"context"
	"fmt"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// previewLimit caps table preview queries.
const previewLimit = 200

var tableCols = []resource.ColSpec[dbx.Table]{
	{Column: resource.Column{Title: "NAME"}, Extract: func(t dbx.Table) string { return t.Name }},
	{Column: resource.Column{Title: "TYPE", Width: 16}, Extract: func(t dbx.Table) string { return t.Type }},
	{Column: resource.Column{Title: "FORMAT", Width: 10}, Extract: func(t dbx.Table) string { return t.Format }},
	{Column: resource.Column{Title: "OWNER", Width: 28, Wide: true}, Extract: func(t dbx.Table) string { return t.Owner }},
	{Column: resource.Column{Title: "UPDATED", Width: 18}, Extract: func(t dbx.Table) string { return relTime(t.UpdatedAt) }},
}

// TablesDef browses tables within a schema.
type TablesDef struct{}

func (TablesDef) Name() string                { return "tables" }
func (TablesDef) Aliases() []string           { return []string{"table", "tbl"} }
func (TablesDef) Args() []string              { return []string{"catalog", "schema"} }
func (TablesDef) Columns() []resource.Column  { return resource.Cols(tableCols) }
func (TablesDef) PollInterval() time.Duration { return 30 * time.Second }
func (TablesDef) Child() string               { return "columns" }

func (TablesDef) ChildScope(parent resource.Scope, row resource.Row) resource.Scope {
	return parent.Merge("table", row.ID)
}

func (TablesDef) Actions() []resource.Action {
	return []resource.Action{
		{
			Key:      "x",
			Name:     "query",
			NeedsRow: true,
			Run: func(_ context.Context, _ *dbx.Clients, scope resource.Scope, row resource.Row) any {
				return view.OpenSQLMsg{Query: previewQuery(scope, row.ID), Execute: false}
			},
		},
	}
}

// Tabs implements resource.Tabber: the tab names EnterMsg produces, in order.
func (TablesDef) Tabs() []string { return []string{"columns", "data", "details"} }

// EnterMsg implements resource.Opener: Enter opens the tabbed table view
// (columns │ data │ details) instead of a plain child drill-down.
func (TablesDef) EnterMsg(c *dbx.Clients, scope resource.Scope, row resource.Row) any {
	catalog, schema, table := scope["catalog"], scope["schema"], row.ID
	return view.OpenTabsMsg{
		Title: table,
		Tabs: []view.TabSpec{
			{Name: "columns", Browse: &view.BrowseTabSpec{Resource: "columns", Scope: scope.Merge("table", table)}},
			{Name: "data", SQL: &view.SQLTabSpec{Query: previewQuery(scope, table), Execute: true}},
			{Name: "details", Detail: func(ctx context.Context) (any, error) {
				dao, err := c.Tables()
				if err != nil {
					return nil, err
				}
				return dao.Get(ctx, catalog, schema, table)
			}},
		},
	}
}

// previewQuery builds the SELECT for a table, backtick-quoting identifiers.
func previewQuery(scope resource.Scope, table string) string {
	return fmt.Sprintf("SELECT * FROM `%s`.`%s`.`%s` LIMIT %d",
		scope["catalog"], scope["schema"], table, previewLimit)
}

func (TablesDef) List(ctx context.Context, c *dbx.Clients, scope resource.Scope) ([]resource.Row, error) {
	dao, err := c.Tables()
	if err != nil {
		return nil, err
	}
	items, err := dao.List(ctx, scope["catalog"], scope["schema"])
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(t dbx.Table) string { return t.Name }, tableCols), nil
}

// Describe fetches the full table detail (columns, properties).
func (TablesDef) Describe(ctx context.Context, c *dbx.Clients, scope resource.Scope, row resource.Row) (any, error) {
	dao, err := c.Tables()
	if err != nil {
		return nil, err
	}
	return dao.Get(ctx, scope["catalog"], scope["schema"], row.ID)
}
