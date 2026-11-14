#!/usr/bin/env bash
# #2338 R2: Group A 22-case targeted suite (exact names; no pipes on the CLI).
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
REGEX='^(TestAdjust2116_RejectedWhileShipping|TestDispatch2122_ShortfallNotification|TestListMerchantRepairServices_SharedTenantNoLeak_2248|TestListRecords_DamageObject|TestListTasks2116_IncludesSubmitterName|TestRepairService_AdjustAcceptPaysDifference|TestRepairService_AdjustDeclineSettlement|TestRepairService_CouponDispatchDeduction_2096|TestRepairService_CreateNormalizesTenant_2134|TestRepairService_DeclineQuote_2093|TestRepairService_DetailSite|TestRepairService_HappyPathSettlementRefund|TestRepairService_ListMineStatusFilter|TestRepairService_Notifications_2090|TestRepairService_PaymentTimeout_PreservesPending_2097|TestRepairService_PaymentsSummary|TestRepairService_SettlementShortfall|TestRepairService_ShortfallPrepay|TestRepairService_StatusGuardsAndTenantIsolation|TestRepairService_TaskListScopes|TestRepairService_Timeline|TestSelectTechnician2122_NoSiteMerchantOnly)$'
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ -run '<22 exact names>' -count=1 -v -timeout 15m"
  ( cd backend && go test ./handlers/ -run "$REGEX" -count=1 -v -timeout 15m )
} 2>&1 | tee docs/evidence/2338/R2-group-a.txt
echo "R2 PASS: 22/22 Group A"
