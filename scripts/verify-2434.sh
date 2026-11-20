#!/usr/bin/env bash
# #2434：扫码直达页返回兜底（safeBack）+ 判定表（同名页/redirectTo）。
# 用法：bash scripts/verify-2434.sh <r1-unit|r2-probe|r3-smoke|r4-decision>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-unit) ART="docs/evidence/2434/R1-unit.txt" ;;
  r2-probe) ART="docs/evidence/2434/R2-probe.txt" ;;
  r3-smoke) ART="docs/evidence/2434/R3-smoke.txt" ;;
  r4-decision) ART="docs/evidence/2434/R4-decision.txt" ;;
  *) echo "usage: $0 <r1-unit|r2-probe|r3-smoke|r4-decision>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2434/' || true)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
mkdir -p "$(dirname "$ART")"

case "$MODE" in
  r1-unit)
    {
      echo "$PROVENANCE"
      ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/back-nav-2434.test.jsx )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r4-decision)
    {
      echo "$PROVENANCE"
      ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/back-nav-2434.test.jsx -t 'R4 判定表' )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-probe)
    {
      echo "$PROVENANCE"
      fail=0
      hits=$(grep -rn "Taro.navigateBack()" frontend-mobile/src/pages-weapp/bind frontend-mobile/src/pages-weapp/login frontend-mobile/src/pages-weapp/profile-complete frontend-mobile/src/pages-weapp/account-select 2>/dev/null || true)
      if [ -n "$hits" ]; then echo "FAIL: bare navigateBack"; echo "$hits"; fail=1; fi
      grep -q "safeBack" frontend-mobile/src/platform/index.weapp.js && echo "WEAPP_SAFEBACK_OK" || { echo "WEAPP_SAFEBACK_MISSING"; fail=1; }
      grep -q "safeBack" frontend-mobile/src/platform/browser.js && echo "H5_SAFEBACK_OK" || { echo "H5_SAFEBACK_MISSING"; fail=1; }
      [ "$fail" = "0" ] && echo "PROBE-OK" || echo "PROBE-FAIL"
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3-smoke)
    {
      echo "$PROVENANCE"
      ( cd frontend-mobile && npm run smoke )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
