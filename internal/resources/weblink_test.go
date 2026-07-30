package resources

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// clientsAt builds clients whose profile points at the test host, so the link
// builders produce real URLs (an empty host yields no link at all).
func clientsAt(daos dbx.DAOs) *dbx.Clients {
	return dbx.NewClientsWithDAOs(dbx.Profile{Name: "test", Host: host}, daos)
}

// TestDetailViewWebLinks covers the `o` link every def hands to the detail views
// it opens — the screens that can't derive one from a def the way the Browser
// does. Each case names the tab (or "" for a standalone view) and its URL.
func TestDetailViewWebLinks(t *testing.T) {
	tests := []struct {
		name string
		msg  func() any
		// want maps tab name → URL; the "" key is the message-level link (a
		// standalone view, or the fallback tabs inherit).
		want map[string]string
	}{
		{
			name: "tables enter",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Tables: fakeTablesDAO{}})
				return TablesDef{}.EnterMsg(c, resource.Scope{"catalog": "main", "schema": "silver"},
					resource.Row{ID: "events"})
			},
			want: map[string]string{
				"": host + "/explore/data/main/silver/events",
				// The data tab lands on the web page showing the same rows.
				"data": host + "/explore/data/main/silver/events?activeTab=sampleData",
			},
		},
		{
			name: "tables query action",
			msg: func() any {
				c := clientsAt(dbx.DAOs{})
				return TablesDef{}.Actions()[0].Run(context.Background(), c,
					resource.Scope{"catalog": "main", "schema": "silver"}, resource.Row{ID: "events"})
			},
			want: map[string]string{"": host + "/explore/data/main/silver/events?activeTab=sampleData"},
		},
		{
			name: "runs logs action",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Jobs: fakeJobsDAO{}})
				return RunsDef{}.Actions()[0].Run(context.Background(), c,
					resource.Scope{"job": "12"}, resource.Row{ID: "99"})
			},
			want: map[string]string{"": host + "/jobs/12/runs/99"},
		},
		{
			name: "taskruns logs action",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Jobs: fakeJobsDAO{}})
				return TaskRunsDef{}.Actions()[0].Run(context.Background(), c,
					resource.Scope{"job": "12", "run": "99"},
					resource.Row{ID: "777", Data: dbx.TaskRun{RunID: 777, Key: "extract"}})
			},
			want: map[string]string{"": host + "/jobs/12/runs/99"},
		},
		{
			name: "taskruns enter",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Jobs: fakeJobsDAO{}})
				return TaskRunsDef{}.EnterMsg(c, resource.Scope{"job": "12", "run": "99"},
					resource.Row{ID: "777", Data: dbx.TaskRun{RunID: 777, Key: "extract"}})
			},
			// Tasks have no page of their own: both tabs open the parent run.
			want: map[string]string{"": host + "/jobs/12/runs/99"},
		},
		{
			name: "pipelines events action",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Pipelines: fakePipelinesDAO{}})
				return PipelinesDef{}.Actions()[0].Run(context.Background(), c, resource.Scope{},
					resource.Row{ID: "pl-1", Data: dbx.Pipeline{ID: "pl-1", Name: "silver-etl"}})
			},
			want: map[string]string{"": host + "/pipelines/pl-1"},
		},
		{
			name: "updates events action",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Pipelines: fakePipelinesDAO{}})
				return UpdatesDef{}.Actions()[0].Run(context.Background(), c,
					resource.Scope{"pipeline": "pl-1"}, resource.Row{ID: "upd-7"})
			},
			want: map[string]string{"": host + "/pipelines/pl-1/updates/upd-7"},
		},
		{
			name: "updates enter",
			msg: func() any {
				c := clientsAt(dbx.DAOs{Pipelines: fakePipelinesDAO{}})
				return UpdatesDef{}.EnterMsg(c, resource.Scope{"pipeline": "pl-1"},
					resource.Row{ID: "upd-7", Data: dbx.PipelineUpdate{ID: "upd-7"}})
			},
			want: map[string]string{"": host + "/pipelines/pl-1/updates/upd-7"},
		},
		{
			name: "apps enter",
			msg: func() any {
				c := clientsAt(dbx.DAOs{})
				return AppsDef{}.EnterMsg(c, resource.Scope{},
					resource.Row{ID: "my-app", Data: dbx.App{Name: "my-app", URL: "https://my-app.apps.dbx"}})
			},
			want: map[string]string{
				"": host + "/apps/my-app",
				// Logs point at the app's own viewer, not the workspace page.
				"logs": "https://my-app.apps.dbx/logz",
			},
		},
		{
			name: "apps logs action",
			msg: func() any {
				c := clientsAt(dbx.DAOs{})
				return AppsDef{}.Actions()[0].Run(context.Background(), c, resource.Scope{},
					resource.Row{ID: "my-app", Data: dbx.App{Name: "my-app", URL: "https://my-app.apps.dbx/"}})
			},
			want: map[string]string{"": "https://my-app.apps.dbx/logz"},
		},
		{
			name: "apps logs action without a deployed url",
			msg: func() any {
				c := clientsAt(dbx.DAOs{})
				return AppsDef{}.Actions()[0].Run(context.Background(), c, resource.Scope{},
					resource.Row{ID: "my-app", Data: dbx.App{Name: "my-app"}})
			},
			want: map[string]string{"": host + "/apps/my-app"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := webLinksOf(t, tc.msg())
			for tab, url := range tc.want {
				assert.Equal(t, url, got[tab], "link for tab %q", tab)
			}
		})
	}
}

// TestDetailViewWebLinksNeedAHost pins the fallback: without a resolvable
// workspace host there is no link, and the view leaves `o` unbound.
func TestDetailViewWebLinksNeedAHost(t *testing.T) {
	c := dbx.NewClientsWithDAOs(dbx.Profile{Name: "test"}, dbx.DAOs{Tables: fakeTablesDAO{}})
	msg := TablesDef{}.EnterMsg(c, resource.Scope{"catalog": "main", "schema": "silver"},
		resource.Row{ID: "events"})
	for tab, url := range webLinksOf(t, msg) {
		assert.Empty(t, url, "tab %q", tab)
	}
}

// webLinksOf extracts the web links from a view-opening message: the
// message-level link under "" plus each tab's own override under its name.
func webLinksOf(t *testing.T, msg any) map[string]string {
	t.Helper()
	switch m := msg.(type) {
	case view.OpenTabsMsg:
		links := map[string]string{"": m.Web.URL}
		for _, tab := range m.Tabs {
			links[tab.Name] = tab.Web.URL
		}
		return links
	case view.OpenSQLMsg:
		return map[string]string{"": m.Web.URL}
	case view.OpenLogMsg:
		return map[string]string{"": m.Web.URL}
	case view.OpenLogTableMsg:
		return map[string]string{"": m.Web.URL}
	}
	require.Failf(t, "unexpected message", "%T does not carry a web link", msg)
	return nil
}
