# Test database lifecycle (handlers)

> #2341. Applies to the `backend/handlers` test suite only; production paths are unaffected.

## Initialization order

1. `TestMain` resolves the run DB name (`TUNELOOP_TEST_DB` or per-node `tuneloop_test_<node>_<hash>`).
2. `EnsureTemplate` ensures the shared template `tuneloop_test_template` exists and is fresh; when missing/stale it rebuilds under `pg_advisory_lock`:
   **`CREATE DATABASE` → AutoMigrate(models) → PostMigrate (iam_sub / plain SQL tables) → seed (system default pricing template) → write meta stamp**.
3. `CREATE DATABASE <run> TEMPLATE tuneloop_test_template` (fast clone).
4. `database.InitDB(run)` → `AssertCentsSchema` (money columns must be bigint) → `m.Run()`.

> **Limitation**: the template does **not** apply `backend/database/migrations`. The migration chain is not fresh-install-safe (001 lacks `IF NOT EXISTS`; `20260630003`'s `ALTER appeals` precedes `20260706002` which creates the table) — tracked by **#2342**. The template only guarantees the model schema plus business seeds.

## fail-fast (money columns)

After cloning, every `models.Cents` column of every model is checked. Any non-`bigint` column (e.g. a test downgraded it to `numeric(10,2)`) fails with:
`FATAL: cents schema drifted (expected bigint): <table.column>=<type>; rebuild the template with FORCE_TEMPLATE_REBUILD=1`.
Reason: `Cents.Scan` treats numeric as yuan and multiplies by 100, silently producing 100x amounts.

## Cleanup

- On suite exit: close the pool → `DROP DATABASE <run> WITH (FORCE)`; the **template is kept** as a shared fixture (rebuildable at any time).
- Set `KEEP_TEST_DB=1` to keep the run DB for debugging.
- Force a template rebuild: `FORCE_TEMPLATE_REBUILD=1 go test ./handlers/ ...` (drops the template first).

## Troubleshooting

| Symptom | Action |
|---|---|
| `FATAL: ensure template ... build failed` | Inspect template build logs; ensure PG reachable and models AutoMigrate; then rerun with `FORCE_TEMPLATE_REBUILD=1` |
| `cents schema drifted` | Some test mutated shared column types (e.g. migration tests); drop the template or rebuild with `FORCE_TEMPLATE_REBUILD=1` |
| Stale template meta (models/seeds changed) | Automatic rebuild; no manual action |
| Concurrent first build on multiple nodes | Serialized by `pg_advisory_lock(7341001)`; the second node waits and reuses |
| `go: could not import ... no such file` (corrupted GOCACHE under concurrent builds) | `go clean -cache` or use a dedicated `GOCACHE=<repo>/tmp/gocache` |
| Suite is slow | TRUNCATE is slow on local overlayfs; template cloning avoids full rebuilds; sharding is deferred (#2341 Out of Scope) |

## Related

- #2335: dev PG exposure/passwordless access (tests must **not** TRUNCATE the shared `jobmaster-postgres:5432`).
- #2342: migration chain fresh-install replay (root cause for the template excluding migrations).
