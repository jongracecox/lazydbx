package resources

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
)

// fakeCatalogsDAO is a struct of func fields — the house pattern for faking
// DAOs (never the SDK's generated mocks).
type fakeCatalogsDAO struct {
	ListFn func(ctx context.Context) ([]dbx.Catalog, error)
}

func (f fakeCatalogsDAO) List(ctx context.Context) ([]dbx.Catalog, error) {
	return f.ListFn(ctx)
}

func clientsWith(dao dbx.CatalogsDAO) *dbx.Clients {
	return dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{Catalogs: dao})
}

func TestCatalogsDefList(t *testing.T) {
	updated := time.Now().Add(-3 * time.Hour)
	c := clientsWith(fakeCatalogsDAO{
		ListFn: func(context.Context) ([]dbx.Catalog, error) {
			return []dbx.Catalog{
				{Name: "main", Type: "MANAGED_CATALOG", Owner: "jon@example.com", UpdatedAt: updated, Comment: "prod data"},
				{Name: "dev", Type: "MANAGED_CATALOG", Owner: "sam@example.com"},
			}, nil
		},
	})

	rows, err := CatalogsDef{}.List(context.Background(), c, resource.Scope{})
	require.NoError(t, err)
	require.Len(t, rows, 2)

	assert.Equal(t, "main", rows[0].ID)
	assert.Equal(t, []string{"main", "MANAGED_CATALOG", "jon@example.com", "3h", "prod data"}, rows[0].Cells)
	assert.Empty(t, rows[1].Cells[3], "zero time renders empty")

	// Row.Data keeps the domain object for describe.
	got, err := CatalogsDef{}.Describe(context.Background(), c, resource.Scope{}, rows[0])
	require.NoError(t, err)
	assert.Equal(t, "main", got.(dbx.Catalog).Name)
}

func TestCatalogsDefListError(t *testing.T) {
	c := clientsWith(fakeCatalogsDAO{
		ListFn: func(context.Context) ([]dbx.Catalog, error) {
			return nil, errors.New("permission denied")
		},
	})

	_, err := CatalogsDef{}.List(context.Background(), c, resource.Scope{})
	assert.ErrorContains(t, err, "permission denied")
}

func TestCatalogsDefShape(t *testing.T) {
	d := CatalogsDef{}
	assert.Equal(t, "catalogs", d.Name())
	assert.Contains(t, d.Aliases(), "cat")
	assert.Empty(t, d.Args(), "catalogs is unscoped")
	assert.Equal(t, "schemas", d.Child())
	assert.Equal(t, resource.Scope{"catalog": "main"},
		d.ChildScope(resource.Scope{}, resource.Row{ID: "main"}))
	assert.Len(t, d.Columns(), 5)
}

func TestRegistryWiring(t *testing.T) {
	reg := NewRegistry()
	cmd, err := reg.Parse("cat")
	require.NoError(t, err)
	assert.Equal(t, "catalogs", cmd.Def.Name())
}
