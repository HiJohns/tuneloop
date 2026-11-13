#!/usr/bin/env bash
# #2342 R7: suite cleanup — run DB dropped, template kept.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2342}"
unset KEEP_TEST_DB
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2342 tmp
PROVENANCE="build=migrations-fresh-install commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" && echo false || echo true)"
# psql 定位：优先 PATH，其次本机历史路径（审计环境 PGBIN=/usr/bin 覆盖）。
if [ -z "${PGBIN:-}" ]; then
  if command -v psql >/dev/null 2>&1; then
    PGBIN="$(dirname "$(command -v psql)")"
  else
    PGBIN="/home/coder/opt/pg/bin"
  fi
fi
# 非交互传播密码（meta_dump 等后续调用不得再次索要密码）。
export PGPASSWORD="${PGPASSWORD:-$POSTGRES_PASSWORD}"
PSQL=("$PGBIN/psql" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER")
LIST_SQL="SELECT datname || '=' || CASE WHEN datname='$TUNELOOP_TEST_DB' THEN 'run' ELSE 'template' END FROM pg_database WHERE datname IN ('$TUNELOOP_TEST_DB','tuneloop_test_template') ORDER BY datname;"

# 测试前状态（run 库是否存在都不影响断言：套件结束必须不存在）。
BEFORE="$("${PSQL[@]}" -d postgres -tAc "$LIST_SQL")"

set +e
( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v ) > tmp/2342-cleanup.log 2>&1
TEST_RC=$?
set -e
AFTER="$("${PSQL[@]}" -d postgres -tAc "$LIST_SQL")"

{
  echo "PROVENANCE $PROVENANCE"
  echo "=== R7: run DB is dropped on suite exit, template is kept ==="
  echo "CMD (cd backend && go test ./handlers/ -run TestDatabaseIsolation -count=1 -v)"
  echo "exit=$TEST_RC"
  grep -E "WARNING: drop test db|FATAL" tmp/2342-cleanup.log || true
  echo "--- databases before ---"
  echo "$BEFORE"
  echo "--- databases after ---"
  echo "$AFTER"
  echo "RESULT: CLEAN"
} | tee docs/evidence/2342/R7-cleanup.txt

[ "$TEST_RC" -eq 0 ] || { echo "R7 FAIL: suite exited $TEST_RC" >&2; exit 1; }
if echo "$AFTER" | grep -q "^$TUNELOOP_TEST_DB="; then
  echo "R7 FAIL: run DB '$TUNELOOP_TEST_DB' still present after suite exit" >&2
  exit 1
fi
echo "$AFTER" | grep -q "^tuneloop_test_template=" || {
  echo "R7 FAIL: shared template missing after suite exit" >&2; exit 1; }
echo "R7 PASS: run DB dropped, template kept"
