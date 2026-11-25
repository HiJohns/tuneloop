#!/usr/bin/env bash
# #2514 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2514.sh r1    # shared batch-pricing builder (node --test + executable probes)
#   bash scripts/verify-2514.sh r2    # inventory/stock page wiring (static structure)
#   bash scripts/verify-2514.sh r3    # instrument list page wiring (static structure)
#   bash scripts/verify-2514.sh r4    # /instruments write-call scan (frontend vs backend routes)
#   bash scripts/verify-2514.sh r5    # api.md §5.6.2 contract
#   bash scripts/verify-2514.sh r6    # frontend build + eslint no-undef
#   bash scripts/verify-2514.sh gate  # full-gate entry (build + lint), used by pre-ready --full-gate-cmd
#
# Capture a row into its artifact:
#   bash scripts/verify-2514.sh r1 > docs/evidence/2514/R1-batch-pricing-fn.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|gate) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|gate" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

HEAD_SHA="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${HEAD_SHA} # dirty=${DIRTY}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }

PAGE_STOCK="frontend-pc/src/pages/InstrumentStock.jsx"
PAGE_LIST="frontend-pc/src/pages/admin/instrument/List.jsx"
SHARED_UTIL="frontend-pc/src/utils/batchPricing.js"
TEST_FILE="frontend-pc/tests/batchPricing.test.mjs"
API_JS="frontend-pc/src/services/api.js"
API_DOC="docs/spec/api/api.md"

run_build() {
  ( cd frontend-pc && bash "$HOME/.config/opencode/scripts/heavy-run.sh" -- npm run build )
}

run_lint() { # logfile
  ( cd frontend-pc && npx eslint src/pages/InstrumentStock.jsx src/pages/admin/instrument/List.jsx \
      src/utils/batchPricing.js tests/batchPricing.test.mjs ) > "$1" 2>&1
}

case "$ROW" in
  r1)
    echo "== R1 shared batch-pricing builder ($SHARED_UTIL) =="
    [ -f "$SHARED_UTIL" ] || fail "shared builder not found: $SHARED_UTIL"
    [ -f "$TEST_FILE" ] || fail "test file not found: $TEST_FILE"

    OUT="$(mktemp)"
    ( cd frontend-pc && node --test tests/batchPricing.test.mjs ) > "$OUT" 2>&1
    RC=$?
    cat "$OUT"
    PASS_N="$(sed -n 's/^# pass \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    FAIL_N="$(sed -n 's/^# fail \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    rm -f "$OUT"
    echo "RESULT: PASS ${PASS_N:-0} FAIL ${FAIL_N:-0}"
    [ "$RC" -eq 0 ] || fail "node --test exit=$RC"
    [ "${FAIL_N:-1}" = "0" ] || fail "node --test reported failures (fail=${FAIL_N:-missing})"
    [ "${PASS_N:-0}" -ge 1 ] || fail "node --test ran no passing test"

    # Executable probes against the real module (not grep): payload shape and rejections.
    PROBE="$( cd frontend-pc && node --input-type=module -e '
      import { buildBatchPricingPayload, batchPricingFeedback } from "./src/utils/batchPricing.js";
      const ok = buildBatchPricingPayload(["i1", "i2"], { price_type: "daily_rate", amount: 100 });
      const items = ok && ok.payload && Array.isArray(ok.payload.items) ? ok.payload.items : [];
      console.log("PAYLOAD_ITEMS=" + items.length);
      console.log("ITEM_KEYS=" + (items[0] ? Object.keys(items[0]).join(",") : "none"));
      console.log("AMOUNT_VALUE=" + (items[0] ? items[0].base_daily_rate : "none"));
      console.log("UNSUPPORTED_REJECTED=" + (buildBatchPricingPayload(["i1"], { price_type: "weekly_rate", amount: 10 }).error ? 1 : 0));
      console.log("ZERO_REJECTED=" + (buildBatchPricingPayload(["i1"], { price_type: "daily_rate", amount: 0 }).error ? 1 : 0));
      console.log("NAN_REJECTED=" + (buildBatchPricingPayload(["i1"], { price_type: "daily_rate", amount: NaN }).error ? 1 : 0));
      console.log("EMPTY_IDS_REJECTED=" + (buildBatchPricingPayload([], { price_type: "daily_rate", amount: 1 }).error ? 1 : 0));
      console.log("FEEDBACK_OK=" + (batchPricingFeedback({ data: { updated: 2 } }, 2).level === "success" ? 1 : 0));
      console.log("FEEDBACK_WARN=" + (batchPricingFeedback({ data: { updated: 1 } }, 2).level === "warning" ? 1 : 0));
      console.log("FEEDBACK_NULL_WARN=" + (batchPricingFeedback(null, 2).level === "warning" ? 1 : 0));
    ' 2>&1 )"
    [ "$?" -eq 0 ] || fail "probe failed: $PROBE"
    PROBE_TAIL="$(printf '%s\n' "$PROBE" | grep -v '^{' | grep -E '^[A-Z_]+=')"
    printf '%s\n' "$PROBE_TAIL"

    get() { printf '%s\n' "$PROBE_TAIL" | sed -n "s/^$1=//p" | tail -1; }
    [ "$(get PAYLOAD_ITEMS)" = "2" ] || fail "expected 2 payload items, got $(get PAYLOAD_ITEMS)"
    [ "$(get ITEM_KEYS)" = "id,base_daily_rate" ] || fail "unexpected item keys: $(get ITEM_KEYS)"
    [ "$(get AMOUNT_VALUE)" = "100" ] || fail "amount must be 元 (got $(get AMOUNT_VALUE))"
    [ "$(get UNSUPPORTED_REJECTED)" = "1" ] || fail "weekly_rate must be rejected before the request"
    [ "$(get ZERO_REJECTED)" = "1" ] || fail "amount 0 must be rejected (backend silently skips <=0)"
    [ "$(get NAN_REJECTED)" = "1" ] || fail "NaN amount must be rejected"
    [ "$(get EMPTY_IDS_REJECTED)" = "1" ] || fail "empty id list must be rejected"
    [ "$(get FEEDBACK_OK)" = "1" ] || fail "updated == requested must be success"
    [ "$(get FEEDBACK_WARN)" = "1" ] || fail "updated < requested must be warning"
    [ "$(get FEEDBACK_NULL_WARN)" = "1" ] || fail "missing data.updated must be warning"
    echo "R1_OK"
    ;;

  r2|r3)
    if [ "$ROW" = "r2" ]; then
      PAGE="$PAGE_STOCK"
      LABEL="R2 stock/inventory page"
    else
      PAGE="$PAGE_LIST"
      LABEL="R3 instrument list page"
    fi
    TAG="R${ROW#r}"
    echo "== ${LABEL} static structure ($PAGE) =="
    [ -f "$PAGE" ] || fail "page not found: $PAGE"

    count() { grep -c -- "$1" "$PAGE" 2>/dev/null || true; }
    count_re() { grep -cE -- "$1" "$PAGE" 2>/dev/null || true; }
    DEAD_PATH="$(count_re "api\.(put|post|delete)\(['\"\`]/instruments/batch/price")"
    SHARED_BUILDER="$(count "buildBatchPricingPayload")"
    USES_SHARED_API="$(count "pricingApi.batchSetPricing")"
    OPTIONS_TYPE="$(count "BATCH_PRICE_TYPE_OPTIONS")"
    OPTIONS_OPERATOR="$(count "BATCH_PRICE_OPERATOR_OPTIONS")"
    PARTIAL_WARN="$(count "batchPricingFeedback")"
    FEEDBACK_LEVEL="$(count "message\[feedback.level\]")"
    LEGACY_TYPES="$(count 'value="weekly_rate"\|value="monthly_rate"\|value="deposit"')"
    LEGACY_OPS="$(count 'value="increase"\|value="decrease"\|value="multiply"')"
    HINT="$(count '当前仅支持按日租金批量设置为固定金额')"

    [ "$SHARED_BUILDER" -ge 1 ] && SHARED_BUILDER=1
    [ "$USES_SHARED_API" -ge 1 ] && USES_SHARED_API=1
    if [ "$OPTIONS_TYPE" -ge 1 ] && [ "$OPTIONS_OPERATOR" -ge 1 ]; then OPTIONS_FROM_SHARED=1; else OPTIONS_FROM_SHARED=0; fi
    if [ "$PARTIAL_WARN" -ge 1 ] && [ "$FEEDBACK_LEVEL" -ge 1 ]; then PARTIAL_WARN=1; else PARTIAL_WARN=0; fi
    [ "$HINT" -ge 1 ] && HINT=1

    echo "DEAD_PATH=$DEAD_PATH"
    echo "SHARED_BUILDER=$SHARED_BUILDER"
    echo "USES_SHARED_API=$USES_SHARED_API"
    echo "OPTIONS_FROM_SHARED=$OPTIONS_FROM_SHARED"
    echo "PARTIAL_WARN=$PARTIAL_WARN"
    echo "LEGACY_PRICE_TYPES=$LEGACY_TYPES"
    echo "LEGACY_OPERATORS=$LEGACY_OPS"
    echo "SCOPE_HINT=$HINT"

    [ "$DEAD_PATH" = "0" ] || fail "dead path /instruments/batch/price still present"
    [ "$SHARED_BUILDER" = "1" ] || fail "page does not use the shared builder"
    [ "$USES_SHARED_API" = "1" ] || fail "page does not call pricingApi.batchSetPricing"
    [ "$OPTIONS_FROM_SHARED" = "1" ] || fail "select options are not sourced from the shared module"
    [ "$PARTIAL_WARN" = "1" ] || fail "partial-update warning path missing"
    [ "$LEGACY_TYPES" = "0" ] || fail "unsupported price types still exposed in UI"
    [ "$LEGACY_OPS" = "0" ] || fail "unsupported operators still exposed in UI"
    [ "$HINT" = "1" ] || fail "scope hint line missing from the modal"
    echo "${TAG}_OK"
    ;;

  r4)
    echo "== R4 /instruments batch write-call scan (frontend pages vs backend routes) =="
    ALL="$(grep -rnE "api\.(put|post|delete)\((['\"\`])/instruments" frontend-pc/src --include='*.js' --include='*.jsx' || true)"
    BATCH="$(printf '%s\n' "$ALL" | grep -E "/instruments/batch" || true)"

    BATCH_PRICE_CALLS="$(printf '%s\n' "$BATCH" | grep -c "/instruments/batch/price" || true)"
    BATCH_PRICING_CALLS="$(printf '%s\n' "$BATCH" | grep -c "/instruments/batch-pricing" || true)"
    BATCH_STATUS_CALLS="$(printf '%s\n' "$BATCH" | grep -c "/instruments/batch/status" || true)"
    BATCH_DELETE_CALLS="$(printf '%s\n' "$BATCH" | grep -cE "/instruments/batch['\"\`]" || true)"
    RESIDUAL="$(printf '%s\n' "$BATCH" \
      | grep -v "/instruments/batch/price" \
      | grep -v "/instruments/batch-pricing" \
      | grep -v "/instruments/batch-import" \
      | grep -v "/instruments/batch/status" \
      | grep -vE "/instruments/batch['\"\`]" || true)"
    RESIDUAL_COUNT=0
    [ -n "$RESIDUAL" ] && RESIDUAL_COUNT="$(printf '%s\n' "$RESIDUAL" | grep -c . || true)"

    BACKEND_ROUTE="$(grep -c '"/instruments/batch-pricing"' backend/main.go || true)"
    BACKEND_IMPORT_ROUTE="$(grep -c '"/instruments/batch-import"' backend/main.go || true)"
    BACKEND_STATUS_ROUTE="$(grep -c '"/instruments/batch/status"' backend/main.go || true)"
    BACKEND_GHOST="$(grep -c '/instruments/batch/price"' backend/main.go || true)"

    echo "BATCH_PRICE_CALLS=$BATCH_PRICE_CALLS"
    echo "BATCH_PRICING_CALLS=$BATCH_PRICING_CALLS"
    echo "BATCH_STATUS_CALLS=$BATCH_STATUS_CALLS"
    echo "BATCH_DELETE_CALLS=$BATCH_DELETE_CALLS"
    echo "BACKEND_BATCH_PRICING_ROUTE=$BACKEND_ROUTE"
    echo "BACKEND_BATCH_IMPORT_ROUTE=$BACKEND_IMPORT_ROUTE"
    echo "BACKEND_BATCH_STATUS_ROUTE=$BACKEND_STATUS_ROUTE"
    echo "BACKEND_GHOST_ROUTE=$BACKEND_GHOST"
    echo "RESIDUAL_OUTSIDE_ALLOWLIST=$RESIDUAL_COUNT"
    echo "-- batch call set (allowlist: batch-pricing / batch-import / batch/status -> #2519 / DELETE batch) --"
    printf '%s\n' "$BATCH" | grep . || true
    echo "-- other /instruments write calls (path params, existing endpoints; informational) --"
    printf '%s\n' "$ALL" | grep -v "/instruments/batch" | grep -c . | sed 's/^/OTHER_WRITE_CALLS=/'
    if [ -n "$RESIDUAL" ]; then
      echo "-- residual (outside allowlist) --"
      printf '%s\n' "$RESIDUAL"
    fi

    [ "$BATCH_PRICE_CALLS" = "0" ] || fail "/instruments/batch/price call still present (404)"
    [ "${BATCH_PRICING_CALLS:-0}" -ge 1 ] || fail "no caller of /instruments/batch-pricing found"
    [ "$BACKEND_ROUTE" -ge 1 ] || fail "backend route /instruments/batch-pricing not registered"
    [ "$BACKEND_IMPORT_ROUTE" -ge 1 ] || fail "backend route /instruments/batch-import not registered"
    [ "$BACKEND_STATUS_ROUTE" = "0" ] || echo "NOTE: backend now registers /instruments/batch/status -> revisit #2519"
    [ "$BACKEND_GHOST" = "0" ] || fail "backend unexpectedly registers the ghost route"
    [ "$RESIDUAL_COUNT" = "0" ] || fail "undeclared /instruments/batch write call(s) found"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 api.md §5.6.2 batch-pricing contract ($API_DOC) =="
    [ -f "$API_DOC" ] || fail "doc not found: $API_DOC"
    SECTION="$(grep -c '^### 5\.6\.2 批量设价$' "$API_DOC" || true)"
    ITEMS="$(grep -c '`items`: \*\*必填\*\*' "$API_DOC" || true)"
    YUAN="$(grep -c 'base_daily_rate` 单位为\*\*元\*\*' "$API_DOC" || true)"
    BOUNDARY="$(grep -c '已知边界（归 #2519）' "$API_DOC" || true)"
    ROUTE="$(grep -c '`PUT /api/instruments/batch-pricing`' "$API_DOC" || true)"
    GHOST="$(grep -c '旧前端调用 `PUT /api/instruments/batch/price`' "$API_DOC" || true)"

    echo "DOC_SECTION=$SECTION"
    echo "DOC_ITEMS_REQUIRED=$ITEMS"
    echo "DOC_UNIT_YUAN=$YUAN"
    echo "DOC_BOUNDARY=$BOUNDARY"
    echo "DOC_ROUTE=$ROUTE"
    echo "DOC_GHOST_NOTE=$GHOST"

    [ "$SECTION" = "1" ] || fail "§5.6.2 section missing"
    [ "$ITEMS" = "1" ] || fail "items required contract missing"
    [ "$YUAN" = "1" ] || fail "元 unit statement missing"
    [ "$BOUNDARY" = "1" ] || fail "#2519 boundary note missing"
    [ "$ROUTE" = "1" ] || fail "PUT /api/instruments/batch-pricing not documented"
    [ "$GHOST" = "1" ] || fail "ghost path note missing"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 frontend build + eslint no-undef =="
    BUILD_LOG="$(mktemp)"
    run_build > "$BUILD_LOG" 2>&1
    BUILD_EXIT=$?
    tail -25 "$BUILD_LOG"
    rm -f "$BUILD_LOG"
    LINT_LOG="$(mktemp)"
    run_lint "$LINT_LOG"
    ESLINT_RC=$?
    cat "$LINT_LOG"
    ESLINT_NO_UNDEF="$(grep -c 'no-undef' "$LINT_LOG" || true)"
    rm -f "$LINT_LOG"
    echo "BUILD_EXIT=$BUILD_EXIT"
    echo "ESLINT_RC=$ESLINT_RC"
    echo "ESLINT_NO_UNDEF=$ESLINT_NO_UNDEF"
    [ "$BUILD_EXIT" = "0" ] || fail "build failed (BUILD_EXIT=$BUILD_EXIT)"
    [ "$ESLINT_NO_UNDEF" = "0" ] || fail "eslint reported no-undef (ESLINT_NO_UNDEF=$ESLINT_NO_UNDEF)"
    [ "$ESLINT_RC" = "0" ] || fail "eslint exit=$ESLINT_RC"
    echo "R6_OK"
    ;;

  gate)
    # Full-gate entry used by pre-ready --full-gate-cmd. It must be runnable in a
    # clean worktree at HEAD, so it installs nothing and only depends on the tree.
    BUILD_LOG="$(mktemp)"
    run_build > "$BUILD_LOG" 2>&1
    BUILD_EXIT=$?
    tail -6 "$BUILD_LOG"
    rm -f "$BUILD_LOG"
    LINT_LOG="$(mktemp)"
    run_lint "$LINT_LOG"
    ESLINT_RC=$?
    ESLINT_NO_UNDEF="$(grep -c 'no-undef' "$LINT_LOG" || true)"
    tail -3 "$LINT_LOG"
    rm -f "$LINT_LOG"
    echo "BUILD_EXIT=$BUILD_EXIT"
    echo "ESLINT_RC=$ESLINT_RC"
    echo "ESLINT_NO_UNDEF=$ESLINT_NO_UNDEF"
    if [ "$BUILD_EXIT" = "0" ] && [ "$ESLINT_RC" = "0" ] && [ "$ESLINT_NO_UNDEF" = "0" ]; then
      echo "GATE_RESULT=PASS"
      exit 0
    fi
    echo "GATE_RESULT=FAIL"
    exit 1
    ;;
esac
