#!/usr/bin/env bash
# #2362 Rev.3：支付端乐币额度/金额一致性 + 双端构建验收。
# 用法：bash scripts/verify-2362.sh <r1-order-snapshot|r2-cap|r3-order-reject|r4-order-amount|r5-build-h5|r6-build-weapp>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-order-snapshot) ART="docs/evidence/2362/R1-snapshot.txt"; TEST='^TestOrderSnapshotRatio_2362$' ;;
  r2-cap)            ART="docs/evidence/2362/R2-cap.txt"; TEST='^TestPointsCapParity_2362$' ;;
  r3-order-reject)   ART="docs/evidence/2362/R3-over-limit.txt"; TEST='^TestOrderRejectsBadPoints_2362$' ;;
  r4-order-amount)   ART="docs/evidence/2362/R4-amount.txt"; TEST='^TestOrderAmountConsistency_2362$' ;;
  r5-build-h5)       ART="docs/evidence/2362/R5-build-h5.txt"; TEST="" ;;
  r6-build-weapp)    ART="docs/evidence/2362/R6-build-weapp.txt"; TEST="" ;;
  *) echo "usage: $0 <r1-order-snapshot|r2-cap|r3-order-reject|r4-order-amount|r5-build-h5|r6-build-weapp>" >&2; exit 2 ;;
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
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2362}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2362/' || true)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1-order-snapshot|r2-cap|r3-order-reject|r4-order-amount)
      cd "$REPO_ROOT/backend"
      go test ./handlers/ -run "$TEST" -count=1 -v
      ;;
    r5-build-h5)
      cd "$REPO_ROOT/frontend-mobile"
      . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1
      nvm use 22 >/dev/null 2>&1
      npm run build:h5
      ;;
    r6-build-weapp)
      cd "$REPO_ROOT/frontend-mobile"
      . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1
      nvm use 22 >/dev/null 2>&1
      npm run build:weapp
      hits=$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' dist-weapp --include='*.wxml' 2>/dev/null | grep -v '/base.wxml$' || true)
      if [ -n "$hits" ]; then
        echo "WXML_PROBE FAIL:"
        printf '%s\n' "$hits"
        exit 1
      fi
      echo "WXML_PROBE PASS"
      ;;
  esac
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
