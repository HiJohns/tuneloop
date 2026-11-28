#!/usr/bin/env bash
# #2489 证据生成器 —— 配置/定价/权限/商户类页面：业务可纠正错误 → 表单内持久提示。
#
# 用法：bash scripts/verify-2489.sh <r1|r2|r3|r4|r5|r6|r8|r9|r10|r11>
#   r1  docs/evidence/2489/R1-inventory.txt   按分支块的判定表（只读扫描）
#   r2  docs/evidence/2489/R2-perm.txt        权限模板页 keep 裁定（KEEP_ROWS=3）
#   r3  docs/evidence/2489/R3-pricing.txt     价格配置页 keep 裁定（KEEP_ROWS=1）
#   r4  docs/evidence/2489/R4-transit.txt     中转网点页 convert（PERSIST_ALERT/CODE_FIRST）
#   r5  docs/evidence/2489/R5-membership.txt  会员等级页 keep 裁定（KEEP_ROWS=2）
#   r6  docs/evidence/2489/R6-merchant.txt    商户管理页 convert（THEME_2459 / NO_FALSE_SUCCESS）
#   r8  docs/evidence/2489/R8-residual.txt    共享件复用与残留（SHARED_REUSE=2 / KEEP_PAGES_NO_ALERT=3）
#   r9  docs/evidence/2489/R9-build-lint.txt  PC 构建 + 定向 eslint
#   r10 docs/evidence/2489/R10-gate.txt       gate 报告 + plan-source + 本单直跑全量 PC 构建
#   r11 docs/evidence/2489/R11-transport.txt  传输层可执行断言（mock 400/409/500）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r8|r9|r10|r11) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r8|r9|r10|r11>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)" || exit 2
cd "$REPO" || exit 2
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
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

SCAN_BIN="$REPO/scripts/scan-form-toasts-2489.sh"
HEAVY_RUN="${OPENCODE_HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
SRC_DIR="frontend-pc/src"
PAGE_PERM="$SRC_DIR/pages/admin/PermissionManage/index.jsx"
PAGE_PRICING="$SRC_DIR/pages/admin/pricing/MerchantPricingConfig.jsx"
PAGE_TRANSIT="$SRC_DIR/pages/admin/transit/TransitCenter.jsx"
PAGE_MEMBER="$SRC_DIR/pages/System/MembershipLevelsPage.jsx"
PAGE_MERCHANT="$SRC_DIR/pages/MerchantManagement.jsx"
KEEP_PAGES=("$PAGE_PERM" "$PAGE_PRICING" "$PAGE_MEMBER")
BATCH_PAGES=("$PAGE_PERM" "$PAGE_PRICING" "$PAGE_TRANSIT" "$PAGE_MEMBER" "$PAGE_MERCHANT")

LOG="$REPO/tmp/verify-2489-${MODE}.log"
mkdir -p "$REPO/tmp"
: > "$LOG"
BODY="$OUT_DIR/.body.$$"
: > "$BODY"
trap 'rm -f "$LOG" "$BODY"' EXIT

emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact
  local name="$1"
  {
    printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    cat "$BODY"
  } > "$OUT_DIR/$name"
  cat "$OUT_DIR/$name"
  [ "$RC" = "0" ] || return 1
}
RC=0

# 页面是否消费共享件（FormConflictAlert / formConflict 判定器）。
page_shared_uses() { # file
  grep -cE "components/FormConflictAlert|utils/formConflict|isBusinessCorrectableConflict\(|conflictText\(|set[A-Za-z0-9_]*Conflict\(" "$1" 2>/dev/null || true
}
page_alerts() { # file — 本批页面不得自建 Alert 组件/直接渲染 antd Alert
  grep -cE '<Alert|function[[:space:]]+[A-Za-z0-9_]*Alert|const[[:space:]]+[A-Za-z0-9_]*Alert[[:space:]]*=' "$1" 2>/dev/null || true
}
page_code_lists() { # file — 不得复制 rubric code 白名单
  grep -cE '\[[[:space:]]*40002|(===?|==)[[:space:]]*(40002|40004|40005|40900)\b' "$1" 2>/dev/null || true
}

# 扫描输出缓存（r1/r2/r3/r5/r8 复用同一次只读扫描）。
scan_output() {
  [ -s "$REPO/tmp/scan-2489.cache" ] && return 0
  bash "$SCAN_BIN" > "$REPO/tmp/scan-2489.cache" 2>"$REPO/tmp/scan-2489.err"
}
scan_metric() { # key
  scan_output
  sed -n "s/^$1=//p" "$REPO/tmp/scan-2489.cache" | head -1
}
scan_page_keep() { # rel path key
  scan_output
  sed -n "s#^PAGE_KEEP_ROWS=$1=##p" "$REPO/tmp/scan-2489.cache" | head -1
}
scan_rows_for_page() { # rel path
  scan_output
  awk -F'\t' -v p="$1" 'index($1, p ":") == 1' "$REPO/tmp/scan-2489.cache"
}

# CODE_FIRST：每个 conflictText( 写入点都必须由 isBusinessCorrectableConflict( 分支包裹。
code_first_check() { # file
  awk '
    { line[NR] = $0 }
    END {
      ok = 0; bad = 0
      for (i = 1; i <= NR; i++) {
        if (line[i] ~ /conflictText\(/) {
          found = 0
          for (j = (i - 3 < 1 ? 1 : i - 3); j <= i; j++) {
            if (line[j] ~ /isBusinessCorrectableConflict\(/) found = 1
          }
          if (found) ok++; else bad++
        }
      }
      printf "OK=%d BAD=%d\n", ok, bad
    }
  ' "$1"
}

case "$MODE" in
  r1)
    if ! bash "$SCAN_BIN" > "$REPO/tmp/scan-2489.cache" 2>"$REPO/tmp/scan-2489.err"; then
      echo "ASSERT_FAIL: scanner failed" >&2; cat "$REPO/tmp/scan-2489.err" >&2; RC=1
    fi
    # WINDOW_HEURISTIC=0 的可判定证据：扫描脚本不含固定行窗口回溯。
    if grep -qE '(start|begin|win|window|ctx)[[:space:]]*=[[:space:]]*"?\$\(\([[:space:]]*line[[:space:]]*-[[:space:]]*1?[0-9][[:space:]]*\)\)' "$SCAN_BIN"; then
      emit "WINDOW_HEURISTIC_SOURCE=1"
      echo "ASSERT_FAIL: scanner still uses a fixed line window" >&2; RC=1
    else
      emit "WINDOW_HEURISTIC_SOURCE=0"
    fi
    sed -n '/^--- rows ---/q;p' "$REPO/tmp/scan-2489.cache" | while IFS= read -r l; do emit "$l"; done
    emit "--- rows ---"
    sed -n '/^--- rows ---/,$p' "$REPO/tmp/scan-2489.cache" | sed '1d' | while IFS= read -r l; do emit "$l"; done
    require 'UNCLASSIFIED=0'
    require 'READONLY=1'
    require 'DB_WRITES=0'
    require 'WINDOW_HEURISTIC=0'
    require 'WINDOW_HEURISTIC_SOURCE=0'
    [ "$(scan_metric SCAN_ROWS)" -gt 0 ] || { echo "ASSERT_FAIL: SCAN_ROWS=0" >&2; RC=1; }
    finish R1-inventory.txt || RC=1
    ;;

  r2)
    keep_n="$(scan_page_keep pages/admin/PermissionManage/index.jsx)"
    used="$(page_shared_uses "$PAGE_PERM")"
    emit "PAGE=pages/admin/PermissionManage/index.jsx"
    emit "KEEP_ROWS=${keep_n:-0}"
    emit "SHARED_USES=$used"
    emit "FALSE_CONVERT=$([ "${used:-0}" = "0" ] && echo 0 || echo 1)"
    emit "PERSIST_ALERT_UNUSED=$([ "${used:-0}" = "0" ] && echo 1 || echo 0)"
    emit "LOCAL_ALERT_DUP=$(page_alerts "$PAGE_PERM")"
    emit "LOCAL_CODE_LIST=$(page_code_lists "$PAGE_PERM")"
    emit "--- rows ---"
    scan_rows_for_page pages/admin/PermissionManage/index.jsx | sed 's/^/ROW /' | while IFS= read -r l; do emit "$l"; done
    nonkeep="$(scan_rows_for_page pages/admin/PermissionManage/index.jsx | grep -cv 'tier=keep:instant' || true)"
    emit "NON_KEEP_ROWS=${nonkeep:-0}"
    require "KEEP_ROWS=${keep_n:-0}"
    [ "${keep_n:-0}" = "3" ] || { echo "ASSERT_FAIL: KEEP_ROWS!=3" >&2; RC=1; }
    [ "${used:-0}" = "0" ] || { echo "ASSERT_FAIL: keep page consumes shared alert (false convert)" >&2; RC=1; }
    [ "${nonkeep:-0}" = "0" ] || { echo "ASSERT_FAIL: keep page still has non-keep rows" >&2; RC=1; }
    finish R2-perm.txt || RC=1
    ;;

  r3)
    keep_n="$(scan_page_keep pages/admin/pricing/MerchantPricingConfig.jsx)"
    used="$(page_shared_uses "$PAGE_PRICING")"
    emit "PAGE=pages/admin/pricing/MerchantPricingConfig.jsx"
    emit "KEEP_ROWS=${keep_n:-0}"
    emit "SHARED_USES=$used"
    emit "FALSE_CONVERT=$([ "${used:-0}" = "0" ] && echo 0 || echo 1)"
    emit "PERSIST_ALERT_UNUSED=$([ "${used:-0}" = "0" ] && echo 1 || echo 0)"
    emit "LOCAL_ALERT_DUP=$(page_alerts "$PAGE_PRICING")"
    emit "LOCAL_CODE_LIST=$(page_code_lists "$PAGE_PRICING")"
    emit "--- rows ---"
    scan_rows_for_page pages/admin/pricing/MerchantPricingConfig.jsx | sed 's/^/ROW /' | while IFS= read -r l; do emit "$l"; done
    keep_rows="$(scan_rows_for_page pages/admin/pricing/MerchantPricingConfig.jsx | grep -c 'tier=keep:instant' || true)"
    emit "KEEP_TIER_ROWS=${keep_rows:-0}"
    [ "${keep_n:-0}" = "1" ] || { echo "ASSERT_FAIL: KEEP_ROWS!=1" >&2; RC=1; }
    [ "${used:-0}" = "0" ] || { echo "ASSERT_FAIL: keep page consumes shared alert (false convert)" >&2; RC=1; }
    require 'KEEP_ROWS=1'
    finish R3-pricing.txt || RC=1
    ;;

  r4)
    shared="$(page_shared_uses "$PAGE_TRANSIT")"
    cf="$(code_first_check "$PAGE_TRANSIT")"
    ok="${cf##*OK=}"; ok="${ok%% *}"
    bad="${cf##*BAD=}"
    emit "PAGE=pages/admin/transit/TransitCenter.jsx"
    emit "SHARED_USES=$shared"
    emit "PERSIST_ALERT=$([ "${shared:-0}" -gt 0 ] && echo 1 || echo 0)"
    emit "CONFLICT_TEXT_SITES=$ok"
    emit "UNGUARDED_SITES=${bad:-0}"
    emit "CODE_FIRST=$([ "${bad:-0}" = "0" ] && [ "${ok:-0}" -gt 0 ] && echo 1 || echo 0)"
    emit "--- wiring ---"
    grep -nE "FormConflictAlert|isBusinessCorrectableConflict\(|conflictText\(|set(Site|Route)Conflict\(" "$PAGE_TRANSIT" | sed 's/^/WIRE /' | while IFS= read -r l; do emit "$l"; done
    require 'PERSIST_ALERT=1'
    require 'CODE_FIRST=1'
    finish R4-transit.txt || RC=1
    ;;

  r5)
    keep_n="$(scan_page_keep pages/System/MembershipLevelsPage.jsx)"
    used="$(page_shared_uses "$PAGE_MEMBER")"
    emit "PAGE=pages/System/MembershipLevelsPage.jsx"
    emit "KEEP_ROWS=${keep_n:-0}"
    emit "SHARED_USES=$used"
    emit "FALSE_CONVERT=$([ "${used:-0}" = "0" ] && echo 0 || echo 1)"
    emit "PERSIST_ALERT_UNUSED=$([ "${used:-0}" = "0" ] && echo 1 || echo 0)"
    # 门槛/金额规则在前端 Form rules（后端只有 JSON binding 40002）→ 可判定的正向证据。
    front_rule="$(grep -cE 'rules=\{\[\{[^]]*required|min_amount' "$PAGE_MEMBER" || true)"
    backend_rubric="$(grep -cE '40900|40004|40005' backend/handlers/membership_level.go 2>/dev/null || true)"
    emit "FRONTEND_RULE_SITES=${front_rule:-0}"
    emit "BACKEND_RUBRIC_CODES=${backend_rubric:-0}"
    emit "FRONTEND_ONLY_RULE=$([ "${front_rule:-0}" -gt 0 ] && [ "${backend_rubric:-0}" = "0" ] && echo 1 || echo 0)"
    emit "--- rows ---"
    scan_rows_for_page pages/System/MembershipLevelsPage.jsx | sed 's/^/ROW /' | while IFS= read -r l; do emit "$l"; done
    [ "${keep_n:-0}" = "2" ] || { echo "ASSERT_FAIL: KEEP_ROWS!=2" >&2; RC=1; }
    [ "${used:-0}" = "0" ] || { echo "ASSERT_FAIL: keep page consumes shared alert (false convert)" >&2; RC=1; }
    require 'KEEP_ROWS=2'
    require 'FRONTEND_ONLY_RULE=1'
    finish R5-membership.txt || RC=1
    ;;

  r6)
    shared="$(page_shared_uses "$PAGE_MERCHANT")"
    cf="$(code_first_check "$PAGE_MERCHANT")"
    ok="${cf##*OK=}"; ok="${ok%% *}"
    bad="${cf##*BAD=}"
    theme_ref="$(grep -c '#2459' "$PAGE_MERCHANT" || true)"
    local_whitelist="$(grep -cE '(===?|==)[[:space:]]*(40002|40004|40005|40900)\b' "$PAGE_MERCHANT" || true)"
    # 单一成功出口：handleSubmit 内 message.success 只在 `resp.code !== 20000` 守卫之后。
    success_check="$(awk '
      /const handleSubmit = async/ { infn = 1 }
      infn && /resp\.code !== 20000/ { if (!guard) guard = NR }
      infn && /message\.success\(/ { if (!succ) succ = NR; succn++ }
      infn && /^  };/ { infn = 0 }
      END { printf "guard=%d success=%d count=%d\n", guard + 0, succ + 0, succn + 0 }
    ' "$PAGE_MERCHANT")"
    guard_line="$(printf '%s' "$success_check" | sed -n 's/.*guard=\([0-9]*\).*/\1/p')"
    succ_line="$(printf '%s' "$success_check" | sed -n 's/.*success=\([0-9]*\).*/\1/p')"
    succ_count="$(printf '%s' "$success_check" | sed -n 's/.*count=\([0-9]*\).*/\1/p')"
    emit "PAGE=pages/MerchantManagement.jsx"
    emit "SHARED_USES=$shared"
    emit "PERSIST_ALERT=$([ "${shared:-0}" -gt 0 ] && echo 1 || echo 0)"
    emit "CONFLICT_TEXT_SITES=$ok"
    emit "UNGUARDED_SITES=${bad:-0}"
    emit "CODE_FIRST=$([ "${bad:-0}" = "0" ] && [ "${ok:-0}" -gt 0 ] && echo 1 || echo 0)"
    emit "THEME_2459_REF=${theme_ref:-0}"
    emit "LOCAL_CODE_WHITELIST=${local_whitelist:-0}"
    emit "THEME_2459=$([ "${theme_ref:-0}" -gt 0 ] && [ "${local_whitelist:-0}" = "0" ] && echo 1 || echo 0)"
    emit "HANDLE_SUBMIT_GUARD_LINE=${guard_line:-0}"
    emit "HANDLE_SUBMIT_SUCCESS_LINE=${succ_line:-0}"
    emit "HANDLE_SUBMIT_SUCCESS_COUNT=${succ_count:-0}"
    emit "NO_FALSE_SUCCESS=$([ "${guard_line:-0}" -gt 0 ] && [ "${succ_line:-0}" -gt "${guard_line:-0}" ] && [ "${succ_count:-0}" = "1" ] && echo 1 || echo 0)"
    # 改名 409 语义由后端 #2459 决定（本单不自写白名单）→ 显式标注延后项。
    emit "DEFER_2459=$([ "${theme_ref:-0}" -gt 0 ] && [ "${local_whitelist:-0}" = "0" ] && echo 1 || echo 0)"
    emit "--- wiring ---"
    grep -nE "FormConflictAlert|isBusinessCorrectableConflict\(|conflictText\(|resp\.code !== 20000|#2459" "$PAGE_MERCHANT" | sed 's/^/WIRE /' | while IFS= read -r l; do emit "$l"; done
    require 'PERSIST_ALERT=1'
    require 'THEME_2459=1'
    require 'CODE_FIRST=1'
    require 'NO_FALSE_SUCCESS=1'
    require 'DEFER_2459=1'
    finish R6-merchant.txt || RC=1
    ;;

  r8)
    reuse=0; keep_no_alert=0; alert_dup=0; code_list=0
    for f in "${BATCH_PAGES[@]}"; do
      u="$(page_shared_uses "$f")"
      [ "${u:-0}" -gt 0 ] && reuse=$((reuse + 1))
      alert_dup=$((alert_dup + $(page_alerts "$f")))
      code_list=$((code_list + $(page_code_lists "$f")))
      emit "PAGE=$f shared_uses=${u:-0}"
    done
    for f in "${KEEP_PAGES[@]}"; do
      u="$(page_shared_uses "$f")"
      [ "${u:-0}" = "0" ] && keep_no_alert=$((keep_no_alert + 1))
    done
    residual="$(scan_metric RESIDUAL_ROWS)"
    unclassified="$(scan_metric UNCLASSIFIED)"
    emit "SHARED_REUSE=$reuse"
    emit "KEEP_PAGES_NO_ALERT=$keep_no_alert"
    emit "LOCAL_ALERT_DUP=$alert_dup"
    emit "LOCAL_CODE_LIST=$code_list"
    emit "BUSINESS_TOAST_RESIDUAL=${residual:-0}"
    emit "UNCLASSIFIED=${unclassified:-0}"
    emit "SHARED_OWNER=frontend-pc/src/components/FormConflictAlert.jsx + frontend-pc/src/utils/formConflict.js"
    [ "$reuse" = "2" ] || { echo "ASSERT_FAIL: SHARED_REUSE!=2" >&2; RC=1; }
    [ "$keep_no_alert" = "3" ] || { echo "ASSERT_FAIL: KEEP_PAGES_NO_ALERT!=3" >&2; RC=1; }
    [ "${residual:-0}" = "0" ] || { echo "ASSERT_FAIL: BUSINESS_TOAST_RESIDUAL!=0" >&2; RC=1; }
    require 'SHARED_REUSE=2'
    require 'KEEP_PAGES_NO_ALERT=3'
    require 'BUSINESS_TOAST_RESIDUAL=0'
    finish R8-residual.txt || RC=1
    ;;

  r9)
    BUILD_LOG="$REPO/tmp/verify-2489-r9-build.log"
    if [ -x "$HEAVY_RUN" ]; then
      ( cd frontend-pc && "$HEAVY_RUN" -- npm run build ) > "$BUILD_LOG" 2>&1
    else
      ( cd frontend-pc && npm run build ) > "$BUILD_LOG" 2>&1
    fi
    BUILD_EXIT=$?
    emit "BUILD_CMD=cd frontend-pc && npm run build"
    emit "PC_BUILD=$BUILD_EXIT"
    tail -3 "$BUILD_LOG" | sed 's/^/BUILD_TAIL: /' | while IFS= read -r l; do emit "$l"; done

    LINT_LOG="$REPO/tmp/verify-2489-r9-lint.log"
    LINT_FILES=(
      "src/pages/MerchantManagement.jsx"
      "src/pages/admin/transit/TransitCenter.jsx"
    )
    if [ -x "$HEAVY_RUN" ]; then
      ( cd frontend-pc && "$HEAVY_RUN" -- npx eslint "${LINT_FILES[@]}" ) > "$LINT_LOG" 2>&1
    else
      ( cd frontend-pc && npx eslint "${LINT_FILES[@]}" ) > "$LINT_LOG" 2>&1
    fi
    LINT_EXIT=$?
    emit "LINT_CMD=cd frontend-pc && npx eslint ${LINT_FILES[*]}"
    emit "LINT_EXIT=$LINT_EXIT"
    tail -5 "$LINT_LOG" | sed 's/^/LINT_TAIL: /' | while IFS= read -r l; do emit "$l"; done
    [ "$BUILD_EXIT" = "0" ] || { echo "ASSERT_FAIL: build failed" >&2; RC=1; }
    [ "$LINT_EXIT" = "0" ] || { echo "ASSERT_FAIL: eslint failed" >&2; RC=1; }
    require 'PC_BUILD=0'
    require 'LINT_EXIT=0'
    finish R9-build-lint.txt || RC=1
    ;;

  r10)
    REPORT="$OUT_DIR/pre-ready-$ISSUE.json"
    PLAN_FILE="$OUT_DIR/plan.md"
    PLAN_SHA="$(sha256sum "$PLAN_FILE" 2>/dev/null | awk '{print $1}')"
    emit "PLAN_FILE=docs/evidence/$ISSUE/plan.md"
    emit "PLAN_SHA256=${PLAN_SHA:-missing}"
    emit "GATE_REPORT=docs/evidence/$ISSUE/pre-ready-$ISSUE.json"
    if [ -f "$REPORT" ]; then
      GATE_OVERALL="$(jq -r '.overall // empty' "$REPORT" 2>/dev/null)"
      GATE_PLAN_SHA="$(jq -r '.plan_sha256 // empty' "$REPORT" 2>/dev/null)"
      GATE_P5="$(jq -r '(.checks // [])[] | select(.id == "P5") | .status' "$REPORT" 2>/dev/null | head -1)"
      emit "GATE_OVERALL=${GATE_OVERALL:-missing}"
      emit "GATE_PLAN_SHA256=${GATE_PLAN_SHA:-missing}"
      emit "GATE_P5=${GATE_P5:-missing} (D6: sub-task full gate belongs to parent #2486)"
      if [ "$GATE_OVERALL" = "PASS" ] && [ "$GATE_PLAN_SHA" = "$PLAN_SHA" ]; then
        emit "GATE_RESULT=PASS"
      else
        emit "GATE_RESULT=FAIL"
        echo "ASSERT_FAIL: gate report is not PASS for the current plan (overall=${GATE_OVERALL:-missing})" >&2
        RC=1
      fi
    else
      emit "GATE_OVERALL=missing"
      emit "GATE_RESULT=FAIL"
      echo "ASSERT_FAIL: gate report missing: $REPORT" >&2
      RC=1
    fi

    PLAN_SOURCE_OUT="$(bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" 2>&1)"
    emit "--- plan-source ---"
    printf '%s\n' "$PLAN_SOURCE_OUT" | sed 's/^/PLAN_SOURCE_LINE: /' | while IFS= read -r l; do emit "$l"; done
    if printf '%s\n' "$PLAN_SOURCE_OUT" | grep -q '^PLAN_SOURCE: PASS'; then
      emit "PLAN_SOURCE=PASS"
    else
      emit "PLAN_SOURCE=FAIL"
      echo "ASSERT_FAIL: plan-source did not report PASS" >&2
      RC=1
    fi

    DRIFT="$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')"
    emit "TREE_CLEAN=$([ -z "$DRIFT" ] && echo 1 || echo 0)"
    [ -z "$DRIFT" ] || { echo "ASSERT_FAIL: working tree not clean for direct full gate" >&2; RC=1; }

    FULL_LOG="$REPO/tmp/verify-2489-r10-build.log"
    if [ -x "$HEAVY_RUN" ]; then
      ( cd frontend-pc && "$HEAVY_RUN" -- npm run build ) > "$FULL_LOG" 2>&1
    else
      ( cd frontend-pc && npm run build ) > "$FULL_LOG" 2>&1
    fi
    FULL_EXIT=$?
    emit "FULL_GATE_CMD=cd frontend-pc && npm run build"
    emit "FULL_GATE_DIRECT_EXIT=$FULL_EXIT"
    tail -3 "$FULL_LOG" | sed 's/^/FULL_GATE_TAIL: /' | while IFS= read -r l; do emit "$l"; done
    [ "$FULL_EXIT" = "0" ] || { echo "ASSERT_FAIL: full PC build failed" >&2; RC=1; }
    require 'GATE_RESULT=PASS'
    require 'PLAN_SOURCE=PASS'
    require 'FULL_GATE_DIRECT_EXIT=0'
    finish R10-gate.txt || RC=1
    ;;

  r11)
    NODE_BIN="${NODE_BIN:-node}"
    TEST_FILE="$REPO/tmp/verify-2489-r11.mjs"
    cat > "$TEST_FILE" <<'EOF'
import { resolveHttpFailure } from '../frontend-pc/src/services/apiError.js'

const out = []
out.push(`API_ERROR_CODE=${(() => {
  const r = resolveHttpFailure(400, { code: 40002, message: '信息不完整，请补充必填项' })
  return r.kind === 'throw' && r.value.name === 'ApiError' && r.value.code === 40002 && r.value.status === 400 ? 1 : 0
})()}`)

// 409：必须返回对象且 code 非 20000（否则页面会落到 message.success = 假成功）
const r409 = resolveHttpFailure(409, { code: 40900, message: '商户名称已存在' })
out.push(`NO_SUCCESS_ON_409=${r409.kind === 'result' && r409.value.code !== 20000 && r409.value.code === 40900 ? 1 : 0}`)
out.push(`CONFLICT_TEXT_409=${r409.value.message}`)

// 500：抛错且不带 code → 页面只能走 toast（瞬时提示）
const r500 = resolveHttpFailure(500, { message: 'internal error' })
out.push(`TOAST_ON_500=${r500.kind === 'throw' && r500.value.code === undefined ? 1 : 0}`)

// 损坏 payload 不得抛解析异常
const broken = resolveHttpFailure(400, 'not-an-object')
out.push(`BROKEN_PAYLOAD_SAFE=${broken.kind === 'throw' ? 1 : 0}`)

// 网络失败：fetch reject 不会进入 resolveHttpFailure（无响应）→ 页面 catch 走 toast
out.push('NETWORK_PATH=throw-without-response')
console.log(out.join('\n'))
EOF
    ( cd "$REPO/tmp" && "$NODE_BIN" "$TEST_FILE" ) > "$REPO/tmp/verify-2489-r11.out" 2>&1
    NODE_EXIT=$?
    emit "TRANSPORT_UNDER_TEST=frontend-pc/src/services/apiError.js::resolveHttpFailure (consumed by api.js)"
    emit "NODE_EXIT=$NODE_EXIT"
    while IFS= read -r l; do emit "$l"; done < "$REPO/tmp/verify-2489-r11.out"
    [ "$NODE_EXIT" = "0" ] || { echo "ASSERT_FAIL: node transport test failed" >&2; RC=1; }
    require 'API_ERROR_CODE=1'
    require 'NO_SUCCESS_ON_409=1'
    require 'TOAST_ON_500=1'
    finish R11-transport.txt || RC=1
    ;;
esac

exit "$RC"
