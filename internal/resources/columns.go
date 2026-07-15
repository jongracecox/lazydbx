package resources

import (
	"context"
	"strconv"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

var columnCols = []resource.ColSpec[dbx.TableColumn]{
	{Column: resource.Column{Title: "#", Width: 4}, Extract: func(c dbx.TableColumn) string { return strconv.Itoa(c.Position) }},
	{Column: resource.Column{Title: "NAME"}, Extract: func(c dbx.TableColumn) string { return c.Name }},
	{Column: resource.Column{Title: "TYPE", Width: 24}, Extract: func(c dbx.TableColumn) string { return c.Type }},
	{Column: resource.Column{Title: "NULLABLE", Width: 8}, Extract: func(c dbx.TableColumn) string {
		if c.Nullable {
			return "yes"
		}
		return "no"
	}},
	{Column: resource.Column{Title: "COMMENT", Wide: true}, Extract: func(c dbx.TableColumn) string { return c.Comment }},
}

// ColumnsDef browses the columns of one table — the leaf of the Unity
// Catalog drill-down.
type ColumnsDef struct{}

func (ColumnsDef) Name() string                { return "columns" }
func (ColumnsDef) Aliases() []string           { return []string{"column", "cols", "col"} }
func (ColumnsDef) Args() []string              { return []string{"catalog", "schema", "table"} }
func (ColumnsDef) Columns() []resource.Column  { return resource.Cols(columnCols) }
func (ColumnsDef) PollInterval() time.Duration { return time.Minute }
func (ColumnsDef) Child() string               { return "" }

func (ColumnsDef) ChildScope(parent resource.Scope, _ resource.Row) resource.Scope {
	return parent
}

func (ColumnsDef) Actions() []resource.Action { return nil }

func (ColumnsDef) List(ctx context.Context, c *dbx.Clients, scope resource.Scope) ([]resource.Row, error) {
	dao, err := c.Tables()
	if err != nil {
		return nil, err
	}
	detail, err := dao.Get(ctx, scope["catalog"], scope["schema"], scope["table"])
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(detail.Columns, func(c dbx.TableColumn) string { return c.Name }, columnCols), nil
}

func (ColumnsDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}
