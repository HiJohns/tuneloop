#!/usr/bin/env bash
# #2489 证据生成器 — 配置/定价/权限/商户/租户类页面的业务可纠正错误改持久提示。
#
# 用法：bash scripts/verify-2489.sh <r1..r10>
#   r1  docs/evidence/2489/R1-inventory.txt       判定表（只读扫描，6 文件每个 toast 一行）
#   r2  docs/evidence/2489/R2-perm.txt            权限模板页接线（静态结构断言）
#   r3  docs/evidence/2489/R3-pricing.txt         价格配置页接线
#   r4  docs/evidence/2489/R4-transit.txt         中转网点页接线
#   r5  docs/evidence/2489/R5-membership.txt      会员等级页接线
#   r6  docs/evidence/2489/R6-merchant.txt        商户管理页接线（#2459 语义：只消费共享判定）
#   r7  docs/evidence/2489/R7-tenant.txt          租户管理页接线
#   r8  docs/evidence/2489/R8-residual.txt        复用/残留度量（可失败）
#   r9  docs/evidence/2489/R9-build-lint.txt      PC 构建 + eslint（改动文件）
#   r10 docs/evidence/2489/R10-gate.txt           gate 报告 + plan-source 四源一致
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8|r9|r10) ;;
  *)
    echo "usage: $0 <r1..r10>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2489
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

SCAN="scripts/scan-form-toasts-2489.sh"
PERM_PAGE="frontend-pc/src/pages/admin/PermissionManage/index.jsx"
PRICING_PAGE="frontend-pc/src/pages/admin/pricing/MerchantPricingConfig.jsx"
TRANSIT_PAGE="frontend-pc/src/pages/admin/transit/TransitCenter.jsx"
MEMBERSHIP_PAGE="frontend-pc/src/pages/System/MembershipLevelsPage.jsx"
MERCHANT_PAGE="frontend-pc/src/pages/MerchantManagement.jsx"
TENANT_PAGE="frontend-pc/src/pages/TenantManagement.jsx"

require_tokens() {
  local log="$1"; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$log" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then
    echo "ASSERT_FAIL: missing tokens:$missing" >&2
    return 1
  fi
}
forbid() {
  if grep -Fq -- "$2" "$1"; then
    echo "ASSERT_FAIL: forbidden token present: [$2]" >&2
    return 1
  fi
}
forbid_re() {
  if grep -Eq -- "$2" "$1"; then
    echo "ASSERT_FAIL: forbidden pattern present: [$2]" >&2
    return 1
  fi
}

# wiring_metrics <page> —— 单页接线度量（静态）
wiring_metrics() {
  local page="$1"
  local import_shared=0 alert=0 predicate=0 text=0 clears=0 local_codes=0
  grep -q "from '.*components/FormConflictAlert'" "$page" && import_shared=1
  grep -q '<FormConflictAlert' "$page" && alert=1
  grep -q 'isBusinessCorrectableConflict(' "$page" && predicate=1
  grep -q 'conflictText(' "$page" && text=1
  clears=$(grep -cE "set[A-Za-z]*Conflict\(''\)" "$page" | tr -d ' ')
  grep -qE '\[[[:space:]]*40002|40004[[:space:]]*,[[:space:]]*40005' "$page" && local_codes=1
  printf 'SHARED_IMPORT=%s\nPERSIST_ALERT=%s\nCODE_FIRST=%s\nCONFLICT_TEXT=%s\nCLEAR_CALLS=%s\nLOCAL_CODE_LIST=%s\n' \
    "$import_shared" "$alert" "$predicate" "$text" "${clears:-0}" "$local_codes"
}

page_row() { # mode name page fallback-token...
  local name="$1" page="$2"
  { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    wiring_metrics "$page"
    echo "PAGE=$page"
  } > "$LOG" 2>&1
  require_tokens "$LOG" 'SHARED_IMPORT=1' 'PERSIST_ALERT=1' 'CODE_FIRST=1' 'CONFLICT_TEXT=1' 'LOCAL_CODE_LIST=0' || RC=1
  local clears
  clears=$(sed -n 's/^CLEAR_CALLS=//p' "$LOG" | tail -1)
  [ "${clears:-0}" -ge 2 ] || { echo "ASSERT_FAIL: CLEAR_CALLS<2 (${clears:-0})" >&2; RC=1; }
  forbid "$LOG" 'PERSIST_ALERT=0' || RC=1
}

LOG="$REPO/tmp/verify-2489-${MODE}.log"
: > "$LOG"
trap 'rm -f "$LOG"' EXIT
RC=0

case "$MODE" in
  r1)
    NAME="R1-inventory.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "SCAN_CMD=bash $SCAN"
      bash "$SCAN" 2>&1
      echo "SCAN_EXIT=$?"
    } > "$LOG" 2>&1
    grep -q '^SCAN_EXIT=0$' "$LOG" || RC=1
    require_tokens "$LOG" 'UNCLASSIFIED_ROWS=0' 'READONLY=1' 'DB_WRITES=0' \
      'SHARED_REUSE=6' 'BUSINESS_TOAST_RESIDUAL=0' 'LOCAL_CODE_LIST=0' || RC=1
    scan_rows=$(sed -n 's/^SCAN_ROWS=//p' "$LOG" | tail -1)
    [ "${scan_rows:-0}" -gt 0 ] || { echo "ASSERT_FAIL: SCAN_ROWS<=0" >&2; RC=1; }
    ;;
  r2) NAME="R2-perm.txt"; page_row r2 "$PERM_PAGE" ;;
  r3) NAME="R3-pricing.txt"; page_row r3 "$PRICING_PAGE" ;;
  r4) NAME="R4-transit.txt"; page_row r4 "$TRANSIT_PAGE" ;;
  r5) NAME="R5-membership.txt"; page_row r5 "$MEMBERSHIP_PAGE" ;;
  r6)
    NAME="R6-merchant.txt"
    page_row r6 "$MERCHANT_PAGE"
    # #2459 语义：只消费共享判定，不自写 code 白名单
    theme=0
    grep -q '2459' "$MERCHANT_PAGE" && theme=1
    { echo "THEME_2459=$theme"; } >> "$LOG"
    require_tokens "$LOG" 'THEME_2459=1' || RC=1
    ;;
  r7) NAME="R7-tenant.txt"; page_row r7 "$TENANT_PAGE" ;;
  r8)
    NAME="R8-residual.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      shared=0; local_dup=0; imports=0
      for p in "$PERM_PAGE" "$PRICING_PAGE" "$TRANSIT_PAGE" "$MEMBERSHIP_PAGE" "$MERCHANT_PAGE" "$TENANT_PAGE"; do
        grep -q '<FormConflictAlert' "$p" && shared=$((shared + 1))
        grep -q "from '.*components/FormConflictAlert'" "$p" && imports=$((imports + 1))
        grep -q '<Alert' "$p" && local_dup=$((local_dup + 1))
      done
      printf 'SHARED_REUSE=%s\nSHARED_IMPORTS=%s\nLOCAL_ALERT_DUP=%s\n' "$shared" "$imports" "$local_dup"
      bash "$SCAN" 2>&1 | grep -E '^(BUSINESS_TOAST_RESIDUAL|LOCAL_CODE_LIST|UNCLASSIFIED_ROWS|SCAN_ROWS|CLASSIFIED_ROWS|READONLY|DB_WRITES)='
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'SHARED_REUSE=6' 'SHARED_IMPORTS=6' 'LOCAL_ALERT_DUP=0' \
      'BUSINESS_TOAST_RESIDUAL=0' 'LOCAL_CODE_LIST=0' 'UNCLASSIFIED_ROWS=0' 'READONLY=1' 'DB_WRITES=0' || RC=1
    forbid "$LOG" 'LOCAL_ALERT_DUP=1' || RC=1
    ;;
  r9)
    NAME="R9-build-lint.txt"
    BUILD=0
    LINT=0
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      ( cd frontend-pc && npm run build ) 2>&1 | tail -10
      BUILD=${PIPESTATUS[0]}
      echo "PC_BUILD=$BUILD"
      echo "LINT_CMD=npx eslint 2489 changed files"
      ( cd frontend-pc && npx eslint \
          "src/pages/admin/PermissionManage/index.jsx" \
          "src/pages/admin/pricing/MerchantPricingConfig.jsx" \
          "src/pages/admin/transit/TransitCenter.jsx" \
          "src/pages/System/MembershipLevelsPage.jsx" \
          "src/pages/MerchantManagement.jsx" \
          "src/pages/TenantManagement.jsx" ) 2>&1 | tail -8
      LINT=${PIPESTATUS[0]}
      echo "LINT_EXIT=$LINT"
      [ "$BUILD" = "0" ] && [ "$LINT" = "0" ] && echo "BUILD_LINT_OK" || echo "BUILD_LINT_FAIL"
    } > "$LOG" 2>&1
    grep -q '^BUILD_LINT_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'PC_BUILD=0' 'LINT_EXIT=0' || RC=1
    forbid "$LOG" 'LINT_EXIT=1' || RC=1
    ;;
  r10)
    NAME="R10-gate.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      # 计划 v2 D6：本单是子任务（Parent: #2486），harness 的 P5 恒为 N/A，
      # 因此全量 PC 构建由本行**直接执行**并记录退出码（不再要求 FULL_SUITE_EXIT / P5=PASS）。
      echo "FULL_GATE_CMD=cd frontend-pc && npm run build"
      ( cd frontend-pc && npm run build ) 2>&1 | tail -8
      FGE="${PIPESTATUS[0]}"
      echo "FULL_GATE_DIRECT_EXIT=$FGE"
      [ "$FGE" = "0" ] || fail=1

      GATE="$OUT_DIR/pre-ready-$ISSUE.json"
      overall="$(jq -r '.overall // empty' "$GATE" 2>/dev/null)"
      if [ -f "$GATE" ] && [ "$overall" = "PASS" ]; then
        echo "GATE_RESULT=PASS"
      elif [ ! -f "$GATE" ]; then
        echo "GATE_RESULT=PENDING(no report yet)"; fail=1
      else
        echo "GATE_RESULT=MISSING_OR_FAIL(${overall:-missing})"; fail=1
      fi
      p5=$(jq -r '.checks[]? | select(.id=="P5") | .status' "$GATE" 2>/dev/null | head -1)
      echo "P5=${p5:-MISSING}"
      gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE" 2>/dev/null)"
      plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
      echo "GATE_PLAN_SHA=$gate_sha"
      if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
        echo "GATE_PLAN_SHA_MATCH=1"
      else
        echo "GATE_PLAN_SHA_MATCH=0"; fail=1
      fi
      rebuilt=0
      for f in R1-inventory.txt R2-perm.txt R3-pricing.txt R4-transit.txt R5-membership.txt \
               R6-merchant.txt R7-tenant.txt R8-residual.txt R9-build-lint.txt; do
        [ -f "$OUT_DIR/$f" ] || continue
        head -1 "$OUT_DIR/$f" | grep -q "commit=$FIX_SHA " && rebuilt=$((rebuilt + 1))
      done
      echo "REBUILT_ROWS=$((rebuilt + 1))"
      echo "REBUILT_OTHERS=$rebuilt"
      bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" > "$REPO/tmp/plan-source-2489.log" 2>&1
      grep '^PLAN_SOURCE:' "$REPO/tmp/plan-source-2489.log" | tail -6
      if grep -q 'PLAN_SOURCE: PASS' "$REPO/tmp/plan-source-2489.log"; then
        echo "PLAN_SOURCE=PASS"
      else
        echo "PLAN_SOURCE=FAIL"; fail=1
      fi
      [ "$fail" = "0" ] && echo "GATE_OK" || echo "GATE_FAIL"
    } > "$LOG" 2>&1
    grep -q '^GATE_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GATE_RESULT=PASS' 'FULL_GATE_DIRECT_EXIT=0' 'PLAN_SOURCE=PASS' \
      'GATE_PLAN_SHA_MATCH=1' 'REBUILT_ROWS=10' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
cp "$LOG" "$ART"
echo "ARTIFACT=$ART"
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)"
