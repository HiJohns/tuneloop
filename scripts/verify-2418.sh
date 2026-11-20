#!/usr/bin/env bash
# #2418：支付闸门 internal 路径同样要求信用评测。
# 用法：bash scripts/verify-2418.sh <r1|r2|r3|r4|r5>
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2418/R1-internal-reduced.txt"; TEST='^TestAssertOrderPayableIfEnforced$' ;;
  r2) ART="docs/evidence/2418/R2-internal-full.txt"; TEST='^TestAssertOrderPayableInternalFull_2418$' ;;
  r3) ART="docs/evidence/2418/R3-zero-deposit.txt"; TEST='^TestAssertOrderPayableZeroDeposit_2418$' ;;
  r4) ART="docs/evidence/2418/R4-doc.txt"; TEST="" ;;
  r5) ART="docs/evidence/2418/R5-build.txt"; TEST="" ;;
  r6) ART="docs/evidence/2418/R6-full-payscore.txt"; TEST='^TestAssertOrderPayableFullPayscore_2418$' ;;
  r7) ART="docs/evidence/2418/R7-zero-deposit-payscore.txt"; TEST='^TestAssertOrderPayableZeroDepositPayscore_2418$' ;;
  r8) ART="docs/evidence/2418/R8-legacy-removed.txt"; TEST="" ;;
  r9) ART="docs/evidence/2418/R9-no-calls.txt"; TEST="" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2418/' || true)" ]; then DIRTY=true; fi
{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1|r2|r3|r6|r7) cd "$REPO_ROOT/backend"; go test ./services/ -run "$TEST" -count=1 -v ;;
    r4)
      cd "$REPO_ROOT"
      if grep -q "三档一致" docs/cases/lease.md && grep -q "订单押金为 0" docs/cases/lease.md && grep -q "approved" docs/cases/lease.md; then
        echo "DOC_OK: docs/cases/lease.md 三档一致 + 零押金判据=订单押金为 0"
      else
        echo "DOC_MISSING: docs/cases/lease.md"; exit 1
      fi
      ;;
    r5) cd "$REPO_ROOT/backend"; go build ./... ;;
    r8)
      cd "$REPO_ROOT"
      hits=$(grep -rnE "func (RequiresCreditAssessment|AssertTierPayable|AssertTierPayableIfEnforced)\(" backend --include='*.go' || true)
      if [ -n "$hits" ]; then echo "LEGACY_PRESENT"; echo "$hits"; exit 1; fi
      echo "LEGACY_REMOVED: 3 symbols absent"
      ( cd backend && go build ./... )
      ;;
    r9)
      cd "$REPO_ROOT"
      hits=$(grep -rn "RequiresCreditAssessment\|AssertTierPayable" backend frontend-pc/src frontend-mobile/src --include='*.go' --include='*.js' --include='*.jsx' --include='*.mjs' 2>/dev/null || true)
      if [ -n "$hits" ]; then echo "REFERENCES_PRESENT"; echo "$hits"; exit 1; fi
      echo "NO_CALLS: legacy gate symbols have zero references"
      ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
