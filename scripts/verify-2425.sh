#!/usr/bin/env bash
# #2425：exchange_token 生命周期（401/40110 + 前端刷新/预取）。
# 用法：bash scripts/verify-2425.sh <r1|r2|r3|r4|r5|r6>
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2425/R1-map-expired.txt" ;;
  r2) ART="docs/evidence/2425/R2-map-existing.txt" ;;
  r3) ART="docs/evidence/2425/R3-decision.txt" ;;
  r4) ART="docs/evidence/2425/R4-clear.txt" ;;
  r5) ART="docs/evidence/2425/R5-prefetch.txt" ;;
  r6) ART="docs/evidence/2425/R6-smoke.txt" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2425/' || true)" ]; then DIRTY=true; fi
run_vitest() {
  ( cd "$REPO_ROOT/frontend-mobile" && npx vitest run --config vitest.config.js scripts/page-smoke/account-select-token-2425.test.jsx -t "$1" )
  local rc=$?
  [ "$rc" = "0" ] && echo "--- PASS: $1" || echo "--- FAIL: $1"
  return "$rc"
}
{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1) cd "$REPO_ROOT/backend" && go test ./services/ -run '^TestWxAuthErrorExpiredToken_2425$' -count=1 -v ;;
    r2) cd "$REPO_ROOT/backend" && go test ./services/ -run '^TestWxAuthErrorExistingMappings_2425$' -count=1 -v ;;
    r3) run_vitest TestAccountSelectRetryDecision_2425 ;;
    r4) run_vitest TestAccountSelectClearToken_2425 ;;
    r5) run_vitest TestAccountPrefetchFreshness_2425 ;;
    r6) cd "$REPO_ROOT/frontend-mobile" && npm run smoke ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
