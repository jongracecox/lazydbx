package dbx

import (
	"context"
	"time"
)

// The DAO interfaces are deliberately narrow: each declares only what the
// corresponding resource def consumes, and they traffic in dbx-owned domain
// types rather than SDK structs. This is the insulation layer that keeps
// pre-1.0 SDK breaking changes contained to dao_impl.go, and it is what
// resource tests fake (a struct of func fields — never the SDK's
// experimental mocks).

// Catalog is a Unity Catalog catalog, reduced to what the UI shows.
type Catalog struct {
	Name      string    `yaml:"name"`
	Owner     string    `yaml:"owner"`
	Type      string    `yaml:"type"`
	Comment   string    `yaml:"comment,omitempty"`
	CreatedAt time.Time `yaml:"created_at,omitempty"`
	UpdatedAt time.Time `yaml:"updated_at,omitempty"`
}

// CatalogsDAO lists Unity Catalog catalogs.
type CatalogsDAO interface {
	List(ctx context.Context) ([]Catalog, error)
}

// Schema is a Unity Catalog schema.
type Schema struct {
	Name      string    `yaml:"name" json:"name"`
	Owner     string    `yaml:"owner" json:"owner"`
	Comment   string    `yaml:"comment,omitempty" json:"comment,omitempty"`
	CreatedAt time.Time `yaml:"created_at,omitempty" json:"created_at,omitempty"`
	UpdatedAt time.Time `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// SchemasDAO lists schemas within a catalog.
type SchemasDAO interface {
	List(ctx context.Context, catalog string) ([]Schema, error)
}

// Table is a Unity Catalog table (columns omitted for list speed).
type Table struct {
	Name      string    `yaml:"name" json:"name"`
	Type      string    `yaml:"type" json:"type"`
	Format    string    `yaml:"format,omitempty" json:"format,omitempty"`
	Owner     string    `yaml:"owner" json:"owner"`
	Comment   string    `yaml:"comment,omitempty" json:"comment,omitempty"`
	UpdatedAt time.Time `yaml:"updated_at,omitempty" json:"updated_at,omitempty"`
}

// TableColumn is one column of a table.
type TableColumn struct {
	Name     string `yaml:"name" json:"name"`
	Type     string `yaml:"type" json:"type"`
	Nullable bool   `yaml:"nullable" json:"nullable"`
	Comment  string `yaml:"comment,omitempty" json:"comment,omitempty"`
	Position int    `yaml:"position" json:"position"`
}

// TableDetail is a full table description, used by the columns view and
// table describe.
type TableDetail struct {
	Table      `yaml:",inline" json:"table"`
	Columns    []TableColumn     `yaml:"columns" json:"columns"`
	Properties map[string]string `yaml:"properties,omitempty" json:"properties,omitempty"`
}

// TablesDAO lists tables within a schema and fetches full table detail.
type TablesDAO interface {
	List(ctx context.Context, catalog, schema string) ([]Table, error)
	Get(ctx context.Context, catalog, schema, table string) (TableDetail, error)
}
