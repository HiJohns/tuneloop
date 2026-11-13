#!/usr/bin/env bash
# #2342 R1/R2: fresh replay of the shimmed migration chain + idempotent rebuild.
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
mkdir -p docs/evidence/2342
PROVENANCE="build=migrations-fresh-install commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
PGBIN="${PGBIN:-/home/coder/opt/pg/bin}"
$PGBIN/psql -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -c "DROP DATABASE IF EXISTS tuneloop_test_template WITH (FORCE)" >/dev/null 2>&1
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v  (builds template: shimmed migrations -> AutoMigrate(missing) -> seed)"
  ( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v )
  echo "--- second build (idempotent, no rebuild expected) ---"
  ( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v )
} 2>&1 | tee docs/evidence/2342/R1-replay.txt
grep -q "migrations replayed" docs/evidence/2342/R1-replay.txt
