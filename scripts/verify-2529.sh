#!/usr/bin/env bash
# #2529 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2529.sh r1    # GET /instruments/:id field contract + 40400 (backend)
#   bash scripts/verify-2529.sh r2    # PC detail state/image helpers + page wiring (static)
#   bash scripts/verify-2529.sh r3    # build / targeted regression / eslint / PC build / plan source
#
# Capture a row into its artifact:
#   bash scripts/verify-2529.sh r1 > docs/evidence/2529/R1-detail-contract.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3) ;;
  *) echo "usage: $0 r1|r2|r3" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

# Go may not be on PATH for non-login shells (heavy-run/gate call us directly).
if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Provenance is anchored to this Issue's fix commit (same convention as #2455):
# regenerating evidence from a later evidence/harness commit must not shift the
# recorded build/commit, otherwise artifact and evidence.json (P12/P14) diverge.
FIX_SHA="$(git log --format=%H --grep='^fix(issue-2529)' -1 2>/dev/null || true)"
[ -n "$FIX_SHA" ] || FIX_SHA="$(git rev-parse HEAD)"
HEAD_SHA="$(git rev-parse --verify "${FIX_SHA}^{commit}")"
SHORT_SHA="$(git rev-parse --short "$HEAD_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${HEAD_SHA} # dirty=${DIRTY}"

fail() { echo "R${ROW#r}_FAIL: $*" >&2; exit 1; }

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
PAGE_DETAIL="frontend-pc/src/pages/AssetDetail.jsx"
PAGE_LIST_USER="frontend-pc/src/pages/InstrumentListUser.jsx"
ID_PHOTO="frontend-pc/src/components/IdPhotoDisplay.jsx"
SHARED_UTIL="frontend-pc/src/utils/assetDetail.js"
TEST_FILE="frontend-pc/tests/assetDetail.test.mjs"

heavy() { # run a heavy command under the host lock (standalone rows only)
  bash "$HEAVY" -- "$@"
}

go_test_row() { # row-name → runs one backend evidence test
  local name="$1"
  local out rc
  out="$(mktemp)"
  ( cd backend && heavy go test ./handlers -run "${name}" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E '^(STATUS|CODE|HAS_[A-Z_]+|NOT_FOUND_CODE)=' "$out" || true
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
    echo "== R1 detail field contract + unknown id (backend) =="
    go_test_row TestAssetDetail2529_R1
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 detail state/hero helpers ($SHARED_UTIL) + page wiring =="
    [ -f "$SHARED_UTIL" ] || fail "helper not found: $SHARED_UTIL"
    [ -f "$TEST_FILE" ] || fail "test file not found: $TEST_FILE"

    OUT="$(mktemp)"
    ( cd frontend-pc && node --test tests/assetDetail.test.mjs ) > "$OUT" 2>&1
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
    # Tree counter: `grep -c` has no -r and on a directory prints 0 / exit 2, so a
    # directory-wide assertion built on it can never fail (audit round 1, Bug 1).
    # Count real matching lines recursively instead.
    count_tree() { grep -rnE -- "$1" "$2" 2>/dev/null | wc -l | tr -d ' '; }

    # Detail page must use the single-record endpoint and stop scanning list page 1.
    DETAIL_USES_GET=0
    if [ "$(count 'instrumentsApi.get(' "$PAGE_DETAIL")" -ge 1 ] && \
       [ "$(count 'assetDetailState' "$PAGE_DETAIL")" -ge 1 ] && \
       [ "$(count 'assetHeroImage' "$PAGE_DETAIL")" -ge 1 ] && \
       [ "$(count 'formatDailyRate' "$PAGE_DETAIL")" -ge 1 ]; then
      DETAIL_USES_GET=1
    fi
    LIST_SCAN=0
    LIST_SCAN=$((LIST_SCAN + $(count 'inventoryApi.list()' "$PAGE_DETAIL")))
    LIST_SCAN=$((LIST_SCAN + $(count_re '\.find\(a => a\.id === id\)' "$PAGE_DETAIL")))

    # Legacy placeholder paths (never committed to public/) must be gone repo-wide.
    PLACEHOLDER_PATTERN='placeholder-instrument\.png|"/placeholder\.png"'
    LEGACY_PLACEHOLDER_REF="$(count_tree "$PLACEHOLDER_PATTERN" frontend-pc/src)"
    LEGACY_PLACEHOLDER_REF="${LEGACY_PLACEHOLDER_REF:-missing}"

    # Red side of the very same probe: the fix commit's parent must still carry
    # both legacy paths (2 hits). If the counter above could never be non-zero,
    # this assertion fails, so the green value cannot be trivially true.
    LEGACY_PLACEHOLDER_REF_PRE_FIX=missing
    PRE_FIX_COMMIT="$(git rev-parse --verify "${HEAD_SHA}^" 2>/dev/null || true)"
    if [ -n "$PRE_FIX_COMMIT" ]; then
      PRE_FIX_DIR="$(mktemp -d)"
      if git archive "$PRE_FIX_COMMIT" frontend-pc/src | tar -x -C "$PRE_FIX_DIR" 2>/dev/null; then
        echo "LEGACY_PLACEHOLDER_REF_PRE_FIX_TREE=$(git rev-parse --short "$PRE_FIX_COMMIT")"
        grep -rnE -- "$PLACEHOLDER_PATTERN" "$PRE_FIX_DIR/frontend-pc/src" 2>/dev/null \
          | sed "s#^$PRE_FIX_DIR/##" || true
        LEGACY_PLACEHOLDER_REF_PRE_FIX="$(count_tree "$PLACEHOLDER_PATTERN" "$PRE_FIX_DIR/frontend-pc/src")"
        LEGACY_PLACEHOLDER_REF_PRE_FIX="${LEGACY_PLACEHOLDER_REF_PRE_FIX:-missing}"
      fi
      rm -rf "$PRE_FIX_DIR"
    fi

    # Both neighbours must fall back to the shared placeholder constant.
    FALLBACK_WIRED=0
    [ "$(count 'IMAGE_PLACEHOLDER' "$PAGE_LIST_USER")" -ge 2 ] && FALLBACK_WIRED=$((FALLBACK_WIRED + 1))
    [ "$(count 'IMAGE_PLACEHOLDER' "$ID_PHOTO")" -ge 2 ] && FALLBACK_WIRED=$((FALLBACK_WIRED + 1))

    echo "ASSET_DETAIL_USES_GET=${DETAIL_USES_GET}"
    echo "LIST_STILL_USES_LIST_SCAN=${LIST_SCAN}"
    echo "LEGACY_PLACEHOLDER_REF=${LEGACY_PLACEHOLDER_REF}"
    echo "LEGACY_PLACEHOLDER_REF_PRE_FIX=${LEGACY_PLACEHOLDER_REF_PRE_FIX}"
    echo "FALLBACK_WIRED=${FALLBACK_WIRED}"

    assert_eq NODE_TEST_RC 0 "$NODE_RC"
    assert_eq TESTS_FAIL 0 "${FAIL_N:-missing}"
    assert_eq ASSET_DETAIL_USES_GET 1 "$DETAIL_USES_GET"
    assert_eq LIST_STILL_USES_LIST_SCAN 0 "${LIST_SCAN:-1}"
    assert_eq LEGACY_PLACEHOLDER_REF 0 "${LEGACY_PLACEHOLDER_REF:-1}"
    assert_eq LEGACY_PLACEHOLDER_REF_PRE_FIX 2 "${LEGACY_PLACEHOLDER_REF_PRE_FIX:-missing}"
    assert_eq FALLBACK_WIRED 2 "${FALLBACK_WIRED:-0}"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 build / targeted regression / eslint / PC build / plan source =="

    TMP_BUILD="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$TMP_BUILD" 2>&1
    BUILD_RC=$?
    tail -5 "$TMP_BUILD"
    rm -f "$TMP_BUILD"

    TMP_TEST="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v \
      -run 'TestAssetDetail2529|TestGetInstrument_|TestInstrumentDetail_|TestGetInstruments_' \
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

    if [ ! -d frontend-pc/node_modules ]; then
      echo "== installing frontend-pc dependencies (clean worktree) =="
      ( cd frontend-pc && npm ci --prefer-offline --no-audit --no-fund --loglevel=error )
    fi

    TMP_LINT="$(mktemp)"
    ( cd frontend-pc && npx eslint src/pages/AssetDetail.jsx src/pages/InstrumentListUser.jsx \
        src/components/IdPhotoDisplay.jsx src/utils/assetDetail.js src/utils/instrumentDisplay.js \
        tests/assetDetail.test.mjs ) > "$TMP_LINT" 2>&1
    ESLINT_RC=$?
    ESLINT_NO_UNDEF="$(grep -c 'no-undef' "$TMP_LINT" || true)"
    tail -3 "$TMP_LINT"
    rm -f "$TMP_LINT"

    TMP_PCBUILD="$(mktemp)"
    ( cd frontend-pc && npm run build ) > "$TMP_PCBUILD" 2>&1
    PC_BUILD_RC=$?
    tail -6 "$TMP_PCBUILD"
    rm -f "$TMP_PCBUILD"

    PLAN_OUT="$(bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue 2529 --fresh 2>&1)"
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

    [ "$GATE_OK" = "1" ] || fail "one or more R3 checks failed"
    [ "$PLAN_SOURCE" = "PASS" ] || fail "plan source mismatch"
    echo "R3_OK"
    ;;
esac
