package view

import (
	"context"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/config"
	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/theme"
)

// fakeWarehouses is a struct-of-funcs WarehousesDAO fake.
type fakeWarehouses struct {
	list func(ctx context.Context) ([]dbx.Warehouse, error)
}

func (f fakeWarehouses) List(ctx context.Context) ([]dbx.Warehouse, error) { return f.list(ctx) }

// fakeStatements is a struct-of-funcs StatementDAO fake that records calls.
type fakeStatements struct {
	submit     func(ctx context.Context, whID, stmt string, limit int) (string, error)
	poll       func(ctx context.Context, id string) (dbx.StatementPoll, error)
	cancel     func(ctx context.Context, id string) error
	cancelCall int
}

func (f *fakeStatements) Submit(ctx context.Context, whID, stmt string, limit int) (string, error) {
	return f.submit(ctx, whID, stmt, limit)
}

func (f *fakeStatements) Poll(ctx context.Context, id string) (dbx.StatementPoll, error) {
	return f.poll(ctx, id)
}

func (f *fakeStatements) Cancel(ctx context.Context, id string) error {
	f.cancelCall++
	if f.cancel != nil {
		return f.cancel(ctx, id)
	}
	return nil
}

func sqlPress(k string) tea.KeyPressMsg {
	switch k {
	case "tab":
		return tea.KeyPressMsg{Code: tea.KeyTab}
	case "esc":
		return tea.KeyPressMsg{Code: tea.KeyEscape}
	case "ctrl+e":
		return tea.KeyPressMsg{Code: 'e', Mod: tea.ModCtrl}
	case "ctrl+k":
		return tea.KeyPressMsg{Code: 'k', Mod: tea.ModCtrl}
	case "ctrl+w":
		return tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl}
	default:
		return tea.KeyPressMsg{Code: rune(k[0]), Text: k}
	}
}

// run drains a command chain until nil, feeding each produced message back
// through Update. It only follows single-message cmds (no tea.Batch), which
// is all the SQL lifecycle uses on the paths under test. Tick cmds are not
// executed (they block); tests inject pollMsg/pollDoneMsg directly.
func newSQLView(t *testing.T, daos dbx.DAOs, query string, autoExec bool) *SQLView {
	t.Helper()
	clients := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, daos)
	return NewSQLView(theme.Default(), clients, config.SQLConfig{RowLimit: 100}, query, autoExec)
}

func runCmd(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd)
	return cmd()
}

func TestSQLWarehouseResolvedOnInit(t *testing.T) {
	daos := dbx.DAOs{
		Warehouses: fakeWarehouses{list: func(context.Context) ([]dbx.Warehouse, error) {
			return []dbx.Warehouse{{ID: "w1", Name: "serverless-sql", State: "RUNNING", Size: "S", Serverless: true}}, nil
		}},
	}
	v := newSQLView(t, daos, "", false)
	msg := runCmd(t, v.loadWarehouses())
	got, _ := v.Update(msg)
	sv := got.(*SQLView)

	assert.True(t, sv.whOK)
	assert.Contains(t, sv.Render(80, 20), "serverless-sql")
}

func TestSQLNoWarehouseWarning(t *testing.T) {
	daos := dbx.DAOs{
		Warehouses: fakeWarehouses{list: func(context.Context) ([]dbx.Warehouse, error) {
			return nil, nil
		}},
	}
	v := newSQLView(t, daos, "", false)
	msg := runCmd(t, v.loadWarehouses())
	got, _ := v.Update(msg)
	sv := got.(*SQLView)

	assert.False(t, sv.whOK)
	assert.Contains(t, sv.Render(80, 20), "no warehouse")
}

func TestSQLExecuteHappyPath(t *testing.T) {
	stmts := &fakeStatements{
		submit: func(context.Context, string, string, int) (string, error) { return "stmt-1", nil },
		poll: func(context.Context, string) (dbx.StatementPoll, error) {
			return dbx.StatementPoll{}, nil // not used directly
		},
	}
	daos := dbx.DAOs{
		Warehouses: fakeWarehouses{list: func(context.Context) ([]dbx.Warehouse, error) {
			return []dbx.Warehouse{{ID: "w1", Name: "wh", State: "RUNNING", Serverless: true}}, nil
		}},
		Statements: stmts,
	}
	v := newSQLView(t, daos, "select 1", false)
	whMsg := runCmd(t, v.loadWarehouses())
	got, _ := v.Update(whMsg)
	sv := got.(*SQLView)

	// Execute → Submit → stmtStartedMsg.
	got, cmd := sv.handleKey(sqlPress("ctrl+e"))
	sv = got.(*SQLView)
	require.Equal(t, statePending, sv.state)
	startMsg := runCmd(t, cmd).(stmtStartedMsg)
	assert.Equal(t, "stmt-1", startMsg.id)

	got, _ = sv.Update(startMsg)
	sv = got.(*SQLView)
	assert.Equal(t, stateRunning, sv.state)

	// Feed a SUCCEEDED poll with a small result.
	result := &dbx.StmtResult{
		Columns: []dbx.StmtColumn{{Name: "id"}, {Name: "name"}},
		Rows:    [][]string{{"1", "alice"}, {"2", "bob"}},
	}
	done := pollDoneMsg{gen: sv.gen, poll: dbx.StatementPoll{State: dbx.StmtSucceeded, Result: result}, elapsed: 1500 * time.Millisecond}
	got, _ = sv.Update(done)
	sv = got.(*SQLView)

	assert.Equal(t, stateSucceeded, sv.state)
	out := sv.Render(120, 24)
	assert.Contains(t, out, "2 rows in 1.5s")
	assert.Contains(t, out, "name")
	assert.Contains(t, out, "alice")
}

func TestSQLFailurePath(t *testing.T) {
	v := newSQLView(t, dbx.DAOs{}, "select boom", false)
	v.warehouses = []dbx.Warehouse{{ID: "w1", Name: "wh"}}
	v.wh, v.whOK = v.warehouses[0], true
	v.gen = 1
	v.state = stateRunning

	done := pollDoneMsg{gen: 1, poll: dbx.StatementPoll{State: dbx.StmtFailed, Message: "syntax error near boom"}}
	got, _ := v.Update(done)
	sv := got.(*SQLView)

	assert.Equal(t, stateFailed, sv.state)
	assert.Contains(t, sv.Render(80, 20), "syntax error near boom")
}

func TestSQLCancelCallsDAO(t *testing.T) {
	stmts := &fakeStatements{
		submit: func(context.Context, string, string, int) (string, error) { return "stmt-1", nil },
		poll:   func(context.Context, string) (dbx.StatementPoll, error) { return dbx.StatementPoll{}, nil },
	}
	v := newSQLView(t, dbx.DAOs{Statements: stmts}, "select 1", false)
	v.wh, v.whOK = dbx.Warehouse{ID: "w1"}, true
	v.gen = 1
	v.state = stateRunning
	v.stmtID = "stmt-1"

	got, cmd := v.handleKey(sqlPress("ctrl+k"))
	sv := got.(*SQLView)
	assert.True(t, sv.canceling)
	runCmd(t, cmd) // triggers Cancel on the fake
	assert.Equal(t, 1, stmts.cancelCall)
}

func TestSQLGenerationGuardDropsStalePoll(t *testing.T) {
	v := newSQLView(t, dbx.DAOs{}, "select 1", false)
	v.gen = 2
	v.state = stateRunning

	// A poll from an earlier execute (gen 1) must be ignored.
	stale := pollDoneMsg{gen: 1, poll: dbx.StatementPoll{State: dbx.StmtSucceeded, Result: &dbx.StmtResult{
		Columns: []dbx.StmtColumn{{Name: "x"}}, Rows: [][]string{{"99"}},
	}}}
	got, _ := v.Update(stale)
	sv := got.(*SQLView)

	assert.Equal(t, stateRunning, sv.state, "stale poll must not change state")
	assert.Nil(t, sv.result)
}

func TestSQLCapturesKeys(t *testing.T) {
	v := newSQLView(t, dbx.DAOs{}, "", false)
	assert.True(t, v.CapturesKeys(), "editor focused on construction")

	got, _ := v.handleKey(sqlPress("tab"))
	sv := got.(*SQLView)
	assert.Equal(t, focusResults, sv.focus)
	assert.False(t, sv.CapturesKeys(), "results focused releases key capture")

	// Picker open captures keys regardless of focus.
	got, _ = sv.handleKey(sqlPress("ctrl+w"))
	sv = got.(*SQLView)
	assert.True(t, sv.CapturesKeys())
}

func TestSQLTabCycler(t *testing.T) {
	v := newSQLView(t, dbx.DAOs{}, "", false)
	require.Equal(t, focusEditor, v.focus, "editor focused on construction")

	// Forward: editor -> results is an internal step (consumes the key).
	assert.True(t, v.AdvanceFocus(true))
	assert.Equal(t, focusResults, v.focus)

	// Forward again: at the boundary, so the container should switch tabs.
	assert.False(t, v.AdvanceFocus(true))
	assert.Equal(t, focusResults, v.focus, "focus unchanged at the boundary")

	// Backward: results -> editor is an internal step.
	assert.True(t, v.AdvanceFocus(false))
	assert.Equal(t, focusEditor, v.focus)

	// Backward again: at the boundary.
	assert.False(t, v.AdvanceFocus(false))
	assert.Equal(t, focusEditor, v.focus)

	// EnterFocus lands on the entry pane for the arrival direction.
	v.EnterFocus(false)
	assert.Equal(t, focusResults, v.focus, "entering backward lands on results")
	v.EnterFocus(true)
	assert.Equal(t, focusEditor, v.focus, "entering forward lands on editor")

	// An open picker swallows the key rather than letting the cycle escape it.
	got, _ := v.handleKey(sqlPress("ctrl+w"))
	sv := got.(*SQLView)
	require.True(t, sv.pickerOpen)
	assert.True(t, sv.AdvanceFocus(true))
}

func TestSQLAutoExecOnInit(t *testing.T) {
	submitted := false
	stmts := &fakeStatements{
		submit: func(context.Context, string, string, int) (string, error) {
			submitted = true
			return "auto-1", nil
		},
		poll: func(context.Context, string) (dbx.StatementPoll, error) { return dbx.StatementPoll{}, nil },
	}
	daos := dbx.DAOs{
		Warehouses: fakeWarehouses{list: func(context.Context) ([]dbx.Warehouse, error) {
			return []dbx.Warehouse{{ID: "w1", Name: "wh", State: "RUNNING", Serverless: true}}, nil
		}},
		Statements: stmts,
	}
	v := newSQLView(t, daos, "select * from t", true)
	assert.Equal(t, focusEditor, v.focus, "auto-exec lands on the SQL editor, not results")
	whMsg := runCmd(t, v.loadWarehouses())
	got, cmd := v.Update(whMsg)
	sv := got.(*SQLView)

	assert.Equal(t, statePending, sv.state, "autoExec kicks off execute")
	require.NotNil(t, cmd)
	_ = runCmd(t, cmd)
	assert.True(t, submitted)
	assert.NotContains(t, sv.Render(80, 20), "no warehouse")
}
