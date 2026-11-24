#!/usr/bin/env bash
# #2480 证据生成器 — 409 业务冲突文案：后端具体 message 优先于 ERROR_CODE_MAP 通用文案。
#
# 用法：bash scripts/verify-2480.sh <r1|r2|r3|r4|r5|r6>
#   r1  R1-pc-resolver.txt      PC 解析器四层顺序（node --test 8 条 fixture）
#   r2  R2-mobile-core.txt      mobile 纯核心同层序（node --test 8 条 fixture）
#   r3  R3-parity.txt           PC ↔ mobile 平价（10 条 fixture）
#   r4  R4-wiring-static.txt    mobile 出口接线静态结构断言（Owner 豁免 D4）
#   r5  R5-build-lint.txt       PC 构建 + 两端 eslint（改动文件）
#   r6  R6-gate.txt             gate 报告 + plan-source 四源一致（读校验）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2480
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
HEAD_SHA="$(git rev-parse HEAD)"
# provenance 锚定本单 fix commit：调用方传 FIX_REF=<fix-commit>；缺省取本单最新 fix 提交，
# 避免被 pre-ready P4（affected-cmd 重跑本脚本）或证据收尾提交改锚点。
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
# dirty 只看源码树：本脚本自己写的证据产物不计入（否则重生成即自称 dirty）。
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
TREE_LINE="# tree=$HEAD_SHA"

PC_SRC="frontend-pc/src/services/errorMessages.js"
PC_TEST="frontend-pc/tests/errorMessages.test.mjs"
MOBILE_CORE="frontend-mobile/src/services/errorMessageResolution.mjs"
MOBILE_SRC="frontend-mobile/src/services/errorMessages.js"
MOBILE_TEST="frontend-mobile/src/services/errorMessageResolution.test.mjs"
MOBILE_API="frontend-mobile/src/services/api.js"
PARITY="scripts/check-error-message-parity-2480.mjs"

# require <log> <token>... —— 任一缺失则 FAIL（机器可判定断言）
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
# forbid <log> <token> —— 出现即 FAIL（负例断言）
forbid() {
  if grep -Fq -- "$2" "$1"; then
    echo "ASSERT_FAIL: forbidden token present: [$2]" >&2
    return 1
  fi
}
# forbid_re <log> <regex> —— 正则版负例断言
forbid_re() {
  if grep -Eq -- "$2" "$1"; then
    echo "ASSERT_FAIL: forbidden pattern present: [$2]" >&2
    return 1
  fi
}

LOG="$REPO/tmp/verify-2480-${MODE}.log"
: > "$LOG"
trap 'rm -f "$LOG"' EXIT
RC=0

case "$MODE" in
  r1)
    NAME="R1-pc-resolver.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      node --test "$PC_TEST" 2>&1
      echo "NODE_TEST_EXIT=$?"
    } > "$LOG" 2>&1
    grep -q '^NODE_TEST_EXIT=0$' "$LOG" || RC=1
    require_tokens "$LOG" \
      'C40900_CN=网点名称已存在' 'L40900_CN=human_message' \
      'C40900_ASCII=数据冲突，请刷新后重试' 'L40900_ASCII=code_map' \
      'C40900_EMPTY=数据冲突，请刷新后重试' \
      'C40912_CN=学生证作为第二证件需介绍信' \
      'C_L1=未找到订单' 'C_NULL=操作失败，请重试' \
      'C49999_ASCII=unknown conflict' \
      'LAYERS=human_message,code_map,code_map,human_message,message_map,fallback,raw_message,human_message' || RC=1
    # 负例：具体文案仍被通用文案遮蔽 → FAIL
    forbid "$LOG" 'C40900_CN=数据冲突，请刷新后重试' || RC=1
    ;;
  r2)
    NAME="R2-mobile-core.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      node --test "$MOBILE_TEST" 2>&1
      echo "NODE_TEST_EXIT=$?"
    } > "$LOG" 2>&1
    grep -q '^NODE_TEST_EXIT=0$' "$LOG" || RC=1
    require_tokens "$LOG" \
      'L_CN=human_message' 'L_ASCII=code_map' 'L_EMPTY=code_map' \
      'L_40912=human_message' 'L_NULL=fallback' \
      'LAYERS=human_message,code_map,code_map,human_message,message_map,fallback,raw_message,human_message' || RC=1
    # 负例：mobile 层序与 PC 不一致 → FAIL（LAYERS 逐字比对已在上面锁定）
    forbid "$LOG" 'L_CN=code_map' || RC=1
    ;;
  r3)
    NAME="R3-parity.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      node "$PARITY" 2>&1
      echo "PARITY_EXIT=$?"
    } > "$LOG" 2>&1
    grep -q '^PARITY_EXIT=0$' "$LOG" || RC=1
    require_tokens "$LOG" 'PARITY=1' 'CASES=10' 'DIFFS=0' || RC=1
    forbid "$LOG" 'PARITY=0' || RC=1
    ;;
  r4)
    NAME="R4-wiring-static.txt"
    # 静态结构断言（evidence_type=static，Owner 豁免见计划 D4）：mobile 出口唯一、
    # 后端原文分支不套 t()、api.js 再导出保持不变。
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      node --input-type=module -e '
        import fs from "node:fs"
        const core = fs.readFileSync("frontend-mobile/src/services/errorMessageResolution.mjs", "utf8")
        const src = fs.readFileSync("frontend-mobile/src/services/errorMessages.js", "utf8")
        const api = fs.readFileSync("frontend-mobile/src/services/api.js", "utf8")
        const fail = (msg) => { console.log("FAIL=" + msg); process.exitCode = 1 }
        console.log("CORE_LAYERS=" + (["message_map", "human_message", "code_map", "raw_message", "fallback"].every((l) => core.includes(`"${l}"`) || core.includes(`\x27${l}\x27`)) ? 1 : 0))
        console.log("CORE_CJK=" + (/\\u4e00/.test(core) ? 1 : 0))
        const defs = src.match(/export function resolveErrorMessage/g) || []
        console.log("SINGLE_EXIT=" + (defs.length === 1 ? 1 : 0))
        console.log("DELEGATES_CORE=" + (/resolveErrorCore\(result/.test(src) ? 1 : 0))
        const rawBranch = src.match(/if \(core\.layer === [^\n]*\) \{\s*\n\s*return core\.text/)
        console.log("RAW_MESSAGE_NO_T=" + (rawBranch ? 1 : 0))
        // 该分支块内不得出现 t( 包装（后端原文 / 可读文案原样返回）
        let block = ""
        if (rawBranch) {
          const start = src.indexOf(rawBranch[0])
          const end = src.indexOf("}", start)
          block = src.slice(start, end + 1)
        }
        console.log("RAW_BRANCH_UNTYPED=" + (block && !/t\(/.test(block) ? 1 : 0))
        const reexport = /export \{[^}]*resolveErrorMessage[^}]*ERROR_CODE_MAP[^}]*ERROR_MESSAGE_MAP[^}]*\} from \x27\.\/errorMessages\x27/.test(api)
        console.log("REEXPORT_KEPT=" + (reexport ? 1 : 0))
        console.log("API_IMPORT_USERS=" + (api.includes("from \x27./errorMessages\x27") ? 1 : 0))
        if (!/message_map/.test(core)) fail("core-missing-message_map")
        if (/export function resolveErrorMessage/.test(core)) fail("core-must-not-export-wrapper")
      ' 2>&1
      echo "STATIC_EXIT=$?"
    } > "$LOG" 2>&1
    grep -q '^STATIC_EXIT=0$' "$LOG" || RC=1
    require_tokens "$LOG" 'SINGLE_EXIT=1' 'RAW_MESSAGE_NO_T=1' 'REEXPORT_KEPT=1' \
      'DELEGATES_CORE=1' 'RAW_BRANCH_UNTYPED=1' 'CORE_LAYERS=1' || RC=1
    # 负例：后端原文分支被 t() 包装，或静态断言报告 FAIL → FAIL
    forbid "$LOG" 'RAW_BRANCH_UNTYPED=0' || RC=1
    forbid_re "$LOG" '^FAIL=' || RC=1
    ;;
  r5)
    NAME="R5-build-lint.txt"
    BUILD=0
    LINT_PC=0
    LINT_MOBILE=0
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      ( cd frontend-pc && npm run build ) 2>&1 | tail -12
      BUILD=${PIPESTATUS[0]}
      echo "PC_BUILD=$BUILD"
      echo "LINT_PC_CMD=npx eslint $PC_SRC $PC_TEST"
      ( cd frontend-pc && npx eslint "src/services/errorMessages.js" "tests/errorMessages.test.mjs" ) 2>&1
      LINT_PC=$?
      echo "LINT_EXIT=$LINT_PC"
      echo "LINT_MOBILE_CMD=npx eslint $MOBILE_CORE $MOBILE_SRC $MOBILE_TEST"
      ( cd frontend-mobile && npx eslint "src/services/errorMessageResolution.mjs" "src/services/errorMessages.js" "src/services/errorMessageResolution.test.mjs" ) 2>&1
      LINT_MOBILE=$?
      echo "LINT_MOBILE_EXIT=$LINT_MOBILE"
      [ "$BUILD" = "0" ] && [ "$LINT_PC" = "0" ] && [ "$LINT_MOBILE" = "0" ] && echo "BUILD_LINT_OK" || echo "BUILD_LINT_FAIL"
    } > "$LOG" 2>&1
    grep -q '^BUILD_LINT_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'PC_BUILD=0' 'LINT_EXIT=0' 'LINT_MOBILE_EXIT=0' || RC=1
    forbid "$LOG" 'LINT_EXIT=1' || RC=1
    ;;
  r6)
    NAME="R6-gate.txt"
    # 读同轮 gate 报告（`--report docs/evidence/2480/pre-ready-2480.json`）：报告未生成时
    # 落 PENDING interim（P3/P12 只校验产物存在性），gate 通过后重跑本行覆写。
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
      p6=$(jq -r '.checks[]? | select(.id=="P5") | .status' "$GATE" 2>/dev/null | head -1)
      echo "P5=${p6:-MISSING}"
      if [ "$p6" = "PASS" ]; then
        echo "FULL_SUITE_EXIT=0 (derived from P5 PASS)"
      else
        echo "FULL_SUITE_EXIT=PENDING"; fail=1
      fi
      gate_sha="$(jq -r '.plan_sha256 // empty' "$GATE" 2>/dev/null)"
      plan_sha="$(sha256sum "$OUT_DIR/plan.md" | awk '{print $1}')"
      echo "GATE_PLAN_SHA=$gate_sha"
      if [ -n "$gate_sha" ] && [ "$gate_sha" = "$plan_sha" ]; then
        echo "GATE_PLAN_SHA_MATCH=1"
      else
        echo "GATE_PLAN_SHA_MATCH=0"; fail=1
      fi
      bash "$HOME/.config/opencode/scripts/plan-source.sh" --issue "$ISSUE" > "$REPO/tmp/plan-source-2480.log" 2>&1
      tail -6 "$REPO/tmp/plan-source-2480.log"
      if grep -q 'PLAN_SOURCE: PASS' "$REPO/tmp/plan-source-2480.log"; then
        echo "PLAN_SOURCE=PASS"
      else
        echo "PLAN_SOURCE=FAIL"; fail=1
      fi
      [ "$fail" = "0" ] && echo "GATE_OK" || echo "GATE_FAIL"
    } > "$LOG" 2>&1
    grep -q '^GATE_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GATE_RESULT=PASS' 'FULL_SUITE_EXIT=0' 'PLAN_SOURCE=PASS' 'GATE_PLAN_SHA_MATCH=1' || RC=1
    # 负例：P5 为 N/A（未跑全量）或四源 sha 不一致 → FAIL
    forbid "$LOG" 'P5=N/A' || RC=1
    forbid "$LOG" 'GATE_PLAN_SHA_MATCH=0' || RC=1
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
