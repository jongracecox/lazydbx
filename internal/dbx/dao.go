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

// Warehouse is a SQL warehouse, reduced to what selection and display need.
type Warehouse struct {
	ID         string `yaml:"id" json:"id"`
	Name       string `yaml:"name" json:"name"`
	State      string `yaml:"state" json:"state"` // RUNNING, STOPPED, STARTING, ...
	Size       string `yaml:"size" json:"size"`
	Serverless bool   `yaml:"serverless" json:"serverless"`
}

// WarehousesDAO lists SQL warehouses.
type WarehousesDAO interface {
	List(ctx context.Context) ([]Warehouse, error)
}

// StmtColumn describes one column of a statement result.
type StmtColumn struct {
	Name string
	Type string
}

// StmtResult is a decoded INLINE statement result.
type StmtResult struct {
	Columns []StmtColumn
	Rows    [][]string
	// Truncated is set when the server returned more data than the first
	// chunk / row limit — the UI shows a "showing first N rows" banner.
	Truncated bool
}

// Statement lifecycle states as returned by StatementPoll.State.
const (
	StmtPending   = "PENDING"
	StmtRunning   = "RUNNING"
	StmtSucceeded = "SUCCEEDED"
	StmtFailed    = "FAILED"
	StmtCanceled  = "CANCELED"
	StmtClosed    = "CLOSED"
)

// StatementPoll is one observation of an executing statement.
type StatementPoll struct {
	State   string
	Result  *StmtResult // non-nil when State == StmtSucceeded
	Message string      // error detail when State == StmtFailed
}

// StatementDAO executes SQL asynchronously: Submit returns immediately with
// a statement ID; Poll observes progress; Cancel aborts.
type StatementDAO interface {
	Submit(ctx context.Context, warehouseID, statement string, rowLimit int) (string, error)
	Poll(ctx context.Context, statementID string) (StatementPoll, error)
	Cancel(ctx context.Context, statementID string) error
}
