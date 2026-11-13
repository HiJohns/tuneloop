#!/usr/bin/env bash
# #2345: dev 库 legacy 孤儿单画像（只读，零 DB 变更；Owner 裁定方案 B：保留 + 文档化）。
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-25432}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
PSQL_BIN="${PSQL_BIN:-$(command -v psql || echo /home/coder/opt/pg/bin/psql)}"
DSN="${TUNELOOP_AUDIT_DSN:-postgresql://${POSTGRES_USER}:${POSTGRES_PASSWORD}@${POSTGRES_HOST}:${POSTGRES_PORT}/tuneloop_debug?sslmode=disable}"
PROVENANCE="build=legacy-data-disposition commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
mkdir -p docs/evidence/2345
OUT=docs/evidence/2345/R1-orphan-profile.txt
{
  echo "PROVENANCE $PROVENANCE"
  echo "DSN ${DSN%%\?*}"
  echo "SQL 1: SELECT count(*) FROM repair_requests;"
  "$PSQL_BIN" "$DSN" -tAc "SELECT count(*) FROM repair_requests"
  echo "SQL 2: SELECT count(*) FROM repair_requests WHERE tenant_id NOT IN (SELECT org_id FROM merchants WHERE org_id IS NOT NULL);"
  "$PSQL_BIN" "$DSN" -tAc "SELECT count(*) FROM repair_requests WHERE tenant_id NOT IN (SELECT org_id FROM merchants WHERE org_id IS NOT NULL)"
  echo "SQL 3: orphan three-factor (merchant/user/records)"
  "$PSQL_BIN" "$DSN" -tAc "SELECT rr.id||' tenant='||rr.tenant_id||' merchants_org='||(SELECT count(*) FROM merchants m WHERE m.org_id=rr.tenant_id)||' merchants_tenant='||(SELECT count(*) FROM merchants m WHERE m.tenant_id=rr.tenant_id)||' users='||(SELECT count(*) FROM users u WHERE u.id::text=rr.user_id OR u.iam_sub=rr.user_id)||' records='||(SELECT count(*) FROM repair_request_records r WHERE r.repair_request_id=rr.id) FROM repair_requests rr WHERE rr.tenant_id NOT IN (SELECT org_id FROM merchants WHERE org_id IS NOT NULL)"
  echo "SQL 4 (marker): SELECT 'RESULT: DOCUMENTED-KEEP (#2345)';"
  "$PSQL_BIN" "$DSN" -tAc "SELECT 'RESULT: DOCUMENTED-KEEP (#2345)'"
} > "$OUT" 2>&1
grep -q "DOCUMENTED-KEEP (#2345)" "$OUT" || { echo "R1 FAIL: marker missing" >&2; exit 1; }
echo "R1 PASS: profile written to $OUT"
