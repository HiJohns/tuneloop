#!/usr/bin/env bash
# #2336 R8: full handlers suite attribution.
# Whitelist = 31 baseline failures (docs/evidence/2336/R3-baseline-pre-existing.txt,
# baseline 3e50bc494) + 4 migration-environment failures deferred to #2341 (D11).
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2336}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p "$TMPDIR" "$GOCACHE" docs/evidence/2336 tmp
PROVENANCE="build=handlers-test-infra commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" && echo false || echo true)"
LOG=tmp/2336-full-handlers.log
BASELINE=tmp/2336-baseline-names.txt
MIGRATION=tmp/2336-migration-names.txt
cat > "$BASELINE" <<'EOF'
TestAdjust2116_RejectedWhileShipping
TestBatchCreateOrder_RentDaysOverridesEndDate
TestCreateRentalOrder
TestDispatch2122_ShortfallNotification
TestGetUserInstrumentDetail
TestInspectReturn_Good_ExecutesRefund
TestListMerchantRepairServices_SharedTenantNoLeak_2248
TestListRecords_DamageObject
TestListTasks2116_IncludesSubmitterName
TestRefundDiff_PointsOverCap
TestRefundDiff_PointsWithinCap
TestRepairService_AdjustAcceptPaysDifference
TestRepairService_AdjustDeclineSettlement
TestRepairService_CouponDispatchDeduction_2096
TestRepairService_CreateNormalizesTenant_2134
TestRepairService_DeclineQuote_2093
TestRepairService_DetailSite
TestRepairService_HappyPathSettlementRefund
TestRepairService_ListMineStatusFilter
TestRepairService_Notifications_2090
TestRepairService_PaymentTimeout_PreservesPending_2097
TestRepairService_PaymentsSummary
TestRepairService_SettlementShortfall
TestRepairService_ShortfallPrepay
TestRepairService_StatusGuardsAndTenantIsolation
TestRepairService_TaskListScopes
TestRepairService_Timeline
TestScenarioA_DamageVariant
TestScenarioC_CancelBoundary
TestSelectTechnician2122_NoSiteMerchantOnly
TestSettlementFlow
EOF
cat > "$MIGRATION" <<'EOF'
TestCentsMigration_20260820001
TestGiftPolicyReferralPresetMigration
TestPromoOverride_CheckConstraintMigration
TestPromoOverride_CrossMerchantForbidden_2260
EOF
set +e
( cd backend && go test ./handlers/ -count=1 -timeout 10m ) > "$LOG" 2>&1
rc=$?
set -e
if grep -q "panic: test timed out" "$LOG"; then
  echo "R8 FAIL: full handlers suite timed out (10m)" >&2
  exit 1
fi
python3 - "$LOG" "$BASELINE" "$MIGRATION" "$rc" "$PROVENANCE" > docs/evidence/2336/R8-attribution.json <<'PY'
import json, re, sys

log, baseline_path, migration_path, rc, provenance = sys.argv[1:6]
text = open(log, errors="replace").read()
fails = sorted(set(re.findall(r"^--- FAIL: ([^ ]+)", text, re.M)))
baseline = {x.strip() for x in open(baseline_path) if x.strip()}
migration = {x.strip() for x in open(migration_path) if x.strip()}
rows, unattributed = [], []
for name in fails:
    if name in baseline:
        rows.append({"name": name, "attribution": "pre-existing(#2338/#2339)"})
    elif name in migration:
        rows.append({"name": name, "attribution": "env-migration(#2341)"})
    else:
        rows.append({"name": name, "attribution": "unattributed"})
        unattributed.append(name)
out = {
    "schema_version": 1,
    "issue": 2336,
    "provenance": provenance,
    "exit": int(rc),
    "fail_count": len(fails),
    "baseline_count": len(baseline),
    "migration_count": len(migration),
    "rows": rows,
    "unattributed": unattributed,
    "pass": not unattributed,
}
print(json.dumps(out, ensure_ascii=False, indent=2))
sys.exit(0 if not unattributed else 1)
PY
echo "R8 PASS: FAIL set is a subset of the 31 baseline + 4 migration whitelist"
