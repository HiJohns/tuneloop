#!/usr/bin/env bash
# #2414：POST /api/user/rental/calculate 押金口径（#2406 规则）验收。
# 用法：bash scripts/verify-2414.sh <r1|r2|r3|r4|r5|r6|r7>
# 每个模式会把带 provenance 头的输出写入 docs/evidence/2414/ 对应产物。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2414/R1-zero-fallback.txt"; TEST='^TestCalculateRentalDeposit_2406Rule_2414$' ;;
  r2) ART="docs/evidence/2414/R2-instrument.txt"; TEST='^TestCalculateRentalDepositInstrumentOverride_2414$' ;;
  r3) ART="docs/evidence/2414/R3-true-zero.txt"; TEST='^TestCalculateRentalDepositTrueZero_2414$' ;;
  r4) ART="docs/evidence/2414/R4-build.txt"; TEST="" ;;
  r5) ART="docs/evidence/2414/R5-units.txt"; TEST='^TestCalculateRentalUnitConsistency_2414$' ;;
  r6) ART="docs/evidence/2414/R6-doc.txt"; TEST="" ;;
  r7) ART="docs/evidence/2414/R7-callers.txt"; TEST="" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2414}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2414/' || true)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r4) cd backend && go build ./... ;;
    r5) cd backend && go test ./handlers/ -run "$TEST" -count=1 -v ;;
    r6)
      f="docs/spec/api/api.md"
      if grep -q "cash_to_pay == total_rent + deposit + shipping_fee" "$f" && \
         grep -q "sum(tiers\[\].subtotal) == total_rent" "$f" && \
         grep -q "gift_points_max" "$f"; then
        echo "DOC_OK: §8.12 units and invariants present"
      else
        echo "DOC_MISSING: §8.12 units/invariants"; exit 1
      fi
      ;;
    r7)
      hits=$(grep -rn "rental/calculate" backend frontend-pc frontend-mobile --include='*.go' --include='*.js' --include='*.jsx' 2>/dev/null || true)
      echo "$hits"
      frontend=$(printf '%s\n' "$hits" | grep -c "frontend-" || true)
      if [ "$frontend" != "0" ]; then echo "CALLERS FAIL: frontend callers=$frontend"; exit 1; fi
      if ! printf '%s\n' "$hits" | grep -q "backend/main.go"; then echo "CALLERS FAIL: route missing"; exit 1; fi
      echo "CALLERS OK: no frontend callers; route + handler tests only"
      ;;
    *) cd backend && go test ./handlers/ -run "$TEST" -count=1 -v ;;
  esac
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
