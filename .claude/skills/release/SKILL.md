---
name: release
description: Cut a lazydbx release — pre-flight checks, tag, and verify the goreleaser pipeline and Homebrew tap. Use when asked to release, tag, or publish a version.
---

# Release lazydbx

Releases are tag-driven: pushing `vX.Y.Z` triggers `.github/workflows/release.yml`
(goreleaser → GitHub Release + Homebrew cask in `jongracecox/homebrew-tap`).

Take the version from the arguments (semver, e.g. `v0.2.0`). Confirm with the
user before pushing the tag — it publishes.

## Pre-flight

```bash
git status --porcelain          # must be empty
git pull --ff-only
gh run list --branch main -L 3  # latest CI on main must be green
make test lint tidy
go run ./cmd/lazydbx version    # sanity
grep -m1 version .goreleaser.yaml   # goreleaser v2 config present
```

Also once per repo: verify the `HOMEBREW_TAP_TOKEN` secret exists
(`gh secret list`) and the `jongracecox/homebrew-tap` repo exists.

## Tag and push

```bash
git tag -a vX.Y.Z -m "vX.Y.Z"
git push origin vX.Y.Z
```

## Verify

```bash
gh run watch                                   # release workflow green
gh release view vX.Y.Z                         # artifacts: darwin/linux/windows, checksums.txt
brew install jongracecox/tap/lazydbx && lazydbx version   # reports vX.Y.Z
```

If the workflow fails after partial publish: `gh release delete vX.Y.Z`,
delete the tag (`git push origin :refs/tags/vX.Y.Z`), fix, re-tag.

## Post-release manual check

Run the TUI once with `TERM=xterm` (16-color) to confirm the theme degrades
gracefully.
