# lazydbx — k9s-style TUI for Databricks: Project Plan

## Context

Databricks has a wide, heterogeneous surface (jobs, runs, pipelines, clusters, warehouses, Unity Catalog, secrets, apps, identity, account-level admin) that today requires either the web UI or memorizing many CLI/API incantations. k9s proved that a fast, keyboard-driven, self-documenting TUI beats both for day-to-day ops work. **lazydbx** replicates the k9s experience for Databricks: blazing-fast navigation, `:` command mode, universal Enter/Esc drill-down, ambient key-hint discoverability, and a colorful, joyful terminal UI. The tool is generic (no company-specific assumptions), uses existing `~/.databrickscfg` profiles, and is the author's first Go project — so best practices (testing, linting, CI, releases) are baked in from commit one.

## Decisions made (with user)

| Decision | Choice |
|---|---|
| Framework | **Bubble Tea v2** (`charm.land/bubbletea/v2` + bubbles/v2 + lipgloss/v2) — testable Elm architecture, fast v2 renderer, ecosystem momentum |
| Layout | **k9s-style**: one full-screen table at a time, `:` command mode, `/` local filter, Enter/Esc view stack, breadcrumbs, header key hints, `?` help |
| Phase-1 verticals | **Catalog + SQL** and **Jobs + Pipelines** |
| Safety | **Read-only first**; mutations arrive later behind confirm dialogs; global `--readonly` flag from day one |

## What makes k9s loved (replication targets)

1. **Command mode** `:` — composable teleport (`:pod ns-x /fred`), aliases, autocomplete; resolution chain: user aliases → registry.
2. **Universal drill-down** — every view is a table; Enter descends the natural hierarchy, Esc pops, breadcrumbs show the trail; `[`/`]` history.
3. **Ambient discoverability** — header shows verb keys valid for the current view; `?` full help; `ctrl-a` all commands.
4. **Instant feel** — local cache; `/` filter is local; first paint ~300ms then relaxed steady-state; UI tick decoupled from API polling; delta-only repaints; overlap-drop + backoff.
5. **Safety & extensibility** — confirm dialogs, `--readonly`, per-context skins (red = prod), YAML aliases/hotkeys/plugins.

## Key research facts (Databricks SDK)

- `github.com/databricks/databricks-sdk-go` v0.159.x, **pre-1.0, breaking changes in ~half of releases → pin exact version, deliberate upgrades with CHANGELOG**.
- Auth: unified auth honors `~/.databrickscfg` profiles, env vars, and the Databricks CLI OAuth token cache (`databricks auth login` sessions are inherited). Clients immutable per profile → cache one client per profile. **No profile-enumeration API → parse the INI ourselves** (`gopkg.in/ini.v1`).
- Account level: `AccountClient` needs accounts host + `account_id`; detect via `Config.HostType()`; degrade gracefully.
- Coverage is complete for phase 1: `w.Catalogs/Schemas/Tables` (TableInfo includes columns; `ListSummaries` for big schemas), `w.StatementExecution` (INLINE+JSON_ARRAY+RowLimit for previews; async = `WaitTimeout:"0s"` + poll + `CancelExecution`; EXTERNAL_LINKS presigned URLs get NO auth header), `w.Jobs` (`ListRuns`, `GetRun`, `GetRunOutput(taskRunId)` — logs capped 5MB, call with *task* run id), `w.Pipelines` (`ListPipelineEvents` with filter/order), `w.Warehouses`, `w.Secrets`, `w.Apps` (start/stop/status), `w.UsersV2/GroupsV2/ServicePrincipalsV2` (+account variants), `w.Permissions`.
- Pagination: `listing.Iterator[T]` everywhere; `ToSliceN` to bound initial loads, lazy-load on scroll.
- Built-in: retries (429/503, Retry-After), ctx cancellation, concurrency-safe clients, client-side 15 rps limiter, `apierr.APIError` for friendly 403/404 states. Set `databricks.WithProduct("lazydbx", version)`; redirect SDK logger to file.
- **Rate-limit hotspots**: workspace SCIM ≈4 req/s (identity views: long TTL + manual refresh only), secrets 1100/min, jobs list 20/s. Poll intervals per resource type, only the visible view refreshes on a timer.
- **Gaps needing raw REST/none**: Databricks Apps logs (undocumented `wss://<app-url>/logz/stream`, OAuth user token, best-effort, deferred), cluster driver logs (no API — only ClusterLogConf delivery read via Files/DBFS), notebook stdout (only exit value via `GetRunOutput`). Escape hatch for new endpoints: `client.New(cfg).Do(...)` keeps SDK auth/retries.
- Everything is poll-based (no watch API) → per-resource pollers → shared cache → decoupled UI ticks (k9s emulation).

## Tech stack ("shopping list")

| Purpose | Choice |
|---|---|
| TUI | `charm.land/bubbletea/v2`, `charm.land/bubbles/v2`, `charm.land/lipgloss/v2` |
| Tables | bubbles/v2 table + custom row-windowing (Evertras/bubble-table v2-compat unverified — decide in Phase 1) |
| Databricks | `databricks-sdk-go` (pinned exact) |
| CLI entry | cobra (minimal: root TUI, `version`, `config`) |
| Config | koanf/v2 + `adrg/xdg` (config `$XDG_CONFIG_HOME/lazydbx/`, cache `$XDG_CACHE_HOME/lazydbx/`, logs+state `$XDG_STATE_HOME/lazydbx/`) |
| Logging | `log/slog` → file only (never stdout/stderr while TUI owns terminal) |
| Tests | stdlib + testify, table-driven; `teatest/v2` + golden files (`x/exp/golden`; pin color profile in tests; `.gitattributes: *.golden -text`) |
| Lint/format | golangci-lint v2 (standard set + curated extras; gofumpt as formatter) |
| Hooks | lefthook (pre-commit: fmt + lint on staged; pre-push: test + `go mod tidy -diff`) |
| CI | GitHub Actions: `ci.yml` (test matrix ubuntu/macos, lint via golangci-lint-action@v8, govulncheck), `release.yml` (tag `v*` → goreleaser v2) |
| Release | goreleaser v2: darwin/linux/windows × amd64/arm64, checksums, changelog, **homebrew_casks** → personal `homebrew-tap` repo (needs PAT secret) |
| Repo layout | `cmd/lazydbx/main.go` + `internal/` only (no `pkg/`) |

## Architecture

### Package structure

```
lazydbx/
├── cmd/lazydbx/main.go        # cobra root: --profile, --readonly, --log-level; wires config→app; runs tea.Program
├── internal/
│   ├── app/                   # Root tea.Model: view stack, message routing, overlays
│   │   ├── app.go             # Init/Update/View; composes header/body/statusbar/overlays
│   │   ├── stack.go           # View stack: Push/Pop, breadcrumb derivation, cursor memory
│   │   ├── keys.go            # Global keys (:, /, ?, esc, ctrl+r, q) — resolved before view-local keys
│   │   ├── messages.go        # ENTIRE message taxonomy in one file
│   │   └── exec.go            # ':' command parse → registry resolve → push view
│   ├── ui/view/               # Things occupying the body region; all implement view.View
│   │   ├── view.go            # View interface + Frame
│   │   ├── browser.go         # THE generic resource browser (one impl for all list views)
│   │   ├── preview.go         # Data preview: wide table, horizontal scroll
│   │   ├── sqledit.go         # SQL editor: textarea + results + statement status
│   │   ├── logview.go         # Log viewer: viewport + search + follow
│   │   ├── describe.go        # YAML-ish detail render
│   │   ├── picker.go          # Profile picker (first screen if no --profile)
│   │   └── help.go            # ? overlay
│   ├── ui/component/          # Dumb widgets: table.go cmdbar.go filterbar.go header.go statusbar.go breadcrumbs.go confirm.go
│   ├── resource/              # Core abstraction — zero SDK imports: def.go registry.go rows.go
│   ├── resources/             # One file per resource def; the ONLY code calling DAOs. register.go wires all.
│   ├── dbx/                   # The ONLY package importing databricks-sdk-go
│   │   ├── profiles.go        # ~/.databrickscfg INI parsing (gopkg.in/ini.v1)
│   │   ├── clients.go         # Per-profile WorkspaceClient/AccountClient cache; WithProduct
│   │   ├── dao.go / dao_impl.go  # Narrow hand-rolled DAO interfaces + thin SDK impls
│   │   └── statement.go       # StatementExecution: submit(WaitTimeout=0s), poll, cancel, decode
│   ├── engine/                # cache.go (keyed rows store) + poller.go (per-key goroutine, jitter, overlap-drop, backoff)
│   ├── config/                # koanf: defaults → config.yaml → env LAZYDBX_ → flags; aliases.yaml
│   ├── theme/                 # lipgloss theme, builtin skins, per-profile accent (prod = red)
│   ├── logging/               # slog → $XDG_STATE_HOME/lazydbx/lazydbx.log; SDK logger bridged
│   └── version/
```

Dependency direction: `cmd → app → ui/view → resource ← resources → dbx`, with `engine` between view and resources. **Iron rules**: only `internal/dbx` imports the SDK; only `internal/resources` calls DAOs; views never do I/O — data arrives as messages from the engine.

### Core abstraction (the k9s trick)

Registry and UI see a **non-generic** `ResourceDef` interface trafficking in pre-rendered rows; each concrete def uses **generic helpers** so column extraction is type-safe inside its own file only:

```go
type Scope map[string]string            // {"catalog":"main","schema":"silver"} or {"job_id":"123"}
type Row struct{ ID string; Cells []string; Data any }
type Column struct{ Title string; Width int; Wide bool }

type Action struct {
    Key, Name string
    Dangerous bool   // confirm dialog; hidden entirely under --readonly
    Run       func(ctx context.Context, c *dbx.Clients, scope Scope, row Row) tea.Msg
}

type ResourceDef interface {
    Name() string; Aliases() []string
    Args() []string                    // positional scope keys: tables → ["catalog","schema"]
    Columns() []Column
    List(ctx context.Context, c *dbx.Clients, scope Scope) ([]Row, error)
    PollInterval() time.Duration
    Child() string                     // resource pushed on Enter ("" = leaf)
    ChildScope(parent Scope, row Row) Scope
    Actions() []Action
    Describe(ctx context.Context, c *dbx.Clients, row Row) (any, error)
}

// generic helpers: ColSpec[T]{Column, Extract func(T) string} + BuildRows[T](items, id, specs) []Row
```

- The generic `browser.go` renders any def; Enter pushes `NewBrowser(registry.Get(def.Child()), def.ChildScope(...))`; Esc pops.
- `:` parsing: `:tables main silver` (or `:tables main.silver`) maps positional args to scope keys; trailing `/text` pre-seeds the filter; aliases resolve first; autocomplete fed by registry.
- Footer/header key hints generate mechanically from `Actions()` + globals — hints can never drift from behavior.
- Non-table views (preview, sqledit, logview, describe) implement `View` directly and are pushed by Actions.

### Bubble Tea architecture

- One root `app.Model` owning `stack []view.Frame` + at most one active overlay (cmdbar/filterbar/help/confirm). Update routing: window size → global keys → overlay → top view.
- `view.View` interface: `Init/Update/View(w,h)/Title()/Hints()`.
- Message taxonomy (all in `app/messages.go`): `navPushMsg/navPopMsg`, `dataMsg{key, rows, err, fetchedAt}`, `stmtSubmittedMsg/stmtPollMsg/stmtDoneMsg`, `statusFlashMsg`, `tickMsg` (500ms cosmetic heartbeat), `promptMsg`, `profileSwitchedMsg`. Views never mutate shared state — pure Updates returning msgs/cmds.
- **Pollers run OUTSIDE the tea loop** (k9s pattern): engine gets `p.Send` as its sink. Browser Init emits `engine.Watch(key{profile,resource,scopeHash})` (refcounted). First watch: serve cached entry synchronously (stale-while-revalidate = instant paint), then poll goroutine: fetch now → tick at `PollInterval()` ±10% jitter → `atomic.Bool` overlap-drop → error backoff (double to 5m, keep stale rows + red badge). Unwatch on pop cancels goroutine; cache survives. `ctrl+r` = RefreshNow. SCIM resources: 15m interval, manual refresh.
- **Exception**: statement polling is a per-view self-rescheduling `tea.Cmd` loop (800ms), not engine state; cancel key fires `CancelExecution`.
- `ctrl+c` intercepted (Bubble Tea default-quits): confirm quit + auto-cancel in-flight statements. SQL execute = `ctrl+e`, cancel = `ctrl+k`.

## Implementation phases (each ends runnable)

### Phase 0 — Scaffold, CI, hello-TUI (~½–1 day)
`go mod init github.com/<owner>/lazydbx`; pin exact deps (bubbletea/bubbles/lipgloss v2, databricks-sdk-go exact, koanf, ini.v1, adrg/xdg, cobra, testify, yaml.v3). Files: main.go, version, logging (slog→file + SDK logger bridge), config, splash-screen app.go, default theme. Tooling: `.golangci.yml` (v2: standard + revive, gocritic, misspell, unparam, copyloopvar, testifylint; gofumpt formatter), `lefthook.yml`, `.github/workflows/ci.yml` (ubuntu/macos matrix: test, golangci-lint-action@v8, govulncheck) + `release.yml` (goreleaser v2, homebrew_casks → personal tap, needs PAT secret), `.goreleaser.yaml`, `.gitattributes` (`*.golden -text`), CLAUDE.md + skills.

**Test coverage wiring** (Phase 0, in ci.yml):
- `go test -race -coverprofile=coverage.out -covermode=atomic ./...`; local `make cover` opens `go tool cover -html`.
- Convert to Cobertura XML with `gocover-cobertura` → `coverage.xml` artifact.
- PR visibility without an external service: `irongut/CodeCoverageSummary` renders the XML into the job summary + a markdown report, posted/updated as a sticky PR comment via `marocchino/sticky-pull-request-comment` — every PR shows overall % and per-package deltas in the GitHub UI. (Codecov can be swapped in later if richer diff-coverage/graphs are wanted.)

**Verify**: `go run ./cmd/lazydbx` shows splash; `lazydbx version`; CI green with a coverage summary visible on the first PR.

### Phase 1 — App shell + generic browser on catalogs (~2–4 days) — the make-or-break phase
All §Architecture abstractions built, exercised by one simple unscoped resource (**catalogs**: fast List API, simple columns NAME/OWNER/TYPE/COMMENT — and it's the root of the UC vertical the user cares most about). Files: resource/{def,registry,rows}+tests, engine/{cache,poller}+tests, dbx/{profiles,clients,dao,dao_impl}+INI-fixture tests, resources/{register,catalogs}+fake-DAO test, ui/view/{view,browser,picker,help}, ui/component/{table,cmdbar,filterbar,header,statusbar,breadcrumbs}, app/{stack,keys,messages,exec}, theme/skins (per-profile accent globs, e.g. `PROD-*: red`). Table = wrapper around bubbles/v2 table (swap internals later).
**Verify**: launch → profile picker → live catalogs table: polling, stale badges, `/` filter, `:catalogs`/`:cat`, `?`, `d` describe, breadcrumbs, prod accent color.

### Phase 2 — Unity Catalog drill-down + preview + SQL runner (~3–5 days)
resources/{schemas,tables,columns} completing the 4-level drill-down from catalogs; dbx/statement.go; preview.go (`p` on table → SELECT * LIMIT 200; horizontal scroll → likely trigger to hand-roll table internals, ~250 LOC); sqledit.go (`:sql`; `s` on table pre-fills query). **Warehouse selection: serverless by default** — resolution order: (1) explicit config `sql.warehouse_id`, (2) first RUNNING serverless warehouse, (3) any serverless warehouse (auto-starts on first query), (4) picker fallback listing all warehouses (small warehouses DAO — the full `:wh` browser view waits until Phase 4). Current warehouse shown in the SQL editor status line with a `ctrl+w` picker to switch.
**Verify**: browse catalog→columns, preview any table, run/cancel ad-hoc SQL against a real warehouse.

### Phase 3 — Jobs & Pipelines vertical + log viewer (~3–4 days)
resources/{jobs,runs,taskruns,pipelines,updates}; `:runs <job-id>`; logview.go (task run `l` → GetRunOutput; pipeline update `l` → ListPipelineEvents); run-state cell coloring via optional `CellStyler` interface; statusbar error-detail overlay (`!`).
**Verify**: triage a failed job run to task logs entirely in the TUI.

### Phase 4 — Compute, secrets, apps + first verbs (~2–3 days)
resources/{clusters,warehouses,secretscopes,secrets,apps}. Secrets = metadata only (**never call GetSecret** — by policy there is no reveal to redact). Apps logs deferred (websocket, undocumented). First mutating verbs: cluster start/stop, job run-now, run cancel — `Dangerous` → confirm dialog, hidden under `--readonly`.

### Phase 5 — Identity, permissions, account level (~2–3 days)
resources/{users,groups,serviceprincipals,grants} (SCIM: 15m TTL + "manual refresh" hint). AccountClient wiring: account-host profiles get account badge + `:account-users`, workspaces list; degrade gracefully otherwise.

### Phase 6 — Polish & delight (ongoing)
- **Field analysis**: on the columns view, an `a` "Analyze" action on a field runs a profiling query (serverless) and renders a beautiful terminal chart view — numeric: min/max/avg/stddev/percentiles + histogram (lipgloss bar/sparkline, k9s `tchart`-style hand-rolled); date/timestamp: row-count distribution defaulting to last 7 days with selectable ranges; string: top-N value frequencies. All computed server-side via a single aggregate SQL statement so it stays fast on huge tables.
- User aliases.yaml, custom skins, command history persistence, `:xray`-style UC tree, `:pulse` dashboard (failing jobs/pipelines counts), hotkeys, k9s-style shell-out plugins if demand exists.

## Testing strategy

| Layer | Approach |
|---|---|
| dbx/profiles | Table-driven on INI fixture strings (workspace/account/malformed) — highest-value pure tests |
| resource framework | Registry alias resolution, BuildRows, `:` arg parsing, filter matching |
| resources/* defs | Fake DAOs (struct of func fields per narrow DAO interface). **Not** the SDK's experimental mocks — they churn with every SDK bump; our DAO interfaces are the insulation layer |
| engine | Injected clock + manual ticks: overlap-drop, backoff, refcounts, stale-then-fresh msg ordering. No timing assertions |
| View Update() | Pure: feed KeyPressMsg/dataMsg, assert state + returned cmds. No rendering |
| Nav flows | teatest/v2 with fake registry+DAOs: script `:warehouses → enter → esc → ?`; final frame → golden |
| Goldens | ≤~10 canonical screens; pin term size (120×40) + color profile in TestMain + injected clock for "Ns ago"; `-update` flag |
| Don't test | Live APIs, SDK internals, lipgloss, exact ANSI outside goldens, cobra plumbing, goroutine timing |

## Agentic boilerplate

**CLAUDE.md**: commands (test/lint/fmt/goldens/run/`make cover`), 10-line architecture map + dependency diagram, the iron rules (SDK only in dbx; views never call DAOs; all msgs in messages.go; new resource = 1 file + registration + fake-DAO test; pure Updates; SDK pinned exactly; Dangerous+readonly gating; never fetch secret values), conventions (table-driven testify tests, gofumpt, `bubbles/key` bindings so hints auto-derive), gotchas (charm.land import paths, teatest color pinning, SCIM limits, ctrl+c reserved).

**Skills** (`.claude/skills/`):
1. `/new-resource <name>` — scaffolds resource def + DAO addition + registration + fake-DAO test; encodes the iron rules.
2. `/run-tui [profile]` — build + launch against a profile, log-tail instructions, smoke checklist.
3. `/update-goldens` — re-run teatest with `-update` under pinned env, show diff for eyeballing.
4. `/release <version>` — checklist: clean tree, CI green, tag, verify goreleaser + tap, brew sanity-install.

## Risks & flagged decisions

1. **Table widget**: Evertras/bubble-table v2-compat unverified — don't adopt. Wrap bubbles/v2 table in Phase 1; hand-roll internals in Phase 2 when preview needs h-scroll + cell styles. Wrapper = one-file swap.
2. **SDK pre-1.0**: pin exact; Renovate with SDK in own non-automerge weekly group; DAO layer localizes breakage; budget ~½ day per bump.
3. **Apps logs**: websocket-only, undocumented — deferred; show status/URL.
4. **Secrets**: never fetch values, period.
5. **Memory bounds**: preview RowLimit 200; SQL RowLimit 10k INLINE with truncation banner; cells clipped at 120 chars (full value via describe); cache cap ~50k rows/entry. EXTERNAL_LINKS/Arrow out of scope (future export feature).
6. **16-color terminals**: lipgloss adaptive colors; manual `TERM=xterm` check in /release checklist.

## Verification (end-to-end)

- Every phase ends with a hand-runnable milestone (listed per phase above) against real Databricks profiles (a workspace profile for phases 1-4, an account-host profile for phase 5).
- `go test ./...` green + golangci-lint clean at every commit (lefthook enforces).
- Phase 1 exit bar: launch-to-first-paint < 1s on cached data; `/` filter instant; Esc/Enter/breadcrumbs/`?` all working on warehouses.
- CI green on GitHub from Phase 0; a `v0.1.0` tag after Phase 2 proves the release pipeline (goreleaser + brew tap install).

