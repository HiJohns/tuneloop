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
DIRTY=false; [ -n "$(git status --porcelain --untracked-files=no)" ] && DIRTY=true
backend() { cd "$REPO/backend"; export TMPDIR="$REPO/tmp/gobuild" GOCACHE="$REPO/tmp/gocache"; }
case "$MODE" in
  r1|r2|r3|r4)
    { echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"; backend; go test ./handlers/ -run "$TESTS" -count=1 -v; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r5)
    { echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"; cd "$REPO/frontend-pc"; npm run build; rc=$?; echo "BUILD_EXIT=$rc"; [ "$rc" = "0" ] && echo "BUILD_OK"; exit "$rc"; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-gate-baseline)
    { echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"; backend; go build ./... && go test ./handlers/ -count=1 -timeout 15m; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
