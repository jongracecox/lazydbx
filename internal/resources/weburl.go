package resources

import (
	"net/url"
	"strings"

	"github.com/jongracecox/lazydbx/internal/resource"
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
	return webURL(host, "explore", "data", scope["catalog"], scope["schema"], row.ID)
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
	return webURL(host, "jobs", scope["job"], "runs", row.ID)
}

// WebURL implements resource.WebLinker. Task runs share their parent run's
// page, so `o` opens that run.
func (TaskRunsDef) WebURL(host string, scope resource.Scope, _ resource.Row) (string, bool) {
	return webURL(host, "jobs", scope["job"], "runs", scope["run"])
}

// WebURL implements resource.WebLinker.
func (PipelinesDef) WebURL(host string, _ resource.Scope, row resource.Row) (string, bool) {
	return webURL(host, "pipelines", row.ID)
}

// WebURL implements resource.WebLinker.
func (UpdatesDef) WebURL(host string, scope resource.Scope, row resource.Row) (string, bool) {
	return webURL(host, "pipelines", scope["pipeline"], "updates", row.ID)
}

// WebURL implements resource.WebLinker: `o` opens the app's management page in
// the workspace UI (row.ID is the app name).
func (AppsDef) WebURL(host string, _ resource.Scope, row resource.Row) (string, bool) {
	return webURL(host, "apps", row.ID)
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
