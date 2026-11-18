#!/usr/bin/env bash
# #2415 S1：AppDialog / DialogHost 验收脚本（Rev.2）。
# 用法：bash scripts/verify-2415.sh <r1-alert|r2-confirm|r3-scroll|r4-probe-build
#        |r5-queue|r6-editable|r7-toast|r8-mount>
# 每个模式写 docs/evidence/2415/ 对应产物（首行 provenance）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-alert)        ART="docs/evidence/2415/R1-alert.txt"; TEST="TestAppDialogAlert_2415" ;;
  r2-confirm)      ART="docs/evidence/2415/R2-confirm.txt"; TEST="TestAppDialogConfirm_2415" ;;
  r3-scroll)       ART="docs/evidence/2415/R3-scroll.txt"; TEST="TestAppDialogScroll_2415" ;;
  r4-probe-build)  ART="docs/evidence/2415/R4-probe-build.txt"; TEST="" ;;
  r5-queue)        ART="docs/evidence/2415/R5-queue.txt"; TEST="TestAppDialogQueue_2415" ;;
  r6-editable)     ART="docs/evidence/2415/R6-editable.txt"; TEST="TestAppDialogEditable_2415" ;;
  r7-toast)        ART="docs/evidence/2415/R7-toast.txt"; TEST="TestAppDialogToast_2415" ;;
  r8-mount)        ART="docs/evidence/2415/R8-mount.txt"; TEST="TestDialogHostMounted_2415" ;;
  *) echo "usage: $0 <r1-alert|r2-confirm|r3-scroll|r4-probe-build|r5-queue|r6-editable|r7-toast|r8-mount>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
mkdir -p "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2415/' || true)" ]; then
  DIRTY=true
fi

run_test() {
  ( cd "$REPO_ROOT/frontend-mobile" && npx vitest run --config vitest.config.js scripts/page-smoke/app-dialog-2415.test.jsx -t "$1" )
  local rc=$?
  if [ "$rc" = "0" ]; then
    echo "--- PASS: $1"
  else
    echo "--- FAIL: $1"
  fi
  return "$rc"
}

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  if [ "$MODE" = "r4-probe-build" ]; then
    cd "$REPO_ROOT"
    modal=$(grep -rn "Taro.showModal" frontend-mobile/src/platform | wc -l | tr -d ' ')
    toast=$(grep -rn "Taro.showToast" frontend-mobile/src/platform | wc -l | tr -d ' ')
    alert=$(grep -rn "window.alert" frontend-mobile/src/platform | wc -l | tr -d ' ')
    confirm=$(grep -rn "window.confirm" frontend-mobile/src/platform | wc -l | tr -d ' ')
    echo "fallback_counts: Taro.showModal=$modal Taro.showToast=$toast window.alert=$alert window.confirm=$confirm"
    if [ "$modal" -gt 1 ] || [ "$toast" -gt 1 ] || [ "$alert" -gt 1 ] || [ "$confirm" -gt 1 ]; then
      echo "FALLBACK_PROBE FAIL"
      exit 1
    fi
    cd "$REPO_ROOT/frontend-mobile"
    npm run build
  else
    run_test "$TEST"
  fi
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
