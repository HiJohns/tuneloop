#!/usr/bin/env bash
# #2452：middleware/service 陈旧测试对齐（仅测试，不改产品）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) NAME="R1-middleware.txt" ;;
  r2) NAME="R2-service.txt" ;;
  r3) NAME="R3-gate.txt" ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/2452}"
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
  export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}" GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
  mkdir -p "$TMPDIR" "$GOCACHE"
}

case "$MODE" in
  r1)
    { echo "$PROV"; backend; go test ./middleware -run '^TestSysPermBitNames_Completeness$' -count=1 -v; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2)
    { echo "$PROV"; backend; go test ./service -run '^(TestResolveCategory|TestValidateRequiredColumns|TestParseHeaderRow)$' -count=1 -v; } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3)
    {
      echo "$PROV"
      backend
      echo "BUILD_CMD=go build ./..."
      go build ./...
      brc=$?
      echo "BUILD_EXIT=$brc"
      [ "$brc" = "0" ] && echo "BUILD_OK"
      go test ./middleware ./service -count=1
      trc=$?
      echo "PACKAGE_GATE_EXIT=$trc"
      [ "$brc" = "0" ] && [ "$trc" = "0" ] && echo "GATE_OK"
      exit "$(( brc != 0 || trc != 0 ))"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
