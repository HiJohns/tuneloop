#!/usr/bin/env bash
# #2634 证据生成器 —— PC 库管工作台「扫码验收」弹窗：补入口 + 可录入/可产出结果的控件。
#
# 用法：bash scripts/verify-2634.sh <r1|r2|r3|r4>
#   r1  docs/evidence/2634/R1-prefix-defect.txt   修前快照（PRE_REF = 最早 fix 的父提交）
#   r2  docs/evidence/2634/R2-resolve-unit.txt    resolveScanCandidates 表驱动（9 fixture）
#   r3  docs/evidence/2634/R3-wiring.txt          弹窗接线（入口/录入/动作复用/相机降级/死码清零）
#   r4  docs/evidence/2634/R4-build-lint.txt      PC 构建 + ESLint + 产物文案探针
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<FIX_REF 40 位> # dirty=<bool>`，
# 次行 `# tree=<HEAD sha>`，第三行 `# pre_ref=<PRE_REF sha>`。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)" || exit 2
cd "$REPO" || exit 2
mkdir -p "$REPO/tmp"
ISSUE=2634
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
if [ -z "${PRE_REF:-}" ]; then
  PRE_REF="$(git rev-parse "$(git log --format=%H --grep="^fix(issue-$ISSUE)" | tail -1)^" 2>/dev/null || true)"
fi
[ -n "$PRE_REF" ] || { echo "FATAL: cannot resolve PRE_REF" >&2; exit 3; }
git merge-base --is-ancestor "$PRE_REF" "$HEAD_SHA" || { echo "FATAL: PRE_REF not ancestor of HEAD" >&2; exit 3; }
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"
PRE_LINE="# pre_ref=$PRE_REF"

PAGE="frontend-pc/src/pages/WarehouseManagement.jsx"
RESOLVER="frontend-pc/src/utils/scanResolve.js"
TESTS="frontend-pc/tests/scanResolve.test.mjs"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

LOG="$REPO/tmp/verify-2634-${MODE}.log"
: > "$LOG"
RC=0
NAME=""

emit() { printf '%s\n' "$*" >> "$LOG"; }
require() { grep -Fq -- "$1" "$LOG" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }

provenance() {
  printf '%s\n%s\n%s\n' "$PROV" "$TREE_LINE" "$PRE_LINE" > "$LOG"
}

case "$MODE" in
  r1)
    NAME="R1-prefix-defect.txt"
    provenance
    PRE_PAGE="$(git show "${PRE_REF}:${PAGE}" 2>/dev/null || true)"
    if [ -z "$PRE_PAGE" ]; then
      emit "FATAL: cannot read ${PAGE} at PRE_REF $PRE_REF"
      RC=1
    else
      OPEN_DEF="$(printf '%s\n' "$PRE_PAGE" | grep -c 'handleOpenScanner' || true)"
      OPEN_CALLERS=$((OPEN_DEF > 0 ? OPEN_DEF - 1 : 0))
      # 注意：用 herestring 而非 `printf | grep -q`（pipefail 下 SIGPIPE 会让管道非 0）。
      ENTRY_BUTTON=0
      grep -q 'onClick={handleOpenScanner}' <<< "$PRE_PAGE" && ENTRY_BUTTON=1
      INPUT_BOUND=0
      grep -q 'value={scanCode}' <<< "$PRE_PAGE" && INPUT_BOUND=1
      QR_COMMENTED=0
      grep -qE '^// import QrScanner' <<< "$PRE_PAGE" && QR_COMMENTED=1
      QR_NEW="$(grep -c 'new QrScanner(' <<< "$PRE_PAGE" || true)"
      MANUAL_HINT=0
      grep -q '暂不支持自动识别' <<< "$PRE_PAGE" && MANUAL_HINT=1
      emit "PRE_REF=$PRE_REF"
      emit "PRE_PAGE_HEADER=$PAGE"
      emit "PRE_OPEN_SCANNER_CALLERS=$OPEN_CALLERS"
      emit "PRE_SCAN_ENTRY_BUTTON=$ENTRY_BUTTON"
      emit "PRE_SCAN_INPUT_BOUND=$INPUT_BOUND"
      emit "PRE_QR_IMPORT_COMMENTED=$QR_COMMENTED"
      emit "PRE_QR_NEW_CALLS=$QR_NEW"
      emit "PRE_MANUAL_HINT=$MANUAL_HINT"
      emit "--- pre-fix scanner block ---"
      printf '%s\n' "$PRE_PAGE" | sed -n '70,105p' | sed 's/^/PRE /' >> "$LOG"
      require 'PRE_OPEN_SCANNER_CALLERS=0'
      require 'PRE_SCAN_ENTRY_BUTTON=0'
      require 'PRE_SCAN_INPUT_BOUND=0'
      require 'PRE_QR_IMPORT_COMMENTED=1'
      require 'PRE_QR_NEW_CALLS=1'
      require 'PRE_MANUAL_HINT=0'
    fi
    echo "EXIT=$RC" >> "$LOG"
    [ "$RC" = "0" ] || echo "ASSERT_FAIL: pre-fix snapshot drifted (PRE_REF may be wrong)" >&2
    ;;

  r2)
    NAME="R2-resolve-unit.txt"
    provenance
    TEST_OUT="$(cd "$REPO" && node --test "$TESTS" 2>&1)"
    TEST_RC=$?
    printf '%s\n' "$TEST_OUT" | sed 's/^/TEST /' >> "$LOG"
    CASES="$(printf '%s\n' "$TEST_OUT" | grep -c '^ok [0-9]' || true)"
    FAILS="$(printf '%s\n' "$TEST_OUT" | grep -c '^not ok' || true)"
    tok() { # <test name> -> 1|0
      if printf '%s\n' "$TEST_OUT" | grep -q "^ok [0-9].*$1" && [ "$FAILS" = "0" ]; then echo 1; else echo 0; fi
    }
    emit "TEST_CMD=node --test $TESTS"
    emit "SCAN_RESOLVE_CASES=$CASES"
    emit "EXACT_ORDER_NO=$(tok EXACT_ORDER_NO)"
    emit "EXACT_SN=$(tok EXACT_SN)"
    emit "CASE_INSENSITIVE=$(tok CASE_INSENSITIVE)"
    emit "FUZZY_FALLBACK=$(tok FUZZY_FALLBACK)"
    emit "EMPTY_CODE_NONE=$(tok EMPTY_CODE_NONE)"
    emit "NO_MATCH_NONE=$(tok NO_MATCH_NONE)"
    emit "CANDIDATE_CAP_5=$(tok CANDIDATE_CAP_5)"
    emit "NULL_LIST_GUARD=$(tok NULL_LIST_GUARD)"
    emit "EXIT=$TEST_RC"
    [ "$TEST_RC" = "0" ] || { echo "ASSERT_FAIL: node --test failed" >&2; RC=1; }
    require 'SCAN_RESOLVE_CASES=9'
    for token in EXACT_ORDER_NO EXACT_SN CASE_INSENSITIVE FUZZY_FALLBACK EMPTY_CODE_NONE \
                 NO_MATCH_NONE CANDIDATE_CAP_5 NULL_LIST_GUARD; do
      require "${token}=1"
    done
    require 'EXIT=0'
    ;;

  r3)
    NAME="R3-wiring.txt"
    provenance
    [ -f "$PAGE" ] || { echo "FATAL: page missing" >&2; exit 2; }
    OPEN_DEF="$(grep -c 'handleOpenScanner' "$PAGE" || true)"
    OPEN_CALLERS=$((OPEN_DEF > 0 ? OPEN_DEF - 1 : 0))
    ENTRY=0
    grep -q 'QrcodeOutlined' "$PAGE" && grep -q '>扫码验收</Button>' "$PAGE" && ENTRY=1
    INPUT_BOUND=0
    grep -q 'value={scanCode}' "$PAGE" && grep -q 'onChange={(e) => setScanCode' "$PAGE" && INPUT_BOUND=1
    SUBMIT=0
    grep -q 'onClick={handleScanQuery}' "$PAGE" && grep -q '查询并定位' "$PAGE" && SUBMIT=1
    API_KEYWORD=0
    grep -q "api.get('/warehouse/orders', { params: { keyword: code" "$PAGE" && API_KEYWORD=1
    REUSE_DELIVERY=0
    grep -q 'handleConfirmDelivery(scanOrder.id, scanSn)' "$PAGE" && REUSE_DELIVERY=1
    REUSE_INSPECT=0
    grep -q 'handleInspectReturn(scanOrder.id, true, scanSn)' "$PAGE" && REUSE_INSPECT=1
    CAMERA_GUARD=0
    grep -q 'navigator.mediaDevices?.getUserMedia' "$PAGE" && grep -q 'scanCameraError' "$PAGE" && CAMERA_GUARD=1
    STREAM_CLEANUP=0
    grep -q 'streamRef' "$PAGE" && grep -q 'getTracks().forEach' "$PAGE" && STREAM_CLEANUP=1
    HELPER=0
    grep -q 'resolveScanCandidates(' "$PAGE" && HELPER=1
    MANUAL_HINT=0
    grep -q '暂不支持自动识别' "$PAGE" && MANUAL_HINT=1
    QR_DEAD="$(grep -c 'new QrScanner(' "$PAGE" || true)"
    QR_COMMENTED="$(grep -cE '^// import QrScanner' "$PAGE" || true)"
    LEGACY_STATE="$(grep -cE 'scannedData|scannerRef' "$PAGE" || true)"
    emit "PAGE=$PAGE"
    emit "ENTRY_BUTTON=$ENTRY"
    emit "OPEN_SCANNER_CALLERS=$OPEN_CALLERS"
    emit "SCAN_INPUT_BOUND=$INPUT_BOUND"
    emit "SCAN_SUBMIT=$SUBMIT"
    emit "SCAN_API_KEYWORD=$API_KEYWORD"
    emit "ACTION_REUSE_DELIVERY=$REUSE_DELIVERY"
    emit "ACTION_REUSE_INSPECT=$REUSE_INSPECT"
    emit "CAMERA_GUARD=$CAMERA_GUARD"
    emit "STREAM_CLEANUP=$STREAM_CLEANUP"
    emit "HELPER_USED=$HELPER"
    emit "MANUAL_HINT=$MANUAL_HINT"
    emit "QR_DEAD_CALL=$QR_DEAD"
    emit "QR_IMPORT_COMMENTED=$QR_COMMENTED"
    emit "LEGACY_SCAN_STATE=$LEGACY_STATE"
    emit "--- wiring ---"
    grep -nE '扫码验收|handleOpenScanner|scanCode|handleScanQuery|keyword: code|handleConfirmDelivery\(scanOrder|handleInspectReturn\(scanOrder|scanCameraError|streamRef|resolveScanCandidates\(|暂不支持自动识别' "$PAGE" | sed 's/^/WIRE /' >> "$LOG"
    for token in 'ENTRY_BUTTON=1' 'OPEN_SCANNER_CALLERS=1' 'SCAN_INPUT_BOUND=1' 'SCAN_SUBMIT=1' \
                 'SCAN_API_KEYWORD=1' 'ACTION_REUSE_DELIVERY=1' 'ACTION_REUSE_INSPECT=1' 'CAMERA_GUARD=1' \
                 'STREAM_CLEANUP=1' 'HELPER_USED=1' 'MANUAL_HINT=1' 'QR_DEAD_CALL=0' \
                 'QR_IMPORT_COMMENTED=0' 'LEGACY_SCAN_STATE=0'; do
      require "$token"
    done
    ;;

  r4)
    NAME="R4-build-lint.txt"
    provenance
    BUILD_LOG="$REPO/tmp/verify-2634-build.log"
    ( cd "$REPO/frontend-pc" && "$HEAVY_RUN" --timeout 1800 -- npm run build ) > "$BUILD_LOG" 2>&1
    BUILD_EXIT=$?
    LINT_LOG="$REPO/tmp/verify-2634-eslint.log"
    ( cd "$REPO/frontend-pc" && npx eslint src/pages/WarehouseManagement.jsx src/utils/scanResolve.js ) > "$LINT_LOG" 2>&1
    LINT_EXIT=$?
    emit "BUILD_CMD=cd frontend-pc && npm run build (heavy-run.sh)"
    emit "PC_BUILD=$BUILD_EXIT"
    tail -4 "$BUILD_LOG" | sed 's/^/BUILD_TAIL: /' >> "$LOG"
    emit "LINT_CMD=cd frontend-pc && npx eslint src/pages/WarehouseManagement.jsx src/utils/scanResolve.js"
    emit "LINT_EXIT=$LINT_EXIT"
    tail -8 "$LINT_LOG" | sed 's/^/LINT_TAIL: /' >> "$LOG"
    DIST="$REPO/frontend-pc/dist"
    BUNDLE_ENTRY=0; BUNDLE_QUERY=0; BUNDLE_HINT=0
    if [ -d "$DIST" ]; then
      grep -rqs '扫码验收' "$DIST" && BUNDLE_ENTRY=1
      grep -rqs '查询并定位' "$DIST" && BUNDLE_QUERY=1
      grep -rqs '暂不支持自动识别' "$DIST" && BUNDLE_HINT=1
    fi
    emit "BUNDLE_ENTRY=$BUNDLE_ENTRY"
    emit "BUNDLE_QUERY=$BUNDLE_QUERY"
    emit "BUNDLE_HINT=$BUNDLE_HINT"
    if [ "$BUILD_EXIT" = "0" ] && [ "$LINT_EXIT" = "0" ] && [ "$BUNDLE_ENTRY" = "1" ] && [ "$BUNDLE_QUERY" = "1" ] && [ "$BUNDLE_HINT" = "1" ]; then
      emit "BUILD_LINT_OK=1"
    else
      emit "BUILD_LINT_OK=0"
    fi
    require 'PC_BUILD=0'
    require 'LINT_EXIT=0'
    require 'BUNDLE_ENTRY=1'
    require 'BUNDLE_QUERY=1'
    require 'BUNDLE_HINT=1'
    require 'BUILD_LINT_OK=1'
    ;;
esac

if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >> "$LOG"
else
  echo "RESULT: PASS ($MODE)" >> "$LOG"
fi
if [ -n "$NAME" ]; then
  cp "$LOG" "$OUT_DIR/$NAME"
  echo "ARTIFACT=$OUT_DIR/$NAME" >&2
fi
tail -1 "$LOG" >&2
[ "$RC" = "0" ] || exit 1
exit 0
