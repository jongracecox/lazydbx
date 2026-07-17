# lazydbx

A lazier way to Databricks. A fast, colorful, keyboard-driven terminal UI —
inspired by [k9s](https://github.com/derailed/k9s) — for browsing Unity
Catalog, running SQL, triaging jobs and pipelines, and managing your
workspaces without leaving the terminal.

> **Status: early development.** The interface and features described below
> are landing phase by phase — see [docs/PLAN.md](docs/PLAN.md).

## Why

The Databricks web UI is powerful but slow to click through, and the CLI has a
hundred subcommands to remember. lazydbx gives you the k9s experience instead:

- `:` **command mode** — `:catalogs`, `:jobs`, `:tables main.silver` — teleport anywhere, with autocomplete and aliases
- **Enter/Esc drill-down** — catalog → schema → table → data preview; job → runs → task logs — one gesture for everything
- `/` **instant filter** on any view, no API round-trip
- **Self-documenting** — the header always shows the keys valid right now; `?` for full help
- **Fast** — local caching, background polling, stale-while-revalidate rendering
- **Safe** — read-only by default early on, confirm dialogs for anything destructive, `--readonly` flag, per-profile accent colors (make prod red!)

## Install

```bash
brew install jongracecox/tap/lazydbx   # once first release is tagged
# or
go install github.com/jongracecox/lazydbx/cmd/lazydbx@latest
```

## Usage

lazydbx uses your existing [Databricks config
profiles](https://docs.databricks.com/aws/en/dev-tools/auth/config-profiles)
(`~/.databrickscfg`) — including OAuth sessions created with
`databricks auth login`.

```bash
lazydbx                    # opens the profile picker
lazydbx --profile mydev    # jump straight into a profile
lazydbx --readonly         # disable all mutating actions
```

Optional positional args launch straight into a resource view, using the same
syntax as the in-app `:` command bar. `esc` from a launched view returns to the
profile picker.

```bash
lazydbx -p mydev jobs                 # open in the jobs list
lazydbx -p mydev schemas prod         # schemas in the 'prod' catalog
lazydbx -p mydev tables main.silver   # drill straight to a schema's tables
lazydbx -p mydev runs 123             # runs for job 123
lazydbx -p mydev jobs /etl            # jobs list pre-filtered to 'etl'
```

## Development

```bash
make tools   # install golangci-lint + lefthook, register git hooks
make test
make run
```

See [CLAUDE.md](CLAUDE.md) for architecture and contribution conventions.

## License

Apache-2.0
