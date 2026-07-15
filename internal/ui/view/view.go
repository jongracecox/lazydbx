// Package view defines the View interface every body view implements, the
// shared UI messages, and the concrete views (browser, describe, picker,
// help). Views are pure Bubble Tea models scoped to the body region: the app
// shell owns the header, status bar, breadcrumbs, and overlays.
package view

import (
	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

// Scoped views expose their drill-down path for header display.
type Scoped interface {
	ScopePath() string
}

// View is one screen in the navigation stack.
type View interface {
	// Init returns the view's startup command (e.g. subscribe to the
	// engine). Called when the view is pushed.
	Init() tea.Cmd
	// Update handles a message and returns the (possibly replaced) view.
	Update(msg tea.Msg) (View, tea.Cmd)
	// Render draws the view into the given body region.
	Render(width, height int) string
	// Title is this view's breadcrumb segment.
	Title() string
	// Hints lists the view-specific key bindings for the header.
	Hints() []key.Binding
	// Close releases resources (e.g. unsubscribe). Called when popped.
	Close()
}
