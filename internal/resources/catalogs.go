// Package resources holds the concrete resource definitions — the only code
// that calls DAOs. One file per resource; wire new defs in register.go.
package resources

import (
	"context"
	"strconv"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

var catalogCols = []resource.ColSpec[dbx.Catalog]{
	{Column: resource.Column{Title: "NAME"}, Extract: func(c dbx.Catalog) string { return c.Name }},
	{Column: resource.Column{Title: "TYPE", Width: 18}, Extract: func(c dbx.Catalog) string { return c.Type }},
	{Column: resource.Column{Title: "OWNER", Width: 28}, Extract: func(c dbx.Catalog) string { return c.Owner }},
	{Column: resource.Column{Title: "UPDATED", Width: 18}, Extract: func(c dbx.Catalog) string { return relTime(c.UpdatedAt) }},
	{Column: resource.Column{Title: "COMMENT", Wide: true}, Extract: func(c dbx.Catalog) string { return dbx.OneLine(c.Comment) }},
}

// CatalogsDef browses Unity Catalog catalogs.
type CatalogsDef struct{}

func (CatalogsDef) Name() string                { return "catalogs" }
func (CatalogsDef) Aliases() []string           { return []string{"catalog", "cat"} }
func (CatalogsDef) Args() []string              { return nil }
func (CatalogsDef) Columns() []resource.Column  { return resource.Cols(catalogCols) }
func (CatalogsDef) PollInterval() time.Duration { return 30 * time.Second }

func (CatalogsDef) Child() string { return "schemas" }

func (CatalogsDef) ChildScope(parent resource.Scope, row resource.Row) resource.Scope {
	return parent.Merge("catalog", row.ID)
}

func (CatalogsDef) Actions() []resource.Action { return nil }

func (CatalogsDef) List(ctx context.Context, c *dbx.Clients, _ resource.Scope) ([]resource.Row, error) {
	dao, err := c.Catalogs()
	if err != nil {
		return nil, err
	}
	items, err := dao.List(ctx)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(c dbx.Catalog) string { return c.Name }, catalogCols), nil
}

func (CatalogsDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}

// relTime renders a timestamp as a compact age like "3d" or "2h".
func relTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return strconv.Itoa(int(d.Minutes())) + "m"
	case d < 24*time.Hour:
		return strconv.Itoa(int(d.Hours())) + "h"
	default:
		return strconv.Itoa(int(d.Hours()/24)) + "d"
	}
}
