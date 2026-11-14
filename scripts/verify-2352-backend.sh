#!/usr/bin/env bash
# #2352 R4/R5/R6：后端押金档位规则（reject-zero / zero-gate / legacy）。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in reject-zero|zero-gate|legacy) ;; *) echo "usage: $0 reject-zero|zero-gate|legacy" >&2; exit 2 ;; esac
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
PROVENANCE="build=deposit-tiers-2352 commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"

case "$MODE" in
  reject-zero) TEST=TestZeroTierRejectedWhenFullDepositPositive_2352; OUT=docs/evidence/2352/R4-reject-zero-full.txt ;;
  zero-gate)   TEST=TestZeroTierRequiresAssessment_2352;            OUT=docs/evidence/2352/R5-zero-assessment-gate.txt ;;
  legacy)      TEST=TestLegacyOrderTiersUnchanged_2352;            OUT=docs/evidence/2352/R6-legacy-orders.txt ;;
esac

{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD (cd backend && go test ./handlers/ -run $TEST -count=1 -v)"
  ( cd backend && go test ./handlers/ -run "$TEST" -count=1 -v )
} 2>&1 | tee "$OUT"

grep -q -- "--- PASS: $TEST" "$OUT" || { echo "$MODE FAIL: test did not pass" >&2; exit 1; }
echo "$OUT: PASS"
