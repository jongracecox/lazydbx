---
name: update-goldens
description: Regenerate teatest golden files for lazydbx after intentional UI changes, and review the diff before staging. Use when golden-file tests fail due to deliberate rendering changes.
---

# Update golden files

Golden files are byte-exact rendered frames. Only regenerate them when the
rendering change is **intentional** — a golden diff is a change-detector, not
an error to silence.

## 1. Confirm the failure is intentional

Read the failing test's diff first (`go test ./... -run <FailingTest> -v`).
If the change is unexpected, fix the code, not the goldens.

## 2. Regenerate under pinned conditions

Golden tests pin terminal size and color profile in `TestMain` — the update
must run with the same environment CI uses:

```bash
go test ./... -update
```

## 3. Eyeball every golden diff

```bash
git diff --stat -- '*.golden'
git diff -- '*.golden' | head -100
```

Check: no accidental size change, no color-profile drift (large ANSI-code
diffs on unchanged screens = environment problem, revert and investigate),
no timestamps leaking in (the clock must be injected in tests).

## 4. Verify clean

```bash
go test ./...   # must pass WITHOUT -update
```

Then stage the goldens together with the code change that caused them.
