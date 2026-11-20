#!/usr/bin/env bash
# #2443：handlers 全量恢复（P1 panic / P2 schema+fixture / P3 full gate）。
# 用法：bash scripts/verify-2443.sh <r1|r2|r3>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART_NAME="R1-inventory.txt" ;;
  r2) ART_NAME="R2-handlers-full.txt" ;;
  r3) ART_NAME="R3-build.txt" ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2443}"
mkdir -p "$OUT_DIR" "$REPO_ROOT/tmp"
ART="$OUT_DIR/$ART_NAME"
BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"

run_backend() {
  cd "$REPO_ROOT/backend"
  set -a
  # shellcheck disable=SC1091
  . ./.env
  set +a
  export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2443}" \
         IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}" \
         IAM_SECRET="${IAM_SECRET:-test-secret-2443}" \
         TUNELOOP_DB_SIMPLE_PROTOCOL=1
}

case "$MODE" in
  r1)
    {
      echo "$PROVENANCE"
      run_backend
      go test ./handlers/ -run '^TestGetInventoryRentSetting$' -count=1 -v
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2)
    {
      echo "$PROVENANCE"
      run_backend
      go test ./handlers/ -count=1 -timeout 10m
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3)
    {
      echo "$PROVENANCE"
      cd "$REPO_ROOT/backend"
      go build ./...
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
