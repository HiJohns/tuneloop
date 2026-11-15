#!/usr/bin/env bash
# #2352 Rev.2 R3-R8：评测标的/评测必经/档位自选/零押金小票/历史兼容/文档。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in amount|state|choice|zero-receipt|legacy|doc) ;; *) echo "usage: $0 amount|state|choice|zero-receipt|legacy|doc" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2352}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
mkdir -p docs/evidence/2352
PROVENANCE="build=credit-flow-2352 commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"

case "$MODE" in
  amount) TEST=TestCreditRiskAmountEqualsInstrumentPrice_2352; OUT=docs/evidence/2352/R3-amount.txt ;;
  state) TEST=TestAllPayscoreOrdersRequireAssessment_2352; OUT=docs/evidence/2352/R4-assessment-state.txt ;;
  choice) TEST=TestPaymentTierChoice_2352; OUT=docs/evidence/2352/R5-tier-choice.txt ;;
  zero-receipt) TEST=TestZeroReceiptDepositIsZero_2352; OUT=docs/evidence/2352/R6-zero-receipt.txt ;;
  legacy) TEST=TestLegacyInternalOrderUnchanged_2352; OUT=docs/evidence/2352/R7-legacy.txt ;;
  doc)
    OUT=docs/evidence/2352/R8-doc.txt
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep assessment URLs in docs/topics/wechat/wechat-pay-integration.md"
      grep -n "credit-assessment" docs/topics/wechat/wechat-pay-integration.md
      grep -n "支付分申请" docs/topics/wechat/wechat-pay-integration.md
    } > "$OUT" 2>&1
    grep -q "/credit-assessment" "$OUT" && grep -q "pages-weapp/credit-assessment/index" "$OUT" && grep -q "支付分申请" "$OUT" \
      || { echo "R8 FAIL: doc missing URLs/purpose" >&2; exit 1; }
    echo "PASS: doc registers both assessment URLs"
    exit 0
    ;;
esac

{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD (cd backend && go test ./handlers/ -run $TEST -count=1 -v)"
  ( cd backend && go test ./handlers/ -run "$TEST" -count=1 -v )
} 2>&1 | tee "$OUT"
grep -q -- "--- PASS: $TEST" "$OUT" || { echo "$MODE FAIL" >&2; exit 1; }
echo "PASS: $MODE"
