#!/usr/bin/env bash
# #2349 R1: 4 strict-schema repair fixtures — red (#2349 body / #2344 drift) vs green (current run).
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2349}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2349 tmp
PROVENANCE="build=strict-repair-fixtures commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"

REGEX='^(TestListMerchantRepairServices_SharedTenantNoLeak_2248|TestRepairService_StatusGuardsAndTenantIsolation|TestRepairService_TaskListScopes|TestRepairService_ListMineStatusFilter)$'
set +e
( cd backend && go test ./handlers/ -run "$REGEX" -count=1 -v ) > tmp/2349-green.log 2>&1
GREEN_RC=$?
set -e

python3 - "$PROVENANCE" "$GREEN_RC" <<'PY'
import json, re, sys
prov, rc = sys.argv[1], int(sys.argv[2])
tests = [
    ("TestListMerchantRepairServices_SharedTenantNoLeak_2248",
     "#2349 正文第 1 行 / #2344 R1-fix.json drift[0]：merchants_tenant_id_code_key 唯一冲突（同 tenant 双商户 code 均空）"),
    ("TestRepairService_StatusGuardsAndTenantIsolation",
     "#2349 正文第 2 行 / #2344 drift[1]：site_members_tenant_id_fkey（repair_service_test.go L67 硬编码 tenant 无 tenants 行）"),
    ("TestRepairService_TaskListScopes",
     "#2349 正文第 3 行 / #2344 drift[2]：site_members_tenant_id_fkey（同上）"),
    ("TestRepairService_ListMineStatusFilter",
     "#2349 正文第 4 行 / #2344 drift[3]：site_members_tenant_id_fkey（同上）"),
]
log = open("tmp/2349-green.log", errors="replace").read()
skip_count = len(re.findall(r"^\s*--- SKIP:", log, re.M))
rows = []
for name, ref in tests:
    passed = bool(re.search(r"^--- PASS: " + re.escape(name) + r"\b", log, re.M))
    failed = bool(re.search(r"^--- FAIL: " + re.escape(name) + r"\b", log, re.M))
    rows.append({"name": name, "red_ref": ref,
                 "green_file": "tmp/2349-green.log#--- PASS: " + name,
                 "pass": passed and not failed})
out = {
    "provenance": prov,
    "test_command": "go test ./handlers/ -run '<4 tests>' -count=1 -v",
    "exit": rc,
    "skip_count": skip_count,
    "rows": rows,
}
json.dump(out, open("docs/evidence/2349/R1-fix.json", "w"), ensure_ascii=False, indent=2)
print("rows=%d pass=%d skip=%d" % (len(rows), sum(r["pass"] for r in rows), skip_count))
sys.exit(0 if rc == 0 and skip_count == 0 and all(r["pass"] for r in rows) else 1)
PY
echo "R1 PASS: 4/4 red/green pairs"
