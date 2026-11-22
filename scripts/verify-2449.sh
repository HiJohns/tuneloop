#!/usr/bin/env bash
# #2449: /users/me site follows JWT oid; staff list site_names.
# Rev.2（审计返工）：dirty 含 untracked、R1/R2 独立用例、R3 显式 build 证据。
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) NAME="R1-me-site.txt" ;;
  r2) NAME="R2-list.txt" ;;
  r3) NAME="R3-build.txt" ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT="${OUT_DIR:-$REPO/docs/evidence/2449}"; mkdir -p "$OUT"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"
DIRTY=false
# #2449 Rev.2 D8：dirty 必须包含 untracked（与 #2436/#2423 同口径）。
[ -n "$(git status --porcelain -uall)" ] && DIRTY=true
PROV="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
backend() {
  cd "$REPO/backend"
  export TMPDIR="${BUILD_TMPDIR:-${TMPDIR:-/home/coder/tuneloop/tmp/2449-build}}"
  export GOCACHE="${BUILD_GOCACHE:-${GOCACHE:-/home/coder/tuneloop/tmp/2449-gocache}}"
  mkdir -p "$TMPDIR" "$GOCACHE"
  set -a; . ./.env; set +a
}
case "$MODE" in
  r1) { echo "$PROV"; backend; go test ./handlers/ -run '^TestGetCurrentUser_SiteByJWTOrg_2449$' -count=1 -v; } 2>&1 | tee "$OUT/$NAME"; exit "${PIPESTATUS[0]}" ;;
  r2) { echo "$PROV"; backend; go test ./handlers/ -run '^TestListStaff_SiteNames_2449$' -count=1 -v; } 2>&1 | tee "$OUT/$NAME"; exit "${PIPESTATUS[0]}" ;;
  r3) {
        echo "$PROV"
        backend
        echo "BUILD_CMD=go build ./..."
        go build ./...
        brc=$?
        echo "BUILD_EXIT=$brc"
        [ "$brc" = "0" ] || { echo "BUILD_FAIL"; exit 1; }
        echo "BUILD_OK"
        go test ./handlers/ -run '2449' -count=1 -v
      } 2>&1 | tee "$OUT/$NAME"; exit "${PIPESTATUS[0]}" ;;
esac
