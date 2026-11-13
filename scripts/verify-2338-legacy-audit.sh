#!/usr/bin/env bash
# #2338 R13 (D9): read-only audit of repair_requests.tenant_id on the dev DB.
# Legacy rows = tenant_id not present in any merchants.org_id (shared-tenant era writes).
# DSN: TUNELOOP_AUDIT_DSN (preferred) or POSTGRES_* of the dev instance.
# Never writes: only SELECT COUNT.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2338}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2338 "$TMPDIR" "$GOCACHE"
PROVENANCE="build=repair-tenant-anchor commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
PSQL_BIN="${PSQL_BIN:-$(command -v psql || echo /home/coder/opt/pg/bin/psql)}"
# #2345：dev 库 legacy 孤儿单已由 Owner 裁定「保留 + 文档化」（方案 B，零 DB 变更）。
# legacy>0 时据此解读为「已知孤儿单，保留」，不视为失败（exit 语义不变）。
KNOWN_ORPHAN_REF="#2345"
DSN="${TUNELOOP_AUDIT_DSN:-}"
if [ -z "$DSN" ]; then
  DSN="postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/${TUNELOOP_AUDIT_DB:-tuneloop_debug}?sslmode=disable"
fi
SQL="SELECT
  count(*) AS total,
  count(*) FILTER (WHERE rr.tenant_id IN (SELECT org_id FROM merchants WHERE org_id IS NOT NULL)) AS org_anchored,
  count(*) FILTER (WHERE rr.tenant_id NOT IN (SELECT org_id FROM merchants WHERE org_id IS NOT NULL)) AS legacy
FROM repair_requests rr"
OUT=docs/evidence/2338/R13-legacy-audit.txt
{
  echo "PROVENANCE $PROVENANCE"
  echo "DSN ${DSN%%\?*}"
  echo "SQL $SQL"
} > "$OUT"
if ! "$PSQL_BIN" "$DSN" -tAc "SELECT 1" >/dev/null 2>&1; then
  echo "BLOCKED: dev DB unreachable (set TUNELOOP_AUDIT_DSN). No data read." | tee -a "$OUT"
  exit 2
fi
RESULT=$("$PSQL_BIN" "$DSN" -tAc "$SQL")
echo "RESULT $RESULT" >> "$OUT"
TOTAL=$(echo "$RESULT" | awk -F'|' '{print $1}')
ORG=$(echo "$RESULT" | awk -F'|' '{print $2}')
LEGACY=$(echo "$RESULT" | awk -F'|' '{print $3}')
echo "total=$TOTAL org_anchored=$ORG legacy=$LEGACY" >> "$OUT"
if [ "${LEGACY:-0}" -gt 0 ]; then
  echo "ACTION: legacy=$LEGACY > 0 → 已知孤儿单，Owner 裁定保留（$KNOWN_ORPHAN_REF）；零 DB 变更。" | tee -a "$OUT"
fi
echo "R13 PASS: total=$TOTAL org_anchored=$ORG legacy=$LEGACY"
