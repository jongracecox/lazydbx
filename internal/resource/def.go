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
	Describe(ctx context.Context, c *dbx.Clients, row Row) (any, error)
}
