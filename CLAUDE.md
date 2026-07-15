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
```

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
- Reserved single keys (do not bind in def Actions): global `q p ? : J C P A`, browser `d s f t r j k / [ ]` (j/k = navigation, [/] = tab switching). Check `?` help for the live map.
- teatest goldens: pin terminal size and color profile in `TestMain`, inject the clock for "Ns ago" badges; `.gitattributes` marks `*.golden -text`.
- Rate limits: workspace SCIM ≈4 req/s (identity resources use 15m poll + manual refresh), jobs list 20/s. Respect per-resource `PollInterval()`.
- `~/.databrickscfg` may contain non-profile sections like `[__settings__]` — the profile parser must skip them.
