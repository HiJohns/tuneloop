#!/usr/bin/env bash
# #2342 R1/R2: fresh replay of the migration chain.
#   R1 = RED (raw migrations, expected to fail on #2341's ALTER-before-CREATE)
#        + GREEN (shimmed replay of the same chain must succeed).
#   R2 = second build on the same template DB (idempotent: no rebuild, meta equal).
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2342}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2342 tmp
PROVENANCE="build=migrations-fresh-install commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" && echo false || echo true)"
PGBIN="${PGBIN:-/home/coder/opt/pg/bin}"
META_SQL="SELECT model_hash, seed_hash, migrations_hash, migrations_count, migrations_version FROM _test_template_meta WHERE id = 1;"

drop_template() {
  "$PGBIN/psql" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres \
    -c "DROP DATABASE IF EXISTS tuneloop_test_template WITH (FORCE)" >/dev/null 2>&1 || true
}

meta_dump() {
  "$PGBIN/psql" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" \
    -d tuneloop_test_template -tAc "$META_SQL" 2>&1
}

# ---------------------------------------------------------------- R1 RED
# Raw (non-shimmed) replay must fail: 20260630003 ALTERs appeals before
# 20260706002 creates it (#2341 upgrade comment). A passing red stage means
# the chain silently stopped exercising the defect -> hard failure.
drop_template
set +e
( export TUNELOOP_TEMPLATE_MIGRATION_SHIM=off
  cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v ) > tmp/2342-red.log 2>&1
RED_RC=$?
set -e
if [ "$RED_RC" -eq 0 ]; then
  echo "R1 FAIL: raw (non-shimmed) replay unexpectedly succeeded" >&2
  exit 1
fi
if ! grep -q 'relation "appeals" does not exist' tmp/2342-red.log; then
  echo "R1 FAIL: red stage failed for an unexpected reason (see tmp/2342-red.log)" >&2
  tail -20 tmp/2342-red.log >&2
  exit 1
fi

# ---------------------------------------------------------------- R1 GREEN
drop_template
{
  echo "PROVENANCE $PROVENANCE"
  echo "=== RED stage: raw migrations (TUNELOOP_TEMPLATE_MIGRATION_SHIM=off), expected failure ==="
  echo "CMD (cd backend && TUNELOOP_TEMPLATE_MIGRATION_SHIM=off go test ./handlers/ -run TestDatabaseIsolation -count=1 -v)"
  echo "exit=$RED_RC (expected non-zero)"
  grep -E 'FATAL: ensure template|relation "appeals" does not exist' tmp/2342-red.log | head -3
  echo "RED: CONFIRMED (chain is not fresh-install-safe without the shim)"
  echo
  echo "=== GREEN stage: shimmed replay (migrations -> AutoMigrate(missing) -> PostAutoMigrate -> seed) ==="
  echo "CMD (cd backend && go test ./handlers/ -run TestDatabaseIsolation -count=1 -v)"
  ( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v )
  echo "--- template meta ---"
  meta_dump
} 2>&1 | tee docs/evidence/2342/R1-replay.txt

grep -q "migrations replayed" docs/evidence/2342/R1-replay.txt
grep -q "Built test template" docs/evidence/2342/R1-replay.txt
grep -q "RED: CONFIRMED" docs/evidence/2342/R1-replay.txt
grep -q "PASS" docs/evidence/2342/R1-replay.txt

# ---------------------------------------------------------------- R2 idempotent
BEFORE="$(meta_dump)"
{
  echo "PROVENANCE $PROVENANCE"
  echo "=== R2: second build on the same template DB (no drop, no rebuild expected) ==="
  echo "CMD (cd backend && go test ./handlers/ -run TestDatabaseIsolation -count=1 -v)"
  ( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v )
  echo "--- template meta before ---"
  echo "$BEFORE"
  echo "--- template meta after ---"
  meta_dump
} 2>&1 | tee docs/evidence/2342/R2-idempotent.txt
AFTER="$(meta_dump)"

if grep -q "migrations replayed" docs/evidence/2342/R2-idempotent.txt; then
  echo "R2 FAIL: template was rebuilt on the second run" >&2
  exit 1
fi
[ -n "$BEFORE" ] || { echo "R2 FAIL: template meta unreadable before second run" >&2; exit 1; }
[ "$BEFORE" = "$AFTER" ] || { echo "R2 FAIL: template meta changed across runs" >&2; exit 1; }
if ! echo "$BEFORE" | grep -q "|217|20261112003$"; then
  echo "R2 FAIL: unexpected meta stamp: $BEFORE" >&2; exit 1
fi

echo "R1 PASS: red (raw replay fails on appeals) + green (shimmed replay ok)"
echo "R2 PASS: second build exit 0, meta unchanged"
