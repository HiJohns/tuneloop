#!/usr/bin/env bash
# #2488 证据生成器 — 内容/展示类页面的业务可纠正错误改持久提示。
#
# 用法：bash scripts/verify-2488.sh <r1..r7>
#   r1 docs/evidence/2488/R1-inventory.txt   判定表（只读扫描：tier + reachable + reason）
#   r2 docs/evidence/2488/R2-banner.txt      轮播配置页接线（含 CATCH_MESSAGE / CLEAR_FIELDS）
#   r3 docs/evidence/2488/R3-mall-image.txt  商城图片页接线（同 R2）
#   r4 docs/evidence/2488/R4-stock.txt       库存操作页接线（STATE_CONFLICT / CLEAR_FIELDS）
#   r5 docs/evidence/2488/R5-residual.txt    复用/残留度量 + 缺陷注入红绿（RESIDUAL_INJECT）
#   r6 docs/evidence/2488/R6-build-lint.txt  PC 构建 + eslint（改动文件）
#   r7 docs/evidence/2488/R7-gate.txt        gate 报告 + plan-source 四源一致
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7) ;;
  *)
    echo "usage: $0 <r1..r7>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
mkdir -p "$REPO/tmp"   # 干净 worktree 无 tmp/（.gitignore），日志重定向需要它
ISSUE=2488
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

SCAN="scripts/scan-form-toasts-2488.sh"
BANNER_PAGE="frontend-pc/src/pages/System/BannerManagePage.jsx"
MALLIMAGE_PAGE="frontend-pc/src/pages/System/MallImageManageCard.jsx"
STOCK_PAGE="frontend-pc/src/pages/InstrumentStock.jsx"
SHARED_FORM_CONFLICT="frontend-pc/src/utils/formConflict.js"
API_JS="frontend-pc/src/services/api.js"
SRC_ROOT="frontend-pc/src"

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

# clear_metrics <page> —— CLEAR_FIELDS 覆盖率（#2488 Rev.5）
#   一个 Modal 内 <Form> 的**可编辑具名字段**数 = 分母；该 <Form> 挂
#   onValuesChange 清空即为分子（一次覆盖全部字段）。
#   这是可证伪的：删掉 onValuesChange 只留单字段 onChange → 覆盖率掉到 1/N。
clear_metrics() {
  awk '
    BEGIN { inblock = 0; forms = 0; fields = 0; covered = 0; hooks = 0 }
    {
      line = $0
      if (inblock == 0 && line ~ /<Form[[:space:]]/) {
        inblock = 1; n = 0; hook = (line ~ /onValuesChange/) ? 1 : 0; next
      }
      if (inblock == 1) {
        if (line ~ /<\/Form>/) {
          forms++; fields += n
          if (hook == 1) { hooks++; covered += n }
          inblock = 0; next
        }
        n += gsub(/name="/, "", line)
      }
    }
    END {
      printf "CLEAR_FIELDS=%d/%d\nCLEAR_FORM_HOOK=%d\nCLEAR_FORMS=%d\n", covered, fields, hooks, forms
    }
  ' "$1"
}

# catch_metrics <page> —— 非业务 ApiError 是否透出后端 message（#2488 Rev.5 Bug B）
#   CATCH_MESSAGE=1           catch 分支使用 `e instanceof ApiError && e.message ? e.message : …
#   CATCH_NETWORK_FALLBACK=1  非 ApiError（网络/未知）仍有中文兜底，不直出技术串
catch_metrics() {
  local page="$1" msg=0 fb=0
  grep -qE 'e instanceof ApiError && e\.message \? e\.message :' "$page" && msg=1
  grep -qE "e instanceof ApiError && e\.message \? e\.message : '[^']+'" "$page" && fb=1
  printf 'CATCH_MESSAGE=%s\nCATCH_NETWORK_FALLBACK=%s\n' "$msg" "$fb"
}

# wiring_metrics <page> —— 单页接线度量（静态）
wiring_metrics() {
  local page="$1"
  local import_shared=0 alert=0 predicate=0 text=0 clears=0 local_codes=0 api_error=0
  grep -q "from '.*components/FormConflictAlert'" "$page" && import_shared=1
  grep -q '<FormConflictAlert' "$page" && alert=1
  grep -q 'isBusinessCorrectableConflict(' "$page" && predicate=1
  grep -q 'conflictText(' "$page" && text=1
  clears=$(grep -cE "set[A-Za-z]*Conflict\(''\)" "$page" | tr -d ' ')
  grep -qE '\[[[:space:]]*40002|40004[[:space:]]*,[[:space:]]*40005' "$page" && local_codes=1
  # #2488 D7（v4）：传输层必须把非 2xx 的 code 带到页面，否则 400+40002 不可达。
  #   ① api.js 定义并抛出 ApiError{code,...}；② 页面 catch 用同一 rubric 判定。
  if grep -q 'class ApiError' "$API_JS" \
     && grep -q 'throw new ApiError' "$API_JS" \
     && grep -qE 'isBusinessCorrectableConflict\(e\)' "$page"; then
    api_error=1
  fi
  printf 'SHARED_IMPORT=%s\nPERSIST_ALERT=%s\nCODE_FIRST=%s\nCONFLICT_TEXT=%s\nCLEAR_CALLS=%s\nLOCAL_CODE_LIST=%s\nAPI_ERROR_CODE=%s\n' \
    "$import_shared" "$alert" "$predicate" "$text" "${clears:-0}" "$local_codes" "$api_error"
}

page_row() { # name page [extra-required-token...]
  local name="$1" page="$2"; shift 2
  local -a extra=("$@")
  { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    wiring_metrics "$page"
    clear_metrics "$page"
    case "$page" in
      "$STOCK_PAGE") ;;
      *) catch_metrics "$page" ;;
    esac
    echo "PAGE=$page"
  } > "$LOG" 2>&1
  require_tokens "$LOG" 'SHARED_IMPORT=1' 'PERSIST_ALERT=1' 'CODE_FIRST=1' 'CONFLICT_TEXT=1' 'LOCAL_CODE_LIST=0' || RC=1
  [ "${#extra[@]}" -gt 0 ] && { require_tokens "$LOG" "${extra[@]}" || RC=1; }
  local clears
  clears=$(sed -n 's/^CLEAR_CALLS=//p' "$LOG" | tail -1)
  [ "${clears:-0}" -ge 2 ] || { echo "ASSERT_FAIL: CLEAR_CALLS<2 (${clears:-0})" >&2; RC=1; }
  forbid "$LOG" 'PERSIST_ALERT=0' || RC=1
  forbid "$LOG" 'CLEAR_FIELDS=0/' || RC=1
}

LOG="$REPO/tmp/verify-2488-${MODE}.log"
: > "$LOG"
INJ_ROOT="$REPO/tmp/verify-2488-inject"
trap 'rm -f "$LOG"; rm -rf "$INJ_ROOT"' EXIT
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
      'SHARED_REUSE=3' 'BUSINESS_TOAST_RESIDUAL=0' 'LOCAL_CODE_LIST=0' \
      'UNREACHABLE_CLAIMED=0' 'DEFER_ROWS=0' 'RESIDUAL_CODE_SITES=0' 'RESIDUAL_PREDICATE_SITES=0' || RC=1
    scan_rows=$(sed -n 's/^SCAN_ROWS=//p' "$LOG" | tail -1)
    [ "${scan_rows:-0}" -gt 0 ] || { echo "ASSERT_FAIL: SCAN_ROWS<=0" >&2; RC=1; }
    reachable_rows=$(grep -cE '^pages/.*reachable=yes' "$LOG")
    [ "${reachable_rows:-0}" = "${scan_rows:-0}" ] \
      || { echo "ASSERT_FAIL: reachable rows ${reachable_rows:-0} != scan rows ${scan_rows:-0}" >&2; RC=1; }
    ;;
  r2)
    NAME="R2-banner.txt"
    page_row r2 "$BANNER_PAGE" 'API_ERROR_CODE=1' 'CATCH_MESSAGE=1' 'CATCH_NETWORK_FALLBACK=1' \
      'CLEAR_FIELDS=4/4' 'CLEAR_FORM_HOOK=1'
    ;;
  r3)
    NAME="R3-mall-image.txt"
    page_row r3 "$MALLIMAGE_PAGE" 'API_ERROR_CODE=1' 'CATCH_MESSAGE=1' 'CATCH_NETWORK_FALLBACK=1' \
      'CLEAR_FIELDS=3/3' 'CLEAR_FORM_HOOK=1'
    ;;
  r4)
    NAME="R4-stock.txt"
    page_row r4 "$STOCK_PAGE" 'CLEAR_FIELDS=10/10' 'CLEAR_FORM_HOOK=3'
    # #2488 D4（v4 修正）：库存冲突的真实可失败入口是 HTTP 409 + 40900（丢失登记/恢复），
    # 不再使用不存在的 40005 fixture；同时断言传输层抛出的错误也被同一 rubric 接住。
    state_conflict=0
    grep -q 'isBusinessCorrectableConflict(resp)' "$STOCK_PAGE" \
      && grep -q 'isBusinessCorrectableConflict(e)' "$STOCK_PAGE" \
      && grep -q '40900' "$SHARED_FORM_CONFLICT" \
      && state_conflict=1
    { echo "STATE_CONFLICT=$state_conflict"; } >> "$LOG"
    require_tokens "$LOG" 'STATE_CONFLICT=1' || RC=1
    ;;
  r5)
    NAME="R5-residual.txt"
    # 缺陷注入红绿（#2488 Rev.5）：在 tmp 副本里把 rubric 分支还原成 toast
    # （predicate 型）或追加一条旧式 code 型 toast，探针必须分别抬高对应计数。
    rm -rf "$INJ_ROOT"; mkdir -p "$INJ_ROOT/predicate" "$INJ_ROOT/code"
    cp -r "$SRC_ROOT" "$INJ_ROOT/predicate/src"
    cp -r "$SRC_ROOT" "$INJ_ROOT/code/src"
    # predicate 型：持久写入回退为 toast（只改 tmp 副本，不动工作树）
    find "$INJ_ROOT/predicate/src" -name '*.jsx' -print0 \
      | xargs -0 perl -pi -e 's/set[A-Za-z]*Conflict\(conflictText\(/message.error(conflictText(/g'
    # code 型：追加一条「硬编码业务码 → toast」的旧式分支
    cat >> "$INJ_ROOT/code/src/pages/InstrumentStock.jsx" <<'INJECT'

// injected legacy residual (code-style) — scan-only fixture, never built
const __injectedLegacyToast = (res) => {
  if (res.code === 40002) {
    message.error(res.message || '注入故障')
  }
}
INJECT

    clean_res=0; clean_code=0; clean_pred=0
    inj_pred_res=0; inj_pred_code=0; inj_pred_pred=0
    inj_code_res=0; inj_code_code=0; inj_code_pred=0
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      shared=0; imports=0
      for p in "$BANNER_PAGE" "$MALLIMAGE_PAGE" "$STOCK_PAGE"; do
        grep -q '<FormConflictAlert' "$p" && shared=$((shared + 1))
        grep -q "from '.*components/FormConflictAlert'" "$p" && imports=$((imports + 1))
      done
      # LOCAL_ALERT_DUP 用扫描脚本的精确口径（只算「把冲突文案渲染进 antd Alert」的本地重复实现，
      # 页面里与本 Issue 无关的说明性 Alert 不计入）。
      printf 'SHARED_REUSE=%s\nSHARED_IMPORTS=%s\n' "$shared" "$imports"
      echo "CLEAN_SCAN:"
      bash "$SCAN" 2>&1 | grep -E '^(BUSINESS_TOAST_RESIDUAL|RESIDUAL_CODE_SITES|RESIDUAL_PREDICATE_SITES|LOCAL_CODE_LIST|LOCAL_ALERT_DUP|UNCLASSIFIED_ROWS|SCAN_ROWS|CLASSIFIED_ROWS|READONLY|DB_WRITES)='
      echo "INJECT_PREDICATE_SCAN:"
      SCAN_SRC_DIR="$INJ_ROOT/predicate/src" bash "$SCAN" 2>&1 \
        | grep -E '^(BUSINESS_TOAST_RESIDUAL|RESIDUAL_CODE_SITES|RESIDUAL_PREDICATE_SITES)='
      echo "INJECT_CODE_SCAN:"
      SCAN_SRC_DIR="$INJ_ROOT/code/src" bash "$SCAN" 2>&1 \
        | grep -E '^(BUSINESS_TOAST_RESIDUAL|RESIDUAL_CODE_SITES|RESIDUAL_PREDICATE_SITES)='
    } > "$LOG" 2>&1

    clean_res=$(awk '/^CLEAN_SCAN:/{f=1} f&&/^BUSINESS_TOAST_RESIDUAL=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    clean_code=$(awk '/^CLEAN_SCAN:/{f=1} f&&/^RESIDUAL_CODE_SITES=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    clean_pred=$(awk '/^CLEAN_SCAN:/{f=1} f&&/^RESIDUAL_PREDICATE_SITES=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    inj_pred_res=$(awk '/^INJECT_PREDICATE_SCAN:/{f=1} f&&/^BUSINESS_TOAST_RESIDUAL=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    inj_pred_code=$(awk '/^INJECT_PREDICATE_SCAN:/{f=1} f&&/^RESIDUAL_CODE_SITES=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    inj_pred_pred=$(awk '/^INJECT_PREDICATE_SCAN:/{f=1} f&&/^RESIDUAL_PREDICATE_SITES=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    inj_code_res=$(awk '/^INJECT_CODE_SCAN:/{f=1} f&&/^BUSINESS_TOAST_RESIDUAL=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    inj_code_code=$(awk '/^INJECT_CODE_SCAN:/{f=1} f&&/^RESIDUAL_CODE_SITES=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")
    inj_code_pred=$(awk '/^INJECT_CODE_SCAN:/{f=1} f&&/^RESIDUAL_PREDICATE_SITES=/{sub(/^[^=]*=/,"");print;exit}' "$LOG")

    residual_inject=0
    [ "${clean_res:-1}" = "0" ] && [ "${inj_pred_res:-0}" -gt 0 ] && residual_inject=1
    split=0
    if [ "${inj_pred_code:-1}" = "0" ] && [ "${inj_pred_pred:-0}" -gt 0 ] \
       && [ "${inj_code_pred:-1}" = "0" ] && [ "${inj_code_code:-0}" -gt 0 ]; then
      split=1
    fi
    { echo "CLEAN_RESIDUAL=$clean_res"
      echo "CLEAN_CODE_RESIDUAL=$clean_code"
      echo "CLEAN_PREDICATE_RESIDUAL=$clean_pred"
      echo "INJECT_PREDICATE_RESIDUAL=$inj_pred_res"
      echo "INJECT_PREDICATE_CODE=$inj_pred_code"
      echo "INJECT_CODE_RESIDUAL=$inj_code_res"
      echo "INJECT_CODE_PREDICATE=$inj_code_pred"
      echo "RESIDUAL_INJECT=$residual_inject"
      echo "RESIDUAL_PREDICATE_SPLIT=$split"
    } >> "$LOG"
    require_tokens "$LOG" 'SHARED_REUSE=3' 'SHARED_IMPORTS=3' 'LOCAL_ALERT_DUP=0' \
      'BUSINESS_TOAST_RESIDUAL=0' 'LOCAL_CODE_LIST=0' 'UNCLASSIFIED_ROWS=0' 'READONLY=1' 'DB_WRITES=0' \
      'RESIDUAL_INJECT=1' 'RESIDUAL_PREDICATE_SPLIT=1' || RC=1
    forbid "$LOG" 'LOCAL_ALERT_DUP=1' || RC=1
    ;;
  r6)
    NAME="R6-build-lint.txt"
    BUILD=0
    LINT=0
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      # 重命令统一走 host 锁（heavy-run 是唯一锁 owner；脚本本身不持锁）
      ( cd frontend-pc && "$HOME/.config/opencode/scripts/heavy-run.sh" --timeout 900 -- npm run build ) 2>&1 | tail -10
      BUILD=${PIPESTATUS[0]}
      echo "PC_BUILD=$BUILD"
      echo "LINT_CMD=npx eslint 2488 changed files"
      ( cd frontend-pc && npx eslint \
          "src/pages/System/BannerManagePage.jsx" \
          "src/pages/System/MallImageManageCard.jsx" \
          "src/pages/InstrumentStock.jsx" ) 2>&1 | tail -8
      LINT=${PIPESTATUS[0]}
      echo "LINT_EXIT=$LINT"
      [ "$BUILD" = "0" ] && [ "$LINT" = "0" ] && echo "BUILD_LINT_OK" || echo "BUILD_LINT_FAIL"
    } > "$LOG" 2>&1
    grep -q '^BUILD_LINT_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'PC_BUILD=0' 'LINT_EXIT=0' || RC=1
    forbid "$LOG" 'LINT_EXIT=1' || RC=1
    ;;
  r7)
    NAME="R7-gate.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      fail=0
      GATE="$OUT_DIR/pre-ready-$ISSUE.json"
      overall="$(jq -r '.overall // empty' "$GATE" 2>/dev/null)"
      if [ -f "$GATE" ] && [ "$overall" = "PASS" ]; then
        echo "GATE_RESULT=PASS"
      elif [ ! -f "$GATE" ]; then
        echo "GATE_RESULT=PENDING(no report yet)"; fail=1
      else
        echo "GATE_RESULT=MISSING_OR_FAIL(${overall:-missing})"; fail=1
      fi
      # 计划 v2 D6：本单是子任务（Parent: #2486），harness 的 P5 恒为 N/A，
      # 因此全量 PC 构建由本行**直接执行**并记录退出码（不再要求 FULL_SUITE_EXIT / P5=PASS）。
      echo "FULL_GATE_CMD=cd frontend-pc && npm run build"
      ( cd frontend-pc && "$HOME/.config/opencode/scripts/heavy-run.sh" --timeout 900 -- npm run build ) 2>&1 | tail -8
      FGE="${PIPESTATUS[0]}"
      echo "FULL_GATE_DIRECT_EXIT=$FGE"
      [ "$FGE" = "0" ] || fail=1
      p5=$(jq -r '.checks[]? | select(.id=="P5") | .status' "$GATE" 2>/dev/null | head -1)
      echo "P5=${p5:-MISSING}"
      gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE" 2>/dev/null)"
      plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
      if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
        echo "GATE_PLAN_SHA_MATCH=1"
      else
        echo "GATE_PLAN_SHA_MATCH=0"; fail=1
      fi
      rebuilt=0
      for f in R1-inventory.txt R2-banner.txt R3-mall-image.txt R4-stock.txt R5-residual.txt R6-build-lint.txt; do
        [ -f "$OUT_DIR/$f" ] || continue
        head -1 "$OUT_DIR/$f" | grep -q "commit=$FIX_SHA " && rebuilt=$((rebuilt + 1))
      done
      echo "REBUILT_ROWS=$((rebuilt + 1))"
      bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" > "$REPO/tmp/plan-source-2488.log" 2>&1
      tail -6 "$REPO/tmp/plan-source-2488.log"
      if grep -q 'PLAN_SOURCE: PASS' "$REPO/tmp/plan-source-2488.log"; then
        echo "PLAN_SOURCE=PASS"
      else
        echo "PLAN_SOURCE=FAIL"; fail=1
      fi
      [ "$fail" = "0" ] && echo "GATE_OK" || echo "GATE_FAIL"
    } > "$LOG" 2>&1
    grep -q '^GATE_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GATE_RESULT=PASS' 'FULL_GATE_DIRECT_EXIT=0' 'PLAN_SOURCE=PASS' \
      'GATE_PLAN_SHA_MATCH=1' 'REBUILT_ROWS=7' || RC=1
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
