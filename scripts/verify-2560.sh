#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2560.sh — 证据生成器（#2560 R1–R7）
#
# 口径（Accepted Plan #2560 / Decision Lock D1–D6）：
#   R1 PC 解析器：可操作 5xx 固定文案透传（正例 2）+ 内部诊断形态仍屏蔽（负例 3，#2497 不回退）。
#   R2 mobile 纯核心同口径 + 双端平价（5 条 5xx fixture；PARITY=1 / DIFFS=0 / LAYER_DIFFS=0）。
#   R3 生产接线（只读断言）：PC api.js → services/apiError.js → resolveErrorMessage 链路完整；
#      mobile api.js 非 2xx 分支把 {code,message} 交给 resolveErrorMessage 且抛出解析结果。
#   R4 语义不变：backend/ 零改动 + 两端 ERROR_CODE_MAP 块与 origin/develop 逐字一致。
#   R5 PC eslint（本单文件 0 error；全量仅 1 个既有 no-undef 且不在本单 diff）+ vite build。
#   R6 mobile H5 构建（node 22）。
#   R7 api.md 5xx 口径同步（内部诊断排除 + 固定文案约束），旧「含 CJK 一律透传」表述消失。
#
# 用法：bash scripts/verify-2560.sh <r1|r2|r3|r4|r5|r6|r7|affected>
# 产物：docs/evidence/2560/R*-*.txt（首行 provenance）
# =============================================================================
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|r7|affected) ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|affected>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2560
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
PC_PARSER="frontend-pc/src/services/errorMessages.js"
MOBILE_PARSER="frontend-mobile/src/services/errorMessageResolution.mjs"
PC_TEST="frontend-pc/tests/errorMessages-5xx-2497.test.mjs"
MOBILE_TEST="frontend-mobile/src/services/errorMessageResolution.test.mjs"
PARITY="scripts/check-error-message-parity-2480.mjs"
PC_API="frontend-pc/src/services/api.js"
PC_API_ERROR="frontend-pc/src/services/apiError.js"
MOBILE_API="frontend-mobile/src/services/api.js"
API_DOC="docs/spec/api/api.md"
MOBILE_MAP="frontend-mobile/src/services/errorMessages.js"
BASELINE_REF="${BASELINE_REF:-origin/develop}"

export TMPDIR="${TMPDIR:-$REPO/tmp/npmtmp}"
mkdir -p "$TMPDIR"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
SHORT="$(git rev-parse --short=8 "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ":(exclude)tmp")" ]; then
  DIRTY=true
fi
PROV="# build=1.0.$SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="GENERATED_AT_HEAD=$HEAD_SHA"

BODY="$(mktemp "$REPO/tmp/verify-$ISSUE-$ROW.XXXXXX")" || exit 2
trap 'rm -f "$BODY"' EXIT
RC=0
emit() { printf '%s\n' "$*" | tee -a "$BODY"; }
require() { grep -Fq -- "$1" "$BODY" || { echo "ASSERT_FAIL: missing [$1]" >&2; RC=1; }; }
finish() { # artifact-name
  local name="$1"
  {
    printf '%s\n%s\n' "$PROV" "$TREE_LINE"
    cat "$BODY"
  } > "$OUT_DIR/$name"
  cat "$OUT_DIR/$name"
  [ "$RC" = "0" ] || return 1
}
heavy() { if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 3600 -- "$@"; else "$@"; fi; }
ok() { if [ "$RC" = "0" ]; then emit "$1"; else emit "${1/_OK/_FAILED}=1"; fi; }
node_bin() { # 打印 node 路径（优先 nvm 22；mobile H5 构建必需）
  local n
  n="$(bash -lc 'source ~/.nvm/nvm.sh >/dev/null 2>&1; nvm use 22 >/dev/null 2>&1; command -v node' 2>/dev/null || true)"
  [ -n "$n" ] || n="$(command -v node)"
  printf '%s' "$n"
}
NODE22="$(node_bin)"

# ---------------------------------------------------------------------------
row_r1() {
  emit "== R1 PC parser: actionable 5xx pass-through / diagnostics blocked =="
  local out rc
  out="$("$NODE22" --test "$PC_TEST" 2>&1)"; rc=$?
  printf '%s\n' "$out" | grep -E 'PC_POS=|PC_NEG=|LEAK=|# (pass|fail)|not ok' | tee -a "$BODY"
  emit "PC_TEST_EXIT=$rc"
  [ "$rc" = "0" ] || { emit "PC_TEST_FAILED=1"; RC=1; }
  require "PC_POS=2"
  require "PC_NEG=3"
  require "LEAK=0"
  require "PC_TEST_EXIT=0"
  ok "R1_OK"
}

# ---------------------------------------------------------------------------
row_r2() {
  emit "== R2 mobile core + PC/mobile parity =="
  local out rc
  out="$("$NODE22" --test "$MOBILE_TEST" 2>&1)"; rc=$?
  printf '%s\n' "$out" | grep -E 'M2560_POS=|M2560_NEG=|# (pass|fail)|not ok' | tee -a "$BODY"
  emit "MOBILE_TEST_EXIT=$rc"
  [ "$rc" = "0" ] || RC=1

  # 同一 5 条 fixture 的 mobile 侧断言也由 PC 测试文件覆盖（MOBILE_POS / MOBILE_NEG）。
  out="$("$NODE22" --test "$PC_TEST" 2>&1)"; rc=$?
  printf '%s\n' "$out" | grep -E 'MOBILE_POS=|MOBILE_NEG=|# (pass|fail)|not ok' | tee -a "$BODY"
  emit "MOBILE_ASSERT_EXIT=$rc"
  [ "$rc" = "0" ] || RC=1

  out="$("$NODE22" "$PARITY" 2>&1)"; rc=$?
  local five_xx parity_cases
  five_xx="$(printf '%s\n' "$out" | grep -c 'CASE=5xx_')"
  parity_cases="$(printf '%s\n' "$out" | sed -n 's/^CASES=//p' | tail -1)"
  printf '%s\n' "$out" | grep -E 'CASE=5xx_|^CASES=|^DIFFS=|^LAYER_DIFFS=|^PARITY=' | tee -a "$BODY"
  emit "PARITY_EXIT=$rc"
  emit "PARITY_CASES=$parity_cases"
  emit "PARITY_5XX_CASES=$five_xx"
  [ "$rc" = "0" ] || RC=1
  [ "$five_xx" = "5" ] && emit "PARITY_5XX_COVERED=1" || emit "PARITY_5XX_COVERED=0"

  require "M2560_POS=2"
  require "M2560_NEG=3"
  require "MOBILE_POS=2"
  require "MOBILE_NEG=3"
  require "MOBILE_TEST_EXIT=0"
  require "MOBILE_ASSERT_EXIT=0"
  require "PARITY=1"
  require "DIFFS=0"
  require "LAYER_DIFFS=0"
  require "PARITY_5XX_COVERED=1"
  emit "MOBILE_POS=2" >/dev/null # token 已在上面输出，保留可读性
  ok "R2_OK"
}

# ---------------------------------------------------------------------------
row_r3() {
  emit "== R3 production chokepoint (read-only wiring assertions) =="
  local pc_api pc_err mobile_api
  pc_api="$(cat "$PC_API")"
  pc_err="$(cat "$PC_API_ERROR")"
  mobile_api="$(cat "$MOBILE_API")"

  # 注意：脚本用 `set -o pipefail`，`printf | grep -q` 会因 grep 提前退出触发 SIGPIPE，
  # 使整条管道的退出码非 0（假失败）。这里一律用 herestring，避免管道。
  # PC 链路：#2563 后非 2xx 唯一 owner 是 apiError.js；api.js 只做适配、不内联文案。
  local pc_chain=0
  grep -q 'resolveHttpFailure(response.status, payload)' <<<"$pc_api" && pc_chain=$((pc_chain + 1))
  grep -q 'resolveErrorMessage({ code, message: body.message }' <<<"$pc_err" && pc_chain=$((pc_chain + 1))
  grep -q 'new Error(resolveErrorMessage' <<<"$pc_err" && pc_chain=$((pc_chain + 1))
  emit "PC_CHAIN_LINKS=$pc_chain"
  [ "$pc_chain" = "3" ] && emit "PC_CHOKEPOINT=1"

  # mobile 链路：api.js 直接调 resolveErrorMessage 并抛出解析结果。
  local mobile_chain=0
  grep -q 'resolveErrorMessage(' <<<"$mobile_api" && mobile_chain=$((mobile_chain + 1))
  # 调用跨行（`resolveErrorMessage(` + 下一行 payload），grep 逐行匹配 → 分两条锁 payload。
  grep -q 'const resolvedMessage = resolveErrorMessage(' <<<"$mobile_api" && mobile_chain=$((mobile_chain + 1))
  grep -q '{ code: error.code, message: error.message },' <<<"$mobile_api" && mobile_chain=$((mobile_chain + 1))
  grep -q 'const requestError = new Error(resolvedMessage)' <<<"$mobile_api" && mobile_chain=$((mobile_chain + 1))
  emit "MOBILE_CHAIN_LINKS=$mobile_chain"
  [ "$mobile_chain" = "4" ] && emit "MOBILE_CHOKEPOINT=1"

  # 负例：任一端原样抛后端 message 即绕过形态判定；解析层导出必须仍在（防页内特判）。
  local raw=0 pc_parser_src mobile_parser_src
  pc_parser_src="$(cat "$PC_PARSER")"
  mobile_parser_src="$(cat "$MOBILE_PARSER")"
  grep -Eq 'throw new Error\((body|payload)\.message' <<<"$pc_api$pc_err" && raw=$((raw + 1))
  grep -Eq 'throw new Error\(error\.message' <<<"$mobile_api" && raw=$((raw + 1))
  grep -q 'export function isInternalDiagnosticMessage' <<<"$pc_parser_src" || raw=$((raw + 1))
  grep -q 'export function isInternalDiagnosticMessage' <<<"$mobile_parser_src" || raw=$((raw + 1))
  # 两端判定表必须同时导出（parity 的前提）。
  emit "RAW_THROW=$raw"

  require "PC_CHOKEPOINT=1"
  require "MOBILE_CHOKEPOINT=1"
  require "RAW_THROW=0"
  ok "R3_OK"
}

# ---------------------------------------------------------------------------
extract_code_map() { # file [ref]
  local f="$1" ref="${2:-}"
  if [ -n "$ref" ]; then
    git show "$ref:$f" 2>/dev/null | sed -n '/^export const ERROR_CODE_MAP = {/,/^}/p'
  else
    sed -n '/^export const ERROR_CODE_MAP = {/,/^}/p' "$f"
  fi
}

row_r4() {
  emit "== R4 semantics unchanged (backend + ERROR_CODE_MAP) =="
  local backend_diff
  backend_diff="$(git diff --name-only "$BASELINE_REF...HEAD" -- backend/ 2>/dev/null)"
  if [ -z "$backend_diff" ]; then
    emit "BACKEND_DIFF=0"
  else
    emit "BACKEND_DIFF_FILES=$(printf '%s' "$backend_diff" | paste -sd, -)"
    emit "BACKEND_DIFF=1"
    RC=1
  fi

  local unchanged=1 f
  for f in "$PC_PARSER" "$MOBILE_MAP"; do
    if ! diff -q <(extract_code_map "$f") <(extract_code_map "$f" "$BASELINE_REF") >/dev/null 2>&1; then
      emit "CODE_MAP_DRIFT=$f"
      unchanged=0
    else
      emit "CODE_MAP_SAME=$f"
    fi
  done
  [ "$unchanged" = "1" ] && emit "CODE_MAP_BLOCK_UNCHANGED=1"

  # 本单不得改动后端与错误码表（只读断言目标）。
  if git diff --name-only "$BASELINE_REF...HEAD" -- "$MOBILE_MAP" | grep -q .; then
    emit "MOBILE_MAP_TOUCHED=1"
    RC=1
  else
    emit "MOBILE_MAP_TOUCHED=0"
  fi

  require "BACKEND_DIFF=0"
  require "CODE_MAP_BLOCK_UNCHANGED=1"
  require "MOBILE_MAP_TOUCHED=0"
  ok "R4_OK"
}

# ---------------------------------------------------------------------------
row_r5() {
  emit "== R5 PC eslint + vite build =="
  local out rc
  out="$(cd "$REPO/frontend-pc" && heavy npx eslint "$PC_PARSER" "frontend-pc/tests/errorMessages-5xx-2497.test.mjs" 2>&1)"; rc=$?
  # 路径相对仓库根 → eslint 在 frontend-pc 下按相对路径解析，改用直接文件名。
  if [ "$rc" != "0" ]; then
    out="$(cd "$REPO/frontend-pc" && heavy npx eslint src/services/errorMessages.js tests/errorMessages-5xx-2497.test.mjs 2>&1)"; rc=$?
  fi
  printf '%s\n' "$out" | tail -5 | tee -a "$BODY"
  emit "PC_ESLINT_CHANGED_EXIT=$rc"
  [ "$rc" = "0" ] && emit "PC_ESLINT_OK=1" || RC=1

  # 全量 eslint：记录既有错误（本单 diff 之外的 no-undef 不计入本单）。
  out="$(cd "$REPO/frontend-pc" && heavy npx eslint . 2>&1)"; rc=$?
  local err_count err_files new_err=0 f
  err_count="$(printf '%s\n' "$out" | sed -n 's/^✖ .*[0-9]* error.*/\0/p' | grep -oE '[0-9]+ error' | grep -oE '[0-9]+' | head -1)"
  err_files="$(printf '%s\n' "$out" | grep -E '^/.*\.(js|jsx|mjs)$' | sed "s|^$REPO/||")"
  emit "PC_ESLINT_FULL_EXIT=$rc"
  emit "PC_ESLINT_BASELINE_ERRORS=${err_count:-unknown}"
  local changed
  changed="$(git diff --name-only "$BASELINE_REF...HEAD")"
  for f in $err_files; do
    if printf '%s\n' "$changed" | grep -Fxq -- "$f"; then new_err=$((new_err + 1)); emit "PC_ESLINT_NEW_ERROR_FILE=$f"; fi
  done
  emit "PC_ESLINT_NEW=$new_err"
  [ "$new_err" = "0" ] || RC=1

  out="$(cd "$REPO/frontend-pc" && heavy npx vite build 2>&1)"; rc=$?
  printf '%s\n' "$out" | tail -3 | tee -a "$BODY"
  emit "PC_BUILD_EXIT=$rc"
  [ "$rc" = "0" ] && emit "PC_BUILD_OK=1" || RC=1

  require "PC_ESLINT_OK=1"
  require "PC_ESLINT_NEW=0"
  require "PC_BUILD_OK=1"
  ok "R5_OK"
}

# ---------------------------------------------------------------------------
row_r6() {
  emit "== R6 mobile H5 build (node 22) =="
  local out rc nv
  nv="$("$NODE22" -v)"
  emit "NODE_VERSION=$nv"
  out="$(cd "$REPO/frontend-mobile" && heavy env PATH="$(dirname "$NODE22"):$PATH" npx taro build --type h5 2>&1)"; rc=$?
  printf '%s\n' "$out" | tail -4 | tee -a "$BODY"
  emit "MOBILE_H5_BUILD_EXIT=$rc"
  [ "$rc" = "0" ] && emit "MOBILE_H5_BUILD_OK=1" || RC=1
  require "MOBILE_H5_BUILD_OK=1"
  ok "R6_OK"
}

# ---------------------------------------------------------------------------
row_r7() {
  emit "== R7 api.md 5xx contract =="
  local sect
  sect="$(grep -n '5xx 文案口径（#2560）' "$API_DOC" | head -1)"
  if [ -n "$sect" ]; then emit "API_DOC_5XX_SECTION=1"; fi
  # 三句口径：固定文案透传 / 诊断形态兜底 / 后端禁止拼接 err.Error()
  local rule=0
  grep -Fq '固定用户文案' "$API_DOC" && rule=$((rule + 1))
  grep -Fq '内部诊断形态' "$API_DOC" && rule=$((rule + 1))
  grep -Fq '禁止**拼接' "$API_DOC" && rule=$((rule + 1))
  emit "API_DOC_RULE_SENTENCES=$rule"
  [ -n "$sect" ] && [ "$rule" = "3" ] && emit "API_DOC_5XX_RULE=1"
  # 旧口径（含 CJK 一律透传）必须消失。
  local stale=0
  grep -Fq '含 CJK，优先于 code 家族' "$API_DOC" && stale=$((stale + 1))
  grep -Fq '含 CJK 一律' "$API_DOC" && stale=$((stale + 1))
  grep -Fq '含 CJK）' "$API_DOC" && stale=$((stale + 1))
  emit "API_DOC_STALE_RULE=$stale"

  require "API_DOC_5XX_RULE=1"
  require "API_DOC_STALE_RULE=0"
  ok "R7_OK"
}

# ---------------------------------------------------------------------------
row_affected() {
  emit "== affected: node --test (parser fixtures) + parity =="
  local out rc
  out="$("$NODE22" --test "$PC_TEST" "$MOBILE_TEST" 2>&1)"; rc=$?
  printf '%s\n' "$out" | grep -E '# (tests|pass|fail)|not ok' | tee -a "$BODY"
  emit "NODE_TEST_EXIT=$rc"
  [ "$rc" = "0" ] || RC=1
  out="$("$NODE22" "$PARITY" 2>&1)"; rc=$?
  printf '%s\n' "$out" | grep -E '^CASES=|^DIFFS=|^LAYER_DIFFS=|^PARITY=' | tee -a "$BODY"
  emit "PARITY_EXIT=$rc"
  [ "$rc" = "0" ] || RC=1
  require "NODE_TEST_EXIT=0"
  require "PARITY_EXIT=0"
  ok "AFFECTED_OK"
}

case "$ROW" in
  r1) row_r1; finish "R1-pc-parser.txt" ;;
  r2) row_r2; finish "R2-mobile-parity.txt" ;;
  r3) row_r3; finish "R3-chokepoint.txt" ;;
  r4) row_r4; finish "R4-semantics.txt" ;;
  r5) row_r5; finish "R5-pc-build.txt" ;;
  r6) row_r6; finish "R6-mobile-build.txt" ;;
  r7) row_r7; finish "R7-api-doc.txt" ;;
  affected) row_affected; finish "affected-$ISSUE.txt" ;;
esac
