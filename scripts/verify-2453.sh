#!/usr/bin/env bash
# #2453：通知/收据/action_data 金额改用执行净额（NetCash）证据驱动。
# 用法：bash scripts/verify-2453.sh <r1-receipt|r2-action-data|r3-shortfall|r4-regression|r5-build>
set -uo pipefail

MODE="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2453}"
TMPBASE="${TMPBASE:-/tmp/opencode}"
export TMPDIR="${TMPDIR:-$TMPBASE/2453-gobuild}"
export GOCACHE="${GOCACHE:-$TMPBASE/2453-gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$OUT_DIR"

if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1

HEAD_SHA="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=1.0.$HEAD_SHORT commit=$HEAD_SHA dirty=$DIRTY"

run_tests() { # testdb regex
  ( cd "$REPO_ROOT/backend" && TUNELOOP_TEST_DB="$1" go test ./handlers -run "$2" -count=1 -v )
}

case "$MODE" in
  r1-receipt|r2-action-data|r3-shortfall)
    case "$MODE" in
      r1-receipt)      ART="$OUT_DIR/R1-receipt.txt";      TEST='^TestBuildRefundReceipt_NetCash_2453$';      OK=RECEIPT_NET_OK ;;
      r2-action-data)  ART="$OUT_DIR/R2-action-data.txt";  TEST='^TestBuildRefundActionData_NetCash_2453$';  OK=ACTION_NET_OK ;;
      r3-shortfall)    ART="$OUT_DIR/R3-shortfall.txt";    TEST='^TestBuildShortfallBreakdownText_NetCash_2453$'; OK=SHORTFALL_NET_OK ;;
    esac
    {
      echo "$PROVENANCE"
      run_tests "tuneloop_test_2453_${MODE//-/_}" "$TEST"
      echo "$OK"
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- PASS' "$ART" || { echo "FAIL: missing PASS"; exit 1; }
    grep -q "$OK" "$ART" || { echo "FAIL: missing $OK"; exit 1; }
    exit "$rc"
    ;;
  r4-regression)
    ART="$OUT_DIR/R4-regression.txt"
    REGEX='^(TestInspectReturn_Good_RefundReceiptNotification|TestBuildRefundReceipt_IncludesAllLines|TestResolveAppeal_Final_RefundReceiptAndStaffNotify|TestExecuteRefund_ShortfallCreatesPaymentRecord|TestBuildRefundReceipt_NetCash_2453|TestBuildRefundActionData_NetCash_2453|TestBuildShortfallBreakdownText_NetCash_2453)$'
    {
      echo "$PROVENANCE"
      run_tests "tuneloop_test_2453_r4" "$REGEX"
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- FAIL' "$ART" && { echo "FAIL: regression failures"; exit 1; }
    grep -q -- '--- PASS: TestBuildRefundReceipt_NetCash_2453' "$ART" || { echo "FAIL: net-vs-gross case missing"; exit 1; }
    pass_count=$(grep -c -- '--- PASS' "$ART")
    echo "R4_PASS_COUNT=$pass_count"
    exit "$rc"
    ;;
  r5-build)
    ART="$OUT_DIR/R5-build.txt"
    {
      echo "$PROVENANCE"
      ( cd backend && go build ./... )
      echo "BUILD_OK"
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q 'BUILD_OK' "$ART" || { echo "FAIL: build failed"; exit 1; }
    exit "$rc"
    ;;
  *)
    echo "usage: $0 <r1-receipt|r2-action-data|r3-shortfall|r4-regression|r5-build>" >&2
    exit 2
    ;;
esac
