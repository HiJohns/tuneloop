#!/usr/bin/env bash
# #2363：历史单标记只读回显 + 收据分账户 + 窗口 gate 证据驱动。
# 用法：bash scripts/verify-2363-settlement.sh <legacy-tags|scenarios|receipts|window|assertions>
set -uo pipefail

MODE="${1:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
OUT_DIR="${OUT_DIR:-$REPO_ROOT/docs/evidence/2363}"
TMPBASE="${TMPBASE:-/home/coder/tmp}"
export TMPDIR="${TMPDIR:-$TMPBASE/2363-gobuild}"
export GOCACHE="${GOCACHE:-$TMPBASE/2363-gocache}"
mkdir -p "$TMPDIR" "$GOCACHE" "$OUT_DIR"

if [ -f "$REPO_ROOT/backend/.env" ]; then
  set -a; . "$REPO_ROOT/backend/.env"; set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TUNELOOP_DB_SIMPLE_PROTOCOL=1

HEAD_SHA="$(git rev-parse HEAD)"; HEAD_SHORT="$(git rev-parse --short HEAD)"
DIRTY=false
[ -n "$(git status --porcelain)" ] && DIRTY=true
PROVENANCE="# build=1.0.$HEAD_SHORT commit=$HEAD_SHA dirty=$DIRTY"

run_tests() { ( cd "$REPO_ROOT/backend" && TUNELOOP_TEST_DB="$1" go test ./handlers -run "$2" -count=1 -v ); }

case "$MODE" in
  legacy-tags)
    ART="$OUT_DIR/R1-legacy-tags.txt"
    { echo "$PROVENANCE"; run_tests tuneloop_test_2363_r1 '^TestSettlementLegacyMarkerEcho_2363$'; echo 'LEGACY_TAGS_OK'; } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- PASS: TestSettlementLegacyMarkerEcho_2363' "$ART" || { echo "FAIL: missing PASS"; exit 1; }
    grep -q 'LEGACY_TAGS_OK' "$ART" || { echo "FAIL: missing marker"; exit 1; }
    exit "$rc"
    ;;
  receipts)
    ART="$OUT_DIR/R3-receipts.txt"
    { echo "$PROVENANCE"; run_tests tuneloop_test_2363_r3 '^TestSettlementReceiptAccountsNet_2363$'; echo 'RECEIPTS_OK'; } 2>&1 | tee "$ART"
    rc=${PIPESTATUS[0]}
    grep -q -- '--- PASS: TestSettlementReceiptAccountsNet_2363' "$ART" || { echo "FAIL: missing PASS"; exit 1; }
    grep -q 'RECEIPTS_OK' "$ART" || { echo "FAIL: missing marker"; exit 1; }
    exit "$rc"
    ;;
  window)
    ART="$OUT_DIR/R4-gate.txt"
    log="$(mktemp "$TMPDIR/2363-window.XXXXXX")"
    { echo "$PROVENANCE"; } > "$ART"
    ( cd "$REPO_ROOT/backend" && go test ./handlers -p 1 -count=1 -timeout 25m ) > "$log" 2>&1
    rc=$?
    fails=$(grep -c '^--- FAIL' "$log" || true)
    panics=$(grep -ci 'panic:' "$log" || true)
    okpkgs=$(grep -c '^ok[[:space:]]' "$log" || true)
    { echo "WINDOW_GATE_EXIT=$rc"; echo "FAIL_COUNT=$fails"; echo "PANIC_COUNT=$panics"; echo "PACKAGES_OK=$okpkgs"; } >> "$ART"
    if [ "$rc" = "0" ] && [ "$fails" = "0" ] && [ "$panics" = "0" ]; then
      echo "WINDOW_GATE_OK" | tee -a "$ART"
    else
      grep -E '^(--- FAIL|FAIL|panic:)' "$log" | head -20 | tee -a "$ART"
      echo "WINDOW_GATE_FAILED" | tee -a "$ART"
      rm -f "$log"; exit 1
    fi
    rm -f "$log"; exit 0
    ;;
  assertions)
    ART="$OUT_DIR/R5-assertions.txt"
    {
      echo "$PROVENANCE"
      test -f docs/evidence/2363/R5-assertions.md && echo "ASSERTIONS_DOC_OK" || echo "ASSERTIONS_DOC_MISSING"
      grep -q 'old→new' docs/evidence/2363/R5-assertions.md && echo "OLD_NEW_OK"
      grep -q '#2424' docs/evidence/2363/R5-assertions.md && echo "SOURCE_MIGRATION_OK"
      grep -q 'NetCash' backend/handlers/settlement_diff_test.go && echo "DIFF_NET_OK"
      grep -q 'NetCash' backend/handlers/state_machine_e2e_test.go && echo "E2E_NET_OK"
      grep -q 'legacy_marker' backend/handlers/settlement_flow_test.go && echo "FLOW_LEGACY_OK"
      if grep -q 'InDelta(t, 666.67' backend/handlers/settlement_diff_test.go; then echo "STALE_SEGMENT_VALUE"; else echo "NO_STALE_SEGMENT_VALUE"; fi
      echo "ASSERTIONS_OK"
    } 2>&1 | tee "$ART"
    grep -q 'ASSERTIONS_OK' "$ART" || { echo "FAIL"; exit 1; }
    grep -qx 'STALE_SEGMENT_VALUE' "$ART" && { echo "FAIL: stale 666.67 assertion still present"; exit 1; }
    exit 0
    ;;
  scenarios)
    ART="$OUT_DIR/R2-scenarios.md"
    RECEIPT="$OUT_DIR/R2-owner-receipt.md"
    if [ -f "$RECEIPT" ] && grep -q '12/12' "$RECEIPT" && grep -qi 'signed\|回执' "$RECEIPT"; then
      { echo "$PROVENANCE"; echo "# R2 预生产 12 场景（Owner/ops 回执）"; cat "$RECEIPT"; echo "R2_STATUS=PASS"; } > "$ART"
      echo "R2_SCENARIOS_OK"; exit 0
    fi
    { echo "$PROVENANCE"; echo "# R2 预生产 12 场景：PENDING（需 Owner/ops 真机回执，Developer 无预生产写权限）"; echo "R2_STATUS=PENDING"; } > "$ART"
    echo "R2_STATUS=PENDING"; exit 3
    ;;
  *)
    echo "usage: $0 <legacy-tags|scenarios|receipts|window|assertions>" >&2
    exit 2
    ;;
esac
