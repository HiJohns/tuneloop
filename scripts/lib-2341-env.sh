#!/usr/bin/env bash
# #2341 shared env: requires a reachable disposable PostgreSQL (NOT jobmaster-postgres).
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
PROVENANCE="build=test-db-lifecycle commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no)" && echo false || echo true)"
PSQL_BIN="${PSQL_BIN:-$(command -v psql || echo /home/coder/opt/pg/bin/psql)}"
PGISREADY_BIN="${PGISREADY_BIN:-$(command -v pg_isready || echo /home/coder/opt/pg/bin/pg_isready)}"
psql_admin() { PGPASSWORD="$POSTGRES_PASSWORD" "$PSQL_BIN" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" -d postgres -tAc "$1"; }
require_pg() {
  if ! PGPASSWORD="$POSTGRES_PASSWORD" "$PGISREADY_BIN" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER" >/dev/null 2>&1; then
    echo "FATAL: PostgreSQL not reachable at $POSTGRES_HOST:$POSTGRES_PORT" >&2
    exit 2
  fi
}
