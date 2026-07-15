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
