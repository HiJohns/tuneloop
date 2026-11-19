#!/usr/bin/env bash
# #2423：spec v2.3 退款纯函数 ComputeRefundSpec 验收（#2417 S1，不接线）。
# 用法：bash scripts/verify-2423.sh <r1|r2|r3|r4|r5>
# 每个模式把带 provenance 头的输出写入 docs/evidence/2423/ 对应产物。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1) ART="docs/evidence/2423/R1-t23.txt"; TEST='^TestComputeRefundSpecT23_2423$' ;;
  r2) ART="docs/evidence/2423/R2-fixture2339.txt"; TEST='^TestComputeRefundSpecFixture2339_2423$' ;;
  r3) ART="docs/evidence/2423/R3-conservation.txt"; TEST='^TestComputeRefundSpecConservation_2423$' ;;
  r4) ART="docs/evidence/2423/R4-bounds.txt"; TEST='^TestComputeRefundSpecBounds_2423$' ;;
  r5) ART="docs/evidence/2423/R5-build.txt"; TEST="" ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2423/' || true)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r5) cd backend && go build ./... ;;
    *) cd backend && go test ./services/ -run "$TEST" -count=1 -v ;;
  esac
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
