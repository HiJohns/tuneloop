#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2578.sh — 证据生成器（#2578 库存监控导出与列表筛选口径对齐）
#
# 口径（docs/spec/api/api.md §5.6.2 注 #2578）：
#   导出 `GET /api/instruments/export` 与列表 `GET /api/instruments` 同一套过滤与可见性：
#   sn 模糊 / category_id 含后代 / status(=stock_status) / exclude_status / applyInstrumentListScope。
#
# 用法：bash scripts/verify-2578.sh <r1..r5>
#   r1 docs/evidence/2578/R1-export-parity.txt   导出=列表同口径（Go 集成测试 + xlsx 逐行断言）
#   r2 docs/evidence/2578/R2-export-params.txt   导出参数纯函数（node --test）
#   r3 docs/evidence/2578/R3-export-wiring.txt   InstrumentStock 导出接线（源码结构断言）
#   r4 docs/evidence/2578/R4-docs.txt            api.md / ui.md 同步（结构断言）
#   r5 docs/evidence/2578/R5-build.txt           eslint / vite build / node --test / go build / go test
#
# 产物首行 provenance：`# build=1.0.<短码> # commit=<sha> # dirty=<bool>`；另起一行 GENERATED_AT_HEAD。
# 重命令一律经 heavy-run.sh 取锁（D8）。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5) ;;
  *)
    echo "usage: $0 <r1..r5>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2578
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
STOCK_JS="frontend-pc/src/pages/InstrumentStock.jsx"
STOCK_UTIL="frontend-pc/src/utils/instrumentStockFilters.js"
STOCK_TEST="frontend-pc/tests/instrumentStockFilters.test.mjs"
EXPORT_TEST="backend/handlers/instrument_export_scope_2578_test.go"
API_DOC="docs/spec/api/api.md"
UI_DOC="docs/spec/ui/ui.md"

# 测试库：一次性 PostgreSQL（#2341 口径，不使用 jobmaster-postgres）。
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor of HEAD" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"
WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"; else "$@"; fi
}

require_tokens() { # log tokens...
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

# handle_export <file> → handleExport 函数体
handle_export() {
  awk '/const handleExport = async \(\) => \{/ {inblk=1} inblk {print} inblk && /^  \}$/ {exit}' "$1"
}

NAME=""
RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-export-parity.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestExportScope2578|TestExportInstruments' -count=1 -v -timeout 10m ) \
      > "$WORK/r1.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# GET /api/instruments/export == GET /api/instruments（sn / 后代分类 / 排除状态 / 网点范围）"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'EXPORT_(SN_FILTER|CATEGORY_DESC|EXCLUDE|SCOPE_SITE|HEADER_SN|ROWS_EQ_LIST)=' "$WORK/r1.test.log" | sed 's/^ *//'
      grep -E '^--- (PASS|FAIL): Test' "$WORK/r1.test.log" | sed 's/^ *//'
      if [ "$TEST_RC" = "0" ]; then echo "R1_OK"; else echo "R1_FAIL"; fi
    } > "$LOG" 2>&1
    grep -q '^R1_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'EXPORT_SN_FILTER=1' 'EXPORT_CATEGORY_DESC=2' \
      'EXPORT_SCOPE_SITE=2' 'EXPORT_ROWS_EQ_LIST=1' || RC=1
    # 附加强断言（计划 G0 全量 token）
    require_tokens "$LOG" 'EXPORT_EXCLUDE=3' 'EXPORT_HEADER_SN=1' || RC=1
    ;;
  r2)
    NAME="R2-export-params.txt"
    ( cd "$REPO/frontend-pc" && heavy node --test --test-reporter=tap "tests/instrumentStockFilters.test.mjs" ) \
      > "$WORK/r2.test.log" 2>&1
    TEST_RC=$?
    PASS_CNT="$(sed -n 's/^# pass \([0-9]\+\)$/\1/p' "$WORK/r2.test.log" | head -1)"
    FAIL_CNT="$(sed -n 's/^# fail \([0-9]\+\)$/\1/p' "$WORK/r2.test.log" | head -1)"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# buildStockExportParams：sn / category_id / status 透传；显式 lost 不叠加 exclude_status；字段清单固定"
      printf 'NODE_TEST_EXIT=%s\n' "$TEST_RC"
      printf 'TESTS_PASS=%s\n' "${PASS_CNT:-0}"
      printf 'TESTS_FAIL=%s\n' "${FAIL_CNT:-1}"
      grep -E '^# EXPORT_' "$WORK/r2.test.log" | sed 's/^# //'
      if [ "$TEST_RC" = "0" ] && [ "${FAIL_CNT:-1}" = "0" ] && [ "${PASS_CNT:-0}" -ge 4 ]; then
        echo "R2_OK"
      else
        echo "R2_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R2_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'TESTS_FAIL=0' 'EXPORT_FIELDS=sn,category_name,status' 'EXPORT_EXCLUDE_WITH_STATUS=' || RC=1
    ;;
  r3)
    NAME="R3-export-wiring.txt"
    handle_export "$REPO/$STOCK_JS" > "$WORK/handleExport.body"
    [ -s "$WORK/handleExport.body" ] || { echo "FATAL: cannot scope handleExport in $STOCK_JS" >&2; exit 3; }
    USES_FETCHBLOB="$(grep -c 'fetchBlob(' "$WORK/handleExport.body" || true)"
    RAW_FETCH="$(grep -c 'fetch(' "$WORK/handleExport.body" || true)"
    PASSES_FILTERS="$(grep -c 'buildStockExportParams(' "$WORK/handleExport.body" || true)"
    XLSX_NAME="$(grep -c '\.xlsx`' "$WORK/handleExport.body" || true)"
    CSV_NAME="$(grep -c '\.csv`' "$WORK/handleExport.body" || true)"
    BUTTON_LABEL="$(grep -c '>导出 Excel<' "$REPO/$STOCK_JS" || true)"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# InstrumentStock.handleExport：fetchBlob + 筛选透传 + xlsx 命名；无裸 fetch / 无 .csv"
      printf 'EXPORT_USES_FETCHBLOB=%s\n' "$([ "$USES_FETCHBLOB" = "1" ] && echo 1 || echo 0)"
      printf 'EXPORT_RAW_FETCH=%s\n' "$RAW_FETCH"
      printf 'EXPORT_XLSX_NAME=%s\n' "$([ "$XLSX_NAME" = "1" ] && echo 1 || echo 0)"
      printf 'EXPORT_CSV_NAME=%s\n' "$CSV_NAME"
      printf 'EXPORT_PASSES_FILTERS=%s\n' "$([ "$PASSES_FILTERS" = "1" ] && echo 1 || echo 0)"
      printf 'EXPORT_BUTTON_LABEL=%s\n' "$([ "$BUTTON_LABEL" = "1" ] && echo 1 || echo 0)"
      echo "--- handleExport ---"
      cat "$WORK/handleExport.body"
      if [ "$USES_FETCHBLOB" = "1" ] && [ "$RAW_FETCH" = "0" ] && [ "$XLSX_NAME" = "1" ] && \
         [ "$CSV_NAME" = "0" ] && [ "$PASSES_FILTERS" = "1" ] && [ "$BUTTON_LABEL" = "1" ]; then
        echo "R3_OK"
      else
        echo "R3_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R3_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'EXPORT_USES_FETCHBLOB=1' 'EXPORT_RAW_FETCH=0' 'EXPORT_XLSX_NAME=1' || RC=1
    ;;
  r4)
    NAME="R4-docs.txt"
    API_SEC="$(awk '/#### 5\.8\.2 导出乐器列表/{f=1} f{print} f && /#### 5\.8\.3/ {exit}' "$REPO/$API_DOC")"
    UI_SEC="$(awk '/### 2\.20\.1 PC 乐器管理/{f=1} f{print} f && /### 2\.20\.2/ {exit}' "$REPO/$UI_DOC")"
    API_HAS_SN=0; grep -Fq '`sn`' <<<"$API_SEC" && API_HAS_SN=1
    API_HAS_CATID=0; grep -Fq '`category_id`' <<<"$API_SEC" && API_HAS_CATID=1
    API_HAS_EXCLUDE=0; grep -Fq '`exclude_status`' <<<"$API_SEC" && API_HAS_EXCLUDE=1
    API_HAS_ALIAS=0; grep -Fq '`stock_status`' <<<"$API_SEC" && API_HAS_ALIAS=1
    API_PARAMS=0
    [ "$API_HAS_SN" = "1" ] && [ "$API_HAS_CATID" = "1" ] && [ "$API_HAS_EXCLUDE" = "1" ] && [ "$API_HAS_ALIAS" = "1" ] && API_PARAMS=1
    API_FIELDS_SN=0
    grep -Fq '识别码' <<<"$API_SEC" && API_FIELDS_SN=1
    UI_XLSX=0; grep -Fq '.xlsx' <<<"$UI_SEC" && UI_XLSX=1
    UI_FOLLOW=0; grep -Fq '跟随当前筛选' <<<"$UI_SEC" && UI_FOLLOW=1
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# docs/spec/api/api.md §5.8.2 与 docs/spec/ui/ui.md §2.20.1 的导出契约同步"
      printf 'API_SECTION_LINES=%s\n' "$(printf '%s\n' "$API_SEC" | wc -l | tr -d ' ')"
      printf 'API_EXPORT_PARAMS=%s\n' "$API_PARAMS"
      printf 'API_EXPORT_FIELDS_SN=%s\n' "$API_FIELDS_SN"
      printf 'UI_EXPORT_XLSX=%s\n' "$UI_XLSX"
      printf 'UI_EXPORT_FOLLOW_FILTER=%s\n' "$UI_FOLLOW"
      if [ "$API_PARAMS" = "1" ] && [ "$API_FIELDS_SN" = "1" ] && [ "$UI_XLSX" = "1" ] && [ "$UI_FOLLOW" = "1" ]; then
        echo "R4_OK"
      else
        echo "R4_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R4_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'API_EXPORT_PARAMS=1' 'API_EXPORT_FIELDS_SN=1' 'UI_EXPORT_FOLLOW_FILTER=1' || RC=1
    ;;
  r5)
    NAME="R5-build.txt"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "ESLINT_CMD=cd frontend-pc && npx eslint src/pages/InstrumentStock.jsx src/utils/instrumentStockFilters.js"
      ( cd "$REPO/frontend-pc" && npx eslint src/pages/InstrumentStock.jsx src/utils/instrumentStockFilters.js ) \
        > "$WORK/eslint.out" 2>&1
      LINT=$?
      tail -6 "$WORK/eslint.out"
      NO_UNDEF="$(grep -c 'no-undef' "$WORK/eslint.out" || true)"
      printf 'ESLINT_EXIT=%s\n' "$LINT"
      printf 'ESLINT_NOUNDEF=%s\n' "$NO_UNDEF"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      ( cd "$REPO/frontend-pc" && heavy npm run build ) > "$WORK/build.out" 2>&1
      BUILD=$?
      tail -4 "$WORK/build.out"
      printf 'PC_BUILD_EXIT=%s\n' "$BUILD"
      echo "PC_TESTS_CMD=cd frontend-pc && node --test tests/instrumentStockFilters.test.mjs"
      ( cd "$REPO/frontend-pc" && heavy node --test --test-reporter=tap tests/instrumentStockFilters.test.mjs ) \
        > "$WORK/unit.out" 2>&1
      UNIT=$?
      grep -E '^# (tests|pass|fail) ' "$WORK/unit.out"
      printf 'PC_TESTS_EXIT=%s\n' "$UNIT"
      echo "GO_BUILD_CMD=cd backend && go build ./..."
      ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/gobuild.out" 2>&1
      GOB=$?
      tail -3 "$WORK/gobuild.out"
      printf 'GO_BUILD_EXIT=%s\n' "$GOB"
      echo "GO_EXPORT_TEST_CMD=cd backend && go test ./handlers -run 'TestExportScope2578|TestExportInstruments' -count=1"
      ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestExportScope2578|TestExportInstruments' \
          -count=1 -timeout 10m ) > "$WORK/gotest.out" 2>&1
      GOT=$?
      tail -3 "$WORK/gotest.out"
      printf 'GO_EXPORT_TEST_EXIT=%s\n' "$GOT"
      if [ "$LINT" = "0" ] && [ "$NO_UNDEF" = "0" ] && [ "$BUILD" = "0" ] && [ "$UNIT" = "0" ] && \
         [ "$GOB" = "0" ] && [ "$GOT" = "0" ]; then
        echo "ESLINT_OK=1"
        echo "PC_BUILD_OK=1"
        echo "PC_TESTS_OK=1"
        echo "GO_BUILD_OK=1"
        echo "GO_EXPORT_TEST_OK=1"
        echo "R5_OK"
      else
        echo "ESLINT_OK=0"
        echo "PC_BUILD_OK=0"
        echo "GO_BUILD_OK=0"
        echo "GO_EXPORT_TEST_OK=0"
        echo "R5_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R5_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'ESLINT_OK=1' 'PC_BUILD_OK=1' 'GO_BUILD_OK=1' 'GO_EXPORT_TEST_OK=1' || RC=1
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
