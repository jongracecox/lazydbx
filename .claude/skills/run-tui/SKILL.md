---
name: run-tui
description: Build and manually verify the lazydbx TUI against a real Databricks profile. Use when asked to run the app, smoke-test a change, or verify TUI behavior by hand.
---

# Run and verify the TUI

A TUI cannot be verified by piping output — it needs a real terminal. Prefer
having the user run it, or use tmux to drive it yourself.

## Launch

```bash
make build
./bin/lazydbx --profile "${1:-$LAZYDBX_TEST_PROFILE}"
```

Pick a **dev/non-prod profile** from `~/.databrickscfg` unless told otherwise
(list names with `grep '^\[' ~/.databrickscfg` — skip `[__settings__]`).

## Watch logs in a second pane

The TUI never writes to stdout/stderr. Tail the log file:

```bash
tail -f "$(./bin/lazydbx log-path 2>/dev/null || echo ~/Library/Application\ Support/lazydbx/lazydbx.log)"
```

(Linux: `~/.local/state/lazydbx/lazydbx.log`.)

## Driving it non-interactively (tmux)

```bash
tmux new-session -d -s lazydbx-test -x 120 -y 40 './bin/lazydbx --profile DEV'
sleep 2 && tmux capture-pane -t lazydbx-test -p     # screenshot the screen
tmux send-keys -t lazydbx-test ':' 'catalogs' Enter
tmux capture-pane -t lazydbx-test -p
tmux kill-session -t lazydbx-test
```

## Smoke checklist

- First paint < 1s; no blank screen while loading (spinner or cached rows).
- `:` opens command bar with autocomplete; `esc` closes it.
- `/` filters instantly; `esc` clears.
- Enter drills down; Esc pops back; breadcrumbs update.
- `?` shows help; header hints match keys that actually work.
- `q` quits cleanly (terminal restored, no stray output).
- No errors in the log tail (SDK traffic at debug level is normal).
