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
replaces the default browser above the picker on the first profile selection.
A scoped launch also seeds the levels above it — `registry.ScopeAncestors` maps
each of `Def.Args()` to a `resource.ScopeLevel{Def, Scope, Select}`: the
resource that lists that arg (`catalog` → catalogs, `schema` → schemas scoped by
that catalog, via `Registry.ScopeLister`'s singular-name match, also used by
shell completion) plus the value chosen from it. So `tables main.silver` opens
with the stack `profiles → catalogs → main → silver`: breadcrumbs show the full
path and `esc` walks back up a level at a time. Each seeded level gets
`Browser.SetSelect(value)` so `esc` reveals it with the cursor already on the row
it came from (a one-shot applied on first load, matching `Row.ID` or
`resource.RowNamer` like auto-open; `component.Table.SelectID` moves the
cursor). Auto-open does the same for the launch item, so `esc` out of an opened
item returns to its row rather than the top of the list. The picker is the same
idea at the top of the stack: `view.NewPicker(th, profiles, current)` starts on
the profile in use (`app.currentProfile`), so `esc` all the way up — or `p` /
`ctrl+p` — lands on the current profile. `Model.Init` (and the
`ProfileSelectedMsg` handler) therefore init **every** view on the stack, not
just the top, and `engine.DataEvent` is **broadcast** to the whole stack
(`Model.broadcast`) rather than forwarded to the top view — data is addressed by
`engine.Key`, and every view below the top (launch ancestors, drill-down
parents) keeps watching its own key, so a revealed ancestor already has its rows
instead of sitting on "loading …". Keys and everything else still go to the top
view only. Args are passed to `app.New` as a `[]string` (not a joined
string) so a quoted item name with spaces survives. Validation lives in
`validateLaunch` (cmd, pre-TUI stderr errors) mirrored by `app.launchViews`
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
- `ctrl+c` is reserved for quit (with confirm when work is in flight). SQL execute
  = `shift+enter` **or** `ctrl+e`, cancel = `ctrl+k`. `shift+enter` only reaches
  the app in terminals that speak the Kitty keyboard protocol / modifyOtherKeys2
  (Bubble Tea requests both by default); elsewhere it arrives as a plain `enter`,
  which is why `ctrl+e` stays bound as the universal fallback. Both are handled
  in `SQLView.handleKey` before the editor/results focus split, so execute works
  from either pane while plain `enter` keeps its per-focus meaning (newline in
  the editor, row detail in the results).
- Data viewer (`SQLView` results grid): the header line is **pinned** — `gridLines[0]` renders as a static line above the viewport; only `gridLines[1:]` (data rows) scroll (`renderResults`). Vertical keys move a **row cursor** (`selRow`, kept in view via `rowOff`/`SetYOffset`, highlighted with `Reverse(true).Bold(true)`), not the raw viewport: `j/k`+arrows = one row, `pgup/pgdn` = one page. Horizontal: `h/l`+arrows step `hStep` (6) cols, `home/end` page a near-screen width (`hPage`), all via `xoff` + `ansi.Cut`. `enter` pushes `view.RowDetail` — a scrollable COLUMN/VALUE list showing the selected row's full, untruncated, wrapped values (the grid truncates cells at `maxCell`). `esc` pops back. Both the grid body and `RowDetail` reserve a right-hand column for `component.Scrollbar` (shaded `█`/`░` bar) when content overflows — a pure formatter over viewport metrics (`TotalLineCount`/`VisibleLineCount`/`YOffset`); reuse it for any viewport-backed view.
- Reserved single keys (do not bind in def Actions): global `q p a ? : J C P A`, browser `d s f t r j k o O /` (j/k = navigation, tab/shift+tab = cycle tabs *and* any internal focus stops (see the tab-conflict note below), `o` = open in browser when the def implements `resource.WebLinker`, `O` = secondary web link when it implements `resource.AltWebLinker` — apps use `o` for the workspace page and `O` for the deployed app; detail screens
also bind `o` — see the `view.WebLink` note below). `a` opens the About splash (`view.About`: centred logo, build metadata, project URL, copyright), `?` opens help. Check `?` help for the live map.
- **`o` on detail screens (`view.WebLink`):** the Browser derives `o` per row from
  `resource.WebLinker`, but detail screens (`Describe`, `LogView`, `LogTable`,
  `SQLView`) have no def to ask, so whoever opens them supplies the URL. They embed
  the `webLink` mixin (`internal/ui/view/weblink.go`) which owns the key, the hint,
  and the "no URL → leave `o` unbound" rule; the opening message carries it —
  `OpenLogMsg/OpenLogTableMsg/OpenSQLMsg.Web`, and `OpenTabsMsg.Web` as the default
  every tab inherits unless its `TabSpec.Web` overrides (tables: table page for the
  details tab, `?activeTab=sampleData` for the data tab; apps: workspace page for
  details, the app's own `/logz` for logs). `app.withWebLink` attaches it at
  construction via `view.WebLinkSetter`; `Browser.describeRow` passes the row's own
  link through so `d` then `o` matches `o` from the list. Build links with the
  helpers in `internal/resources/weburl.go` (`tableLink`, `jobRunLink`,
  `pipelineUpdateLink`, `appLogsLink`, …) — they take the `(url, ok)` pair straight
  through, so an unknown host silently means "no link". Exception: `SQLView` binds
  `o` only with the **results** focused — with the editor focused `o` is a typed
  character.
- **Navigation best practice — resolving `tab` conflicts:** when a view inside a `Tabbed` container also wants `tab` for its own focus movement (e.g. `SQLView`'s editor↔results split), do NOT let the two fight over the key. Instead fold the view's internal focus stops into the global cycle: implement `view.TabCycler` (`AdvanceFocus(forward)` moves internal focus and returns `true`, or `false` at its boundary so the container switches tabs; `EnterFocus(forward)` lands on the entry stop when the cycle arrives). `Tabbed.cycle` walks columns → data-editor → data-results → details on `tab` and reverses on `shift+tab` (this is the only tab-switch key — `[`/`]` are *not* bound). A view keeps its own `tab` handler only for standalone (non-tabbed) use, e.g. `SQLView` via `OpenSQLMsg`. Apply the same pattern to any future screen with a `tab`-key conflict.
- **Never paint the terminal's last column on the bottom line.** `component.StatusBar.Render`
  lays the bar out in `width-1` and leaves the final cell blank on purpose. The status bar is
  the last line of the frame, so its final character sits in the bottom-right cell; writing
  there parks the cursor in the pending-wrap state, and a terminal that mishandles that wraps
  the line and scrolls the whole frame up by one. Bubble Tea's diff renderer only repaints that
  cell when the line's content *shifts*, which is exactly what the freshness timer does when it
  grows a digit — so the corruption surfaced as "`9s ago` → `10s ago` breaks the layout" (it
  also fired on each refresh, when `⟳ refreshing…` swaps back to `⟳ 0s ago`). Verify with a pty
  capture: a healthy frame contains **no** `ESC[?7l` autowrap guards, because nothing reaches
  the last column.
- teatest goldens: pin terminal size and color profile in `TestMain`, inject the clock for "Ns ago" badges; `.gitattributes` marks `*.golden -text`.
- Rate limits: workspace SCIM ≈4 req/s (identity resources use 15m poll + manual refresh), jobs list 20/s. Respect per-resource `PollInterval()`.
- `~/.databrickscfg` may contain non-profile sections like `[__settings__]` — the profile parser must skip them.
- App logs have no SDK call: they stream over a WebSocket at `<App.Url>/logz/stream` on the app's own host (not the workspace host; `/logz` itself is just the HTML viewer). `appsDAO.GetLogs` is the one sanctioned raw authenticated connection in `dbx` — it dials with `golang.org/x/net/websocket`, copies auth headers from `w.Config.Authenticate`, sends the search filter (empty = all logs, required before the server streams), then drains `{timestamp,source,severity,message}` frames (timestamp is epoch **seconds**, not ISO) into `[]AppLogEntry` until an idle gap (a lone NUL byte = "no logs"). App hosts may require app-scoped OAuth, so a plain PAT can be rejected — the error surfaces to the viewer.
- App logs render in `view.LogTable` (not the plain `LogView`): a `component.Table`-based record list — one collapsed line per record (TIME/SEV/MESSAGE), severity-colored, `/` filters the *full* record (not just the truncated cell), `enter` expands the selected record to pretty JSON, `s` sorts, `f`/`+`/`-` drive follow. The severity color falls back to a level word detected at the start of the message when the structured `severity` is `UNKNOWN`. The apps `l` action emits `view.OpenLogTableMsg`; the Enter tab uses `LogTableTabSpec`.
- Profile highlight colors: there is **no prod auto-detection** — the UI is always the orange theme. A user opts a profile into a colored header by pressing `c` on the profile picker (`view.ColorPicker`), which writes an exact-name entry to config `skins:` (`config.SaveSkin`, the only code that writes config back to disk; it touches only the `skins` map and drops YAML comments). `skins:` maps profile-name globs → a color name from `theme.accents`; the color is the **background** of a name+host chip in the header (`theme.HighlightColor`, resolved in `app.View`; foreground picked by `theme.Contrast` for legibility on any accent), leaving the rest of the UI on the default accent. Config records its source path (`Config.path`) so SaveSkin writes back to the file it was loaded from.
- Table view tabs are `columns │ data │ details │ properties` (`--tab` accepts all
  four). Each pane owns one thing: the **details** tab renders a `tableSummary`
  (catalog/schema + the `dbx.Table` fields + *counts* of columns and properties),
  deliberately **not** the raw `dbx.TableDetail` — the columns list and the
  properties map would bury it (UC returns a `spark.sql.statistics.colStats.*`
  entry per column per statistic: 723 properties on one real gold table, 712 of
  them generated). The **properties** tab is a `view.Tree` over
  `resources.propTree`, which splits the dotted keys into a trie (`delta.feature.
  appendOnly` → delta → feature → appendOnly) so that whole generated block
  collapses to one `▸ spark: {712}` line and sorts last. `d` on a table *row*
  still dumps the whole `TableDetail` as the raw escape hatch.
- Epoch timestamps in properties get a `TreeNode.Note` gloss — `lastCommitTimestamp:
  1785273330000 (2026-07-28 16:15:30.000)`, raw value kept, note greyed. Nothing
  proves an integer is a time, so `resources.epochNote` needs **both** halves of
  the guess: the name reads like a time (`_at`/`At` suffix — not a bare "at",
  which would match "format" — or contains created/updated/modified/deleted/
  expire/timestamp/time/date) **and** the digit count maps to a precision
  (10=s, 13=ms, 16=µs, 19=ns) that lands in 2000–2100. So `lastUpdateVersion:
  6215` and `deltaFileStatistics: \`load_date\`` stay untouched. Keep the
  heuristic in `resources` (property semantics), not in `view.Tree`.
- `view.Tree` is the generic collapsible tree (jless-style), reusable for any
  nested key/value data — declare a tab with `TabSpec.Tree` (`TreeTabSpec.Fetch`
  returns `[]view.TreeNode`; the app builds it via `NewLazyTree`) or construct it
  directly with `NewTree`. It opens **fully expanded**; `left/h` collapses (or
  jumps to the parent on a leaf / already-closed branch), `right/l` expands (or
  steps into an open one), `-`/`+` fold and unfold everything, `enter` toggles a
  branch and pushes a `RowDetail` with the full untruncated value on a leaf, `/`
  filters paths *and* values (survivors keep their ancestors and their whole
  subtree, and collapse state is ignored while filtering so a hit can't hide
  inside a closed branch). The forest is flattened into `[]treeItem` once on load
  — collapse and filter only recompute `visible`, keyed by dotted path so the
  cursor sticks to its node across both. Producers own the nesting (dotted keys,
  JSON, …); the view owns state and rendering.
- CLI launch item selection: a trailing positional beyond a resource's scope args is `Command.Item` — the row to open directly (`apps my-app`, `jobs 'Nightly ETL'`, `tables main.silver orders`). `/text` is still the list pre-filter, distinct from Item. Auto-open matches Item against `Row.ID` or `resource.RowNamer.RowName` (jobs match by name; Row.ID is numeric). Two trailing positionals = error.
