#!/usr/bin/env bash
# #2537 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2537.sh r1    # instrument list overdue_daily_fee == pricing-v2
#   bash scripts/verify-2537.sh r2    # deposit / overdue display helpers (node --test)
#   bash scripts/verify-2537.sh r3    # two-page column sets (static)
#   bash scripts/verify-2537.sh r4    # SN is the detail entry, no view/detail buttons (static)
#   bash scripts/verify-2537.sh r5    # docs (api.md / ui.md)
#   bash scripts/verify-2537.sh r6    # build + targeted tests + eslint + PC build
#
# Capture a row into its artifact:
#   bash scripts/verify-2537.sh r1 > docs/evidence/2537/R1-list-field.txt
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

# Backend integration tests need the Postgres test database (docs/ops/test-db-lifecycle.md).
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2537)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
HEAD_SHA="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
echo "GENERATED_AT_HEAD=${HEAD_SHA}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }
HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

STOCK_PAGE="frontend-pc/src/pages/InstrumentStock.jsx"
LIST_PAGE="frontend-pc/src/pages/admin/instrument/List.jsx"
HELPERS="frontend-pc/src/utils/instrumentDisplay.js"
HELPER_TESTS="frontend-pc/tests/instrumentDisplay.test.mjs"

# columns_block FILE ARRAY_DECL → the main table column definitions only
# (stops at the first `  ]`, so unrelated tables later in the file are excluded).
columns_block() {
  awk -v decl="$2" '
    index($0, decl) { inb = 1 }
    inb { print }
    inb && /^  \][[:space:]]*$/ { exit }
  ' "$1"
}

case "$ROW" in
  r1)
    echo "== R1 instrument list overdue_daily_fee == pricing-v2 =="
    OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -run TestInstrumentListOverdue2537_R1 -count=1 -v ) > "$OUT" 2>&1
    rc=$?
    grep -E '^(LIST_OVERDUE_[ABC]|PRICING_V2_OVERDUE_A)=' "$OUT" || true
    grep -E '^--- (PASS|FAIL)' "$OUT" || true
    if [ "$rc" -ne 0 ]; then tail -20 "$OUT"; rm -f "$OUT"; fail "go test exit=${rc}"; fi
    rm -f "$OUT"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 deposit / overdue display helpers =="
    OUT="$(mktemp)"
    node --test "$HELPER_TESTS" > "$OUT" 2>&1
    rc=$?
    tests_pass="$(sed -n 's/^# pass \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    tests_fail="$(sed -n 's/^# fail \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    tail -8 "$OUT"
    rm -f "$OUT"

    # Machine-readable value assertions straight from the helper module.
    VALUES="$(node --input-type=module -e "
import { formatDepositFull, formatDepositReduced, formatOverdueDailyFee } from './$HELPERS'
console.log('FULL_NORMAL=' + formatDepositFull({ deposit: 50000 }))
console.log('FULL_NULL=' + formatDepositFull({ deposit: null }))
console.log('ZERO=' + formatDepositFull({ deposit: 0 }))
console.log('REDUCED_NULL=' + formatDepositReduced({ deposit_reduced: null }))
console.log('REDUCED_ZERO=' + formatDepositReduced({ deposit_reduced: 0 }))
console.log('OVERDUE_NORMAL=' + formatOverdueDailyFee({ overdue_daily_fee: 3600 }))
console.log('OVERDUE_NULL=' + formatOverdueDailyFee({ overdue_daily_fee: null }))
")"
    printf '%s\n' "$VALUES"
    echo "TESTS_PASS=${tests_pass:-0}"
    echo "TESTS_FAIL=${tests_fail:-1}"

    [ "$rc" -eq 0 ] || fail "node --test failed (rc=${rc})"
    [ "${tests_fail:-1}" = "0" ] || fail "node --test reported ${tests_fail} failure(s)"
    [ "${tests_pass:-0}" -ge 9 ] || fail "expected >=9 passing tests, got ${tests_pass:-0}"
    printf '%s\n' "$VALUES" | grep -qx 'FULL_NULL=-' || fail "全额押金 null must render -"
    printf '%s\n' "$VALUES" | grep -qx 'REDUCED_NULL=未配置' || fail "差额押金 null must render 未配置"
    printf '%s\n' "$VALUES" | grep -qx 'ZERO=¥0.00' || fail "explicit zero deposit must stay visible"
    printf '%s\n' "$VALUES" | grep -qx 'OVERDUE_NULL=-' || fail "overdue null must render -"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 two-page column sets =="
    [ -f "$STOCK_PAGE" ] || fail "missing $STOCK_PAGE"
    [ -f "$LIST_PAGE" ] || fail "missing $LIST_PAGE"

    STOCK_COLS="$(columns_block "$STOCK_PAGE" 'const columns = [')"
    LIST_COLS="$(columns_block "$LIST_PAGE" 'const baseColumns = [')"

    LIST_HAS_RATE_COL="$(printf '%s\n' "$LIST_COLS" | grep -cE "title: '标准日租金'|dataIndex: 'resolved_daily_rate'" || true)"
    LIST_HAS_DEPOSIT_COL="$(printf '%s\n' "$LIST_COLS" | grep -cE "title: '押金'|dataIndex: 'deposit'" || true)"
    STOCK_HAS_LEVEL_COL="$(printf '%s\n' "$STOCK_COLS" | grep -cE "title: '级别'|dataIndex: 'level_name'" || true)"
    STOCK_HAS_STATUS_COL="$(printf '%s\n' "$STOCK_COLS" | grep -cE "title: '状态'" || true)"
    STOCK_DEPOSIT_COLS="$(printf '%s\n' "$STOCK_COLS" | grep -cE "title: '全额押金'|title: '差额押金'" || true)"
    STOCK_OVERDUE_COL="$(printf '%s\n' "$STOCK_COLS" | grep -cE "title: '逾期租金'" || true)"

    echo "LIST_HAS_RATE_COL=${LIST_HAS_RATE_COL:-0}"
    echo "LIST_HAS_DEPOSIT_COL=${LIST_HAS_DEPOSIT_COL:-0}"
    echo "STOCK_HAS_LEVEL_COL=${STOCK_HAS_LEVEL_COL:-0}"
    echo "STOCK_HAS_STATUS_COL=${STOCK_HAS_STATUS_COL:-0}"
    echo "STOCK_DEPOSIT_COLS=${STOCK_DEPOSIT_COLS:-0}"
    echo "STOCK_OVERDUE_COL=${STOCK_OVERDUE_COL:-0}"

    [ "${LIST_HAS_RATE_COL:-1}" = "0" ] || fail "instrument list still declares the daily-rate column"
    [ "${LIST_HAS_DEPOSIT_COL:-1}" = "0" ] || fail "instrument list still declares a deposit column"
    [ "${STOCK_HAS_LEVEL_COL:-1}" = "0" ] || fail "stock page still declares the level column"
    [ "${STOCK_HAS_STATUS_COL:-1}" = "0" ] || fail "stock page still declares the status column"
    [ "${STOCK_DEPOSIT_COLS:-0}" = "2" ] || fail "stock page must have 全额押金 + 差额押金 columns"
    [ "${STOCK_OVERDUE_COL:-0}" = "1" ] || fail "stock page must have the 逾期租金 column"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 SN is the detail entry =="
    STOCK_COLS_R4="$(columns_block "$STOCK_PAGE" 'const columns = [')"
    LIST_COLS_R4="$(columns_block "$LIST_PAGE" 'const baseColumns = [')"

    STOCK_SN_LINK=0
    printf '%s\n' "$STOCK_COLS_R4" | awk "/识别码/,/^    },?$/" | grep -q 'navigate(`/site/stock/' && STOCK_SN_LINK=1
    LIST_SN_LINK=0
    printf '%s\n' "$LIST_COLS_R4" | awk "/识别码/,/^    },?$/" | grep -q 'navigate(`/instruments/detail/' && LIST_SN_LINK=1
    SN_LINK_COUNT=$((STOCK_SN_LINK + LIST_SN_LINK))
    DETAIL_BUTTON_COUNT="$(printf '%s\n' "$STOCK_COLS_R4" | grep -c '详情</Button>' || true)"
    VIEW_BUTTON_COUNT="$(printf '%s\n' "$LIST_COLS_R4" | grep -c '查看</Button>' || true)"

    echo "SN_LINK_COUNT=${SN_LINK_COUNT}"
    echo "DETAIL_BUTTON_COUNT=${DETAIL_BUTTON_COUNT:-0}"
    echo "VIEW_BUTTON_COUNT=${VIEW_BUTTON_COUNT:-0}"

    [ "$SN_LINK_COUNT" = "2" ] || fail "both pages must turn 识别码 into the detail link"
    [ "${DETAIL_BUTTON_COUNT:-1}" = "0" ] || fail "stock page still renders a 详情 button"
    [ "${VIEW_BUTTON_COUNT:-1}" = "0" ] || fail "instrument list still renders a 查看 button"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 docs =="
    DOC_OVERDUE="$(grep -c '`overdue_daily_fee`' docs/spec/api/api.md || true)"
    UI_LIST_COLUMNS=0
    grep -q '逾期租金' docs/spec/ui/ui.md && grep -q '全额押金' docs/spec/ui/ui.md && \
      grep -q '差额押金' docs/spec/ui/ui.md && UI_LIST_COLUMNS=1
    UI_SN_ENTRY=0
    grep -q '识别码即详情入口' docs/spec/ui/ui.md && UI_SN_ENTRY=1

    echo "DOC_OVERDUE=${DOC_OVERDUE:-0}"
    echo "UI_LIST_COLUMNS=${UI_LIST_COLUMNS}"
    echo "UI_SN_ENTRY=${UI_SN_ENTRY}"

    [ "${DOC_OVERDUE:-0}" -ge 1 ] || fail "api.md must document overdue_daily_fee"
    [ "$UI_LIST_COLUMNS" = "1" ] || fail "ui.md must document 逾期租金/全额押金/差额押金"
    [ "$UI_SN_ENTRY" = "1" ] || fail "ui.md must document 识别码即详情入口"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 build + targeted tests =="
    BUILD_LOG="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_LOG" 2>&1
    GO_BUILD_RC=$?
    tail -5 "$BUILD_LOG"; rm -f "$BUILD_LOG"

    TEST_LOG="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v \
      -run 'TestInstrumentListOverdue2537_R1|TestInstrumentListPricing2528' ) > "$TEST_LOG" 2>&1
    GO_TEST_RC=$?
    GO_TEST_FAIL="$(grep -c '^--- FAIL' "$TEST_LOG" || true)"
    grep -E '^--- (PASS|FAIL)' "$TEST_LOG" | tail -8 || true
    rm -f "$TEST_LOG"

    LINT_LOG="$(mktemp)"
    ( cd frontend-pc && ./node_modules/.bin/eslint src/utils/instrumentDisplay.js src/pages/InstrumentStock.jsx src/pages/admin/instrument/List.jsx ) > "$LINT_LOG" 2>&1
    ESLINT_RC=$?
    tail -5 "$LINT_LOG"; rm -f "$LINT_LOG"

    PC_LOG="$(mktemp)"
    ( cd frontend-pc && npm run build ) > "$PC_LOG" 2>&1
    PC_BUILD_RC=$?
    tail -3 "$PC_LOG"; rm -f "$PC_LOG"

    echo "GO_BUILD_RC=${GO_BUILD_RC}"
    echo "GO_TEST_RC=${GO_TEST_RC}"
    echo "GO_TEST_FAIL=${GO_TEST_FAIL:-0}"
    echo "ESLINT_RC=${ESLINT_RC}"
    echo "PC_BUILD_RC=${PC_BUILD_RC}"

    [ "$GO_BUILD_RC" = "0" ] && echo "GO_BUILD_OK=1" || fail "go build ./... failed (rc=${GO_BUILD_RC})"
    [ "$GO_TEST_RC" = "0" ] && [ "${GO_TEST_FAIL:-1}" = "0" ] && echo "GO_TEST_OK=1" || fail "backend targeted tests failed (rc=${GO_TEST_RC}, fails=${GO_TEST_FAIL:-?})"
    [ "$ESLINT_RC" = "0" ] && echo "ESLINT_OK=1" || fail "eslint failed (rc=${ESLINT_RC})"
    [ "$PC_BUILD_RC" = "0" ] && echo "PC_BUILD_OK=1" || fail "npm run build failed (rc=${PC_BUILD_RC})"
    echo "R6_OK"
    ;;
esac
