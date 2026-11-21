#!/usr/bin/env bash
# #2449: /users/me site follows JWT oid; staff list site_names.
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) NAME="R1-me-site.txt" ;;
  r2) NAME="R2-list.txt" ;;
  r3) NAME="R3-build.txt" ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac
REPO="$(git rev-parse --show-toplevel)"; cd "$REPO"
OUT="${OUT_DIR:-$REPO/docs/evidence/2449}"; mkdir -p "$OUT" "$REPO/tmp/gobuild" "$REPO/tmp/gocache"
ART="$OUT/$NAME"; BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"
DIRTY=false; [ -n "$(git status --porcelain --untracked-files=no)" ] && DIRTY=true
PROV="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
backend() { cd "$REPO/backend"; export TMPDIR="$REPO/tmp/gobuild" GOCACHE="$REPO/tmp/gocache"; set -a; . ./.env; set +a; }
case "$MODE" in
  r1) { echo "$PROV"; backend; go test ./handlers/ -run '^TestGetCurrentUser_SiteByJWTOrg_2449$' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r2) { echo "$PROV"; backend; go test ./handlers/ -run '^TestGetCurrentUser_SiteByJWTOrg_2449$' -count=1 -v; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
  r3) { echo "$PROV"; backend; go build ./...; go test ./handlers/ -run '2449' -count=1; } 2>&1 | tee "$ART"; exit "${PIPESTATUS[0]}" ;;
esac
