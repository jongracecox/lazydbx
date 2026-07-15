package view

import (
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
