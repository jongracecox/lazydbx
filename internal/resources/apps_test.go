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
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// fakeAppsDAO is a struct of func fields — the house pattern for faking DAOs.
type fakeAppsDAO struct {
	ListFn    func(ctx context.Context) ([]dbx.App, error)
	GetLogsFn func(ctx context.Context, appURL string) ([]dbx.AppLogEntry, error)
}

func (f fakeAppsDAO) List(ctx context.Context) ([]dbx.App, error) { return f.ListFn(ctx) }

func (f fakeAppsDAO) GetLogs(ctx context.Context, appURL string) ([]dbx.AppLogEntry, error) {
	return f.GetLogsFn(ctx, appURL)
}

func clientsWithApps(dao dbx.AppsDAO) *dbx.Clients {
	return dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{Apps: dao})
}

func TestAppsDefList(t *testing.T) {
	created := time.Now().Add(-3 * time.Hour)
	c := clientsWithApps(fakeAppsDAO{
		ListFn: func(context.Context) ([]dbx.App, error) {
			return []dbx.App{
				{
					Name: "dash", ComputeState: "ACTIVE", AppState: "RUNNING",
					ActiveDeploymentState: "SUCCEEDED", URL: "https://dash.example.com",
					Creator: "jon@example.com", CreatedAt: created,
				},
			}, nil
		},
	})

	rows, err := AppsDef{}.List(context.Background(), c, resource.Scope{})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, "dash", rows[0].ID)
	assert.Equal(t, []string{"dash", "ACTIVE", "RUNNING", "SUCCEEDED", "https://dash.example.com", "jon@example.com", "3h"}, rows[0].Cells)
}

func TestAppsDefListError(t *testing.T) {
	c := clientsWithApps(fakeAppsDAO{
		ListFn: func(context.Context) ([]dbx.App, error) { return nil, errors.New("boom") },
	})
	_, err := AppsDef{}.List(context.Background(), c, resource.Scope{})
	assert.ErrorContains(t, err, "boom")
}

func TestAppsDefShape(t *testing.T) {
	d := AppsDef{}
	assert.Equal(t, "apps", d.Name())
	assert.Contains(t, d.Aliases(), "app")
	assert.Empty(t, d.Args())
	assert.Empty(t, d.Child(), "apps is a leaf; Enter opens tabs")
	assert.Equal(t, 15*time.Second, d.PollInterval())
}

func TestAppsDefCellClass(t *testing.T) {
	d := AppsDef{}
	assert.Equal(t, resource.CellGood, d.CellClass(appComputeCol, "ACTIVE"))
	assert.Equal(t, resource.CellRunning, d.CellClass(appStatusCol, "RUNNING"))
	assert.Equal(t, resource.CellBad, d.CellClass(appStatusCol, "CRASHED"))
	assert.Equal(t, resource.CellGood, d.CellClass(appDeploymentCol, "SUCCEEDED"))
	assert.Equal(t, resource.CellDefault, d.CellClass(0, "ACTIVE"), "only state columns are colored")
}

func fakeLogs(records ...dbx.AppLogEntry) func(context.Context, string) ([]dbx.AppLogEntry, error) {
	return func(context.Context, string) ([]dbx.AppLogEntry, error) { return records, nil }
}

func TestAppsDefLogsAction(t *testing.T) {
	var gotURL string
	c := clientsWithApps(fakeAppsDAO{
		GetLogsFn: func(_ context.Context, appURL string) ([]dbx.AppLogEntry, error) {
			gotURL = appURL
			return []dbx.AppLogEntry{{Message: "hello", Severity: "INFO"}}, nil
		},
	})

	actions := AppsDef{}.Actions()
	require.Len(t, actions, 1)
	action := actions[0]
	assert.Equal(t, "l", action.Key)
	assert.True(t, action.NeedsRow)

	row := resource.Row{ID: "dash", Data: dbx.App{Name: "dash", URL: "https://dash.example.com", ComputeState: "STARTING"}}
	msg := action.Run(context.Background(), c, resource.Scope{}, row)
	open, ok := msg.(view.OpenLogTableMsg)
	require.True(t, ok)
	assert.Equal(t, "logs/dash", open.Title)
	assert.True(t, open.Follow, "logs follow by default")

	records, err := open.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "hello", records[0].Message)
	assert.Equal(t, "INFO", records[0].Severity)
	assert.Equal(t, "https://dash.example.com", gotURL)
}

func TestAppsDefLogsActionFollowsRegardlessOfState(t *testing.T) {
	c := clientsWithApps(fakeAppsDAO{GetLogsFn: fakeLogs()})
	row := resource.Row{ID: "dash", Data: dbx.App{Name: "dash", ComputeState: "STOPPED", AppState: "UNAVAILABLE"}}
	msg := AppsDef{}.Actions()[0].Run(context.Background(), c, resource.Scope{}, row)
	assert.True(t, msg.(view.OpenLogTableMsg).Follow, "logs follow by default even for a stopped app")
}

func TestAppsEnterOpensTabs(t *testing.T) {
	var gotURL string
	c := clientsWithApps(fakeAppsDAO{
		GetLogsFn: func(_ context.Context, appURL string) ([]dbx.AppLogEntry, error) {
			gotURL = appURL
			return []dbx.AppLogEntry{{Message: "hello"}}, nil
		},
	})
	app := dbx.App{Name: "dash", URL: "https://dash.example.com", ComputeState: "STARTING"}

	msg := AppsDef{}.EnterMsg(c, resource.Scope{}, resource.Row{ID: "dash", Data: app})
	open, ok := msg.(view.OpenTabsMsg)
	require.True(t, ok)
	assert.Equal(t, "dash", open.Title)
	require.Len(t, open.Tabs, 2)
	assert.Equal(t, "details", open.Tabs[0].Name)
	assert.Equal(t, "logs", open.Tabs[1].Name)
	require.NotNil(t, open.Tabs[1].LogTable)
	assert.True(t, open.Tabs[1].LogTable.Follow, "starting app follows")

	detail, err := open.Tabs[0].Detail(context.Background())
	require.NoError(t, err)
	assert.Equal(t, app, detail)

	records, err := open.Tabs[1].LogTable.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, "hello", records[0].Message)
	assert.Equal(t, "https://dash.example.com", gotURL)
}

func TestAppsEnterStaleCache(t *testing.T) {
	// Disk-cached rows round-trip Data through JSON, arriving as maps (keyed by
	// the App json tags). Enter should still open the view, reconstructing the
	// app — including the URL its logs tab needs — rather than dead-ending.
	var gotURL string
	c := clientsWithApps(fakeAppsDAO{
		GetLogsFn: func(_ context.Context, appURL string) ([]dbx.AppLogEntry, error) {
			gotURL = appURL
			return nil, nil
		},
	})
	cached := map[string]any{"name": "dash", "url": "https://dash.example.com", "compute_state": "ACTIVE"}

	msg := AppsDef{}.EnterMsg(c, resource.Scope{}, resource.Row{ID: "dash", Data: cached})
	open, ok := msg.(view.OpenTabsMsg)
	require.True(t, ok, "opens the tabbed view instead of flashing")
	assert.Equal(t, "dash", open.Title)
	require.Len(t, open.Tabs, 2)

	_, err := open.Tabs[1].LogTable.Fetch(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "https://dash.example.com", gotURL, "URL recovered from the cached map")
}

func TestAppsLogsActionStaleCache(t *testing.T) {
	// The `l` action recovers the app from a cached map too, so it never flashes.
	c := clientsWithApps(fakeAppsDAO{GetLogsFn: fakeLogs()})
	cached := map[string]any{"name": "dash", "url": "https://dash.example.com"}
	msg := AppsDef{}.Actions()[0].Run(context.Background(), c, resource.Scope{}, resource.Row{ID: "dash", Data: cached})
	open, ok := msg.(view.OpenLogTableMsg)
	require.True(t, ok)
	assert.Equal(t, "logs/dash", open.Title)
}

func TestAppsWebURL(t *testing.T) {
	// o → workspace management page.
	url, ok := AppsDef{}.WebURL("https://workspace.example.com", resource.Scope{},
		resource.Row{ID: "dash", Data: dbx.App{Name: "dash", URL: "https://dash.example.com"}})
	assert.True(t, ok)
	assert.Equal(t, "https://workspace.example.com/apps/dash", url)
}

func TestAppsAltWebURL(t *testing.T) {
	// O → deployed app URL.
	d := AppsDef{}
	assert.Equal(t, "open app", d.AltWebHint())

	url, ok := d.AltWebURL("https://workspace.example.com", resource.Scope{},
		resource.Row{ID: "dash", Data: dbx.App{Name: "dash", URL: "https://dash.example.com"}})
	assert.True(t, ok)
	assert.Equal(t, "https://dash.example.com", url, "opens the deployed app host")

	_, ok = d.AltWebURL("https://workspace.example.com", resource.Scope{},
		resource.Row{ID: "dash", Data: dbx.App{Name: "dash"}})
	assert.False(t, ok, "no URL yet → no alt web link")
}

func TestAppsRegistered(t *testing.T) {
	reg := NewRegistry()
	for _, name := range []string{"apps", "app"} {
		_, ok := reg.Get(name)
		assert.True(t, ok, name)
	}
}

// TestStateClassAppStates covers the app-specific states folded into the shared
// stateClass map.
func TestStateClassAppStates(t *testing.T) {
	tests := []struct {
		value string
		want  resource.CellClass
	}{
		{"ACTIVE", resource.CellGood},
		{"CRASHED", resource.CellBad},
		{"DEPLOYING", resource.CellRunning},
		{"UPDATING", resource.CellRunning},
		{"IN_PROGRESS", resource.CellRunning},
		{"STOPPED", resource.CellWarn},
		{"STOPPING", resource.CellWarn},
		{"UNAVAILABLE", resource.CellWarn},
		{"DELETING", resource.CellWarn},
	}
	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			assert.Equal(t, tt.want, stateClass(tt.value))
		})
	}
}
