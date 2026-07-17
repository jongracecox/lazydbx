// Package resource defines the core abstraction every browsable Databricks
// resource implements. The UI renders any ResourceDef through one generic
// browser view; the registry maps `:` commands to defs. This package never
// imports the Databricks SDK — concrete defs live in internal/resources and
// reach the API through the narrow DAO interfaces in internal/dbx.
package resource

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/jongracecox/lazydbx/internal/dbx"
)

// Scope parameterizes a view, e.g. {"catalog": "main", "schema": "silver"}
// for a tables view or {"job_id": "123"} for a runs view.
type Scope map[string]string

// Merge returns a copy of s with an extra key set. The receiver is not
// modified, so parent scopes are safe to share across drill-downs.
func (s Scope) Merge(key, value string) Scope {
	out := make(Scope, len(s)+1)
	for k, v := range s {
		out[k] = v
	}
	out[key] = value
	return out
}

// Hash returns a stable string form of the scope, used in cache keys.
func (s Scope) Hash() string {
	if len(s) == 0 {
		return ""
	}
	keys := make([]string, 0, len(s))
	for k := range s {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + s[k]
	}
	return strings.Join(parts, ",")
}

// Row is one pre-rendered table row. Data retains the original API object;
// it is only ever type-asserted back inside the def that created the row.
type Row struct {
	ID    string   // stable identity: cursor preservation, drill-down key
	Cells []string // aligned to Columns()
	Data  any
}

// Column describes one table column.
type Column struct {
	Title string
	// Width semantics: 0 = flex (share remaining space), >0 = fixed width.
	Width int
	// Wide columns are hidden unless the terminal is wide enough.
	Wide bool
}

// Action is a verb key available on a resource view beyond the universal
// Enter/Esc/describe bindings.
type Action struct {
	Key  string // e.g. "l"
	Name string // shown in header hints, e.g. "Logs"
	// Dangerous actions require a confirmation dialog and are hidden
	// entirely when the app runs with --readonly.
	Dangerous bool
	// NeedsRow actions are no-ops when the table is empty.
	NeedsRow bool
	// Run performs the action. It executes inside a tea.Cmd, so it may do
	// I/O; it returns a message for the app to route (declared as any here
	// to keep this package free of UI imports).
	Run func(ctx context.Context, c *dbx.Clients, scope Scope, row Row) any
}

// CellClass is a semantic classification of a rendered cell value, mapped
// to theme styles by the table component (defs know values, not colors).
type CellClass int

// Cell classes.
const (
	CellDefault CellClass = iota
	CellGood              // e.g. SUCCESS, RUNNING pipeline in healthy state
	CellBad               // e.g. FAILED, INTERNAL_ERROR
	CellWarn              // e.g. CANCELED, TIMEDOUT, SKIPPED
	CellRunning           // e.g. RUNNING, PENDING — in-flight states
)

// Styler is optionally implemented by defs whose cells deserve semantic
// coloring (run states, health columns). col indexes into Columns().
type Styler interface {
	CellClass(col int, value string) CellClass
}

// Tagger is optionally implemented by defs whose rows carry tags (e.g. job
// custom tags). The browser offers an interactive tag filter for them.
// Returned tags should be stable, display-ready strings like "env=prod".
type Tagger interface {
	RowTags(row Row) []string
}

// Opener is optionally implemented by defs whose Enter opens a richer view
// than the default child drill-down (e.g. tables open a tabbed detail).
// EnterMsg returns the message the browser emits; it overrides Child().
// Like Action.Run, it receives clients so it can bind fetch closures.
type Opener interface {
	EnterMsg(c *dbx.Clients, scope Scope, row Row) any
}

// RowNamer is optionally implemented by defs whose rows carry a human name
// distinct from Row.ID (e.g. jobs: Row.ID is the numeric job id, but the CLI
// refers to jobs by name). The name is offered as the shell-completion
// candidate and accepted as a launch Item selector alongside the ID. It must
// read from Row.Cells (not Row.Data), so it also works on rows restored from
// the on-disk cache, whose Data is a generic map.
type RowNamer interface {
	RowName(row Row) string
}

// Tabber is optionally implemented by Opener defs to expose their tab names
// statically — before EnterMsg runs with live data. The names and order must
// match the tabs EnterMsg produces. It lets the CLI validate and complete a
// `--tab` launch selection (see cmd/lazydbx) without opening the workspace.
type Tabber interface {
	Tabs() []string
}

// WebLinker is optionally implemented by defs whose rows map to a page in the
// Databricks workspace web UI. The browser binds `o` to open that page in the
// system browser. host is the workspace base URL (e.g.
// "https://xxx.cloud.databricks.com", no trailing slash); ok is false when the
// row has no stable web location (e.g. host unknown).
type WebLinker interface {
	WebURL(host string, scope Scope, row Row) (url string, ok bool)
}

// AltWebLinker is optionally implemented by defs that have a second, distinct
// web target beyond WebLinker's primary one — the browser binds `O` to it. For
// apps, `o` (WebLinker) opens the workspace management page while `O` opens the
// deployed app itself. AltWebHint is the short verb shown in the key help
// (e.g. "open app"); AltWebURL returns ok=false when the row has no such link.
type AltWebLinker interface {
	AltWebURL(host string, scope Scope, row Row) (url string, ok bool)
	AltWebHint() string
}

// Def is the interface every browsable resource implements.
type Def interface {
	// Name is the canonical command name, e.g. "tables".
	Name() string
	// Aliases are alternative command names, e.g. ["table", "tbl"].
	Aliases() []string
	// Args names the positional scope keys required by this resource, in
	// order, e.g. tables → ["catalog", "schema"]. Empty for unscoped.
	Args() []string
	Columns() []Column
	List(ctx context.Context, c *dbx.Clients, scope Scope) ([]Row, error)
	// PollInterval is the steady-state refresh cadence; the engine adds
	// jitter and backoff. Use long intervals for rate-limited APIs (SCIM).
	PollInterval() time.Duration
	// Child names the resource pushed when the user presses Enter on a
	// row; "" marks a leaf.
	Child() string
	// ChildScope builds the drilled-down scope from the selected row.
	ChildScope(parent Scope, row Row) Scope
	Actions() []Action
	// Describe returns the detail object rendered by the describe view.
	Describe(ctx context.Context, c *dbx.Clients, scope Scope, row Row) (any, error)
}
