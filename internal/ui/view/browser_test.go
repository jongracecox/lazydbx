package view

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
)

// crumbDef is a stub def for title/scope-path tests.
type crumbDef struct {
	name string
	args []string
}

func (d crumbDef) Name() string                                               { return d.name }
func (d crumbDef) Aliases() []string                                          { return nil }
func (d crumbDef) Args() []string                                             { return d.args }
func (d crumbDef) Columns() []resource.Column                                 { return nil }
func (d crumbDef) PollInterval() time.Duration                                { return time.Hour }
func (d crumbDef) Child() string                                              { return "" }
func (d crumbDef) ChildScope(p resource.Scope, _ resource.Row) resource.Scope { return p }
func (d crumbDef) Actions() []resource.Action                                 { return nil }

func (d crumbDef) List(context.Context, *dbx.Clients, resource.Scope) ([]resource.Row, error) {
	return nil, nil
}

func (d crumbDef) Describe(context.Context, *dbx.Clients, resource.Scope, resource.Row) (any, error) {
	return nil, nil
}

func newTestBrowser(def resource.Def, scope resource.Scope) *Browser {
	clients := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{})
	eng := engine.New(func(engine.DataEvent) {}, nil)
	return NewBrowser(def, scope, clients, eng, theme.Default(), "")
}

func TestBrowserTitleAndScopePath(t *testing.T) {
	tests := []struct {
		name      string
		def       crumbDef
		scope     resource.Scope
		wantTitle string
	}{
		{
			name:      "root resource uses its name",
			def:       crumbDef{name: "catalogs"},
			scope:     resource.Scope{},
			wantTitle: "catalogs",
		},
		{
			name:      "scoped view titled by selected item",
			def:       crumbDef{name: "schemas", args: []string{"catalog"}},
			scope:     resource.Scope{"catalog": "qsic_internal"},
			wantTitle: "qsic_internal",
		},
		{
			name:      "deep scope titled by deepest item",
			def:       crumbDef{name: "columns", args: []string{"catalog", "schema", "table"}},
			scope:     resource.Scope{"catalog": "dev_v2", "schema": "20_gold", "table": "accounts"},
			wantTitle: "accounts",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTestBrowser(tt.def, tt.scope)
			assert.Equal(t, tt.wantTitle, b.Title())
		})
	}
}
