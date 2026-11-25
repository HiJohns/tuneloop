#!/bin/bash
# Evidence generator for #2513 (S5 维修服务 site 过滤/守卫 ID 域修复).
#
# Rows: r1 r2 r3 r4 (定向用例 + 特征串扫描), r5 (既有维修服务套件回归),
#       r6 (构建 + 同类扫描), gate (供 pre-ready --full-gate-cmd 使用),
#       ledger (从最新 fix commit 派生 evidence.json 的 build/commit/dirty 并执行 R7 断言).
#
# 用法（证据生成必须在该单 fix commit 之后、干净树上执行）：
#   bash scripts/verify-2513.sh r1 > docs/evidence/2513/R1-pending-dispatch.txt
#
# 退出码：0 = 该行 PASS；非 0 = FAIL（不得交付）。
set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
BACKEND_DIR="$ROOT_DIR/backend"

# 非登录 shell 下 go 可能不在 PATH（运行机默认 /usr/local/go）
if ! command -v go >/dev/null 2>&1; then
  for cand in /usr/local/go/bin /usr/lib/go/bin; do
    [ -x "$cand/go" ] && PATH="$cand:$PATH"
  done
fi

# Provenance 锚定本单最新的 fix commit（P12/P13 口径一致）：证据在 fix commit
# 之后重新生成时锚点不变；只允许证据目录与 tmp/ 处于未提交状态。
COMMIT="$(git -C "$ROOT_DIR" log --format=%H --grep="^fix(issue-2513)" -1 2>/dev/null || true)"
[ -n "$COMMIT" ] || COMMIT="$(git -C "$ROOT_DIR" rev-parse HEAD)"
SHORT="$(git -C "$ROOT_DIR" rev-parse --short=8 "$COMMIT")"
if [ -n "$(git -C "$ROOT_DIR" status --porcelain --untracked-files=no -- . \
  ":(exclude)docs/evidence/2513" ':(exclude)tmp')" ]; then
  DIRTY="true"
else
  DIRTY="false"
fi
FIX_COMMIT="$COMMIT"

provenance() {
  echo "# build=1.0.$SHORT # commit=$COMMIT # dirty=$DIRTY"
}

# 抽取 go test -v 输出中的 key=value 标记（词边界匹配，最后一条）
marker() {
  local key=$1 out=$2
  printf '%s\n' "$out" | grep -oE "\\b${key}=[0-9]+" | tail -1 | cut -d= -f2
}

run_test() {
  ( cd "$BACKEND_DIR" && go test ./handlers -run "$1" -count=1 -v 2>&1 )
}

fail_row() {
  local row=$1 reason=$2
  echo "${row}_FAIL"
  echo "REASON=$reason"
  exit 1
}

row_r1() {
  local out v_rows
  out="$(run_test '^TestRepairService2513_PendingDispatchMerchantSeesLegacy$')"
  if ! printf '%s\n' "$out" | grep -q -- "--- PASS: TestRepairService2513_PendingDispatchMerchantSeesLegacy"; then
    printf '%s\n' "$out" | tail -40
    fail_row R1 "test-not-pass"
  fi
  echo "MERCHANT_ROWS=$(marker MERCHANT_ROWS "$out")"
  echo "MERCHANT_STAFF_ROWS=$(marker MERCHANT_STAFF_ROWS "$out")"
  echo "LEGACY_VISIBLE=$(marker LEGACY_VISIBLE "$out")"
  echo "OLD_SITE_EQ_OID_ROWS=$(marker OLD_SITE_EQ_OID_ROWS "$out")"
  [ "$(marker MERCHANT_ROWS "$out")" = "2" ] || fail_row R1 "merchant-rows-not-2"
  [ "$(marker MERCHANT_STAFF_ROWS "$out")" = "2" ] || fail_row R1 "merchant-staff-rows-not-2"
  [ "$(marker LEGACY_VISIBLE "$out")" = "1" ] || fail_row R1 "legacy-not-visible"
  echo "R1_OK"
}

row_r2() {
  local out rows
  out="$(run_test '^TestRepairService2513_ListTasksSiteIncludesLegacy$')"
  if ! printf '%s\n' "$out" | grep -q -- "--- PASS: TestRepairService2513_ListTasksSiteIncludesLegacy"; then
    printf '%s\n' "$out" | tail -40
    fail_row R2 "test-not-pass"
  fi
  rows="$(marker SITE_SCOPE_ROWS "$out")"
  echo "SITE_SCOPE_ROWS=$rows"
  echo "OLD_FILTER_ROWS=$(marker OLD_FILTER_ROWS "$out")"
  [ "$rows" = "2" ] || fail_row R2 "site-scope-rows-not-2"
  echo "R2_OK"
}

row_r3() {
  local out allowed status direct
  out="$(run_test '^TestRepairService2513_StaffAllowedLegacySiteRow$')"
  if ! printf '%s\n' "$out" | grep -q -- "--- PASS: TestRepairService2513_StaffAllowedLegacySiteRow"; then
    printf '%s\n' "$out" | tail -40
    fail_row R3 "test-not-pass"
  fi
  allowed="$(marker ALLOWED_LEGACY "$out")"
  status="$(marker QUOTE_STATUS "$out")"
  # 旧分支（rr.SiteID != oid）必须从**代码行**彻底消失（注释里记录历史形态不算残留）
  direct="$(grep -v '^[[:space:]]*//' "$BACKEND_DIR/handlers/repair_service.go" | grep -c 'rr\.SiteID != oid' || true)"
  echo "ALLOWED_LEGACY=$allowed"
  echo "QUOTE_STATUS=$status"
  echo "OLD_DIRECT_OID_COMPARE=$direct"
  [ "$allowed" = "1" ] || fail_row R3 "legacy-not-allowed"
  [ "$status" = "200" ] || fail_row R3 "quote-not-200"
  [ "$direct" = "0" ] || fail_row R3 "direct-oid-compare-present"
  echo "R3_OK"
}

row_r4() {
  local out other_rows other_allowed site_rows site_pending
  out="$(run_test '^TestRepairService2513_Negative$')"
  if ! printf '%s\n' "$out" | grep -q -- "--- PASS: TestRepairService2513_Negative"; then
    printf '%s\n' "$out" | tail -40
    fail_row R4 "test-not-pass"
  fi
  other_rows="$(marker OTHER_TENANT_ROWS "$out")"
  other_allowed="$(marker OTHER_TENANT_ALLOWED "$out")"
  site_rows="$(marker SITE_STAFF_ROWS "$out")"
  site_pending="$(marker SITE_STAFF_PENDING_ROWS "$out")"
  echo "OTHER_TENANT_ROWS=$other_rows"
  echo "OTHER_TENANT_ALLOWED=$other_allowed"
  echo "SITE_STAFF_ROWS=$site_rows"
  echo "SITE_STAFF_PENDING_ROWS=$site_pending"
  [ "$other_rows" = "0" ] || fail_row R4 "other-tenant-rows"
  [ "$other_allowed" = "0" ] || fail_row R4 "other-tenant-allowed"
  [ "$site_rows" = "0" ] || fail_row R4 "site-staff-rows"
  [ "$site_pending" = "0" ] || fail_row R4 "site-staff-pending-rows"
  echo "R4_OK"
}

row_r5() {
  local out fails ok_lines
  out="$(run_test '^(TestRepairService|TestListTasks|TestRepair)')"
  fails="$(printf '%s\n' "$out" | grep -c -- '^--- FAIL' || true)"
  ok_lines="$(printf '%s\n' "$out" | grep -c -- '^--- PASS' || true)"
  echo "PASS_COUNT=$ok_lines"
  echo "FAIL_COUNT=$fails"
  if [ "$fails" != "0" ]; then
    printf '%s\n' "$out" | grep -- '^--- FAIL' | head -20
    fail_row R5 "existing-suite-failures"
  fi
  if ! printf '%s\n' "$out" | grep -q '^ok '; then
    printf '%s\n' "$out" | tail -30
    fail_row R5 "suite-not-ok"
  fi
  echo "REGRESSION_PASS=1"
  echo "R5_OK"
}

row_r6() {
  local build_rc site_eq_org site_filter gate_shared
  ( cd "$BACKEND_DIR" && go build ./... >/tmp/verify-2513-build.log 2>&1 )
  build_rc=$?
  echo "BUILD_RC=$build_rc"
  # 同类扫描：`site_id = ?`（ID 域误用）/ `site_id IS NULL`（旧谓词）在维修服务 handler 中必须为 0
  site_eq_org="$(grep -c 'site_id = ?' "$BACKEND_DIR/handlers/repair_service.go" || true)"
  site_filter="$(grep -c 'site_id IS NULL' "$BACKEND_DIR/handlers/repair_service.go" || true)"
  gate_shared="$(grep -c 'repairServiceMerchantLevel(db, ctx)' "$BACKEND_DIR/handlers/repair_service.go" || true)"
  echo "SITE_ID_EQ_ORG=$site_eq_org"
  echo "SITE_FILTER_IN_SERVICE=$site_filter"
  if [ "$gate_shared" = "2" ]; then
    echo "MERCHANT_GATE_SHARED=1"
  else
    echo "MERCHANT_GATE_SHARED=0"
  fi
  [ "$build_rc" = "0" ] || { tail -20 /tmp/verify-2513-build.log; fail_row R6 "build-failed"; }
  [ "$site_eq_org" = "0" ] || fail_row R6 "site-id-eq-org-residue"
  [ "$site_filter" = "0" ] || fail_row R6 "site-filter-residue"
  [ "$gate_shared" = "2" ] || fail_row R6 "merchant-gate-not-shared"
  echo "R6_OK"
}

row_gate() {
  local rc
  ( cd "$BACKEND_DIR" && go build ./... && go vet ./handlers/ && \
    go test ./handlers -run '^(TestRepairService|TestListTasks|TestRepair)' -count=1 ) >/tmp/verify-2513-gate.log 2>&1
  rc=$?
  echo "GATE_RC=$rc"
  if [ "$rc" != "0" ]; then
    tail -40 /tmp/verify-2513-gate.log
    echo "GATE_FAIL"
    exit 1
  fi
  grep -c -- '^--- PASS' /tmp/verify-2513-gate.log | sed 's/^/PASS_COUNT=/'
  echo "GATE_OK"
}

# 账本（evidence.json）锚点派生：禁止手改 build/commit/dirty（Round 1 审计 Bug 1）。
# 从最新 fix commit + artifact provenance 派生并写回，同时执行 R7 的可执行断言。
# 用法：ledger [--require-gate]（--require-gate 追加校验 gate 报告 provenance == fix commit）
row_ledger() {
  local require_gate="${1:-}"
  local outdir="$ROOT_DIR/docs/evidence/2513"
  local ev="$outdir/evidence.json"
  local gate="$outdir/pre-ready-2513.json"
  local want line f rid count total passcount gate_commit dirty_json
  want="# build=1.0.$SHORT # commit=$FIX_COMMIT # dirty=$DIRTY"

  if [ "$DIRTY" = "true" ]; then
    fail_row LEDGER "working-tree-dirty(git status must be clean except docs/evidence/2513, tmp/)"
  fi

  for f in R1-pending-dispatch.txt R2-list-tasks-site.txt R3-staff-allowed.txt \
           R4-negative.txt R5-regression.txt R6-build-scan.txt; do
    [ -f "$outdir/$f" ] || fail_row LEDGER "artifact-missing:$f"
    line="$(head -1 "$outdir/$f")"
    [ "$line" = "$want" ] || fail_row LEDGER "artifact-provenance-mismatch:$f"
  done
  echo "ARTIFACT_PROVENANCE=OK"

  if [ "$DIRTY" = "true" ]; then dirty_json=true; else dirty_json=false; fi
  if ! jq --arg build "1.0.$SHORT" --arg commit "$FIX_COMMIT" --argjson dirty "$dirty_json" \
      '.build = $build | .commit = $commit | .dirty = $dirty' "$ev" > "$ev.tmp.$$"; then
    rm -f "$ev.tmp.$$"
    fail_row LEDGER "evidence-json-rewrite-failed"
  fi
  mv "$ev.tmp.$$" "$ev"
  echo "LEDGER_BUILD=1.0.$SHORT"
  echo "LEDGER_COMMIT=$FIX_COMMIT"
  echo "LEDGER_DIRTY=$DIRTY"

  total="$(jq -r '.rows | length' "$ev")"
  passcount="$(jq -r '[.rows[] | select(.pass == true)] | length' "$ev")"
  for rid in R1 R2 R3 R4 R5 R6 R7; do
    count="$(jq -r --arg id "$rid" '[.rows[] | select(.id == $id)] | length' "$ev")"
    [ "$count" = "1" ] || fail_row LEDGER "row-${rid}-count=${count}"
  done
  [ "$passcount" = "$total" ] || fail_row LEDGER "rows-not-all-pass:${passcount}/${total}"
  echo "LEDGER_ROWS=$passcount/$total"

  [ -f "$gate" ] || fail_row LEDGER "gate-report-missing"
  # 可执行断言（Round 1 审计 Bug 1）：gate 报告顶层键是 overall
  jq -e '.overall == "PASS"' "$gate" >/dev/null || fail_row LEDGER "gate-not-pass"
  echo "GATE_OVERALL=PASS"

  if [ "$require_gate" = "--require-gate" ]; then
    gate_commit="$(jq -r '.provenance.commit // empty' "$gate")"
    [ "$gate_commit" = "$FIX_COMMIT" ] || fail_row LEDGER "gate-report-stale:${gate_commit:-missing}"
    echo "GATE_PROVENANCE_COMMIT=$gate_commit"
  fi
  echo "LEDGER_OK"
}

main() {
  local row="${1:-}"
  provenance
  case "$row" in
    r1) row_r1 ;;
    r2) row_r2 ;;
    r3) row_r3 ;;
    r4) row_r4 ;;
    r5) row_r5 ;;
    r6) row_r6 ;;
    gate) row_gate ;;
    ledger) row_ledger "${2:-}" ;;
    *)
      echo "usage: $0 r1|r2|r3|r4|r5|r6|gate|ledger [--require-gate]" >&2
      exit 2
      ;;
  esac
}

main "$@"
