#!/usr/bin/env bash
# #2528 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2528.sh r1    # list price fields + null boundaries (backend)
#   bash scripts/verify-2528.sh r2    # list ↔ pricing-v2 parity (backend)
#   bash scripts/verify-2528.sh r3    # PC display helper (node --test) + page wiring (static)
#   bash scripts/verify-2528.sh r4    # build / targeted tests / eslint / PC build / plan source
#   bash scripts/verify-2528.sh gate  # full-gate entry for pre-ready --full-gate-cmd
#
# Capture a row into its artifact:
#   bash scripts/verify-2528.sh r1 > docs/evidence/2528/R1-list-fields.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|gate) ;;
  *) echo "usage: $0 r1|r2|r3|r4|gate" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

# Go may not be on PATH for non-login shells (heavy-run/gate call us directly).
if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

HEAD_SHA="$(git rev-parse HEAD)"
SHORT_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${HEAD_SHA} # dirty=${DIRTY}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
PAGE_STOCK="frontend-pc/src/pages/InstrumentStock.jsx"
PAGE_LIST="frontend-pc/src/pages/admin/instrument/List.jsx"
SHARED_UTIL="frontend-pc/src/utils/instrumentDisplay.js"
TEST_FILE="frontend-pc/tests/instrumentDisplay.test.mjs"
API_DOC="docs/spec/api/api.md"
UI_DOC="docs/spec/ui/ui.md"
EVIDENCE_DIR="docs/evidence/2528"

ensure_deps() {
  # The gate executes in a clean worktree created by pre-ready-check.sh, where
  # node_modules is absent (never committed). Install from the lockfile when
  # missing so the gate is self-contained; warm worktrees skip this.
  [ -d frontend-pc/node_modules ] && return 0
  echo "== installing frontend-pc dependencies (clean worktree) =="
  ( cd frontend-pc && npm ci --prefer-offline --no-audit --no-fund --loglevel=error )
}

heavy() { # run a heavy command under the host lock (standalone rows only)
  bash "$HEAVY" -- "$@"
}

go_test_row() { # row-name → runs one backend evidence test
  local name="$1"
  local out rc
  out="$(mktemp)"
  ( cd backend && heavy go test ./handlers -run "${name}" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E '^(TOTAL|F[0-9]_[A-Z_]+|PARITY_[A-Z]+)=' "$out" || true
  grep -E '^(--- )?(PASS|FAIL|ok |FAIL\s)' "$out" | tail -6 || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -25 "$out"
    rm -f "$out"
    fail "go test ${name} exit=${rc}"
  fi
  rm -f "$out"
}

assert_eq() { # key expected actual
  if [ "$3" != "$2" ]; then
    fail "$1 expected '$2' got '$3'"
  fi
}

case "$ROW" in
  r1)
    echo "== R1 instrument list price fields (backend) =="
    go_test_row TestInstrumentListPricing2528_R1
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 instrument list ↔ pricing-v2 parity (backend) =="
    go_test_row TestInstrumentListPricing2528_R2
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 PC display helper ($SHARED_UTIL) + page wiring =="
    [ -f "$SHARED_UTIL" ] || fail "helper not found: $SHARED_UTIL"
    [ -f "$TEST_FILE" ] || fail "test file not found: $TEST_FILE"

    OUT="$(mktemp)"
    ( cd frontend-pc && node --test tests/instrumentDisplay.test.mjs ) > "$OUT" 2>&1
    NODE_RC=$?
    grep -E '^(ok|not ok) ' "$OUT" || true
    PASS_N="$(sed -n 's/^# pass \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    FAIL_N="$(sed -n 's/^# fail \([0-9][0-9]*\)$/\1/p' "$OUT" | tail -1)"
    rm -f "$OUT"

    echo "NODE_TEST_RC=${NODE_RC}"
    echo "TESTS_PASS=${PASS_N:-0}"
    echo "TESTS_FAIL=${FAIL_N:-missing}"

    count() { grep -c -- "$1" "$2" 2>/dev/null || true; }
    count_re() { grep -cE -- "$1" "$2" 2>/dev/null || true; }

    # Page wiring: helper imported and used by both list pages.
    wire() { # file
      local f="$1"
      [ -f "$f" ] || { echo 0; return; }
      local a b c d
      a="$(count 'instrumentImageSrc' "$f")"
      b="$(count 'formatDailyRate' "$f")"
      c="$(count 'formatDeposit' "$f")"
      d="$(count 'INSTRUMENT_IMAGE_PLACEHOLDER' "$f")"
      if [ "$a" -ge 1 ] && [ "$b" -ge 1 ] && [ "$c" -ge 1 ] && [ "$d" -ge 1 ]; then echo 1; else echo 0; fi
    }
    STOCK_WIRED="$(wire "$PAGE_STOCK")"
    LIST_WIRED="$(wire "$PAGE_LIST")"

    # Legacy defects must be gone: 404 placeholder asset + raw cents rendering.
    # (Input Truth: `grep -rn "default-instrument.jpg" frontend-pc/src` must be 0.)
    LEGACY_ASSET_REF="$(grep -rn 'default-instrument\.jpg' frontend-pc/src 2>/dev/null | wc -l || true)"
    LEGACY_RAW_CENTS=0
    for f in "$PAGE_STOCK" "$PAGE_LIST"; do
      n="$(count 'value.toLocaleString()' "$f")"
      LEGACY_RAW_CENTS=$((LEGACY_RAW_CENTS + ${n:-0}))
    done

    # Both pages gate price columns with the same management-level proxy.
    MANAGER_GATE_BOTH=0
    if [ "$(count "hasCusPerm('instrument:delete')" "$PAGE_STOCK")" -ge 1 ] && \
       [ "$(count "hasCusPerm('instrument:delete')" "$PAGE_LIST")" -ge 1 ]; then
      MANAGER_GATE_BOTH=1
    fi

    # Contract docs must state the new column names / units (#2528 Core Update).
    API_DOC_FIELDS="$(count 'resolved_daily_rate' "$API_DOC")"
    UI_DOC_COLUMN="$(count '标准日租金' "$UI_DOC")"

    echo "STOCK_WIRED=${STOCK_WIRED}"
    echo "LIST_WIRED=${LIST_WIRED}"
    echo "LEGACY_ASSET_REF=${LEGACY_ASSET_REF}"
    echo "LEGACY_RAW_CENTS=${LEGACY_RAW_CENTS}"
    echo "MANAGER_GATE_BOTH=${MANAGER_GATE_BOTH}"
    echo "API_DOC_FIELDS=${API_DOC_FIELDS}"
    echo "UI_DOC_COLUMN=${UI_DOC_COLUMN}"

    assert_eq NODE_TEST_RC 0 "$NODE_RC"
    assert_eq TESTS_FAIL 0 "${FAIL_N:-missing}"
    assert_eq TESTS_PASS 12 "${PASS_N:-0}"
    assert_eq STOCK_WIRED 1 "$STOCK_WIRED"
    assert_eq LIST_WIRED 1 "$LIST_WIRED"
    assert_eq LEGACY_ASSET_REF 0 "$LEGACY_ASSET_REF"
    assert_eq LEGACY_RAW_CENTS 0 "$LEGACY_RAW_CENTS"
    assert_eq MANAGER_GATE_BOTH 1 "$MANAGER_GATE_BOTH"
    if [ "${API_DOC_FIELDS:-0}" -lt 1 ]; then fail "api.md does not document resolved_daily_rate"; fi
    if [ "${UI_DOC_COLUMN:-0}" -lt 1 ]; then fail "ui.md does not document 标准日租金 column"; fi
    echo "R3_OK"
    ;;

  r4|gate)
    WRAPPED=1
    [ "$ROW" = "gate" ] && WRAPPED=0   # gate runs under pre-ready's lock already
    run_heavy() {
      if [ "$WRAPPED" = "1" ]; then heavy "$@"; else "$@"; fi
    }

    echo "== R4 build / targeted tests / eslint / PC build / plan source =="

    TMP_BUILD="$(mktemp)"
    ( cd backend && run_heavy go build ./... ) > "$TMP_BUILD" 2>&1
    BUILD_RC=$?
    tail -5 "$TMP_BUILD"
    rm -f "$TMP_BUILD"

    TMP_TEST="$(mktemp)"
    ( cd backend && run_heavy go test ./handlers -count=1 -v \
      -run 'TestInstrumentListPricing2528|TestGetInstruments_|TestRentSetting_|TestPricingResponseDeposit_' \
      ) > "$TMP_TEST" 2>&1
    TARGETED_RC=$?
    TEST_FAIL_COUNT="$(grep -c '^--- FAIL' "$TMP_TEST" || true)"
    PASS_LINES="$(grep -cE '^--- PASS' "$TMP_TEST" || true)"
    grep -E '^--- (PASS|FAIL)' "$TMP_TEST" | tail -8 || true
    tail -3 "$TMP_TEST"
    rm -f "$TMP_TEST"
    if [ "$TARGETED_RC" -ne 0 ] && [ "${TEST_FAIL_COUNT:-0}" = "0" ]; then
      TEST_FAIL_COUNT=1   # build/panic in the test binary must also count as failure
    fi

    ensure_deps || fail "could not install frontend-pc dependencies"

    TMP_LINT="$(mktemp)"
    ( cd frontend-pc && npx eslint src/pages/InstrumentStock.jsx src/pages/admin/instrument/List.jsx \
        src/utils/instrumentDisplay.js tests/instrumentDisplay.test.mjs ) \
      > "$TMP_LINT" 2>&1
    ESLINT_RC=$?
    ESLINT_NO_UNDEF="$(grep -c 'no-undef' "$TMP_LINT" || true)"
    tail -3 "$TMP_LINT"
    rm -f "$TMP_LINT"

    TMP_PCBUILD="$(mktemp)"
    ( cd frontend-pc && npm run build ) > "$TMP_PCBUILD" 2>&1
    PC_BUILD_RC=$?
    tail -6 "$TMP_PCBUILD"
    rm -f "$TMP_PCBUILD"

    PLAN_OUT="$(bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue 2528 --fresh 2>&1)"
    PLAN_RC=$?
    printf '%s\n' "$PLAN_OUT" | head -3
    if [ "$PLAN_RC" -eq 0 ]; then PLAN_SOURCE=PASS; else PLAN_SOURCE=FAIL; fi

    echo "BUILD_RC=${BUILD_RC}"
    echo "TEST_TARGETED_RC=${TARGETED_RC}"
    echo "TEST_FAIL_COUNT=${TEST_FAIL_COUNT}"
    echo "TEST_PASS_COUNT=${PASS_LINES}"
    echo "ESLINT_RC=${ESLINT_RC}"
    echo "ESLINT_NO_UNDEF=${ESLINT_NO_UNDEF}"
    echo "PC_BUILD_RC=${PC_BUILD_RC}"
    echo "PLAN_SOURCE=${PLAN_SOURCE}"

    if [ "$BUILD_RC" -eq 0 ] && [ "${TEST_FAIL_COUNT:-1}" -eq 0 ] && [ "$ESLINT_RC" -eq 0 ] && \
       [ "${ESLINT_NO_UNDEF:-1}" -eq 0 ] && [ "$PC_BUILD_RC" -eq 0 ]; then
      echo "GATE_RESULT=PASS"
      GATE_OK=1
    else
      echo "GATE_RESULT=FAIL"
      GATE_OK=0
    fi

    [ "$GATE_OK" = "1" ] || fail "one or more R4 checks failed"
    [ "$PLAN_SOURCE" = "PASS" ] || fail "plan source mismatch"
    echo "R4_OK"
    ;;
esac
