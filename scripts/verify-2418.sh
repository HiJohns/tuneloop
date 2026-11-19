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
  *) echo "usage: $0 <r1|r2|r3|r4|r5>" >&2; exit 2 ;;
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
    r1|r2|r3) cd "$REPO_ROOT/backend"; go test ./services/ -run "$TEST" -count=1 -v ;;
    r4)
      cd "$REPO_ROOT"
      if grep -q "internal 路径" docs/cases/lease.md && grep -q "approved" docs/cases/lease.md; then
        echo "DOC_OK: docs/cases/lease.md internal path requires approved"
      else
        echo "DOC_MISSING: docs/cases/lease.md"; exit 1
      fi
      ;;
    r5) cd "$REPO_ROOT/backend"; go build ./... ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
