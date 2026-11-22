#!/usr/bin/env bash
# #2444 Rev.1: explicit zero deposit — rent-setting parity + batch/save semantics.
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) NAME="R1-list.txt"; TESTS='^TestRentSettingDepositParity_2444$' ;;
  r2) NAME="R2-no-recompute.txt"; TESTS='^TestBatchUpdateRentOmittedDeposit_2444$' ;;
  r3) NAME="R3-legacy-pin.txt"; TESTS='^TestBatchUpdateRentLegacyRoundTrip_2444$' ;;
  r4) NAME="R4-zero.txt"; TESTS='^TestCreateInstrumentExplicitZeroDeposit_2444$|^TestUpdateInstrumentExplicitZeroDeposit_2444$' ;;
  r5) NAME="R5-build.txt" ;;
  r6-gate-baseline) NAME="R6-gate.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6-gate-baseline>" >&2; exit 2 ;;
esac
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT="${OUT_DIR:-$REPO/docs/evidence/2444}"; mkdir -p "$OUT" "$REPO/tmp/gobuild" "$REPO/tmp/gocache"
ART="$OUT/$NAME"
FIX_REF="${FIX_REF:-HEAD}"
FIX_SHA="$(git rev-parse "$FIX_REF")"; FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
BUILD="1.0.$FIX_SHORT"; COMMIT="$FIX_SHA"
HEAD_SHA="$(git rev-parse --short HEAD)"
DIRTY=false; [ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=$BUILD commit=$COMMIT dirty=$DIRTY tree=$HEAD_SHA"
backend() { cd "$REPO/backend"; export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"; }
case "$MODE" in
  r1|r2|r3|r4)
    { echo "$PROV"; backend; go test ./handlers/ -run "$TESTS" -count=1 -v; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5)
    { echo "$PROV"; cd "$REPO/frontend-pc"; npm run build; rc=$?; echo "BUILD_EXIT=$rc"; [ "$rc" = "0" ] && echo "BUILD_OK"; exit "$rc"; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-gate-baseline)
    {
      echo "$PROV"
      echo "TREE_COMMIT=$(git rev-parse HEAD)"
      backend
      echo "BUILD_CMD=go build ./..."
      go build ./...
      brc=$?
      echo "BUILD_EXIT=$brc"
      [ "$brc" = "0" ] && echo "BUILD_OK"
      LOG="$REPO/tmp/2444-r6.log"
      echo "FULL_SUITE_CMD=go test ./handlers/ -count=1 -timeout 15m"
      go test ./handlers/ -count=1 -timeout 15m > "$LOG" 2>&1
      trc=$?
      cat "$LOG"
      fails="$(grep -c '^--- FAIL' "$LOG" || true)"
      panics="$(grep -c '^panic:' "$LOG" || true)"
      echo "FULL_SUITE_EXIT=$trc FAIL_COUNT=${fails:-0} PANIC_COUNT=${panics:-0}"
      [ "$brc" = "0" ] && [ "$trc" = "0" ] && [ "${fails:-1}" = "0" ] && [ "${panics:-1}" = "0" ] && echo "GATE_OK"
      [ "$brc" = "0" ] && [ "$trc" = "0" ] && [ "${fails:-1}" = "0" ] && [ "${panics:-1}" = "0" ]
      exit $?
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
