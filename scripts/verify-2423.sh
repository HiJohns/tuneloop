#!/usr/bin/env bash
# #2423 Rev.2：spec v2.3 退款纯函数 ComputeRefundSpec 验收（#2417 S1，不接线）。
# 用法：bash scripts/verify-2423.sh <r0-spec|r1|r2|r3|r4|r5|r6-gate|r7-body>
# Rev.3：r6-gate = 有效全量 gate（build + handlers 全量 + refund spec），r7-body = Issue body 守恒全式。
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
  r6-gate) ART_NAME="R6-full-gate.txt"; TEST="" ;;
  r7-body) ART_NAME="R7-body.txt"; TEST="" ;;
  *) echo "usage: $0 <r0-spec|r1|r2|r3|r4|r5|r6-gate|r7-body>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2423}"
ART="$OUT_DIR/$ART_NAME"
mkdir -p "$OUT_DIR"

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

# Rev.3 D5：证据针对 fix commit；FIX_COMMIT 显式传入时校验其为 HEAD 祖先，
# 并在第二行记录实际评测树（evaluated_at），避免把后续无关提交冒充为 fix。
EVAL_AT="$(git rev-parse HEAD)"
COMMIT="$(git rev-parse "${FIX_COMMIT:-HEAD}")" || { echo "FATAL: bad FIX_COMMIT" >&2; exit 3; }
git merge-base --is-ancestor "$COMMIT" "$EVAL_AT" || { echo "FATAL: FIX_COMMIT $COMMIT is not an ancestor of HEAD" >&2; exit 3; }
BUILD="1.0.$(git rev-parse --short "$COMMIT")"
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

  r6-gate)
    {
      echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
      echo "# evaluated_at=$EVAL_AT"
      echo "## R6 有效全量 gate（含 P1）：go build + handlers 全量 + refund spec"
      fail=0
      if ( cd backend && go build ./... ); then echo "BUILD_OK"; else echo "BUILD_FAIL"; fail=1; fi
      ( cd backend && go test ./handlers/ -count=1 -timeout 15m ) 2>&1 | tee /tmp/opencode/2423-r6-handlers.log
      h_rc=${PIPESTATUS[0]}
      if [ "$h_rc" -eq 0 ] && ! grep -qE '^(panic:|--- FAIL)' /tmp/opencode/2423-r6-handlers.log && grep -qE '^ok[[:space:]]+tuneloop-backend/handlers' /tmp/opencode/2423-r6-handlers.log; then
        echo "HANDLERS_OK"
      else
        echo "HANDLERS_FAIL rc=$h_rc"
        fail=1
      fi
      ( cd backend && go test ./services/ -run '^TestComputeRefundSpec' -count=1 -v ) 2>&1 | tee /tmp/opencode/2423-r6-refund.log
      r_rc=${PIPESTATUS[0]}
      if [ "$r_rc" -eq 0 ] && ! grep -qE '^(panic:|--- FAIL)' /tmp/opencode/2423-r6-refund.log; then
        echo "REFUND_SPEC_OK"
      else
        echo "REFUND_SPEC_FAIL rc=$r_rc"
        fail=1
      fi
      [ "$fail" -eq 0 ] && echo "FULL_GATE_OK" || echo "FULL_GATE_FAIL"
    } 2>&1 | tee "$ART"
    grep -q "^FULL_GATE_OK$" "$ART"
    exit $?
    ;;

  r7-body)
    {
      echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
      echo "# evaluated_at=$EVAL_AT"
      echo "## R7 Issue body 守恒式（spec §4 全式，禁止旧简化式）"
      if [ -n "${BODY_FILE:-}" ]; then
        echo "BODY_SOURCE=$BODY_FILE"
        cp "$BODY_FILE" /tmp/opencode/2423-r7-body.md
      else
        gh issue view 2423 --json body -q .body > /tmp/opencode/2423-r7-body.md
        echo "BODY_SOURCE=gh issue view 2423"
      fi
      python3 - /tmp/opencode/2423-r7-body.md <<'PYPATCH'
import sys
body = open(sys.argv[1], encoding='utf-8').read()
lines = [l.strip() for l in body.splitlines() if 'net_cash' in l and 'cash_paid' in l]
full = [l for l in lines if 'R_after_coupon' in l and 'deductions_total' in l]
old = [l for l in lines if l not in full]
print('FORMULA_LINES=%d FULL=%d OLD=%d' % (len(lines), len(full), len(old)))
for l in lines:
    print('LINE:', l)
if full and not old:
    print('BODY_FULL_FORMULA_OK')
else:
    print('BODY_FAIL')
    sys.exit(1)
PYPATCH
    } 2>&1 | tee "$ART"
    grep -q "^BODY_FULL_FORMULA_OK$" "$ART"
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
