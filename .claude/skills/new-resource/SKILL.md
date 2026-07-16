---
name: new-resource
description: Scaffold a new Databricks resource for lazydbx — resource def, DAO addition, registration, and fake-DAO test. Use when adding any new browsable resource type (e.g. "add clusters", "add a secrets view").
---

# Add a new resource

Adding a resource touches exactly four places. Nothing else should change —
if you find yourself editing the browser, app, or engine, stop and reconsider.

Take the resource name from the arguments (e.g. `clusters`). Then:

## 1. DAO interface + implementation (`internal/dbx/dao.go`, `dao_impl.go`)

- Add a narrow interface with ONLY the methods this resource needs, e.g.
  `ClustersDAO { List(ctx) ([]compute.ClusterDetails, error) }`.
- Implement it in `dao_impl.go` delegating to the SDK. Use
  `listing.ToSliceN(ctx, it, N)` to bound unbounded lists.
- Expose it from `dbx.Clients` (follow the pattern of existing accessors).
- This is the ONLY file pair where `databricks-sdk-go` may be imported.

## 2. Resource def (`internal/resources/<name>.go`)

Follow an existing def (e.g. `catalogs.go`) exactly:

- `ColSpec[T]` slice for columns with extract funcs.
- `List()` calls the DAO, returns `resource.BuildRows(...)`.
- `Args()` lists positional scope keys if the resource is scoped
  (e.g. tables → `["catalog", "schema"]`); empty otherwise.
- `Child()`/`ChildScope()` if Enter should drill down; `""` for leaves.
- `PollInterval()`: 5–10s for cheap APIs; **15m for SCIM/identity resources**
  (rate limit ≈4 req/s); consult `docs/PLAN.md` rate-limit table.
- Actions: mutating ones MUST set `Dangerous: true`.
- Never call `GetSecret` — secrets are metadata-only by policy.
- If the resource has a page in the Databricks workspace UI, implement
  `resource.WebLinker` so `o` opens it in the browser. Add a `WebURL` method
  in `internal/resources/weburl.go` (keep all the URL builders together) using
  the shared `webURL(host, segments...)` helper — it escapes each segment and
  returns `ok=false` when the host or any segment is empty. Build the path from
  `scope` + `row.ID` (e.g. `webURL(host, "explore", "data", scope["catalog"],
  row.ID)`). Leaves with no page of their own can point at their parent's page
  (see `ColumnsDef`/`TaskRunsDef`). Skip this only for resources with no
  workspace URL at all.

## 3. Registration (`internal/resources/register.go`)

One line: `reg.MustRegister(NewXxxDef(...))` with name + aliases
(singular, plural, short form — e.g. `clusters`, `cluster`, `cl`).

## 4. Fake-DAO test (`internal/resources/<name>_test.go`)

Table-driven with testify. Fake DAO = struct of func fields (do NOT use the
SDK's experimental mocks). Assert: row IDs and cells from canned SDK structs,
`ChildScope` composition, alias resolution, readonly gating of dangerous
actions. If you added `WebURL`, add a case to `weburl_test.go` (expected URL
plus a not-linkable case with an empty host or missing scope segment).

## Verify

```bash
make test lint
go run ./cmd/lazydbx --profile <a-dev-profile>   # then `:<name>` in the TUI
```
