#!/usr/bin/env bash
# #2609 证据生成器 — PC 三页网络错误文案收口（唯一出口 userFacingErrorMessage）
#                + 可证伪的 catch 守卫探针。
#
# 用法：bash scripts/verify-2609.sh <r1..r6>
#   r1 docs/evidence/2609/R1-catch-guard.txt   8 处接线 + 探针结构判定 + 缺陷注入红绿
#   r2 docs/evidence/2609/R2-helper-runtime.txt 运行时语义（node --test，含朴素实现对照）
#   r3 docs/evidence/2609/R3-build-lint.txt    PC 构建 + ESLint（改动文件）+ 单测复跑
#   r4 docs/evidence/2609/R4-residual-scan.txt 同类残留对称扫描（域外计数锁定 117/37）
#   r5 docs/evidence/2609/R5-api-doc.txt       api.md §1.3 口径同步（结构断言）
#   r6 docs/evidence/2609/R6-2488-harness.txt  强化后的 #2488 harness 可复跑（不覆写其证据）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
#
# 判定口径唯一 owner：scripts/scan-catch-guard-2609.sh（结构判定）。
# 本脚本只做编排 + 断言，不得自行 grep 业务字面量（那正是 #2609 的验收盲点）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6) ;;
  *)
    echo "usage: $0 <r1..r6>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
mkdir -p "$REPO/tmp"   # 干净 worktree 无 tmp/（.gitignore），日志重定向需要它
ISSUE=2609
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

SCAN="scripts/scan-catch-guard-2609.sh"
HELPER="frontend-pc/src/services/apiError.js"
BANNER_PAGE="frontend-pc/src/pages/System/BannerManagePage.jsx"
MALLIMAGE_PAGE="frontend-pc/src/pages/System/MallImageManageCard.jsx"
STOCK_PAGE="frontend-pc/src/pages/InstrumentStock.jsx"
IN_PAGES="pages/System/BannerManagePage.jsx pages/System/MallImageManageCard.jsx pages/InstrumentStock.jsx"
API_DOC="docs/spec/api/api.md"

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

# field <section> <key> —— 从 LOG 的某个 `NAME:` 段落里取 KEY=VALUE 的 VALUE。
field() {
  awk -v sec="$1:" -v key="$2=" '
    $0 == sec { f = 1; next }
    f && /^[A-Z_]+:$/ { f = 0 }
    f && index($0, key) == 1 { sub(/^[^=]*=/, ""); print; exit }
  ' "$LOG"
}

LOG="$REPO/tmp/verify-2609-${MODE}.log"
: > "$LOG"
INJ_ROOT="$REPO/tmp/verify-2609-inject"
H2488="$REPO/tmp/verify-2609-2488"
trap 'rm -f "$LOG"; rm -rf "$INJ_ROOT"' EXIT
RC=0

case "$MODE" in
  r1)
    NAME="R1-catch-guard.txt"
    # 缺陷注入（红）：只在 tmp 副本上变异，绝不动工作树。
    #   raw   = 还原 #2609 修前写法 `e?.message || '<中文>'`
    #   nofb  = 保留守卫但兜底改成 `e.message`（守卫存在、兜底不可用）
    rm -rf "$INJ_ROOT"
    for variant in raw nofb; do
      mkdir -p "$INJ_ROOT/$variant/src/pages/System"
      cp "$BANNER_PAGE" "$INJ_ROOT/$variant/src/pages/System/BannerManagePage.jsx"
      cp "$MALLIMAGE_PAGE" "$INJ_ROOT/$variant/src/pages/System/MallImageManageCard.jsx"
      cp "$STOCK_PAGE" "$INJ_ROOT/$variant/src/pages/InstrumentStock.jsx"
    done
    find "$INJ_ROOT/raw/src" -name '*.jsx' -print0 \
      | xargs -0 perl -pi -e "s/userFacingErrorMessage\(e, ('[^']*')\)/e?.message || \$1/g"
    find "$INJ_ROOT/nofb/src" -name '*.jsx' -print0 \
      | xargs -0 perl -pi -e "s/userFacingErrorMessage\(e, ('[^']*')\)/userFacingErrorMessage(e, e.message)/g"

    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "SCAN_CMD=bash $SCAN --files $IN_PAGES"
      echo "SCOPE_CATCH:"
      bash "$SCAN" --files "$IN_PAGES" 2>&1
      echo "SCAN_EXIT=$?"
      echo "INJECT_RAW_SCAN:"
      bash "$SCAN" --src "$INJ_ROOT/raw/src" --helper "$HELPER" --files "$IN_PAGES" 2>&1
      echo "INJECT_NOFB_SCAN:"
      bash "$SCAN" --src "$INJ_ROOT/nofb/src" --helper "$HELPER" --files "$IN_PAGES" 2>&1
    } > "$LOG" 2>&1

    inj_raw_guarded=$(field INJECT_RAW_SCAN CATCH_GUARDED)
    inj_raw_message=$(field INJECT_RAW_SCAN CATCH_MESSAGE)
    inj_raw_fallback=$(field INJECT_RAW_SCAN CATCH_NETWORK_FALLBACK)
    inj_nofb_message=$(field INJECT_NOFB_SCAN CATCH_MESSAGE)
    inj_nofb_fallback=$(field INJECT_NOFB_SCAN CATCH_NETWORK_FALLBACK)

    inject_red=0
    if [ "${inj_raw_guarded:-x}" = "0" ] \
       && [ "${inj_raw_message:-x}" = "0" ] \
       && [ "${inj_raw_fallback:-x}" = "0" ] \
       && [ "${inj_nofb_message:-x}" = "1" ] \
       && [ "${inj_nofb_fallback:-x}" = "0" ]; then
      inject_red=1
    fi

    { echo "INJ_RAW_GUARDED=${inj_raw_guarded:-missing}"
      echo "INJ_RAW_MESSAGE=${inj_raw_message:-missing}"
      echo "INJ_RAW_FALLBACK=${inj_raw_fallback:-missing}"
      echo "INJ_NOFB_MESSAGE=${inj_nofb_message:-missing}"
      echo "INJ_NOFB_FALLBACK=${inj_nofb_fallback:-missing}"
      echo "CATCH_INJECT_RED=$inject_red"
    } >> "$LOG"

    forbid "$LOG" 'CATCH_RAW=1' || RC=1
    require_tokens "$LOG" 'SCAN_EXIT=0' 'CATCH_SITES=8' 'CATCH_GUARDED=8' 'CATCH_RAW=0' \
      'CATCH_MESSAGE=1' 'CATCH_NETWORK_FALLBACK=1' 'HELPER_GATE=1' \
      'INJ_RAW_GUARDED=0' 'INJ_RAW_MESSAGE=0' 'INJ_RAW_FALLBACK=0' \
      'INJ_NOFB_MESSAGE=1' 'INJ_NOFB_FALLBACK=0' 'CATCH_INJECT_RED=1' || RC=1
    ;;
  r2)
    NAME="R2-helper-runtime.txt"
    TEST_EXIT=0
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "HELPER_TEST_CMD=cd frontend-pc && node --test tests/userFacingErrorMessage.test.mjs"
      ( cd frontend-pc && node --test tests/userFacingErrorMessage.test.mjs ) 2>&1 \
        | grep -E '^(# )?(HELPER_IMPL|NETWORK_FALLBACK|APIERROR_PASSTHROUGH|APIERROR_EMPTY_FALLBACK|PLAIN_ERROR_FALLBACK|NON_OBJECT_FALLBACK|FALLBACK_VERBATIM|ASCII_LEAK|NAIVE_LEAK|NAIVE_MISMATCH|RUNTIME_INJECT_RED)=|^# (pass|fail) '
      TEST_EXIT=${PIPESTATUS[0]}
      echo "HELPER_TEST_EXIT=$TEST_EXIT"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'HELPER_TEST_EXIT=0' 'NETWORK_FALLBACK=1' 'APIERROR_PASSTHROUGH=1' \
      'APIERROR_EMPTY_FALLBACK=1' 'PLAIN_ERROR_FALLBACK=1' 'NON_OBJECT_FALLBACK=1' \
      'FALLBACK_VERBATIM=1' 'ASCII_LEAK=0' 'NAIVE_LEAK=1' 'RUNTIME_INJECT_RED=1' || RC=1
    ;;
  r3)
    NAME="R3-build-lint.txt"
    BUILD=0
    LINT=0
    TEST=0
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      # 重命令统一走 host 锁（heavy-run 是唯一锁 owner；脚本本身不持锁）
      ( cd frontend-pc && "$HOME/.config/opencode/scripts/heavy-run.sh" --timeout 900 -- npm run build ) 2>&1 | tail -10
      BUILD=${PIPESTATUS[0]}
      echo "PC_BUILD=$BUILD"
      echo "LINT_CMD=npx eslint 2609 changed files"
      ( cd frontend-pc && npx eslint \
          "src/services/apiError.js" \
          "src/pages/System/BannerManagePage.jsx" \
          "src/pages/System/MallImageManageCard.jsx" \
          "src/pages/InstrumentStock.jsx" ) 2>&1 | tail -8
      LINT=${PIPESTATUS[0]}
      echo "LINT_EXIT=$LINT"
      echo "HELPER_TEST_CMD=cd frontend-pc && node --test tests/userFacingErrorMessage.test.mjs"
      ( cd frontend-pc && node --test tests/userFacingErrorMessage.test.mjs ) > "$REPO/tmp/verify-2609-r3-helper.log" 2>&1
      TEST=$?
      echo "HELPER_TEST_EXIT=$TEST"
      [ "$BUILD" = "0" ] && [ "$LINT" = "0" ] && [ "$TEST" = "0" ] \
        && echo "BUILD_LINT_OK" || echo "BUILD_LINT_FAIL"
    } > "$LOG" 2>&1
    grep -q '^BUILD_LINT_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'PC_BUILD=0' 'LINT_EXIT=0' 'HELPER_TEST_EXIT=0' 'BUILD_LINT_OK' || RC=1
    ;;
  r4)
    NAME="R4-residual-scan.txt"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "FULL_SRC_SCAN:"
      bash "$SCAN" 2>&1
      echo "INSCOPE_SCAN:"
      bash "$SCAN" --files "$IN_PAGES" 2>&1
    } > "$LOG" 2>&1

    total_raw=$(field FULL_SRC_SCAN CATCH_RAW)
    total_raw_files=$(field FULL_SRC_SCAN CATCH_RAW_FILES)
    in_raw=$(field INSCOPE_SCAN CATCH_RAW)
    in_raw_files=$(field INSCOPE_SCAN CATCH_RAW_FILES)
    in_sites=$(field INSCOPE_SCAN CATCH_SITES)

    out_sites=$(( ${total_raw:-0} - ${in_raw:-0} ))
    out_files=$(( ${total_raw_files:-0} - ${in_raw_files:-0} ))
    followup=0
    grep -qF '#2612' "$SCAN" && followup=2612

    { echo "RESIDUAL_INSCOPE_RAW=${in_raw:-missing}"
      echo "INSCOPE_SITES=${in_sites:-missing}"
      echo "OUT_OF_SCOPE_SITES=$out_sites"
      echo "OUT_OF_SCOPE_FILES=$out_files"
      echo "FOLLOWUP_ISSUE=$followup"
    } >> "$LOG"
    require_tokens "$LOG" 'RESIDUAL_INSCOPE_RAW=0' 'INSCOPE_SITES=8' \
      'OUT_OF_SCOPE_SITES=117' 'OUT_OF_SCOPE_FILES=37' 'FOLLOWUP_ISSUE=2612' || RC=1
    ;;
  r5)
    NAME="R5-api-doc.txt"
    network_rule=0
    forbid_inline=0
    helper_ref=0
    ascii_clause=0
    if grep -qF '传输层网络错误（#2609）' "$API_DOC"; then network_rule=1; fi
    if grep -qE '禁止.*页面内联' "$API_DOC"; then forbid_inline=1; fi
    if grep -qF 'userFacingErrorMessage' "$API_DOC"; then helper_ref=1; fi
    # 断言必须落在**本条新增段落**上：整篇文档里旧段落也有「ASCII 机器串不外溢」，
    # 用全文 grep 会恒真（与 #2609 要修的「不可证伪探针」同型）。故按行关联：
    # 含“传输层网络错误（#2609）”的那一行必须同时写明 ASCII 不外溢口径。
    if grep -F '传输层网络错误（#2609）' "$API_DOC" | grep -qF 'ASCII 机器串不外溢'; then ascii_clause=1; fi
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "DOC=$API_DOC"
      echo "API_NETWORK_RULE=$network_rule"
      echo "API_FORBID_INLINE=$forbid_inline"
      echo "API_HELPER_REF=$helper_ref"
      echo "API_ASCII_CLAUSE=$ascii_clause"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'API_NETWORK_RULE=1' 'API_FORBID_INLINE=1' \
      'API_HELPER_REF=1' 'API_ASCII_CLAUSE=1' || RC=1
    ;;
  r6)
    NAME="R6-2488-harness.txt"
    R2_EXIT=0; R3_EXIT=0; R4_EXIT=0
    rm -rf "$H2488"; mkdir -p "$H2488"
    { printf '%s\n%s\n' "$PROV" "$TREE_LINE"
      echo "HARNESS=scripts/verify-2488.sh (OUT_DIR=tmp/verify-2609-2488, FIX_REF=$FIX_SHA)"
      for mode in r2 r3 r4; do
        OUT_DIR="$H2488" FIX_REF="$FIX_SHA" bash scripts/verify-2488.sh "$mode" > "$REPO/tmp/verify-2609-2488-$mode.out" 2>&1
        rc=$?
        echo "HARNESS_${mode}_EXIT=$rc"
        tail -3 "$REPO/tmp/verify-2609-2488-$mode.out" | sed "s/^/[${mode}] /"
        case "$mode" in
          r2) R2_EXIT=$rc ;;
          r3) R3_EXIT=$rc ;;
          r4) R4_EXIT=$rc ;;
        esac
      done

      # stock 页的 catch 口径必须真的被断言（旧版 `case "$STOCK_PAGE") ;;` 直接跳过）。
      stock_cov=0
      if [ -f "$H2488/R4-stock.txt" ] \
         && grep -q '^CATCH_MESSAGE=1$' "$H2488/R4-stock.txt" \
         && grep -q '^CATCH_NETWORK_FALLBACK=1$' "$H2488/R4-stock.txt"; then
        stock_cov=1
      fi
      echo "STOCK_CATCH_COVERED=$stock_cov"

      # 旧探针（字面量 grep）在修后代码上恒为 0 —— 说明它与写法耦合、必须换成结构判定。
      old_hits=0
      for page in "$BANNER_PAGE" "$MALLIMAGE_PAGE"; do
        grep -qE "e\\?\\.message \\|\\| '|e\\.message \\|\\| '" "$page" && old_hits=$((old_hits + 1))
        grep -qE "e\\?\\.message \\|\\| '[^']+'" "$page" && old_hits=$((old_hits + 1))
      done
      echo "OLD_LITERAL_HITS=$old_hits"
      [ "$old_hits" = "0" ] && echo "OLD_METRIC_WOULD_FAIL=1" || echo "OLD_METRIC_WOULD_FAIL=0"

      # #2488 已 done，其证据绑 fd2d0fdd8 provenance：本单不得覆写。
      if [ -z "$(git status --porcelain docs/evidence/2488)" ]; then
        echo "EVIDENCE_2488_UNTOUCHED=1"
      else
        echo "EVIDENCE_2488_UNTOUCHED=0"
      fi
    } > "$LOG" 2>&1

    require_tokens "$LOG" 'HARNESS_r2_EXIT=0' 'HARNESS_r3_EXIT=0' 'HARNESS_r4_EXIT=0' \
      'STOCK_CATCH_COVERED=1' 'OLD_METRIC_WOULD_FAIL=1' 'EVIDENCE_2488_UNTOUCHED=1' || RC=1
    # 契约 token（Evidence Contract R6 行要求的形态）。
    { echo "R2_EXIT=$R2_EXIT"
      echo "R3_EXIT=$R3_EXIT"
      echo "R4_EXIT=$R4_EXIT"
    } >> "$LOG"
    require_tokens "$LOG" 'R2_EXIT=0' 'R3_EXIT=0' 'R4_EXIT=0' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
# 产物写入：日志（含 RESULT 行）一律先落到 LOG，再复制为 artifact。
# 关键：复制之后不得再向 stdout 写任何字节——plan 的合同命令形如
#   `bash scripts/verify-2609.sh r1 > docs/evidence/2609/R1-catch-guard.txt`
# 若复制后再 echo 到 stdout，shell 的 fd 偏移会从 0 开始覆盖 artifact 开头，
# 把 provenance 首行（# build=...）覆盖成 `ARTIFACT=...`。故状态行只走 stderr。
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >> "$LOG"
else
  echo "RESULT: PASS ($MODE)" >> "$LOG"
fi
cp "$LOG" "$ART"
echo "ARTIFACT=$ART" >&2
tail -1 "$LOG" >&2
if [ "$RC" != "0" ]; then
  exit 1
fi
exit 0
