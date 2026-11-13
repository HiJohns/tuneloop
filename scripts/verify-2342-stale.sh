#!/usr/bin/env bash
# #2342 R3: stale meta stamp -> detected -> template rebuilt -> fresh stamp landed.
# Tampering is limited to the disposable local template DB (never dev/prod data).
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
PSQL=("$PGBIN/psql" -h "$POSTGRES_HOST" -p "$POSTGRES_PORT" -U "$POSTGRES_USER")
META_SQL="SELECT model_hash, seed_hash, migrations_hash, migrations_count, migrations_version FROM _test_template_meta WHERE id = 1;"

template_exists() {
  [ "$("${PSQL[@]}" -d postgres -tAc "SELECT count(*) FROM pg_database WHERE datname='tuneloop_test_template'")" = "1" ]
}
meta_dump() { "${PSQL[@]}" -d tuneloop_test_template -tAc "$META_SQL"; }

# 0) 模板必须存在（不存在时先正常构建一次，保证本脚本可独立运行）。
if ! template_exists; then
  ( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v ) > tmp/2342-stale-bootstrap.log 2>&1
fi
HEALTHY="$(meta_dump)"
[ -n "$HEALTHY" ] || { echo "R3 FAIL: template meta missing before tampering" >&2; exit 1; }

# 1) 篡改 meta 戳（只改本地一次性模板库）。
"${PSQL[@]}" -d tuneloop_test_template -c \
  "UPDATE _test_template_meta SET migrations_hash='stale-tampered', migrations_count=0 WHERE id = 1" >/dev/null
TAMPERED="$(meta_dump)"

# 2) 再构建：必须检测过期并重建。
set +e
( cd backend && go test ./handlers/ -run 'TestDatabaseIsolation' -count=1 -v ) > tmp/2342-stale-rerun.log 2>&1
REBUILD_RC=$?
set -e
HEALED="$(meta_dump)"

{
  echo "PROVENANCE $PROVENANCE"
  echo "=== R3: tamper template meta stamp -> next build must rebuild ==="
  echo "--- meta before tampering ---"
  echo "$HEALTHY"
  echo "--- meta after tampering (migrations_hash='stale-tampered', count=0) ---"
  echo "$TAMPERED"
  echo "exit=$REBUILD_RC"
  echo "--- rebuild log (expect 'Built test template' + 'migrations replayed') ---"
  grep -E "Built test template|migrations replayed|FATAL" tmp/2342-stale-rerun.log || true
  echo "--- meta after rebuild ---"
  echo "$HEALED"
  echo "RESULT: HEALED"
} | tee docs/evidence/2342/R3-stale-rebuild.txt

[ "$REBUILD_RC" -eq 0 ] || { echo "R3 FAIL: rebuild run exited $REBUILD_RC" >&2; exit 1; }
# 负例：未重建（静默复用旧戳）或戳未刷新 => 失败。
grep -q "Built test template" docs/evidence/2342/R3-stale-rebuild.txt || {
  echo "R3 FAIL: stale template was reused silently (no rebuild)" >&2; exit 1; }
grep -q "migrations replayed" docs/evidence/2342/R3-stale-rebuild.txt || {
  echo "R3 FAIL: rebuild did not replay migrations" >&2; exit 1; }
[ "$HEALED" != "$TAMPERED" ] || { echo "R3 FAIL: meta stamp not refreshed" >&2; exit 1; }
echo "$HEALED" | grep -q "|217|20261112003$" || {
  echo "R3 FAIL: fresh stamp mismatch: $HEALED" >&2; exit 1; }
[ "$HEALED" = "$HEALTHY" ] || { echo "R3 FAIL: healed stamp differs from healthy stamp" >&2; exit 1; }

echo "R3 PASS: stale stamp detected, template rebuilt, fresh stamp landed"
