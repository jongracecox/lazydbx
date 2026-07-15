package app

import "time"

// App-internal messages. Cross-package UI messages (nav, flash, profile
// selection) live in internal/ui/view/msgs.go; engine data events are
// engine.DataEvent. This file holds only what the app model sends itself.

// tickMsg is the 1s UI heartbeat: expires flashes and re-renders freshness
// badges. It carries no data payload — views compute ages from time.Now at
// render.
type tickMsg time.Time
