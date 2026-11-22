#!/usr/bin/env bash
# #2444 Rev.2：显式零押金 — 干净树 preflight + 定向测试 + PC 构建 + provenance 自检。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r0-preflight) NAME="R0-preflight.txt" ;;
  r1) NAME="R1-list.txt" ;;
  r2) NAME="R2-no-recompute.txt" ;;
  r3) NAME="R3-legacy-pin.txt" ;;
  r4) NAME="R4-zero.txt" ;;
  r5) NAME="R5-build.txt" ;;
  r6-provenance) NAME="R6-provenance.txt" ;;
  *) echo "usage: $0 <r0-preflight|r1|r2|r3|r4|r5|r6-provenance>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT="${OUT_DIR:-$REPO/docs/evidence/2444}"; mkdir -p "$OUT"

FIX_REF="${FIX_REF:-e053d587a}"
FIX_SHA="$(git rev-parse "$FIX_REF")"; FIX_SHORT="$(git rev-parse --short "$FIX_REF")"
HEAD_SHA="$(git rev-parse --short HEAD)"
BUILD="1.0.$FIX_SHORT"; COMMIT="$FIX_SHA"
DIRTY=false; [ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=$BUILD commit=$COMMIT dirty=$DIRTY tree=$HEAD_SHA"

backend() {
  cd "$REPO/backend"
  set -a
  # shellcheck disable=SC1091
  . "${ENV_FILE:-/home/coder/tuneloop/backend/.env}"
  set +a
  export TMPDIR="${TMPDIR:-/home/coder/tmp/2444-build}" GOCACHE="${GOCACHE:-/home/coder/tmp/2444-gocache}"
  mkdir -p "$TMPDIR" "$GOCACHE"
}

case "$MODE" in
  r0-preflight)
    {
      echo "$PROV"
      echo "TREE_COMMIT=$(git rev-parse HEAD)"
      status="$(git status --porcelain -uall || true)"
      if [ -n "$status" ]; then
        echo "FAIL: worktree dirty/untracked"
        printf '%s\n' "$status"
        exit 1
      fi
      echo "STATUS_CLEAN"
      echo "ENV_FILE=${ENV_FILE:-/home/coder/tuneloop/backend/.env}"
    } 2>&1 | tee "$OUT/$NAME"
    exit "${PIPESTATUS[0]}"
    ;;
  r1|r2|r3|r4)
    case "$MODE" in
      r1) TESTS='^TestRentSettingDepositParity_2444$' ;;
      r2) TESTS='^TestBatchUpdateRentOmittedDeposit_2444$' ;;
      r3) TESTS='^TestBatchUpdateRentLegacyRoundTrip_2444$' ;;
      r4) TESTS='^TestCreateInstrumentExplicitZeroDeposit_2444$|^TestUpdateInstrumentExplicitZeroDeposit_2444$' ;;
    esac
    { echo "$PROV"; backend; go test ./handlers/ -run "$TESTS" -count=1 -v; } 2>&1 | tee "$OUT/$NAME"
    exit "${PIPESTATUS[0]}"
    ;;
  r5)
    { echo "$PROV"; cd "$REPO/frontend-pc"; npm run build; rc=$?; echo "BUILD_EXIT=$rc"; [ "$rc" = "0" ] && echo "BUILD_OK"; exit "$rc"; } 2>&1 | tee "$OUT/$NAME"
    exit "${PIPESTATUS[0]}"
    ;;
  r6-provenance)
    {
      echo "$PROV"
      fail=0
      for f in R0-preflight R1-list R2-no-recompute R3-legacy-pin R4-zero R5-build; do
        p="$OUT/$f.txt"
        if [ ! -f "$p" ]; then echo "FAIL: missing $p"; fail=1; continue; fi
        first="$(head -1 "$p")"
        case "$first" in
          "# build=$BUILD commit=$FIX_SHA dirty=false "*) echo "OK $f" ;;
          *) echo "FAIL: provenance mismatch in $f: $first"; fail=1 ;;
        esac
      done
      EV="$OUT/evidence.json"
      if [ ! -f "$EV" ]; then
        echo "FAIL: evidence.json missing"; fail=1
      elif command -v jq >/dev/null 2>&1; then
        ev_build="$(jq -r '.build // empty' "$EV")"
        ev_commit="$(jq -r '.commit // empty' "$EV")"
        ev_dirty="$(jq -r '.dirty // empty' "$EV")"
        ev_tree="$(jq -r '.tree // empty' "$EV")"
        [ "$ev_build" = "$BUILD" ] || { echo "FAIL: evidence build=$ev_build want $BUILD"; fail=1; }
        [ "$ev_commit" = "$FIX_SHA" ] || { echo "FAIL: evidence commit=$ev_commit want $FIX_SHA"; fail=1; }
        [ "$ev_dirty" = "false" ] || { echo "FAIL: evidence dirty=$ev_dirty"; fail=1; }
        [ "$ev_tree" = "$(git rev-parse HEAD)" ] || { echo "FAIL: evidence tree=$ev_tree want $(git rev-parse HEAD)"; fail=1; }
      else
        echo "WARN: jq unavailable; evidence.json content not cross-checked"
      fi
      [ "$fail" = "0" ] && echo "PROVENANCE_OK"
      exit "$fail"
    } 2>&1 | tee "$OUT/$NAME"
    exit "${PIPESTATUS[0]}"
    ;;
esac
