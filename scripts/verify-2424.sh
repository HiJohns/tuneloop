#!/usr/bin/env bash
# #2424 S2：结算退款口径切换（毛额/净额分列 + 净额执行）证据驱动。
#
# 用法：bash scripts/verify-2424.sh <r1-phase-a|r2-regression|r3-switch|r4-matrix|r5-execution|r6-doc|r7-build>
#
# Phase A（R1/R2）必须在 Phase A 提交（624ce9d80）的树上运行：该阶段 gate 的
# 断言是「旧字段不变」（666.67/506.67/0），Phase B 切换后同一断言不再成立（D6）。
# 其余行在当前 HEAD 运行。产物写入 $OUT_DIR（默认 docs/evidence/2424）；
# 生成前要求工作树干净（dirty 包含 untracked，D7）。
set -uo pipefail

MODE="${1:-}"
PHASE_A_COMMIT="624ce9d80"

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2424}"
TMPBASE="${TMPBASE:-/tmp/opencode}"
export TMPDIR="${TMPDIR:-$TMPBASE/2424-gobuild}"
export GOCACHE="${GOCACHE:-$TMPBASE/2424-gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$OUT_DIR"

if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1

HEAD_SHA="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=1.0.$HEAD_SHORT commit=$HEAD_SHA dirty=$DIRTY"

phase_a_provenance() {
  local sha short
  sha="$(git rev-parse "$PHASE_A_COMMIT")"
  short="$(git rev-parse --short "$PHASE_A_COMMIT")"
  printf '# build=1.0.%s commit=%s dirty=false phase_a_commit=%s\n' "$short" "$sha" "$sha"
}

run_tests_in_tree() { # tree testdb regex
  ( cd "$1/backend" && TUNELOOP_TEST_DB="$2" go test ./handlers -run "$3" -count=1 -v )
}

run_tests_here() { # testdb regex
  ( cd "$REPO_ROOT/backend" && TUNELOOP_TEST_DB="$1" go test ./handlers -run "$2" -count=1 -v )
}

prepare_phase_a_tree() {
  local tmp
  tmp="$(mktemp -d "$TMPBASE/2424-phase-a.XXXXXX")" || return 1
  git archive "$PHASE_A_COMMIT" | tar -x -C "$tmp" || return 1
  if [ -f "$REPO_ROOT/backend/.env" ]; then
    ln -sfn "$REPO_ROOT/backend/.env" "$tmp/backend/.env"
  fi
  printf '%s' "$tmp"
}

MATRIX_FILES=(
  settlement_diff_test.go settlement_1743_test.go settlement_1836_test.go
  renewal_settlement_1997_test.go settlement_1855_test.go user_settlement_test.go
  state_machine_e2e_test.go rental_settlement_matrix_2268_test.go shortfall_1746_test.go
  settlement_1763_test.go settlement_1764_test.go settlement_1803_test.go
  settlement_1918_test.go inspect_return_1801_test.go order_term_version_renewal_2282_test.go
  settlement_flow_test.go payscore_settlement_2332_test.go lease_early_return_test.go
  settlement_ledger_read_2281_test.go settlement_spec_preview_2424_test.go
)

case "$MODE" in
  r1-phase-a|r2-regression)
    TREE="$(prepare_phase_a_tree)" || { echo "FAIL: cannot extract $PHASE_A_COMMIT"; exit 1; }
    trap 'rm -rf "$TREE"' EXIT
    if [ "$MODE" = "r1-phase-a" ]; then
      ART="$OUT_DIR/R1-phase-a.txt"
      {
        phase_a_provenance
        run_tests_in_tree "$TREE" "tuneloop_test_2424_r1" '^TestSettlementSpecPreview_2424$'
      } 2>&1 | tee "$ART"
      rc=${PIPESTATUS[0]}
      grep -q -- '--- PASS: TestSettlementSpecPreview_2424' "$ART" || { echo "FAIL: missing PASS"; exit 1; }
      grep -q 'legacy_total=666.67' "$ART" || { echo "FAIL: legacy fields changed in Phase A"; exit 1; }
      grep -q 'PHASE_A_OK' "$ART" || { echo "FAIL: missing PHASE_A_OK"; exit 1; }
      exit "$rc"
    fi
    ART="$OUT_DIR/R2-regression.txt"
    REGEX='^(TestComputeSettlement_SegmentScenarios|TestSettlement_UserScenarioB_NoDepositNoCoupon|TestRefundDiff_PointsOverCap|TestRefundDiff_PointsWithinCap|TestSettlement_PostDamageStates|TestComputeSettlement_SupplementWaive_CancelsShortfall|TestComputeSettlement_LateReturn)$'
    {
      phase_a_provenance
      run_tests_in_tree "$TREE" "tuneloop_test_2424_r2" "$REGEX"
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- FAIL' "$ART" && { echo "FAIL: regression failures"; exit 1; }
    grep -q -- '--- PASS' "$ART" || { echo "FAIL: no PASS in regression"; exit 1; }
    exit "$rc"
    ;;
  r3-switch)
    ART="$OUT_DIR/R3-switch.txt"
    {
      echo "$PROVENANCE"
      run_tests_here "tuneloop_test_2424_r3" '^TestSettlementSpecSwitch_2424$'
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- PASS: TestSettlementSpecSwitch_2424' "$ART" || { echo "FAIL: missing PASS"; exit 1; }
    exit "$rc"
    ;;
  r4-matrix)
    ART="$OUT_DIR/R4-matrix.txt"
    TEST_NAMES="$(grep -h '^func Test' "${MATRIX_FILES[@]/#/backend/handlers/}" \
      | sed -E 's/^func (Test[A-Za-z0-9_]+).*/\1/' | sort -u)"
    [ -n "$TEST_NAMES" ] || { echo "FAIL: no tests found"; exit 1; }
    REGEX="^($(printf '%s\n' "$TEST_NAMES" | paste -sd'|' -))$"
    {
      echo "$PROVENANCE"
      echo "# files=${#MATRIX_FILES[@]} tests=$(printf '%s\n' "$TEST_NAMES" | wc -l | tr -d ' ')"
      run_tests_here "tuneloop_test_2424_r4" "$REGEX"
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- FAIL' "$ART" && { echo "FAIL: matrix failures"; exit 1; }
    DERIV="$OUT_DIR/R4-derivation.md"
    [ -f "$DERIV" ] || { echo "FAIL: missing R4-derivation.md"; exit 1; }
    for marker in 'old→new' 'fixture' '券' 'gross' 'net'; do
      grep -q "$marker" "$DERIV" || { echo "FAIL: derivation missing marker: $marker"; exit 1; }
    done
    pass_count=$(grep -c -- '--- PASS' "$ART")
    echo "R4_PASS_COUNT=$pass_count"
    [ "$rc" = "0" ] || exit "$rc"
    exit 0
    ;;
  r5-execution)
    ART="$OUT_DIR/R5-execution.txt"
    {
      echo "$PROVENANCE"
      run_tests_here "tuneloop_test_2424_r5" '^TestExecuteRefundNet_2424$'
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- PASS: TestExecuteRefundNet_2424' "$ART" || { echo "FAIL: missing PASS"; exit 1; }
    exit "$rc"
    ;;
  r6-doc)
    ART="$OUT_DIR/R6-doc.txt"
    {
      echo "$PROVENANCE"
      echo "== docs/cases/settlement-refund-spec-v2.md =="
      grep -nE '实现状态|历史口径|毛额|net_cash|执行口径|coupon_discount' docs/cases/settlement-refund-spec-v2.md | head -20
      echo "== docs/cases/lease.md (L-06) =="
      grep -nE '对外毛额 / 执行净额|优惠券权威|net_cash|settlements.cash_refundable' docs/cases/lease.md | head -20
    } 2>&1 | tee "$ART"
    grep -q '历史口径' "$ART" || { echo "FAIL: 段模型历史口径缺失"; exit 1; }
    grep -q '对外毛额 / 执行净额' "$ART" || { echo "FAIL: 毛/净字段语义缺失"; exit 1; }
    grep -q '优惠券权威' "$ART" || { echo "FAIL: 券权威口径缺失"; exit 1; }
    exit 0
    ;;
  r7-build)
    ART="$OUT_DIR/R7-build.txt"
    {
      echo "$PROVENANCE"
      ( cd backend && go build ./... )
      echo "BUILD_OK"
    } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q 'BUILD_OK' "$ART" || { echo "FAIL: build failed"; exit 1; }
    exit "$rc"
    ;;
  *)
    echo "usage: $0 <r1-phase-a|r2-regression|r3-switch|r4-matrix|r5-execution|r6-doc|r7-build>" >&2
    exit 2
    ;;
esac
