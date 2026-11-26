#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2579.sh — 证据生成器（#2579 导出字段取值修复）
#
# 口径（docs/spec/api/api.md §5.8.2 导出字段）：
#   金额（daily_rate / deposit / overdue_daily_fee）= instrumentListPriceFields 单一链（分→元 2 位，
#   null → `-`，显式 0 → `0.00`）；level = InstrumentLevel.Caption → 回落 → `-`；category_name 直取；
#   已删除字段 brand / model / monthly_rate / stock 不再导出。
#
# 用法：bash scripts/verify-2579.sh <r1..r5> [--expect-red]
#   r1 docs/evidence/2579/R1-field-values.txt     逐单元格断言（A/B/C 三台乐器）
#   r1 --expect-red docs/evidence/2579/R2-red.txt 修复前采集红证（断言失败 + 空白单元格计数）
#   r2 docs/evidence/2579/R2-red-green.txt        红绿对照（引用红证 + 当前绿证）
#   r3 docs/evidence/2579/R3-fields-static.txt    死字段移除 + 单一链复用（源码结构断言）
#   r4 docs/evidence/2579/R4-docs.txt             文档同步（api.md / ui.md）
#   r5 docs/evidence/2579/R5-build.txt            构建 / lint / 回归
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`；另起一行 GENERATED_AT_HEAD。
# 重命令一律经 heavy-run.sh 取锁（D8）。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
EXPECT_RED=0
case "${2:-}" in
  --expect-red) EXPECT_RED=1 ;;
  "") ;;
  *) echo "usage: $0 <r1..r5> [--expect-red]" >&2; exit 2 ;;
esac
case "$MODE" in
  r1|r2|r3|r4|r5) ;;
  *)
    echo "usage: $0 <r1..r5> [--expect-red]" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2579
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
SERVICE_GO="backend/service/instrument.go"
HANDLER_GO="backend/handlers/instrument_import.go"
LIST_JSX="frontend-pc/src/pages/admin/instrument/List.jsx"
STOCK_JSX="frontend-pc/src/pages/InstrumentStock.jsx"
API_DOC="docs/spec/api/api.md"
UI_DOC="docs/spec/ui/ui.md"

export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" --reverse | head -1 2>/dev/null || true)"
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

go_test_2579() { # out
  ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestExportFieldValues2579' -count=1 -v -timeout 10m ) > "$1" 2>&1
  return $?
}

# 从 go test -v 输出中抽取 token（行前缀为 `file.go:NNN:`，不能锚定行首）。
extract_tokens() { # log
  grep -oE '(MONEY_[A-Z_]+|LEVEL_CAPTION_A|CATEGORY_NAME_A|STATUS_A|HEADER_COUNT|HEADER_JOINED)=.*' "$1" | sort -u
}

# 红证计数：金额/级别 token 为空（旧实现 getFieldValue 返回空串），即「空白列」。
count_blank_cells() { # log
  extract_tokens "$1" \
    | grep -E '^(MONEY_[A-Z_]+|LEVEL_CAPTION_A|CATEGORY_NAME_A)=' \
    | awk -F= '{ v=$2; gsub(/^[ \t]+|[ \t]+$/, "", v); if (v=="") c++ } END { print c+0 }'
}

NAME=""
RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    if [ "$EXPECT_RED" = "1" ]; then
      NAME="R2-red.txt"
      go_test_2579 "$WORK/r1.test.log"; TEST_RC=$?
      BLANKS="$(count_blank_cells "$WORK/r1.test.log")"
      {
        printf '%s\n' "$PROV"
        printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
        echo "# 修复前红证：同一断言集（A/B/C 三台乐器的金额/级别/分类单元格）"
        printf 'RED_STAGE=pre-fix\n'
        printf 'RED_TEST_EXIT=%s\n' "$TEST_RC"
        printf 'RED_BLANK_CELLS=%s\n' "$BLANKS"
        extract_tokens "$WORK/r1.test.log"
        grep -E '^(FAIL|--- FAIL)' "$WORK/r1.test.log" | sed 's/^ *//' | head -3
        if [ "$TEST_RC" != "0" ] && [ "$BLANKS" -ge 1 ]; then
          echo "RED_CAPTURED=1"
          echo "R1_RED_OK"
        else
          echo "RED_CAPTURED=0"
          echo "R1_RED_FAIL"
        fi
      } > "$LOG" 2>&1
      grep -q '^R1_RED_OK$' "$LOG" || RC=1
      require_tokens "$LOG" 'RED_CAPTURED=1' 'RED_TEST_EXIT=1' 'RED_BLANK_CELLS=' || RC=1
      BLANK_VAL="$(sed -n 's/^RED_BLANK_CELLS=\([0-9]\+\)$/\1/p' "$LOG" | head -1)"
      [ "${BLANK_VAL:-0}" -ge 1 ] || RC=1
    else
      NAME="R1-field-values.txt"
      go_test_2579 "$WORK/r1.test.log"; TEST_RC=$?
      {
        printf '%s\n' "$PROV"
        printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
        echo "# 逐单元格断言：金额（分→元 2 位 / null → - / 显式 0 → 0.00）、级别 caption、分类名、表头无死字段"
        printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
        extract_tokens "$WORK/r1.test.log"
        [ "$TEST_RC" = "0" ] && echo "R1_OK" || echo "R1_FAIL"
      } > "$LOG" 2>&1
      grep -q '^R1_OK$' "$LOG" || RC=1
      require_tokens "$LOG" 'GO_TEST_EXIT=0' 'MONEY_DAILY_A=100.00' 'MONEY_DAILY_B=150.00' \
        'MONEY_DEPOSIT_B=0.00' 'MONEY_DAILY_C=-' 'LEVEL_CAPTION_A=专业' 'CATEGORY_NAME_A=大提琴' \
        'STATUS_A=available' || RC=1
      # 表头不得含死字段
      if grep -E '^HEADER_JOINED=' "$LOG" | grep -Eq '品牌|型号|月租金|库存数量'; then
        echo "ASSERT_FAIL: 表头仍含已删除字段" >&2
        RC=1
      fi
    fi
    ;;
  r2)
    NAME="R2-red-green.txt"
    RED_FILE="$OUT_DIR/R2-red.txt"
    go_test_2579 "$WORK/r2.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 红绿对照（红证必须由固定 commit 之前的实现采集）"
      if [ -f "$RED_FILE" ]; then
        echo "--- red ---"
        sed -n '1,20p' "$RED_FILE"
        RED_CAPTURED="$(sed -n 's/^RED_CAPTURED=//p' "$RED_FILE" | head -1)"
        RED_BLANKS="$(sed -n 's/^RED_BLANK_CELLS=//p' "$RED_FILE" | head -1)"
      else
        RED_CAPTURED=""
        RED_BLANKS=""
      fi
      printf 'RED_CAPTURED=%s\n' "${RED_CAPTURED:-0}"
      printf 'RED_BLANK_CELLS=%s\n' "${RED_BLANKS:-0}"
      echo "--- green ---"
      printf 'GREEN_TEST_EXIT=%s\n' "$TEST_RC"
      extract_tokens "$WORK/r2.test.log" | grep -E '^(MONEY_[A-Z_]+|LEVEL_CAPTION_A|CATEGORY_NAME_A)='
      if [ "${RED_CAPTURED:-0}" = "1" ] && [ "${RED_BLANKS:-0}" -ge 1 ] && [ "$TEST_RC" = "0" ]; then
        echo "GREEN_OK=1"
        echo "R2_OK"
      else
        echo "GREEN_OK=0"
        echo "R2_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R2_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'RED_CAPTURED=1' 'GREEN_OK=1' 'GREEN_TEST_EXIT=0' || RC=1
    ;;
  r3)
    NAME="R3-fields-static.txt"
    HEADER_BLOCK="$(awk '/^var headerMap = map\[string\]string\{/,/^\}/' "$REPO/$SERVICE_GO")"
    DEAD_IN_HEADER="$(printf '%s' "$HEADER_BLOCK" | grep -cE '"(brand|model|monthly_rate|stock)"' || true)"
    DECLARED="$(printf '%s' "$HEADER_BLOCK" | grep -cE '^\s+"[a-z_]+":' || true)"
    DEAD_IN_PC="$(grep -cE "value: '(brand|model|stock|monthly_rate)'" "$REPO/$LIST_JSX" || true)"
    SINGLE_CHAIN="$(grep -c 'instrumentListPriceFields(' "$REPO/$HANDLER_GO" || true)"
    MONEY_FIELDS="$(grep -c 'opts.MoneyFields' "$REPO/$SERVICE_GO" || true)"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 死字段移除（headerMap / List 字段选择器）+ 金额单一链复用（handler 注入 MoneyFields）"
      printf 'DEAD_FIELDS_IN_HEADER=%s\n' "$DEAD_IN_HEADER"
      printf 'DECLARED_FIELDS=%s\n' "$DECLARED"
      printf 'DEAD_FIELDS_IN_PC_OPTIONS=%s\n' "$DEAD_IN_PC"
      printf 'SINGLE_CHAIN_REUSE=%s\n' "$([ "$SINGLE_CHAIN" -ge 1 ] && echo 1 || echo 0)"
      printf 'MONEY_FIELDS_REF=%s\n' "$MONEY_FIELDS"
      echo "--- headerMap ---"
      printf '%s\n' "$HEADER_BLOCK"
      if [ "$DEAD_IN_HEADER" = "0" ] && [ "$DECLARED" = "9" ] && [ "$DEAD_IN_PC" = "0" ] && \
         [ "$SINGLE_CHAIN" -ge 1 ] && [ "$MONEY_FIELDS" -ge 1 ]; then
        echo "R3_OK"
      else
        echo "R3_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R3_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'DEAD_FIELDS_IN_HEADER=0' 'DEAD_FIELDS_IN_PC_OPTIONS=0' 'SINGLE_CHAIN_REUSE=1' || RC=1
    ;;
  r4)
    NAME="R4-docs.txt"
    API_SEC="$(awk '/^#### 5\.8\.2 导出乐器列表/{f=1} f{print} f&&/^#### 5\.8\.3/{exit}' "$REPO/$API_DOC")"
    UI_SEC="$(awk '/^### 2\.20\.1 PC 乐器管理/{f=1} f{print} f&&/^### 2\.20\.2/{exit}' "$REPO/$UI_DOC")"
    API_FIELDS=0
    grep -Fq 'overdue_daily_fee' <<<"$API_SEC" && grep -Fq '逾期租金' <<<"$API_SEC" && API_FIELDS=1
    API_REMOVED=0
    grep -Fq 'brand' <<<"$API_SEC" && grep -Fq '已移除' <<<"$API_SEC" && API_REMOVED=1
    API_MONEY=0
    grep -Fq '分' <<<"$API_SEC" && grep -Fq '2 位小数' <<<"$API_SEC" && API_MONEY=1
    UI_FIELDS=0
    grep -Fq 'sn,category_name,level,daily_rate,deposit,overdue_daily_fee,status' <<<"$UI_SEC" && UI_FIELDS=1
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 文档同步：api.md 导出字段集合/金额单位/已移除字段；ui.md 两页导出字段集合"
      printf 'API_EXPORT_FIELDS=%s\n' "$API_FIELDS"
      printf 'API_REMOVED_FIELDS=%s\n' "$API_REMOVED"
      printf 'API_MONEY_UNIT=%s\n' "$API_MONEY"
      printf 'UI_EXPORT_FIELDS=%s\n' "$UI_FIELDS"
      if [ "$API_FIELDS" = "1" ] && [ "$API_REMOVED" = "1" ] && [ "$API_MONEY" = "1" ] && [ "$UI_FIELDS" = "1" ]; then
        echo "R4_OK"
      else
        echo "R4_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R4_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'API_EXPORT_FIELDS=1' 'API_REMOVED_FIELDS=1' 'API_MONEY_UNIT=1' 'UI_EXPORT_FIELDS=1' || RC=1
    ;;
  r5)
    NAME="R5-build.txt"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "GO_BUILD_CMD=cd backend && go build ./..."
      ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/gobuild.out" 2>&1
      GOB=$?
      tail -3 "$WORK/gobuild.out"
      printf 'GO_BUILD_EXIT=%s\n' "$GOB"
      echo "GO_TEST_2579_CMD=cd backend && go test ./handlers -run 'TestExportFieldValues2579|TestExportInstruments' -count=1"
      ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestExportFieldValues2579|TestExportInstruments' -count=1 ) > "$WORK/gotest.out" 2>&1
      GOT=$?
      tail -3 "$WORK/gotest.out"
      printf 'GO_TEST_2579_EXIT=%s\n' "$GOT"
      echo "ESLINT_CMD=cd frontend-pc && npx eslint src/pages/admin/instrument/List.jsx src/pages/InstrumentStock.jsx"
      ( cd "$REPO/frontend-pc" && npx eslint src/pages/admin/instrument/List.jsx src/pages/InstrumentStock.jsx ) > "$WORK/eslint.out" 2>&1
      LINT=$?
      tail -5 "$WORK/eslint.out"
      NO_UNDEF="$(grep -c 'no-undef' "$WORK/eslint.out" || true)"
      printf 'ESLINT_EXIT=%s\n' "$LINT"
      printf 'ESLINT_NOUNDEF=%s\n' "$NO_UNDEF"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      ( cd "$REPO/frontend-pc" && heavy npm run build ) > "$WORK/pcbuild.out" 2>&1
      PCB=$?
      tail -4 "$WORK/pcbuild.out"
      printf 'PC_BUILD_EXIT=%s\n' "$PCB"
      if [ "$GOB" = "0" ] && [ "$GOT" = "0" ] && [ "$NO_UNDEF" = "0" ] && [ "$PCB" = "0" ]; then
        echo "GO_BUILD_OK=1"
        echo "GO_TEST_2579_OK=1"
        echo "ESLINT_NOUNDEF=0"
        echo "PC_BUILD_OK=1"
        echo "R5_OK"
      else
        echo "GO_BUILD_OK=0"
        echo "R5_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R5_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_BUILD_OK=1' 'GO_TEST_2579_OK=1' 'ESLINT_NOUNDEF=0' 'PC_BUILD_OK=1' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
cp "$LOG" "$ART"
echo "ARTIFACT=$ART"
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE${EXPECT_RED:+ expect-red=$EXPECT_RED})" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)"
