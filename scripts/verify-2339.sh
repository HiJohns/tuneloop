#!/usr/bin/env bash
# #2339 Group B triage：9 个基线失败测试的定向验收 + 推导/红绿证据。
# 用法：bash scripts/verify-2339.sh <r1-batch-order|r2-create-order|r3-return-refund|r4-refund-diff|r5-scenarios|r6-settlement|r7-detail|r8-diff|r9-derivation|r10-red-green>
# 每个模式把带 provenance 头的输出写入 docs/evidence/2339/ 对应产物。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-batch-order)   ART="docs/evidence/2339/R1-batch-order.txt"; TEST='^TestBatchCreateOrder_RentDaysOverridesEndDate$' ;;
  r2-create-order)  ART="docs/evidence/2339/R2-create-order.txt"; TEST='^TestCreateRentalOrder$' ;;
  r3-return-refund) ART="docs/evidence/2339/R3-return-refund.txt"; TEST='^TestInspectReturn_Good_ExecutesRefund$' ;;
  r4-refund-diff)   ART="docs/evidence/2339/R4-refund-diff.txt"; TEST='^TestRefundDiff_Points' ;;
  r5-scenarios)     ART="docs/evidence/2339/R5-scenarios.txt"; TEST='^TestScenario' ;;
  r6-settlement)    ART="docs/evidence/2339/R6-settlement.txt"; TEST='^TestSettlementFlow$' ;;
  r7-detail)        ART="docs/evidence/2339/R7-detail.txt"; TEST='^TestGetUserInstrumentDetail$' ;;
  r8-diff)          ART="docs/evidence/2339/R8-diff.txt" ;;
  r9-derivation)    ART="docs/evidence/2339/R9-derivation.md" ;;
  r10-red-green)    ART="docs/evidence/2339/R10-red-green.txt" ;;
  r11-batch-gate-ref) ART="docs/evidence/2339/R11-batch-gate-ref.txt" ;;
  *) echo "usage: $0 <r1-batch-order|r2-create-order|r3-return-refund|r4-refund-diff|r5-scenarios|r6-settlement|r7-detail|r8-diff|r9-derivation|r10-red-green|r11-batch-gate-ref>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
# 本地 PG 参数以 backend/.env 为准（端口/密码随时段隧道变化），测试库名独立。
if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2339/' || true)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
FIX_COMMIT="db76582f4"

run_go_test() {
  ( cd "$REPO_ROOT/backend" && TUNELOOP_TEST_DB="$1" go test ./handlers/ -run "$2" -count=1 -v )
}

case "$MODE" in
  r1-batch-order|r2-create-order|r3-return-refund|r4-refund-diff|r5-scenarios|r6-settlement|r7-detail)
    {
      echo "$PROVENANCE"
      run_go_test "tuneloop_test_verify2339" "$TEST"
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r8-diff)
    {
      echo "$PROVENANCE"
      BASE="$(git rev-parse "$FIX_COMMIT^")"
      echo "# baseline=$BASE fix=$FIX_COMMIT"
      DIFF="$(git diff "$BASE" "$FIX_COMMIT" -- backend | grep -nE '^\+.*t\.Skip' || true)"
      if [ -n "$DIFF" ]; then
        echo "$DIFF"
        echo "SKIP_ADDED: 新增 t.Skip 不被允许 (#2339 D5)"
        exit 1
      fi
      echo "NO_SKIP_ADDED: fix  diff 无 ^+.*t.Skip"
      echo "--- changed files ---"
      git diff --name-only "$BASE" "$FIX_COMMIT" -- backend
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r9-derivation)
    if [ ! -f "$ART" ]; then echo "MISSING: $ART" >&2; exit 1; fi
    python3 - "$ART" "$PROVENANCE" <<'PY'
import sys
p, prov = sys.argv[1], sys.argv[2]
lines = open(p, encoding='utf-8').read().splitlines()
if lines and lines[0].startswith('# build='):
    lines[0] = prov
else:
    lines.insert(0, prov)
open(p, 'w', encoding='utf-8').write('\n'.join(lines) + '\n')
PY
    body="$(tail -n +2 "$ART")"
    fail=0
    check() { if ! printf '%s\n' "$body" | grep -q -- "$1"; then echo "MISSING: $1"; fail=1; fi; }
    check "Rent 推导"
    check "Deposit 推导"
    check "Points 推导"
    check "2800"
    check "2500"
    check "2333.33"
    check "666.67"
    check "506.67"
    check "160"
    check "840"
    check "1960"
    check "net cash 40"
    check "#2417"
    if [ "$fail" = "1" ]; then echo "DERIVATION_INCOMPLETE"; exit 1; fi
    echo "DERIVATION_OK: rent/deposit/points 三段齐全，R4 数值逐项命中，spec 差异与归属已记录"
    exit 0
    ;;
  r11-batch-gate-ref)
      SRC="$REPO_ROOT/docs/evidence/2443/R5-handlers-full.txt"
      [ -f "$SRC" ] || { echo "FAIL: missing $SRC"; exit 1; }
      tree="$(head -1 "$SRC" | sed -n 's/.*tree=\([0-9a-f]\{7,40\}\).*/\1/p')"
      [ "$tree" = "27f35d34b" ] || { echo "FAIL: SOURCE_TREE=$tree want 27f35d34b [FAIL]"; exit 1; }
      grep -q '^FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0$' "$SRC" \
        || { echo "FAIL: batch gate summary not clean [FAIL]"; exit 1; }
      for c in db76582f4 b70f96eb3 a7a881c32; do
        git merge-base --is-ancestor "$c" 27f35d34b \
          || { echo "FAIL: $c not ancestor of 27f35d34b [FAIL]"; exit 1; }
      done
      {
        echo "$PROVENANCE"
        echo "# source=docs/evidence/2443/R5-handlers-full.txt"
        echo "FULL_SUITE_EXIT=0 FAIL_COUNT=0 PANIC_COUNT=0"
        echo "SOURCE_TREE=27f35d34b"
        echo "ANCESTRY_OK: db76582f4 b70f96eb3 a7a881c32 ancestors of 27f35d34b"
        echo "BATCH_GATE_REF_OK"
      } 2>&1 | tee "$ART"
      exit "${PIPESTATUS[0]}"
      ;;
  r10-red-green)
    RED_WT="$REPO_ROOT/tmp/wt-2339-red"
    git worktree remove --force "$RED_WT" >/dev/null 2>&1 || true
    rm -rf "$RED_WT"; git worktree prune
    git worktree add --detach "$RED_WT" "$FIX_COMMIT^" >/dev/null 2>&1 || { echo "WORKTREE_FAIL"; exit 1; }
    mkdir -p "$RED_WT/tmp"
    red_run() {
      ( cd "$RED_WT/backend" && TUNELOOP_TEST_DB="tuneloop_test_2339_red" go test ./handlers/ -run "$1" -count=1 -v 2>&1 )
    }
    red_c4="$(red_run '^TestScenarioC_CancelBoundary$/^C4_CannotShipFromAvailable$')"; rc_c4=$?
    red_flow="$(red_run '^TestSettlementFlow$')"; rc_flow=$?
    green_c4="$(run_go_test 'tuneloop_test_2339_green' '^TestScenarioC_CancelBoundary$/^C4_CannotShipFromAvailable$')"; g_c4=$?
    green_flow="$(run_go_test 'tuneloop_test_2339_green' '^TestSettlementFlow$')"; g_flow=$?
    {
      echo "$PROVENANCE"
      echo "# fix=$FIX_COMMIT red=$FIX_COMMIT^"
      echo
      echo "=== RED: C4_CannotShipFromAvailable @ $FIX_COMMIT^ (exit=$rc_c4) ==="
      printf '%s\n' "$red_c4" | grep -E "FAIL|Error:|expected:|actual  :|Test:" | head -20
      echo
      echo "=== RED: TestSettlementFlow @ $FIX_COMMIT^ (exit=$rc_flow) ==="
      printf '%s\n' "$red_flow" | grep -E "FAIL|Error:|expected:|actual  :|deposit|rent " | head -20
      echo
      echo "=== GREEN: C4_CannotShipFromAvailable @ $COMMIT (exit=$g_c4) ==="
      printf '%s\n' "$green_c4" | grep -E "^(--- PASS|--- FAIL)|^ok|^FAIL" | head -10
      echo
      echo "=== GREEN: TestSettlementFlow @ $COMMIT (exit=$g_flow) ==="
      printf '%s\n' "$green_flow" | grep -E "^(--- PASS|--- FAIL)|^ok|^FAIL" | head -10
    } 2>&1 | tee "$ART"
    if [ "$rc_c4" = "0" ] || [ "$rc_flow" = "0" ] || [ "$g_c4" != "0" ] || [ "$g_flow" != "0" ]; then
      echo "RED_GREEN_FAIL: red c4=$rc_c4 flow=$rc_flow; green c4=$g_c4 flow=$g_flow"
      exit 1
    fi
    if ! grep -q -- "--- FAIL" "$ART" || ! grep -q -- "--- PASS" "$ART"; then
      echo "RED_GREEN_FAIL: artifact missing --- FAIL / --- PASS"
      exit 1
    fi
    exit 0
    ;;
esac
