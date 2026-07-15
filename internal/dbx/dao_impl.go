package dbx

import (
	"context"
	"fmt"
	"time"

	"github.com/databricks/databricks-sdk-go"
	"github.com/databricks/databricks-sdk-go/service/catalog"
)

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
	return out, nil
}

func millisToTime(ms int64) time.Time {
	if ms == 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms).UTC()
}
