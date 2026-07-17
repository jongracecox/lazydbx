# lazydbx

A k9s-style terminal UI for Databricks: `:` command mode, `/` filter, Enter/Esc
drill-down, breadcrumbs, header key hints. Go + Bubble Tea v2. Generic tool —
no company-specific assumptions ever.

The full project plan (architecture rationale, phases, research) lives in
`docs/PLAN.md`.

## Commands

```bash
make build          # build ./bin/lazydbx
make test           # go test -race ./...
make cover          # coverage report (opens HTML)
make lint           # golangci-lint run
make fmt            # golangci-lint fmt (gofumpt + goimports)
make tidy           # go mod tidy -diff (check only)
make tools          # install golangci-lint + lefthook, register hooks
go test ./internal/app -update   # regenerate golden files
go run ./cmd/lazydbx --profile <name>   # run against a profile
go run ./cmd/lazydbx -p <name> tables main.silver   # launch into a resource
go run ./cmd/lazydbx -p <name> tables main.silver orders   # open one table
go run ./cmd/lazydbx -p <name> apps my-app --tab logs   # open item onto a tab
```

Positional launch args (`lazydbx [flags] [resource [args...] [item] [/filter]]`)
reuse the `:` command grammar via `registry.ParseArgs`; the parsed command
replaces the default browser above the picker on the first profile selection
(`esc` → picker). Args are passed to `app.New` as a `[]string` (not a joined
string) so a quoted item name with spaces survives. Validation lives in
`validateLaunch` (cmd, pre-TUI stderr errors) mirrored by `app.launchView`
(in-app fallback). Root uses `cobra.ArbitraryArgs` so args fall through to `RunE`
while `version`/`completion`/`__prefetch` still route as subcommands.

Grammar (`registry.ParseArgs`): first field = resource; next positionals map to
`Def.Args()` (leading `main.silver` is dotted sugar); **one** positional beyond
the scope args is `Command.Item` (the row to open directly); a trailing `/text`
is `Command.Filter` (list pre-filter). `Item != ""` → `browser.SetAutoOpen(item,
tab)`, which opens that row once it loads. Auto-open matches `Row.ID` OR
`resource.RowNamer.RowName(row)` — jobs implement `RowNamer` (Row.ID is the
numeric id; the NAME cell is the CLI handle), so `jobs 'Nightly ETL'` works.

`--tab <name>` selects the item's initial tab: needs a `resource.Tabber` (whose
`Tabs()` lists names statically — apps/tables/taskruns/updates) plus an Item.
`validateLaunch`→`validateTab` checks all three pre-TUI; on open `enterRowTab`
sets `OpenTabsMsg.Active` to the matching tab index (→ `NewTabbed(..., active)`).

Shell completion lives in `cmd/lazydbx/completion.go` (cobra `ValidArgsFunction`
+ `RegisterFlagCompletionFunc`; install via `lazydbx completion <bash|zsh|fish>`).
It completes resource names and `--tab`/`--profile`/`--log-level` values, and —
when a profile resolves (`--profile`, `$DATABRICKS_CONFIG_PROFILE`, or config) —
scope args and item names (all bare) *from the workspace*: `scopeArgLister` finds
the parent lister by singular-name match; item names come from the def's own
rows projected via `RowNamer`. **Completion never blocks on the network**: it
serves the on-disk cache (reused from `engine.Store`, shared with the TUI, 5-min
TTL) and, on a cold/stale entry, spawns a detached `__prefetch` child (see
`spawnPrefetch`/`detach_*.go`) that refreshes the cache for the next press — so a
cold entry completes to nothing the first time, then warms.

Logs go to `$XDG_STATE_HOME/lazydbx/lazydbx.log` (macOS: `~/Library/Application Support/lazydbx/`).
Never print to stdout/stderr while the TUI runs.

## Architecture

```
cmd/lazydbx        → cobra entry; flags → config → app
internal/app       → root tea.Model: view stack, ALL messages (messages.go), global keys, ':' exec
internal/ui/view   → body views: browser (generic list), preview, sqledit, logview, describe, picker, help
internal/ui/component → dumb widgets: table, cmdbar, filterbar, header, statusbar, breadcrumbs, confirm
internal/resource  → core abstraction: ResourceDef, Row, Scope, Action, registry (NO SDK imports)
internal/resources → one file per resource def; the ONLY code calling DAOs; register.go wires all
internal/dbx       → the ONLY package importing databricks-sdk-go: profiles, clients, DAOs, statement
internal/engine    → poll/cache: per-key goroutines, stale-while-revalidate, overlap-drop, backoff
internal/{config,theme,logging,version} → leaves
```

Dependency direction: `cmd → app → ui/view → resource ← resources → dbx`, engine between view and resources. One sanctioned extra edge: `resources → ui/view` for actions that return view messages (e.g. tables' preview action returns `view.OpenSQLMsg`); `ui/view` must never import `resources`.

## Iron rules

1. Only `internal/dbx` imports `databricks-sdk-go`. Everything else uses the narrow DAO interfaces in `dbx/dao.go`.
2. Views never call DAOs or do I/O. Data arrives as messages from the engine; `Update` functions stay pure (return commands, never block).
3. Tea messages live in exactly two files: cross-package UI messages (nav, flash, drill-down, profile selection) in `internal/ui/view/msgs.go`; app-internal ones in `internal/app/messages.go`. Engine data arrives as `engine.DataEvent`. Nowhere else.
4. A new resource = one file in `internal/resources/` + a line in `register.go` + a fake-DAO test. Nothing else should need touching (use `/new-resource`).
5. The Databricks SDK version is pinned exactly (pre-1.0, breaking changes ~every other release). Never bump it as a side effect of other work.
6. Mutating actions must set `Dangerous: true` (confirm dialog) and are hidden entirely under `--readonly`.
7. Never fetch secret values — secrets views show metadata only. There is no GetSecret call anywhere, by policy.
8. All colors come from `internal/theme` — no hardcoded colors in views/components.

## Conventions

- Table-driven tests with testify (`assert`/`require`); fake DAOs are structs of func fields — do NOT use the SDK's `experimental/mocks`.
- gofumpt formatting; goimports local prefix `github.com/jongracecox/lazydbx`.
- Errors wrapped with `%w` and context: `fmt.Errorf("loading profile %s: %w", name, err)`.
- Key bindings use `charm.land/bubbles/v2/key` so help/hints derive mechanically from bindings.
- Commit messages: conventional-commit style (`feat:`, `fix:`, `docs:`, `chore:`).

## Gotchas

- Bubble Tea v2 import paths are `charm.land/...` (vanity), not `github.com/charmbracelet/...`.
- v2 API: `View() tea.View` (set `v.AltScreen = true` in the view, not a program option); `lipgloss.Color(...)` is a constructor returning `color.Color`, not a type; keys match via `tea.KeyPressMsg.String()` (e.g. `"ctrl+r"`).
- `ctrl+c` is reserved for quit (with confirm when work is in flight). SQL execute = `ctrl+e`, cancel = `ctrl+k`.
- Reserved single keys (do not bind in def Actions): global `q p ? : J C P A`, browser `d s f t r j k o O / [ ]` (j/k = navigation, [/] = jump whole tabs, tab/shift+tab = cycle tabs *and* any internal focus stops (see the tab-conflict note below), `o` = open in browser when the def implements `resource.WebLinker`, `O` = secondary web link when it implements `resource.AltWebLinker` — apps use `o` for the workspace page and `O` for the deployed app). Check `?` help for the live map.
- **Navigation best practice — resolving `tab` conflicts:** when a view inside a `Tabbed` container also wants `tab` for its own focus movement (e.g. `SQLView`'s editor↔results split), do NOT let the two fight over the key. Instead fold the view's internal focus stops into the global cycle: implement `view.TabCycler` (`AdvanceFocus(forward)` moves internal focus and returns `true`, or `false` at its boundary so the container switches tabs; `EnterFocus(forward)` lands on the entry stop when the cycle arrives). `Tabbed.cycle` walks columns → data-editor → data-results → details on `tab` and reverses on `shift+tab`; `[`/`]` stay as coarse whole-tab jumps that skip internal stops (and remain literal characters while the active tab `CapturesKeys`, e.g. typing in the editor). A view keeps its own `tab` handler only for standalone (non-tabbed) use, e.g. `SQLView` via `OpenSQLMsg`. Apply the same pattern to any future screen with a `tab`-key conflict.
- teatest goldens: pin terminal size and color profile in `TestMain`, inject the clock for "Ns ago" badges; `.gitattributes` marks `*.golden -text`.
- Rate limits: workspace SCIM ≈4 req/s (identity resources use 15m poll + manual refresh), jobs list 20/s. Respect per-resource `PollInterval()`.
- `~/.databrickscfg` may contain non-profile sections like `[__settings__]` — the profile parser must skip them.
- App logs have no SDK call: they stream over a WebSocket at `<App.Url>/logz/stream` on the app's own host (not the workspace host; `/logz` itself is just the HTML viewer). `appsDAO.GetLogs` is the one sanctioned raw authenticated connection in `dbx` — it dials with `golang.org/x/net/websocket`, copies auth headers from `w.Config.Authenticate`, sends the search filter (empty = all logs, required before the server streams), then drains `{timestamp,source,severity,message}` frames (timestamp is epoch **seconds**, not ISO) into `[]AppLogEntry` until an idle gap (a lone NUL byte = "no logs"). App hosts may require app-scoped OAuth, so a plain PAT can be rejected — the error surfaces to the viewer.
- App logs render in `view.LogTable` (not the plain `LogView`): a `component.Table`-based record list — one collapsed line per record (TIME/SEV/MESSAGE), severity-colored, `/` filters the *full* record (not just the truncated cell), `enter` expands the selected record to pretty JSON, `s` sorts, `f`/`+`/`-` drive follow. The severity color falls back to a level word detected at the start of the message when the structured `severity` is `UNKNOWN`. The apps `l` action emits `view.OpenLogTableMsg`; the Enter tab uses `LogTableTabSpec`.
- CLI launch item selection: a trailing positional beyond a resource's scope args is `Command.Item` — the row to open directly (`apps my-app`, `jobs 'Nightly ETL'`, `tables main.silver orders`). `/text` is still the list pre-filter, distinct from Item. Auto-open matches Item against `Row.ID` or `resource.RowNamer.RowName` (jobs match by name; Row.ID is numeric). Two trailing positionals = error.
