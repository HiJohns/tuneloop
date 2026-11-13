#!/usr/bin/env bash
# #2338 R3: full handlers suite must not exceed the Group B whitelist (#2339).
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
LOG=tmp/2338-full-handlers.log
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ -count=1 -timeout 12m"
  ( cd backend && go test ./handlers/ -count=1 -timeout 12m ) > "$LOG" 2>&1 || true
  echo "-- failing tests --"
  grep -E '^--- FAIL' "$LOG" | sed -E 's/^--- FAIL: ([^ ]+).*/\1/' | sort -u
} | tee docs/evidence/2338/R3-containment.txt
FAILS=$(grep -E '^--- FAIL' "$LOG" | sed -E 's/^--- FAIL: ([^ ]+).*/\1/' | sort -u)
COUNT=$(printf '%s\n' "$FAILS" | grep -c . || true)
ALLOWED=$(printf '%s\n' TestBatchCreateOrder_RentDaysOverridesEndDate TestCreateRentalOrder TestGetUserInstrumentDetail TestInspectReturn_Good_ExecutesRefund TestRefundDiff_PointsOverCap TestRefundDiff_PointsWithinCap TestScenarioA_DamageVariant TestScenarioC_CancelBoundary TestSettlementFlow | sort -u)
OUT=$(printf '%s\n' "$FAILS" | grep -vxF -f <(printf '%s\n' "$ALLOWED") || true)
if [ "$COUNT" -gt 9 ] || [ -n "$OUT" ]; then
  echo "R3 FAIL: count=$COUNT out-of-whitelist=$OUT" | tee -a docs/evidence/2338/R3-containment.txt >&2
  exit 1
fi
echo "R3 PASS: fail_count=$COUNT all within Group B whitelist (#2339)" | tee -a docs/evidence/2338/R3-containment.txt
