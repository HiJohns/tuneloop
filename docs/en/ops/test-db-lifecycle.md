# Test database lifecycle (handlers)

> #2341 (template DB + fail-fast + dirty self-healing) and #2342 (fresh-install replay of the migration chain).
> Applies to the `backend/handlers` test suite only; production paths are unaffected.

## Initialization order

1. `TestMain` resolves the run DB name (`TUNELOOP_TEST_DB` or per-node `tuneloop_test_<node>_<hash>`).
2. `EnsureTemplate` ensures the shared template `tuneloop_test_template` exists and is fresh; when missing/stale it rebuilds under `pg_advisory_lock` using the same order as a production fresh install:
   **`CREATE DATABASE` → migration chain replay (idempotent shim, #2342) → AutoMigrate (missing tables only) → PostAutoMigrate (iam_sub / plain SQL tables) → seed (system default pricing template) → write meta stamp**.
3. `CREATE DATABASE <run> TEMPLATE tuneloop_test_template` (fast clone).
4. `database.InitDB(run)` → `AssertCentsSchema` (money columns must be bigint) → `m.Run()`.

The meta stamp (`_test_template_meta`) is the model-list hash + seed fingerprint + **migration-chain fingerprint / up-file count / final schema version**; any change marks the template stale and triggers a rebuild.

## Migration replay and the idempotent shim (#2342)

The 217 up files in `backend/database/migrations/` are **not** modified (golang-migrate tracks version + dirty only, with no content checksum; editing history gives zero benefit on applied databases and costs merge conflicts). Instead the test template builder applies an idempotent transform in memory and replays the whole chain; files on disk stay untouched and the production `migrate` path never references this code.

| Original statement | Rewrite | Why |
|---|---|---|
| `CREATE TABLE x` | `CREATE TABLE IF NOT EXISTS x` | historical tables (e.g. 001) coexist with AutoMigrate and repeated replays |
| `CREATE [UNIQUE] INDEX i ON t` | `CREATE [UNIQUE] INDEX IF NOT EXISTS i ON t` | indexes can be created repeatedly |
| same, but the target table does not exist yet | wrap in `DO $$ ... IF to_regclass('t') IS NOT NULL THEN EXECUTE ...` | `20260630003` indexes `appeals` while the table is created later by `20260706002` |
| `ALTER TABLE t ...` | `ALTER TABLE IF EXISTS t ...` | historical ALTER-before-CREATE ordering |
| `ALTER TABLE t ADD COLUMN c` | `ADD COLUMN IF NOT EXISTS c` | repeated column adds |
| `INSERT/UPDATE/DELETE/WITH ...` referencing a possibly missing table | wrap in `DO $$ ... IF to_regclass(...) IS NOT NULL THEN EXECUTE ...` | early backfills precede table creation; on an empty database there is nothing to backfill, so skipping is equivalent |

Final schema = migration chain ∪ AutoMigrate(missing tables), matching a long-lived database's end state. On success the meta records `migrations_hash / migrations_count / migrations_version`, and the log prints:

```text
[test-template] migrations replayed: files=399 up=217 statements=1730 ... version=20261112003
```

### Failure handling (no silent skips)

- A replay failure produces `FATAL: ensure template: template: build failed: ...` and **exits the process**; it never degrades into "skip migrations". The error names the failing statement and SQLSTATE.
- A statement shape not covered by the rules (D5) must be handled by adding a rule for that statement (`CREATE/ALTER/index/DML guard`) — never by relaxing the failure into `|| true`.
- A missing or empty migrations directory fails explicitly (`template: migrations signature` / `migration source: no .sql files`).
- Evidence/diagnostic mode only: `TUNELOOP_TEMPLATE_MIGRATION_SHIM=off` skips the transform and replays raw files, reproducing #2341's red (`relation "appeals" does not exist`). The signature carries `shim=off`, so such a build can never be considered up to date by a normal template.

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
| `FATAL: ensure template ... build failed: shimmed migrations: up: ...` | Replay failure: add a shim rule for the failing statement (#2342 D5), **never** skip it; then rerun with `FORCE_TEMPLATE_REBUILD=1` |
| `relation "..." does not exist` while `TUNELOOP_TEMPLATE_MIGRATION_SHIM=off` is exported | Expected red in evidence mode (#2341); do not set that variable for normal builds |
| `cents schema drifted` | Some test mutated shared column types (e.g. migration tests); drop the template or rebuild with `FORCE_TEMPLATE_REBUILD=1` |
| Stale template meta (models/seeds/migration files changed) | Automatic rebuild; no manual action |
| Old template reused after a migration edit | Should not happen: `migrations_hash` is computed from the shimmed content and participates in freshness; if in doubt use `FORCE_TEMPLATE_REBUILD=1` |
| Concurrent first build on multiple nodes | Serialized by `pg_advisory_lock(7341001)`; the second node waits and reuses |
| `go: could not import ... no such file` (corrupted GOCACHE under concurrent builds) | `go clean -cache` or use a dedicated `GOCACHE=<repo>/tmp/gocache` |
| Suite is slow | TRUNCATE is slow on local overlayfs; template cloning avoids full rebuilds; sharding is deferred (#2341 Out of Scope) |

## Related

- #2335: dev PG exposure/passwordless access (tests must **not** TRUNCATE the shared `jobmaster-postgres:5432`).
- #2341: template DB + meta stamp + money-column fail-fast (origin of this document).
- #2342: fresh-install replay of the migration chain (shim design and verification).
- #2344: modernizing 6 existing fixtures under the strict (production-shaped) schema (separate workflow).
