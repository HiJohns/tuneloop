#!/usr/bin/env bash
# #2344 R1: 6 strict-schema fixture fixes — red (#2342 escalation list) vs green (current -v run).
# Also records D5 drift (fixtures outside the 6) without claiming them as fixed.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2344}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2344 tmp
PROVENANCE="build=strict-schema-fixtures commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"

# 6 条（#2342 升级评论实测清单）
TESTS=(
  TestConfirmationSessionHandler_Confirm
  TestOrgManagement
  TestPersonnel2067_MembershipAnchorAndDirect
  TestPersonnel_MerchantView_TenantIsolation2336
  TestCancelOrderByCustomer_StatusGuard
  TestDiscountCodeFlow
)
# 红证据引用（逐条对应 #2342 升级评论的失败类别）
RED_REF=(
  "site_members_tenant_id_fkey（缺 tenants 行）"
  "site_members_tenant_id_fkey（缺 tenants 行）"
  "site_members_tenant_id_fkey（缺 tenants 行）"
  "site_members_tenant_id_fkey（缺 tenants 行）"
  "uni_instrument_levels_caption 不存在（局部 AutoMigrate 触发 GORM DROP CONSTRAINT）"
  "idx_pricing_templates_system_default 唯一冲突（模板种子 + 用例再插）"
)
# D5 drift：6 之外的同类 fixture 漂移（#2338 收尾时发现，不在本 Issue Files 内）
DRIFT_TESTS=(
  TestListMerchantRepairServices_SharedTenantNoLeak_2248
  TestRepairService_StatusGuardsAndTenantIsolation
  TestRepairService_TaskListScopes
  TestRepairService_ListMineStatusFilter
)
DRIFT_REASON=(
  "merchants_tenant_id_code_key 唯一冲突（同 tenant 两商户 code 均空）"
  "site_members_tenant_id_fkey（repair_service_test.go:67 硬编码 tenant）"
  "site_members_tenant_id_fkey（同上）"
  "site_members_tenant_id_fkey（同上）"
)

REGEX="^($(IFS='|'; echo "${TESTS[*]}"))\$"
set +e
( cd backend && go test ./handlers/ -run "$REGEX" -count=1 -v ) > tmp/2344-green.log 2>&1
GREEN_RC=$?
set -e

python3 - "$PROVENANCE" "$GREEN_RC" <<'PY'
import json, re, sys
prov, rc = sys.argv[1], int(sys.argv[2])
tests = [
    ("TestConfirmationSessionHandler_Confirm", "site_members_tenant_id_fkey（缺 tenants 行）"),
    ("TestOrgManagement", "site_members_tenant_id_fkey（缺 tenants 行）"),
    ("TestPersonnel2067_MembershipAnchorAndDirect", "site_members_tenant_id_fkey（缺 tenants 行）"),
    ("TestPersonnel_MerchantView_TenantIsolation2336", "site_members_tenant_id_fkey（缺 tenants 行）"),
    ("TestCancelOrderByCustomer_StatusGuard", "uni_instrument_levels_caption 不存在（局部 AutoMigrate 触发 GORM DROP CONSTRAINT）"),
    ("TestDiscountCodeFlow", "idx_pricing_templates_system_default 唯一冲突（模板种子 + 用例再插）"),
]
drift = [
    ("TestListMerchantRepairServices_SharedTenantNoLeak_2248", "merchants_tenant_id_code_key 唯一冲突（同 tenant 两商户 code 均空）"),
    ("TestRepairService_StatusGuardsAndTenantIsolation", "site_members_tenant_id_fkey（repair_service_test.go:67 硬编码 tenant）"),
    ("TestRepairService_TaskListScopes", "site_members_tenant_id_fkey（同上）"),
    ("TestRepairService_ListMineStatusFilter", "site_members_tenant_id_fkey（同上）"),
]
log = open("tmp/2344-green.log", errors="replace").read()
skip_count = len(re.findall(r"^\s*--- SKIP:", log, re.M))
rows = []
for name, ref in tests:
    passed = bool(re.search(r"^--- PASS: " + re.escape(name) + r"\b", log, re.M))
    failed = bool(re.search(r"^--- FAIL: " + re.escape(name) + r"\b", log, re.M))
    rows.append({
        "name": name,
        "red_ref": "#2342 升级评论：" + ref,
        "green_file": "tmp/2344-green.log#--- PASS: " + name,
        "pass": passed and not failed,
    })
drift_rows = [{"name": n, "reason": r, "out_of_scope": True, "pass": False,
               "note": "D5 drift：需 Architect 扩清单（不在本 Issue Files）"} for n, r in drift]
out = {
    "provenance": prov,
    "test_command": "go test ./handlers/ -run '<6 tests>' -count=1 -v",
    "exit": rc,
    "skip_count": skip_count,
    "rows": rows,
    "drift": drift_rows,
}
json.dump(out, open("docs/evidence/2344/R1-fix.json", "w"), ensure_ascii=False, indent=2)
print("rows=%d pass=%d skip=%d drift=%d" % (len(rows), sum(r["pass"] for r in rows), skip_count, len(drift_rows)))
sys.exit(0 if all(r["pass"] for r in rows) and skip_count == 0 and rc == 0 else 1)
PY
echo "R1 PASS: 6/6 strict-schema fixtures green (drift recorded separately)"
