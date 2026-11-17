#!/usr/bin/env bash
# #2339 Group B triage：9 个基线失败测试的定向验收。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  r1-batch-order|r2-create-order|r3-return-refund|r4-refund-diff|r5-scenarios|r6-settlement|r7-detail) ;;
  *) echo "usage: $0 <r1-batch-order|r2-create-order|r3-return-refund|r4-refund-diff|r5-scenarios|r6-settlement|r7-detail>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2339}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1
mkdir -p "$TMPDIR" "$GOCACHE"
cd "$REPO_ROOT/backend"
case "$MODE" in
  r1-batch-order)   go test ./handlers/ -run '^TestBatchCreateOrder_RentDaysOverridesEndDate$' -count=1 -v ;;
  r2-create-order)  go test ./handlers/ -run '^TestCreateRentalOrder$' -count=1 -v ;;
  r3-return-refund) go test ./handlers/ -run '^TestInspectReturn_Good_ExecutesRefund$' -count=1 -v ;;
  r4-refund-diff)   go test ./handlers/ -run '^TestRefundDiff_Points' -count=1 -v ;;
  r5-scenarios)     go test ./handlers/ -run '^TestScenario' -count=1 -v ;;
  r6-settlement)    go test ./handlers/ -run '^TestSettlementFlow$' -count=1 -v ;;
  r7-detail)        go test ./handlers/ -run '^TestGetUserInstrumentDetail$' -count=1 -v ;;
esac
