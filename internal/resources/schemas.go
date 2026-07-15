package resources

import (
	"context"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

var schemaCols = []resource.ColSpec[dbx.Schema]{
	{Column: resource.Column{Title: "NAME"}, Extract: func(s dbx.Schema) string { return s.Name }},
	{Column: resource.Column{Title: "OWNER", Width: 28}, Extract: func(s dbx.Schema) string { return s.Owner }},
	{Column: resource.Column{Title: "UPDATED", Width: 18}, Extract: func(s dbx.Schema) string { return relTime(s.UpdatedAt) }},
	{Column: resource.Column{Title: "COMMENT", Wide: true}, Extract: func(s dbx.Schema) string { return dbx.OneLine(s.Comment) }},
}

// SchemasDef browses schemas within a catalog.
type SchemasDef struct{}

func (SchemasDef) Name() string                { return "schemas" }
func (SchemasDef) Aliases() []string           { return []string{"schema", "sch"} }
func (SchemasDef) Args() []string              { return []string{"catalog"} }
func (SchemasDef) Columns() []resource.Column  { return resource.Cols(schemaCols) }
func (SchemasDef) PollInterval() time.Duration { return 30 * time.Second }
func (SchemasDef) Child() string               { return "tables" }

func (SchemasDef) ChildScope(parent resource.Scope, row resource.Row) resource.Scope {
	return parent.Merge("schema", row.ID)
}

func (SchemasDef) Actions() []resource.Action { return nil }

func (SchemasDef) List(ctx context.Context, c *dbx.Clients, scope resource.Scope) ([]resource.Row, error) {
	dao, err := c.Schemas()
	if err != nil {
		return nil, err
	}
	items, err := dao.List(ctx, scope["catalog"])
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(s dbx.Schema) string { return s.Name }, schemaCols), nil
}

func (SchemasDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}
