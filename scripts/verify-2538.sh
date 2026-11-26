#!/usr/bin/env bash
# #2538 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2538.sh r1    # /site/stock/:id redirects to /instruments/detail/:id
#   bash scripts/verify-2538.sh r2    # header summary helpers (node --test + values)
#   bash scripts/verify-2538.sh r3    # stub + 流转轨迹/关联单据 removed
#   bash scripts/verify-2538.sh r4    # 日志 tab, activity-log call, hero image
#   bash scripts/verify-2538.sh r5    # docs (ui.md)
#   bash scripts/verify-2538.sh r6    # node --test + eslint + PC build + go build
#
# Capture a row into its artifact:
#   bash scripts/verify-2538.sh r1 > docs/evidence/2538/R1-route-redirect.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2538)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
HEAD_SHA="$(git rev-parse HEAD)"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
echo "GENERATED_AT_HEAD=${HEAD_SHA}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }
HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

APP="frontend-pc/src/App.jsx"
DETAIL="frontend-pc/src/pages/admin/instrument/Detail.jsx"
SUMMARY_UTIL="frontend-pc/src/utils/assetSummary.js"
SUMMARY_TEST="frontend-pc/tests/assetSummary.test.mjs"

case "$ROW" in
  r1)
    echo "== R1 /site/stock/:id → /instruments/detail/:id =="
    NAVIGATE_REDIRECT=0
    grep -q 'Navigate to={`/instruments/detail/${id}`} replace' "$APP" && NAVIGATE_REDIRECT=1
    ROUTE_WIRED=0
    grep -q '<Route path="/site/stock/:id" element={<ProtectedRoute><LegacyStockDetailRedirect />' "$APP" && ROUTE_WIRED=1
    REDIRECT_TARGET=/instruments/detail/
    STOCK_LIST_ROUTE_UNCHANGED=0
    grep -q '<Route path="/site/stock" element={<ProtectedRoute><InstrumentStock />' "$APP" && STOCK_LIST_ROUTE_UNCHANGED=1
    STUB_ROUTE_REMOVED=0
    # Only real imports/routes count — comments may legitimately name the deleted stub.
    grep -qE "import AssetDetail[^A-Za-z]|<AssetDetail[ /<>]" "$APP" || STUB_ROUTE_REMOVED=1

    echo "NAVIGATE_REDIRECT=${NAVIGATE_REDIRECT}"
    echo "ROUTE_WIRED=${ROUTE_WIRED}"
    echo "REDIRECT_TARGET=${REDIRECT_TARGET}"
    echo "STOCK_LIST_ROUTE_UNCHANGED=${STOCK_LIST_ROUTE_UNCHANGED}"
    echo "STUB_ROUTE_REMOVED=${STUB_ROUTE_REMOVED}"

    [ "$NAVIGATE_REDIRECT" = "1" ] || fail "App.jsx has no <Navigate> redirect to the unified detail page"
    [ "$ROUTE_WIRED" = "1" ] || fail "the /site/stock/:id route is not wired to the redirect component"
    [ "$STOCK_LIST_ROUTE_UNCHANGED" = "1" ] || fail "the /site/stock list route must stay unchanged"
    [ "$STUB_ROUTE_REMOVED" = "1" ] || fail "App.jsx still references the removed AssetDetail stub"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 header summary helpers =="
    OUT="$(mktemp)"
    node --test "$SUMMARY_TEST" > "$OUT" 2>&1
    rc=$?
    TESTS_PASS="$(sed -n 's/^# pass \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    TESTS_FAIL="$(sed -n 's/^# fail \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    tail -8 "$OUT"
    rm -f "$OUT"

    VALUES="$(node --input-type=module -e "
import { buildAssetSummary } from './$SUMMARY_UTIL'
const rows = buildAssetSummary(
  { total_price: 360000, created_at: '2026-01-02T03:04:05Z' },
  { base_daily_rate: 10000, overdue_daily_fee: 3600, deposit: 50000, deposit_reduced: null }
)
const map = Object.fromEntries(rows.map(r => [r.key, r]))
console.log('LABELS=' + rows.length)
console.log('DAILY=' + map.daily_rate.value)
console.log('OVERDUE=' + map.overdue_daily_fee.value)
console.log('DEPOSIT=' + map.deposit.value)
console.log('REDUCED_EMPTY=' + map.deposit_reduced.value)
console.log('TOTAL_PRICE=' + map.total_price.value)
console.log('JOIN_DATE=' + map.created_at.value)
")"
    printf '%s\n' "$VALUES"
    echo "TESTS_PASS=${TESTS_PASS:-0}"
    echo "TESTS_FAIL=${TESTS_FAIL:-1}"

    [ "$rc" -eq 0 ] || fail "node --test failed (rc=${rc})"
    [ "${TESTS_FAIL:-1}" = "0" ] || fail "node --test reported ${TESTS_FAIL} failure(s)"
    [ "${TESTS_PASS:-0}" -ge 8 ] || fail "expected >=8 passing tests, got ${TESTS_PASS:-0}"
    printf '%s\n' "$VALUES" | grep -qx 'LABELS=6' || fail "summary must expose exactly 6 items"
    printf '%s\n' "$VALUES" | grep -qx 'REDUCED_EMPTY=未配置' || fail "missing reduced deposit must render 未配置"
    TOTAL_PRICE_CENTS=0
    printf '%s\n' "$VALUES" | grep -qx 'TOTAL_PRICE=¥3600.00' && TOTAL_PRICE_CENTS=1
    JOIN_DATE_FROM_CREATED_AT=0
    printf '%s\n' "$VALUES" | grep -qE '^JOIN_DATE=.+月.+日' && JOIN_DATE_FROM_CREATED_AT=1
    echo "TOTAL_PRICE_CENTS=${TOTAL_PRICE_CENTS}"
    echo "JOIN_DATE_FROM_CREATED_AT=${JOIN_DATE_FROM_CREATED_AT}"
    [ "$TOTAL_PRICE_CENTS" = "1" ] || fail "估值 must be total_price cents (¥3600.00 for 360000)"
    [ "$JOIN_DATE_FROM_CREATED_AT" = "1" ] || fail "加入日期 must come from created_at"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 stub + legacy labels removed =="
    STUB_FILES=0
    for f in frontend-pc/src/pages/AssetDetail.jsx frontend-pc/src/utils/assetDetail.js frontend-pc/tests/assetDetail.test.mjs; do
      [ -e "$f" ] && STUB_FILES=$((STUB_FILES + 1))
    done
    ASSETDETAIL_IMPORTS="$(grep -rE "from '[^']*assetDetail(\.js)?'|import AssetDetail" frontend-pc/src frontend-pc/tests 2>/dev/null | wc -l | tr -d ' ')"
    TIMELINE_TAB_LABEL="$(grep -rF "'流转轨迹'" frontend-pc/src 2>/dev/null | wc -l | tr -d ' ')"
    DOCUMENTS_TAB_LABEL="$(grep -rF "'关联单据'" frontend-pc/src 2>/dev/null | wc -l | tr -d ' ')"

    echo "STUB_FILES=${STUB_FILES}"
    echo "ASSETDETAIL_IMPORTS=${ASSETDETAIL_IMPORTS}"
    echo "TIMELINE_TAB_LABEL=${TIMELINE_TAB_LABEL}"
    echo "DOCUMENTS_TAB_LABEL=${DOCUMENTS_TAB_LABEL}"

    [ "$STUB_FILES" = "0" ] || fail "stub files still present: AssetDetail.jsx / assetDetail.js / assetDetail.test.mjs"
    [ "$ASSETDETAIL_IMPORTS" = "0" ] || fail "a module still imports the removed assetDetail helpers"
    [ "$TIMELINE_TAB_LABEL" = "0" ] || fail "a '流转轨迹' tab label is still rendered"
    [ "$DOCUMENTS_TAB_LABEL" = "0" ] || fail "a '关联单据' placeholder is still rendered"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 log tab + hero image =="
    LOG_TAB=0
    grep -qF "label: '日志'" "$DETAIL" && LOG_TAB=1
    ACTIVITY_LOG_CALL=0
    grep -q 'getActivityLog(instrumentId)' "$DETAIL" && ACTIVITY_LOG_CALL=1
    HERO_IMAGE=0
    grep -q 'instrumentImageSrc' "$DETAIL" && grep -q 'INSTRUMENT_IMAGE_PLACEHOLDER' "$DETAIL" && HERO_IMAGE=1
    SUMMARY_RENDERED=0
    grep -q 'buildAssetSummary(instrument, pricingV2)' "$DETAIL" && SUMMARY_RENDERED=1
    CREATE_TIME_ROW=0
    grep -qF 'label="创建时间"' "$DETAIL" && CREATE_TIME_ROW=1

    echo "LOG_TAB=${LOG_TAB}"
    echo "ACTIVITY_LOG_CALL=${ACTIVITY_LOG_CALL}"
    echo "HERO_IMAGE=${HERO_IMAGE}"
    echo "SUMMARY_RENDERED=${SUMMARY_RENDERED}"
    echo "CREATE_TIME_ROW=${CREATE_TIME_ROW}"

    [ "$LOG_TAB" = "1" ] || fail "the 日志 tab is missing"
    [ "$ACTIVITY_LOG_CALL" = "1" ] || fail "the activity-log call is missing"
    [ "$HERO_IMAGE" = "1" ] || fail "the header hero image is not wired to instrumentImageSrc"
    [ "$SUMMARY_RENDERED" = "1" ] || fail "the header summary does not render buildAssetSummary(...)"
    [ "$CREATE_TIME_ROW" = "0" ] || fail "基本信息 still duplicates 创建时间 (value moved to 加入日期)"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 docs =="
    DOC_SUMMARY=0
    grep -q '页头摘要' docs/spec/ui/ui.md && DOC_SUMMARY=1
    DOC_JOIN_DATE=0
    grep -q '加入日期' docs/spec/ui/ui.md && DOC_JOIN_DATE=1
    DOC_REDIRECT=0
    grep -q '兼容重定向' docs/spec/ui/ui.md && grep -q '/site/stock/:id' docs/spec/ui/ui.md && DOC_REDIRECT=1
    DOC_LOGS=0
    grep -q 'ActivityLogTab' docs/spec/ui/ui.md && DOC_LOGS=1

    echo "DOC_SUMMARY=${DOC_SUMMARY}"
    echo "DOC_JOIN_DATE=${DOC_JOIN_DATE}"
    echo "DOC_REDIRECT=${DOC_REDIRECT}"
    echo "DOC_LOGS=${DOC_LOGS}"

    [ "$DOC_SUMMARY" = "1" ] || fail "ui.md must document the header summary"
    [ "$DOC_JOIN_DATE" = "1" ] || fail "ui.md must document 加入日期"
    [ "$DOC_REDIRECT" = "1" ] || fail "ui.md must document the /site/stock/:id redirect"
    [ "$DOC_LOGS" = "1" ] || fail "ui.md must document the 日志 tab as the only event view"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 tests + lint + builds =="
    TEST_LOG="$(mktemp)"
    ( cd frontend-pc && node --test tests/assetSummary.test.mjs tests/instrumentDisplay.test.mjs ) > "$TEST_LOG" 2>&1
    NODE_TEST_RC=$?
    grep -E '^# (tests|pass|fail)' "$TEST_LOG" || true
    rm -f "$TEST_LOG"

    LINT_LOG="$(mktemp)"
    ( cd frontend-pc && ./node_modules/.bin/eslint src/App.jsx src/pages/admin/instrument/Detail.jsx src/utils/assetSummary.js tests/assetSummary.test.mjs ) > "$LINT_LOG" 2>&1
    ESLINT_RC=$?
    tail -5 "$LINT_LOG"; rm -f "$LINT_LOG"

    BUILD_LOG="$(mktemp)"
    ( cd frontend-pc && npm run build ) > "$BUILD_LOG" 2>&1
    PC_BUILD_RC=$?
    tail -3 "$BUILD_LOG"; rm -f "$BUILD_LOG"

    GO_LOG="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$GO_LOG" 2>&1
    GO_BUILD_RC=$?
    tail -3 "$GO_LOG"; rm -f "$GO_LOG"

    echo "NODE_TEST_RC=${NODE_TEST_RC}"
    echo "ESLINT_RC=${ESLINT_RC}"
    echo "PC_BUILD_RC=${PC_BUILD_RC}"
    echo "GO_BUILD_RC=${GO_BUILD_RC}"

    [ "$NODE_TEST_RC" = "0" ] && echo "NODE_TEST_OK=1" || fail "node --test failed (rc=${NODE_TEST_RC})"
    [ "$ESLINT_RC" = "0" ] && echo "ESLINT_OK=1" || fail "eslint failed (rc=${ESLINT_RC})"
    [ "$PC_BUILD_RC" = "0" ] && echo "PC_BUILD_OK=1" || fail "npm run build failed (rc=${PC_BUILD_RC})"
    [ "$GO_BUILD_RC" = "0" ] && echo "GO_BUILD_OK=1" || fail "go build ./... failed (rc=${GO_BUILD_RC})"
    echo "R6_OK"
    ;;
esac
