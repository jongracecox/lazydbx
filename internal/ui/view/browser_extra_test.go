package view

import (
	"context"
	"testing"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/engine"
	"github.com/jongracecox/lazydbx/internal/resource"
)

// childDef is a crumbDef that drills down to a child resource.
type childDef struct {
	crumbDef
	child string
}

func (d childDef) Child() string { return d.child }
func (d childDef) ChildScope(p resource.Scope, row resource.Row) resource.Scope {
	next := resource.Scope{}
	for k, v := range p {
		next[k] = v
	}
	next[d.name] = row.ID
	return next
}

// webDef implements WebLinker + AltWebLinker.
type webDef struct {
	crumbDef
	ok    bool
	altOk bool
}

func (d webDef) WebURL(_ string, _ resource.Scope, row resource.Row) (string, bool) {
	return "https://example/" + row.ID, d.ok
}

func (d webDef) AltWebURL(_ string, _ resource.Scope, row resource.Row) (string, bool) {
	return "https://alt/" + row.ID, d.altOk
}
func (d webDef) AltWebHint() string { return "open app" }

// actionDef exposes one action.
type actionDef struct {
	crumbDef
	ran *bool
}

type actionRanMsg struct{}

func (d actionDef) Actions() []resource.Action {
	return []resource.Action{{
		Key: "x", Name: "do-x", NeedsRow: true,
		Run: func(context.Context, *dbx.Clients, resource.Scope, resource.Row) any {
			*d.ran = true
			return actionRanMsg{}
		},
	}}
}

func withRows(t *testing.T, b *Browser, ids ...string) {
	t.Helper()
	b.Render(100, 20)
	rows := make([]resource.Row, len(ids))
	for i, id := range ids {
		rows[i] = resource.Row{ID: id, Cells: []string{id}}
	}
	b.applyData(engine.DataEvent{Key: b.key, Rows: rows})
}

func col() []resource.Column { return []resource.Column{{Title: "NAME"}} }

// --- lifecycle / interface methods ---

func TestBrowserInitAndClose(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	cmd := b.Init()
	require.NotNil(t, cmd)
	assert.Nil(t, cmd(), "Init subscribes and returns no follow-up message")
	b.Close() // must not panic
}

func TestBrowserCapturesKeys(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	assert.False(t, b.CapturesKeys(), "idle browser yields keys to global shortcuts")

	_, _ = b.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	assert.True(t, b.CapturesKeys(), "filtering captures keys")
}

func TestBrowserHintsVaryByCapability(t *testing.T) {
	// Plain def with a child: enter drills into the child.
	child := childDef{crumbDef{name: "catalogs", cols: col()}, "schemas"}
	hints := hintKeys(newTestBrowser(child, resource.Scope{}).Hints())
	assert.Contains(t, hints, "enter")
	assert.Contains(t, hints, "d")
	assert.Contains(t, hints, "r")

	// WebLinker/AltWebLinker add o/O.
	web := newTestBrowser(webDef{crumbDef: crumbDef{name: "apps", cols: col()}, ok: true, altOk: true}, resource.Scope{})
	wk := hintKeys(web.Hints())
	assert.Contains(t, wk, "o")
	assert.Contains(t, wk, "O")

	// Tagger adds t.
	tagged := newTestBrowser(taggedDef{crumbDef: crumbDef{name: "jobs", cols: col()}}, resource.Scope{})
	assert.Contains(t, hintKeys(tagged.Hints()), "t")
}

func hintKeys(binds []key.Binding) []string {
	out := make([]string, len(binds))
	for i, b := range binds {
		out[i] = b.Help().Key
	}
	return out
}

// --- navigation keys ---

func TestBrowserEnterDrillsDown(t *testing.T) {
	b := newTestBrowser(childDef{crumbDef{name: "catalogs", cols: col()}, "schemas"}, resource.Scope{})
	withRows(t, b, "main", "dev")

	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd)
	drill, ok := cmd().(DrillDownMsg)
	require.True(t, ok)
	assert.Equal(t, "schemas", drill.Resource)
	assert.Equal(t, "main", drill.Scope["catalogs"])
}

func TestBrowserEnterLeafDescribes(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "columns", cols: col()}, resource.Scope{})
	withRows(t, b, "id")

	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	require.NotNil(t, cmd, "a leaf's Enter falls back to describe")
	_, ok := cmd().(PushMsg)
	assert.True(t, ok)
}

func TestBrowserDescribeKey(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	withRows(t, b, "a")

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'd', Text: "d"})
	require.NotNil(t, cmd)
	push, ok := cmd().(PushMsg)
	require.True(t, ok)
	assert.NotNil(t, push.View)
}

func TestBrowserRefreshKey(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	_, cmd := b.Update(tea.KeyPressMsg{Code: 'r', Text: "r"})
	require.NotNil(t, cmd)
	flash, ok := cmd().(FlashMsg)
	require.True(t, ok)
	assert.Contains(t, flash.Text, "refresh")
}

func TestBrowserEscPopsWhenClean(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	require.NotNil(t, cmd)
	_, ok := cmd().(PopMsg)
	assert.True(t, ok)
}

func TestBrowserEscClearsFilterFirst(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	withRows(t, b, "alpha", "beta")
	b.filterVal = "alpha"
	b.refreshTable()
	require.Equal(t, 1, b.table.Len())

	_, cmd := b.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	assert.Nil(t, cmd, "esc clears the filter instead of popping")
	assert.Empty(t, b.filterVal)
	assert.Equal(t, 2, b.table.Len())
}

func TestBrowserOpenWeb(t *testing.T) {
	b := newTestBrowser(webDef{crumbDef: crumbDef{name: "apps", cols: col()}, ok: true}, resource.Scope{})
	withRows(t, b, "my-app")

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	require.NotNil(t, cmd)
	open, ok := cmd().(OpenURLMsg)
	require.True(t, ok)
	assert.Contains(t, open.URL, "my-app")
}

func TestBrowserOpenWebNoLink(t *testing.T) {
	b := newTestBrowser(webDef{crumbDef: crumbDef{name: "apps", cols: col()}, ok: false}, resource.Scope{})
	withRows(t, b, "my-app")

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'o', Text: "o"})
	require.NotNil(t, cmd)
	flash, ok := cmd().(FlashMsg)
	require.True(t, ok)
	assert.Contains(t, flash.Text, "no web link")
}

func TestBrowserOpenAltWeb(t *testing.T) {
	b := newTestBrowser(webDef{crumbDef: crumbDef{name: "apps", cols: col()}, altOk: true}, resource.Scope{})
	withRows(t, b, "my-app")

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'O', Text: "O"})
	require.NotNil(t, cmd)
	open, ok := cmd().(OpenURLMsg)
	require.True(t, ok)
	assert.Contains(t, open.URL, "alt/my-app")
}

func TestBrowserRunAction(t *testing.T) {
	ran := false
	b := newTestBrowser(actionDef{crumbDef: crumbDef{name: "jobs", cols: col()}, ran: &ran}, resource.Scope{})
	withRows(t, b, "a")

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	require.NotNil(t, cmd)
	_, ok := cmd().(actionRanMsg)
	assert.True(t, ok)
	assert.True(t, ran, "action ran")
}

func TestBrowserActionNeedsRowNoop(t *testing.T) {
	ran := false
	b := newTestBrowser(actionDef{crumbDef: crumbDef{name: "jobs", cols: col()}, ran: &ran}, resource.Scope{})
	b.Render(100, 20) // no rows loaded

	_, cmd := b.Update(tea.KeyPressMsg{Code: 'x', Text: "x"})
	assert.Nil(t, cmd, "NeedsRow action is a no-op with an empty table")
	assert.False(t, ran)
}

// --- render / status ---

func TestBrowserRenderLoading(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	assert.Contains(t, b.Render(100, 20), "loading")
}

func TestBrowserRenderLoaded(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	withRows(t, b, "alpha")
	assert.Contains(t, b.Render(100, 20), "alpha")
}

func TestBrowserStatus(t *testing.T) {
	b := newTestBrowser(crumbDef{name: "jobs", cols: col()}, resource.Scope{})
	assert.Empty(t, b.Status(time.Now()), "no status before load")

	withRows(t, b, "a", "b")
	status := b.Status(time.Now())
	assert.Contains(t, status, "2/2", "shows visible/total counts")
	assert.Contains(t, status, "ago")
}
