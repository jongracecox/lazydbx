package view

import (
	"context"

	"github.com/jongracecox/lazydbx/internal/dbx"
	"github.com/jongracecox/lazydbx/internal/resource"
	"github.com/jongracecox/lazydbx/internal/ui/component"
)

// Cross-package UI messages. Views emit these from commands; the root app
// model routes them. App-internal messages live in internal/app; everything
// else belongs here so views and app share one vocabulary without cycles.

// Flash level aliases so views don't import component just to flash.
const (
	FlashInfo  = component.FlashInfo
	FlashWarn  = component.FlashWarn
	FlashError = component.FlashError
)

// PushMsg pushes a view onto the navigation stack.
type PushMsg struct{ View View }

// PopMsg pops the top view (Esc).
type PopMsg struct{}

// DrillDownMsg asks the app to open a child resource browser. Views emit the
// child's name and scope; the app resolves the def via the registry and
// constructs the browser (views have no registry access by design).
type DrillDownMsg struct {
	Resource string
	Scope    resource.Scope
}

// FlashMsg shows a transient message in the status bar.
type FlashMsg struct {
	Level component.FlashLevel
	Text  string
}

// ProfileSelectedMsg is emitted by the profile picker.
type ProfileSelectedMsg struct{ Profile dbx.Profile }

// OpenSQLMsg asks the app to open the SQL editor/preview view pre-filled with
// Query. When Execute is true the statement runs immediately on the view's
// Init — this is how table preview launches a ready-to-run query without the
// user pressing execute.
type OpenSQLMsg struct {
	Query   string
	Execute bool
}

// TabSpec declares one tab of an OpenTabsMsg without constructing the view
// (views need theme/engine wiring only the app has). Exactly one content
// field must be set.
type TabSpec struct {
	Name string
	// Log shows a log viewer over the fetched text.
	Log *LogTabSpec
	// Detail shows a lazily fetched describe view.
	Detail func(ctx context.Context) (any, error)
	// Browse shows a resource browser.
	Browse *BrowseTabSpec
	// SQL shows the SQL editor/preview.
	SQL *SQLTabSpec
}

// LogTabSpec parameterizes a log tab.
type LogTabSpec struct {
	Fetch  func(ctx context.Context) (string, error)
	Follow bool
}

// BrowseTabSpec parameterizes a resource-browser tab.
type BrowseTabSpec struct {
	Resource string
	Scope    resource.Scope
}

// SQLTabSpec parameterizes a SQL tab.
type SQLTabSpec struct {
	Query   string
	Execute bool
}

// OpenTabsMsg asks the app to open a tabbed view — how defs whose Enter
// outgrows plain drill-down (tables, task runs, updates) declare their
// sibling views.
type OpenTabsMsg struct {
	Title string
	Tabs  []TabSpec
}

// OpenLogMsg asks the app to open the log viewer on a text source. Fetch is
// re-invoked while following, so it must be safe to call repeatedly. This is
// how resource actions (task-run logs, pipeline events) launch the viewer
// without constructing views themselves.
type OpenLogMsg struct {
	Title  string
	Follow bool // start with follow-tail enabled (for in-flight runs)
	Fetch  func(ctx context.Context) (string, error)
}
