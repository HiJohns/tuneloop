#!/usr/bin/env bash
# #2423 Rev.2：spec v2.3 退款纯函数 ComputeRefundSpec 验收（#2417 S1，不接线）。
# 用法：bash scripts/verify-2423.sh <r0-spec|r1|r2|r3|r4|r5>
# 产物写入 OUT_DIR（默认 docs/evidence/2423）；隔离 worktree 生成时把 OUT_DIR 指到树外，
# 使 provenance 的 dirty=false 为真实快照（dirty 判定含 untracked）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r0-spec) ART_NAME="R0-spec.txt"; TEST="" ;;
  r1) ART_NAME="R1-t23.txt"; TEST='^TestComputeRefundSpecT23_2423$' ;;
  r2) ART_NAME="R2-fixture2339.txt"; TEST='^TestComputeRefundSpecFixture2339_2423$' ;;
  r3) ART_NAME="R3-conservation.txt"; TEST='^TestComputeRefundSpecConservation_2423$' ;;
  r4) ART_NAME="R4-bounds.txt"; TEST='^TestComputeRefundSpecBounds_2423$' ;;
  r5) ART_NAME="R5-build.txt"; TEST="" ;;
  *) echo "usage: $0 <r0-spec|r1|r2|r3|r4|r5>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2423}"
ART="$OUT_DIR/$ART_NAME"
mkdir -p "$OUT_DIR"

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
# #2423 Rev.2 D3：dirty 必须包含 untracked，且不得整体排除证据目录。
if [ -n "$(git status --porcelain -uall)" ]; then
  DIRTY=true
fi

case "$MODE" in
  r0-spec)
    {
      echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
      echo "## 口径一致性：cases 用例文档 vs 实现（deposit_refund）"
      fail=0
      SPEC_DOC="docs/cases/settlement-refund-spec-v2.md"
      ACC_DOC="docs/cases/settlement-accounting-rules.md"
      for doc in "$SPEC_DOC" "$ACC_DOC"; do
        if grep -Eq 'deposit_deductions_cents[[:space:]]*=[[:space:]]*min\(deposit_held_cents, undisputed_deductions_cents\)' "$doc" \
           && grep -Eq 'deposit_refund_cents[[:space:]]*=[[:space:]]*deposit_held_cents[[:space:]]*-[[:space:]]*deposit_deductions_cents' "$doc"; then
          echo "DOC_OK: $doc"
        else
          echo "DOC_FAIL: $doc 未含 cases 公式"; fail=1
        fi
        if grep -Eq 'deposit_cash_collected_net[[:space:]]*-[[:space:]]*deposit_held' "$doc"; then
          echo "DOC_FAIL: $doc 仍把 collected−held 当作退款公式"; fail=1
        fi
      done
      if grep -q 'depositDeductions := minInt64(in.DepositHeldCents, in.OtherFeeCents)' backend/services/settlement_refund_spec.go \
         && grep -q 'depositRefund := in.DepositHeldCents - depositDeductions' backend/services/settlement_refund_spec.go; then
        echo "IMPL_OK: backend/services/settlement_refund_spec.go"
      else
        echo "IMPL_FAIL: 实现公式与 cases 不一致"; fail=1
      fi
      [ "$fail" = "0" ] && echo "SPEC_OK" || echo "SPEC_FAIL"
    } 2>&1 | tee "$ART"
    grep -q "SPEC_OK" "$ART"
    exit $?
    ;;

  r5)
    {
      echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
      echo "BUILD_CMD=go build ./..."
      ( cd backend && go build ./... )
      rc=$?
      echo "BUILD_EXIT=$rc"
      [ "$rc" = "0" ] && echo "BUILD_OK" || echo "BUILD_FAIL"
    } 2>&1 | tee "$ART"
    grep -q "BUILD_OK" "$ART"
    exit $?
    ;;

  *)
    {
      echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
      ( cd backend && go test ./services/ -run "$TEST" -count=1 -v )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
esac
