package resources

import (
	"net/url"
	"strings"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/view"
)

// webURL joins the workspace host with a set of path segments, escaping each
// segment. It returns ok=false when the host is unknown or any segment is
// empty — the caller then falls back to a "no web link" flash rather than
// opening a malformed URL.
//
// Databricks workspace page shapes (host + these paths):
//   - Unity Catalog: /explore/data/{catalog}[/{schema}[/{table}]]
//   - Jobs:          /jobs/{jobID}[/runs/{runID}]
//   - Pipelines:     /pipelines/{pipelineID}[/updates/{updateID}]
func webURL(host string, segments ...string) (string, bool) {
	if host == "" {
		return "", false
	}
	parts := make([]string, len(segments))
	for i, s := range segments {
		if s == "" {
			return "", false
		}
		parts[i] = url.PathEscape(s)
	}
	return host + "/" + strings.Join(parts, "/"), true
}

// WebURL implements resource.WebLinker.
func (CatalogsDef) WebURL(host string, _ resource.Scope, row resource.Row) (string, bool) {
	return webURL(host, "explore", "data", row.ID)
}

// WebURL implements resource.WebLinker.
func (SchemasDef) WebURL(host string, scope resource.Scope, row resource.Row) (string, bool) {
	return webURL(host, "explore", "data", scope["catalog"], row.ID)
}

// WebURL implements resource.WebLinker.
func (TablesDef) WebURL(host string, scope resource.Scope, row resource.Row) (string, bool) {
	return tableURL(host, scope["catalog"], scope["schema"], row.ID)
}

// tableURL is a table's page in the Catalog Explorer.
func tableURL(host, catalog, schema, table string) (string, bool) {
	return webURL(host, "explore", "data", catalog, schema, table)
}

// tableSampleDataURL is a table's page opened on its Sample Data tab — the web
// equivalent of the data tab's preview query.
func tableSampleDataURL(host, catalog, schema, table string) (string, bool) {
	u, ok := tableURL(host, catalog, schema, table)
	if !ok {
		return "", false
	}
	return u + "?activeTab=sampleData", true
}

// jobRunURL is one job run's page.
func jobRunURL(host, jobID, runID string) (string, bool) {
	return webURL(host, "jobs", jobID, "runs", runID)
}

// pipelineURL is a pipeline's page; pipelineUpdateURL is one of its updates.
func pipelineURL(host, pipelineID string) (string, bool) {
	return webURL(host, "pipelines", pipelineID)
}

func pipelineUpdateURL(host, pipelineID, updateID string) (string, bool) {
	return webURL(host, "pipelines", pipelineID, "updates", updateID)
}

// appURL is an app's management page in the workspace UI.
func appURL(host, name string) (string, bool) {
	return webURL(host, "apps", name)
}

// Detail views (logs, describe, SQL results) can't derive `o` from a def the way
// the Browser does, so the def hands them a view.WebLink when it opens them.
// The builders below wrap the URL functions above: each takes the (url, ok) pair
// straight through, so a URL that can't be built (unknown host, missing scope)
// yields the zero link and the view simply leaves `o` unbound.

// webLinkTo builds a link from a URL builder's (url, ok) pair.
func webLinkTo(url string, ok bool) view.WebLink {
	if !ok {
		return view.WebLink{}
	}
	return view.WebLink{URL: url}
}

// named labels a link's `o` binding in the header (no-op on the zero link).
func named(l view.WebLink, hint string) view.WebLink {
	if l.URL != "" {
		l.Hint = hint
	}
	return l
}

// tableLink / tableSampleDataLink point at a table's Catalog Explorer page —
// the second on its Sample Data tab, the web twin of the data tab's preview.
func tableLink(host, catalog, schema, table string) view.WebLink {
	return named(webLinkTo(tableURL(host, catalog, schema, table)), "open table")
}

func tableSampleDataLink(host, catalog, schema, table string) view.WebLink {
	return named(webLinkTo(tableSampleDataURL(host, catalog, schema, table)), "open sample data")
}

// jobRunLink points at one job run's page — the page behind a run's logs,
// details, and task runs alike.
func jobRunLink(host, jobID, runID string) view.WebLink {
	return named(webLinkTo(jobRunURL(host, jobID, runID)), "open run")
}

// pipelineLink / pipelineUpdateLink point at a pipeline and one of its updates.
func pipelineLink(host, pipelineID string) view.WebLink {
	return named(webLinkTo(pipelineURL(host, pipelineID)), "open pipeline")
}

func pipelineUpdateLink(host, pipelineID, updateID string) view.WebLink {
	return named(webLinkTo(pipelineUpdateURL(host, pipelineID, updateID)), "open update")
}

// appLink points at an app's management page in the workspace UI.
func appLink(host, name string) view.WebLink {
	return named(webLinkTo(appURL(host, name)), "open app page")
}

// appLogsLink points at the app's own log viewer (`/logz` on the app host — the
// HTML page behind the stream the logs view drains). Apps without a deployed URL
// fall back to their workspace page.
func appLogsLink(host string, app dbx.App) view.WebLink {
	if app.URL == "" {
		return appLink(host, app.Name)
	}
	return named(view.WebLink{URL: strings.TrimSuffix(app.URL, "/") + "/logz"}, "open log viewer")
}

// WebURL implements resource.WebLinker. Columns have no page of their own, so
// `o` opens the parent table in the Catalog Explorer.
func (ColumnsDef) WebURL(host string, scope resource.Scope, _ resource.Row) (string, bool) {
	return webURL(host, "explore", "data", scope["catalog"], scope["schema"], scope["table"])
}

// WebURL implements resource.WebLinker.
func (JobsDef) WebURL(host string, _ resource.Scope, row resource.Row) (string, bool) {
	return webURL(host, "jobs", row.ID)
}

// WebURL implements resource.WebLinker.
func (RunsDef) WebURL(host string, scope resource.Scope, row resource.Row) (string, bool) {
	return jobRunURL(host, scope["job"], row.ID)
}

// WebURL implements resource.WebLinker. Task runs share their parent run's
// page, so `o` opens that run.
func (TaskRunsDef) WebURL(host string, scope resource.Scope, _ resource.Row) (string, bool) {
	return jobRunURL(host, scope["job"], scope["run"])
}

// WebURL implements resource.WebLinker.
func (PipelinesDef) WebURL(host string, _ resource.Scope, row resource.Row) (string, bool) {
	return pipelineURL(host, row.ID)
}

// WebURL implements resource.WebLinker.
func (UpdatesDef) WebURL(host string, scope resource.Scope, row resource.Row) (string, bool) {
	return pipelineUpdateURL(host, scope["pipeline"], row.ID)
}

// WebURL implements resource.WebLinker: `o` opens the app's management page in
// the workspace UI (row.ID is the app name).
func (AppsDef) WebURL(host string, _ resource.Scope, row resource.Row) (string, bool) {
	return appURL(host, row.ID)
}

// AltWebURL implements resource.AltWebLinker: `O` opens the deployed app itself
// on its own host, when it has one.
func (AppsDef) AltWebURL(_ string, _ resource.Scope, row resource.Row) (string, bool) {
	app := appFromRow(row)
	if app.URL == "" {
		return "", false
	}
	return app.URL, true
}

// AltWebHint labels the `O` binding in the key help.
func (AppsDef) AltWebHint() string { return "open app" }
