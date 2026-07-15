package dbx

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/catalog"
)

// sortByName orders any named slice alphabetically, case-insensitive — the
// UC APIs return arbitrary order and browsing wants stable, scannable lists.
func sortByName[T any](items []T, name func(T) string) {
	sort.Slice(items, func(i, j int) bool {
		return strings.ToLower(name(items[i])) < strings.ToLower(name(items[j]))
	})
}

// catalogsDAO implements CatalogsDAO against the SDK. Implementations stay
// thin: pagination and type mapping only, no business logic.
type catalogsDAO struct {
	w *databricks.WorkspaceClient
}

func (d catalogsDAO) List(ctx context.Context) ([]Catalog, error) {
	infos, err := d.w.Catalogs.ListAll(ctx, catalog.ListCatalogsRequest{})
	if err != nil {
		return nil, fmt.Errorf("listing catalogs: %w", err)
	}
	out := make([]Catalog, 0, len(infos))
	for i := range infos {
		ci := &infos[i]
		out = append(out, Catalog{
			Name:      ci.Name,
			Owner:     ci.Owner,
			Type:      string(ci.CatalogType),
			Comment:   ci.Comment,
			CreatedAt: millisToTime(ci.CreatedAt),
			UpdatedAt: millisToTime(ci.UpdatedAt),
		})
	}
	sortByName(out, func(c Catalog) string { return c.Name })
	return out, nil
}

type schemasDAO struct {
	w *databricks.WorkspaceClient
}

func (d schemasDAO) List(ctx context.Context, cat string) ([]Schema, error) {
	infos, err := d.w.Schemas.ListAll(ctx, catalog.ListSchemasRequest{CatalogName: cat})
	if err != nil {
		return nil, fmt.Errorf("listing schemas in %s: %w", cat, err)
	}
	out := make([]Schema, 0, len(infos))
	for i := range infos {
		si := &infos[i]
		out = append(out, Schema{
			Name:      si.Name,
			Owner:     si.Owner,
			Comment:   si.Comment,
			CreatedAt: millisToTime(si.CreatedAt),
			UpdatedAt: millisToTime(si.UpdatedAt),
		})
	}
	sortByName(out, func(s Schema) string { return s.Name })
	return out, nil
}

type tablesDAO struct {
	w *databricks.WorkspaceClient
}

func (d tablesDAO) List(ctx context.Context, cat, schema string) ([]Table, error) {
	infos, err := d.w.Tables.ListAll(ctx, catalog.ListTablesRequest{
		CatalogName: cat,
		SchemaName:  schema,
		OmitColumns: true,
	})
	if err != nil {
		return nil, fmt.Errorf("listing tables in %s.%s: %w", cat, schema, err)
	}
	out := make([]Table, 0, len(infos))
	for i := range infos {
		out = append(out, tableFromInfo(&infos[i]))
	}
	sortByName(out, func(t Table) string { return t.Name })
	return out, nil
}

func (d tablesDAO) Get(ctx context.Context, cat, schema, table string) (TableDetail, error) {
	full := cat + "." + schema + "." + table
	info, err := d.w.Tables.GetByFullName(ctx, full)
	if err != nil {
		return TableDetail{}, fmt.Errorf("describing table %s: %w", full, err)
	}
	detail := TableDetail{Table: tableFromInfo(info), Properties: info.Properties}
	for i := range info.Columns {
		col := &info.Columns[i]
		pos := col.Position
		if pos == 0 {
			pos = i
		}
		detail.Columns = append(detail.Columns, TableColumn{
			Name:     col.Name,
			Type:     col.TypeText,
			Nullable: col.Nullable,
			Comment:  col.Comment,
			Position: pos,
		})
	}
	sort.Slice(detail.Columns, func(i, j int) bool { return detail.Columns[i].Position < detail.Columns[j].Position })
	return detail, nil
}

func tableFromInfo(ti *catalog.TableInfo) Table {
	return Table{
		Name:      ti.Name,
		Type:      string(ti.TableType),
		Format:    string(ti.DataSourceFormat),
		Owner:     ti.Owner,
		Comment:   ti.Comment,
		UpdatedAt: millisToTime(ti.UpdatedAt),
	}
}

func millisToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
