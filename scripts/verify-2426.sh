#!/usr/bin/env bash
# #2426：needs_merge 展示/脱敏/PC 轮询收口。
# 用法：bash scripts/verify-2426.sh <r1|r2|r3|r4>
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2426/R1-mask.txt" ;;
  r2) ART="docs/evidence/2426/R2-poll.txt" ;;
  r3) ART="docs/evidence/2426/R3-manual.md" ;;
  r4) ART="docs/evidence/2426/R4-smoke.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
mkdir -p "$(dirname "$ART")"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2426/' || true)" ]; then DIRTY=true; fi
if [ "$MODE" = "r3" ]; then
  f="docs/evidence/2426/R3-manual.md"
  if grep -q 'executor: user' "$f" && grep -q 'RESULT: PASS' "$f"; then echo "MANUAL_OK: $f"; exit 0; fi
  echo "MANUAL_PENDING: $f（Owner 真机回执未签）"; exit 1
fi
run_test() {
  ( cd "$REPO_ROOT/frontend-mobile" && npx vitest run --config vitest.config.js scripts/page-smoke/wechat-bind-ui-2426.test.jsx -t "$1" )
  local rc=$?
  [ "$rc" = "0" ] && echo "--- PASS: $1" || echo "--- FAIL: $1"
  return "$rc"
}
{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1) run_test TestMaskOpenid_2426 ;;
    r2) run_test TestClassifyBindPollStatus_2426 ;;
    r4) cd "$REPO_ROOT/frontend-mobile" && npm run smoke ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
