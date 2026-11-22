#!/usr/bin/env bash
# #2434 Rev.4：扫码直达页返回兜底（R0 红例 / R1 判定表 / R2 探针 / R3 smoke /
# R4 边界 / R5 Bind 真实渲染 / R6 溯源 / P5 全量 gate）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r0-red) NAME="R0-red.txt" ;;
  r1-unit) NAME="R1-unit.txt" ;;
  r2-probe) NAME="R2-probe.txt" ;;
  r3-smoke) NAME="R3-smoke.txt" ;;
  r4-decision) NAME="R4-decision.txt" ;;
  r5-bind-render) NAME="R5-bind-render.txt" ;;
  r6-provenance) NAME="R6-provenance.txt" ;;
  p5-full) NAME="P5-full-gate.txt" ;;
  *) echo "usage: $0 <r0-red|r1-unit|r2-probe|r3-smoke|r4-decision|r5-bind-render|r6-provenance|p5-full>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/2434}"
mkdir -p "$OUT_DIR"
ART="$OUT_DIR/$NAME"

FIX_REF="${FIX_REF:-HEAD}"
FIX_SHA="$(git rev-parse "$FIX_REF")"; FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
HEAD_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.$FIX_SHORT"
DIRTY=false
[ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=$BUILD commit=$FIX_SHA dirty=$DIRTY tree=$HEAD_SHA"

backend() {
  cd "$REPO/backend"
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
  export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}" \
         TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2434_vr}" \
         IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" IAM_SECRET="${IAM_SECRET:-test-secret-2434}"
  mkdir -p "$TMPDIR" "$GOCACHE"
}

case "$MODE" in
  r0-red)
    {
      echo "$PROV"
      SRC="$(git show 2eed6f8a9^:frontend-mobile/src/pages-weapp/bind/Bind.jsx)"
      count="$(printf '%s\n' "$SRC" | grep -c 'Taro.navigateBack()' || true)"
      safe="$(printf '%s\n' "$SRC" | grep -c 'navigation.safeBack' || true)"
      echo "PRE_FIX_NAVIGATE_BACK_COUNT=$count"
      echo "PRE_FIX_SAFEBACK_COUNT=$safe"
      if [ "$count" = "3" ] && [ "$safe" = "0" ]; then
        echo "RED_CONFIRMED"
      else
        echo "FAIL: expected 3 bare navigateBack and 0 safeBack"
        exit 1
      fi
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r1-unit|r4-decision|r5-bind-render)
    {
      echo "$PROV"
      cd "$REPO/frontend-mobile"
      case "$MODE" in
        r1-unit) npx vitest run --config vitest.config.js scripts/page-smoke/back-nav-2434.test.jsx ;;
        r4-decision) npx vitest run --config vitest.config.js scripts/page-smoke/back-nav-2434.test.jsx -t 'R4 判定表' ;;
        r5-bind-render) npx vitest run --config vitest.config.js scripts/page-smoke/back-nav-2434.test.jsx -t 'R5 Bind 空栈渲染' ;;
      esac
      rc=$?
      echo "MODE_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-probe)
    {
      echo "$PROV"
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
      echo "$PROV"
      cd "$REPO/frontend-mobile"
      npm run smoke
      rc=$?
      echo "SMOKE_EXIT=$rc"
      exit "$rc"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-provenance)
    {
      echo "$PROV"
      fail=0
      status="$(git status --porcelain -uall || true)"
      if [ -n "$status" ]; then
        echo "FAIL: worktree is dirty"
        printf '%s\n' "$status"
        fail=1
      else
        echo "WORKTREE_CLEAN_OK"
      fi
      for f in R0-red R1-unit R2-probe R3-smoke R4-decision R5-bind-render; do
        p="$OUT_DIR/$f.txt"
        if [ ! -f "$p" ]; then echo "FAIL: missing $p"; fail=1; continue; fi
        first="$(head -1 "$p")"
        case "$first" in
          "# build=$BUILD commit=$FIX_SHA dirty=false "*) echo "OK $f" ;;
          *) echo "FAIL: provenance mismatch in $f: $first"; fail=1 ;;
        esac
      done
      [ "$fail" = "0" ] && echo "PROVENANCE_OK"
      exit "$fail"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  p5-full)
    {
      echo "$PROV"
      backend
      echo "BUILD_CMD=go build ./..."
      go build ./...
      brc=$?
      echo "BUILD_EXIT=$brc"
      [ "$brc" = "0" ] && echo "BUILD_OK"
      mkdir -p "$REPO/tmp"
      LOG="$REPO/tmp/2434-full-gate.log"
      echo "FULL_SUITE_CMD=go test ./... -count=1 -timeout 20m"
      go test ./... -count=1 -timeout 20m > "$LOG" 2>&1
      trc=$?
      cat "$LOG"
      fails="$(grep -c '^--- FAIL' "$LOG" || true)"
      echo "FULL_SUITE_EXIT=$trc FAIL_COUNT=${fails:-0}"
      bad="$(grep -E '^FAIL[[:space:]]+tuneloop-backend/' "$LOG" | awk '{print $2}' | grep -vE 'tuneloop-backend/(database|middleware|service|services)$' || true)"
      baseline="$(grep -E '^FAIL[[:space:]]+tuneloop-backend/' "$LOG" | awk '{print $2}' | grep -E 'tuneloop-backend/(database|middleware|service|services)$' || true)"
      if [ -n "$baseline" ]; then
        echo "BASELINE_FAIL:"
        printf '%s\n' "$baseline"
      fi
      if [ -n "$bad" ]; then
        echo "FAIL: non-baseline package failures:"
        printf '%s\n' "$bad"
        exit 1
      fi
      [ "$brc" = "0" ] || exit 1
      echo "P5_BASELINE_OK"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
