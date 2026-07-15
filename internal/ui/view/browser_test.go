package view

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/favorites"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/theme"
)

// crumbDef is a stub def for title/scope-path tests.
type crumbDef struct {
	name string
	args []string
	cols []resource.Column
}

func (d crumbDef) Name() string                                               { return d.name }
func (d crumbDef) Aliases() []string                                          { return nil }
func (d crumbDef) Args() []string                                             { return d.args }
func (d crumbDef) Columns() []resource.Column                                 { return d.cols }
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
	return NewBrowser(def, scope, clients, eng, theme.Default(), "", nil)
}

func TestBrowserFavorites(t *testing.T) {
	def := crumbDef{name: "jobs"}
	clients := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{})
	eng := engine.New(func(engine.DataEvent) {}, nil)
	favs := favorites.NewStore(filepath.Join(t.TempDir(), "f.json"))
	b := NewBrowser(def, resource.Scope{}, clients, eng, theme.Default(), "", favs)
	b.Render(100, 20) // size the table

	b.applyData(engine.DataEvent{
		Key: b.key,
		Rows: []resource.Row{
			{ID: "alpha", Cells: []string{"alpha"}},
			{ID: "beta", Cells: []string{"beta"}},
			{ID: "gamma", Cells: []string{"gamma"}},
		},
	})

	// Cursor starts on alpha; move to beta and star it.
	_, _ = b.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	_, cmd := b.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	require.NotNil(t, cmd)
	flash := cmd().(FlashMsg)
	assert.Contains(t, flash.Text, "favorited beta")

	// beta floats to the top with the star marker; others keep order.
	row, ok := b.table.SelectedRow()
	require.True(t, ok)
	assert.Equal(t, "beta", row.ID, "cursor follows the starred row by ID")
	first, _ := b.table.SelectedRow()
	assert.Equal(t, favMarker, first.Cells[0])
	assert.True(t, favs.IsFavorite("test", "jobs|", "beta"), "persisted")

	// Unstar restores original order.
	_, cmd = b.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	assert.Contains(t, cmd().(FlashMsg).Text, "unfavorited beta")
	assert.False(t, favs.IsFavorite("test", "jobs|", "beta"))
}

func TestBrowserSkipsStaleRowsWithWrongArity(t *testing.T) {
	// def with 2 columns; cached rows carry 1 cell (older schema).
	def := crumbDef{name: "jobs", cols: []resource.Column{{Title: "A"}, {Title: "B"}}}
	b := newTestBrowser(def, resource.Scope{})
	b.Render(100, 20)

	b.applyData(engine.DataEvent{Key: b.key, Stale: true, Rows: []resource.Row{{ID: "old", Cells: []string{"only-one"}}}})
	assert.False(t, b.loaded, "mismatched stale cache is not rendered")

	b.applyData(engine.DataEvent{Key: b.key, Rows: []resource.Row{{ID: "new", Cells: []string{"a", "b"}}}})
	assert.True(t, b.loaded, "fresh data with the right arity renders")
	assert.Len(t, b.allRows, 1)
	assert.Equal(t, "new", b.allRows[0].ID)
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
