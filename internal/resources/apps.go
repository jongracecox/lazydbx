package resources

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

var appCols = []resource.ColSpec[dbx.App]{
	{Column: resource.Column{Title: "NAME"}, Extract: func(a dbx.App) string { return a.Name }},
	{Column: resource.Column{Title: "COMPUTE", Width: 12}, Extract: func(a dbx.App) string { return a.ComputeState }},
	{Column: resource.Column{Title: "STATUS", Width: 12}, Extract: func(a dbx.App) string { return a.AppState }},
	{Column: resource.Column{Title: "DEPLOYMENT", Width: 12}, Extract: func(a dbx.App) string { return a.ActiveDeploymentState }},
	{Column: resource.Column{Title: "URL", Wide: true}, Extract: func(a dbx.App) string { return a.URL }},
	{Column: resource.Column{Title: "CREATOR", Width: 28, Wide: true}, Extract: func(a dbx.App) string { return a.Creator }},
	{Column: resource.Column{Title: "CREATED", Width: 12, Wide: true}, Extract: func(a dbx.App) string { return relTime(a.CreatedAt) }},
}

// Column indices colored via Styler.
const (
	appComputeCol    = 1
	appStatusCol     = 2
	appDeploymentCol = 3
)

// AppsDef browses Databricks Apps.
type AppsDef struct{}

func (AppsDef) Name() string               { return "apps" }
func (AppsDef) Aliases() []string          { return []string{"app"} }
func (AppsDef) Args() []string             { return nil }
func (AppsDef) Columns() []resource.Column { return resource.Cols(appCols) }

// PollInterval is 15s — apps state changes matter and the list API is not
// heavily rate-limited.
func (AppsDef) PollInterval() time.Duration { return 15 * time.Second }

// Child is empty: Enter is handled by Opener (tabbed detail + logs).
func (AppsDef) Child() string { return "" }

func (AppsDef) ChildScope(parent resource.Scope, _ resource.Row) resource.Scope {
	return parent
}

// CellClass colors the COMPUTE, STATUS, and DEPLOYMENT columns semantically.
func (AppsDef) CellClass(col int, value string) resource.CellClass {
	switch col {
	case appComputeCol, appStatusCol, appDeploymentCol:
		return stateClass(value)
	default:
		return resource.CellDefault
	}
}

// Actions: `l` opens the app's runtime logs directly.
func (AppsDef) Actions() []resource.Action {
	return []resource.Action{
		{
			Key:      "l",
			Name:     "logs",
			NeedsRow: true,
			Run: func(_ context.Context, c *dbx.Clients, _ resource.Scope, row resource.Row) any {
				app := appFromRow(row)
				return view.OpenLogTableMsg{
					Title:  "logs/" + app.Name,
					Follow: true,
					Fetch:  appLogsFetch(c, app.URL),
				}
			},
		},
	}
}

// Tabs implements resource.Tabber: the tab names EnterMsg produces, in order.
func (AppsDef) Tabs() []string { return []string{"details", "logs"} }

// EnterMsg implements resource.Opener: selecting an app opens tabs — its logs
// beside its metadata. Rows restored from the on-disk cache arrive with Data as
// a map rather than a dbx.App; appFromRow recovers the app so Enter always
// opens the view (the tabs then load/refresh) instead of dead-ending.
func (AppsDef) EnterMsg(c *dbx.Clients, _ resource.Scope, row resource.Row) any {
	app := appFromRow(row)
	return view.OpenTabsMsg{
		Title: app.Name,
		Tabs: []view.TabSpec{
			{Name: "details", Detail: func(context.Context) (any, error) { return app, nil }},
			{Name: "logs", LogTable: &view.LogTableTabSpec{
				Follow: true,
				Fetch:  appLogsFetch(c, app.URL),
			}},
		},
	}
}

func (AppsDef) List(ctx context.Context, c *dbx.Clients, _ resource.Scope) ([]resource.Row, error) {
	dao, err := c.Apps()
	if err != nil {
		return nil, err
	}
	items, err := dao.List(ctx)
	if err != nil {
		return nil, err
	}
	return resource.BuildRows(items, func(a dbx.App) string { return a.Name }, appCols), nil
}

func (AppsDef) Describe(_ context.Context, _ *dbx.Clients, _ resource.Scope, row resource.Row) (any, error) {
	return row.Data, nil
}

// appLogsFetch returns a fetch closure that re-acquires the DAO lazily, fetches
// the app's log records, and projects them to the view's LogRecord type.
func appLogsFetch(c *dbx.Clients, appURL string) func(ctx context.Context) ([]view.LogRecord, error) {
	return func(ctx context.Context) ([]view.LogRecord, error) {
		dao, err := c.Apps()
		if err != nil {
			return nil, err
		}
		entries, err := dao.GetLogs(ctx, appURL)
		if err != nil {
			return nil, err
		}
		records := make([]view.LogRecord, len(entries))
		for i, e := range entries {
			records[i] = view.LogRecord{
				Time:     e.Time,
				Severity: e.Severity,
				Source:   e.Source,
				Message:  e.Message,
				Raw:      e.Raw,
			}
		}
		return records, nil
	}
}

// appFromRow recovers a dbx.App from a row. Live rows carry a dbx.App directly;
// rows restored from the on-disk cache round-trip Data through JSON and arrive
// as map[string]any, so we re-decode them (App's json tags make this exact).
// Worst case it falls back to the row ID (the app name), so callers always get
// a usable app and the view opens rather than dead-ending on a flash.
func appFromRow(row resource.Row) dbx.App {
	if app, ok := row.Data.(dbx.App); ok {
		return app
	}
	if m, ok := row.Data.(map[string]any); ok {
		if b, err := json.Marshal(m); err == nil {
			var app dbx.App
			if json.Unmarshal(b, &app) == nil && app.Name != "" {
				return app
			}
		}
	}
	return dbx.App{Name: row.ID}
}
