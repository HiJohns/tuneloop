#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2577.sh — 证据生成器（#2577 R1–R5）
#
# 口径（Accepted Plan #2577 / Decision Lock D1–D10）：
#   R1 筛选语义纯函数单测（`node --test`）+ 断言集（参数/URL 回写/重置/词表/选项）。
#   R2 只读后端回归测试：页面发送的参数组合在真实 Postgres 数据上成立
#      （sn / 后代分类展开 / stock_status / exclude_status；total == len(list)）。
#   R3 页面接线结构断言（控件存在、整页 Spin 移除、筛选重置与选择清空已接线、
#      列集合无回归）。PC 无组件测试运行器 → Owner 豁免（D9）。
#   R4 文档同步（ui.md 筛选清单/词表/URL 口径；api.md 参数未漂移）。
#   R5 构建 / lint / 单测 / go build（重命令一律经 heavy-run.sh，D10）。
#
# 用法：bash scripts/verify-2577.sh <r1|r2|r3|r4|r5>
# 产物：docs/evidence/2577/R*-*.txt（首行 provenance）
# =============================================================================
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5) ;;
  *) echo "usage: $0 <r1|r2|r3|r4|r5>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2577
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"

export PATH="/usr/local/go/bin:$PATH"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"

HEAD_SHA="$(git rev-parse HEAD)"
if [ -z "${FIX_REF:-}" ]; then
  FIX_REF="$(git log --format=%H --grep="^fix(issue-$ISSUE)" -1 2>/dev/null || true)"
fi
FIX_SHA="$(git rev-parse "${FIX_REF:-HEAD}")"
git merge-base --is-ancestor "$FIX_SHA" "$HEAD_SHA" || { echo "FATAL: FIX_REF not ancestor" >&2; exit 3; }
FIX_SHORT="$(git rev-parse --short "$FIX_SHA")"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ]; then
  DIRTY=true
fi
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"
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

case "$ROW" in
r1)
  TESTS_LOG="$REPO/tmp/verify-$ISSUE-r1-tests.log"
  ( cd "$REPO/frontend-pc" && heavy node --test tests/instrumentStockFilters.test.mjs ) > "$TESTS_LOG" 2>&1
  TESTS_EXIT=$?
  PASS_COUNT="$(grep -c '^✔' "$TESTS_LOG" || true)"; PASS_COUNT="${PASS_COUNT:-0}"
  FAIL_COUNT="$(grep -c '^✖' "$TESTS_LOG" || true)"; FAIL_COUNT="${FAIL_COUNT:-0}"
  emit "TESTS_CMD=cd frontend-pc && node --test tests/instrumentStockFilters.test.mjs"
  emit "TESTS_EXIT=$TESTS_EXIT"
  emit "TESTS_PASS=$PASS_COUNT"
  emit "TESTS_FAIL=$FAIL_COUNT"
  [ "$TESTS_EXIT" = "0" ] || RC=1
  [ "$FAIL_COUNT" = "0" ] || RC=1
  [ "$PASS_COUNT" -ge 12 ] || RC=1

  # 断言集：直接 import 生产纯函数（绝对路径），输出计划登记的键值。
  ASSERT_JS="$REPO/tmp/verify-$ISSUE-r1-assert.mjs"
  cat > "$ASSERT_JS" <<'JS'
import {
  applyFilterPatch,
  buildStockFilterQuery,
  buildStockListParams,
  readStockFilters,
  stockStatusLabel,
  stockStatusOptions,
} from '__MODULE__'

const read = readStockFilters(new URLSearchParams('status=available&sn=CVZ-104&category_id=c1'))
const snParams = buildStockListParams({ filters: { sn: 'CVZ-104' }, page: 1, pageSize: 20, isManagerLike: true })
const categoryParams = buildStockListParams({ filters: { categoryId: 'c1' }, page: 1, pageSize: 20, isManagerLike: true })
const statusParams = buildStockListParams({ filters: { status: 'available' }, page: 1, pageSize: 20, isManagerLike: true })
const explicitLost = buildStockListParams({ filters: { status: 'lost' }, page: 1, pageSize: 20, isManagerLike: false })
const nonManager = buildStockListParams({ filters: {}, page: 1, pageSize: 20, isManagerLike: false })
const patched = applyFilterPatch({ filters: read, page: 3, selectedRowKeys: ['a', 'b'] }, { sn: 'CVZ-105' })
const urlQuery = new URLSearchParams(buildStockFilterQuery({ status: 'available' })).toString()
const options = stockStatusOptions({
  statuses: ['available', 'rented', 'maintenance', 'reserved', 'shipping', 'lost', 'archived', 'sold'].map((value) => ({ value })),
  isManagerLike: false,
  current: read.status,
})

const push = (key, value) => console.log(key + '=' + (value == null ? '' : value))
push('PARAMS_SN', snParams.sn)
push('PARAMS_CATEGORY', categoryParams.category_id)
push('PARAMS_STATUS', statusParams.stock_status)
push('PARAMS_EXCLUDE', explicitLost.exclude_status)
push('PARAMS_EXCLUDE_NONMGR', nonManager.exclude_status)
push('FILTER_PATCH_PAGE', patched.page)
push('FILTER_PATCH_SELECTED', patched.selectedRowKeys.length)
push('URL_STATUS', urlQuery)
push('LABEL_AVAILABLE', stockStatusLabel('available'))
push('OPTIONS_NONMGR', options.length)
JS
  sed "s|__MODULE__|$REPO/frontend-pc/src/utils/instrumentStockFilters.js|" "$ASSERT_JS" > "$ASSERT_JS.run.mjs"
  ASSERT_LOG="$REPO/tmp/verify-$ISSUE-r1-assert.log"
  ( cd "$REPO/frontend-pc" && node "$ASSERT_JS.run.mjs" ) > "$ASSERT_LOG" 2>&1
  ASSERT_EXIT=$?
  while IFS= read -r line; do [ -n "$line" ] && emit "$line"; done < "$ASSERT_LOG"
  emit "ASSERT_EXIT=$ASSERT_EXIT"
  [ "$ASSERT_EXIT" = "0" ] || RC=1

  require "TESTS_FAIL=0"
  require "PARAMS_SN=CVZ-104"
  require "PARAMS_CATEGORY=c1"
  require "PARAMS_STATUS=available"
  require "PARAMS_EXCLUDE="
  require "FILTER_PATCH_PAGE=1"
  require "FILTER_PATCH_SELECTED=0"
  require "URL_STATUS=status=available"
  require "LABEL_AVAILABLE=待租"
  require "OPTIONS_NONMGR=5"
  grep -qE '^TESTS_PASS=([1-9][0-9]+)' "$BODY" || { echo "ASSERT_FAIL: TESTS_PASS>=12" >&2; RC=1; }
  finish R1-filter-unit.txt || RC=1
  ;;

r2)
  LOG="$REPO/tmp/verify-$ISSUE-r2.log"
  ( cd "$REPO/backend" && heavy go test ./handlers -run 'TestInstrumentStockFilters2577' -count=1 -v -timeout 10m ) > "$LOG" 2>&1
  TRC=$?
  grep -E 'SN_HITS=|CATEGORY_HITS=|STATUS_HITS=|EXCLUDE_HITS=|COMBINED_HITS=|TOTAL_MATCH=' "$LOG" \
    | sed 's/^[[:space:]]*[^:]*: //' | while IFS= read -r l; do emit "$l"; done
  emit "GO_TEST_EXIT=$TRC"
  [ "$TRC" = "0" ] || RC=1
  require 'SN_HITS=1'
  require 'CATEGORY_HITS=2'
  require 'STATUS_HITS=3'
  require 'EXCLUDE_HITS=3'
  require 'TOTAL_MATCH=1'
  finish R2-api-params.txt || RC=1
  ;;

r3)
  PAGE="frontend-pc/src/pages/InstrumentStock.jsx"
  [ -f "$PAGE" ] || { echo "FATAL: $PAGE missing" >&2; exit 2; }
  SCAN_OUT="$(cd "$REPO" && python3 - "$PAGE" <<'PY'
import re, sys

path = sys.argv[1]
src = open(path, encoding='utf-8').read()

def hits(pattern):
    return len(re.findall(pattern, src))

start = src.index('const columns = [')
end = src.index('// #2043: 行级删除', start)
block = src[start:end]
titles = re.findall(r"title: '([^']+)'", block)
data_titles = [t for t in titles if t != '操作']

print(f'STOCK_COL_COUNT={len(data_titles)}')
print(f'STOCK_COL_HEADERS={",".join(data_titles)}')
print(f'STOCK_ACTION_COL={1 if "操作" in titles else 0}')
print(f'SN_SEARCH_INPUT={1 if "Input.Search" in src and "搜索识别码" in src else 0}')
print(f'CATEGORY_SELECT={hits(r"patchFilters\(\{ categoryId:")}')
print(f'STATUS_SELECT={hits(r"patchFilters\(\{ status:")}')
print(f'CLEAR_BUTTON={1 if "const clearFilters" in src and "清空筛选" in src else 0}')
print(f'TABLE_LOADING={1 if re.search(r"<Table[\s\S]{0,400}?loading=\{loading\}", src) else 0}')
print(f'EARLY_SPIN_RETURN={hits(r"if \(loading\) \{\s*\n\s*return")}')
print(f'FILTER_RESET_WIRED={1 if "applyFilterPatch" in src else 0}')
print(f'SELECTION_CLEAR_WIRED={1 if re.search(r"setSelectedRowKeys\(next\.selectedRowKeys\)", src) else 0}')
print(f'LEGACY_STATUS_BANNER={1 if "当前筛选: 状态" in src else 0}')
print(f'STATUS_COLORS_CONST={hits(r"const statusColors")}')
print(f'FILTER_FN_IMPORT={1 if "instrumentStockFilters" in src else 0}')
PY
)"
  printf '%s\n' "$SCAN_OUT" | while IFS= read -r l; do [ -n "$l" ] && emit "$l"; done
  N_COL="$(printf '%s\n' "$SCAN_OUT" | sed -n 's/^STOCK_COL_COUNT=//p')"
  EARLY="$(printf '%s\n' "$SCAN_OUT" | sed -n 's/^EARLY_SPIN_RETURN=//p')"
  BANNER="$(printf '%s\n' "$SCAN_OUT" | sed -n 's/^LEGACY_STATUS_BANNER=//p')"
  COLORS="$(printf '%s\n' "$SCAN_OUT" | sed -n 's/^STATUS_COLORS_CONST=//p')"
  [ "$N_COL" = "8" ] || RC=1
  [ "$EARLY" = "0" ] || RC=1
  [ "$BANNER" = "0" ] || RC=1
  [ "$COLORS" = "0" ] || RC=1
  require 'STOCK_COL_COUNT=8'
  require 'SN_SEARCH_INPUT=1'
  require 'CATEGORY_SELECT=1'
  require 'STATUS_SELECT=1'
  require 'CLEAR_BUTTON=1'
  require 'TABLE_LOADING=1'
  require 'EARLY_SPIN_RETURN=0'
  require 'FILTER_RESET_WIRED=1'
  require 'SELECTION_CLEAR_WIRED=1'
  finish R3-page-wiring.txt || RC=1
  ;;

r4)
  UI="docs/spec/ui/ui.md"
  API="docs/spec/api/api.md"
  DOC_OUT="$(cd "$REPO" && python3 - "$UI" "$API" <<'PY'
import sys

ui = open(sys.argv[1], encoding='utf-8').read()
api = open(sys.argv[2], encoding='utf-8').read()

start = ui.find('**筛选（#2577）**')
block = ui[start:start + 2500] if start >= 0 else ''

controls = all(k in block for k in ['识别码搜索', '分类筛选', '状态筛选', '清空筛选'])
vocab_ok = all(k in block for k in ['待租', '在租', '维修中', '已预订', '配送中', '已丢失', '已下架', '已售出'])
url_params = all(k in block for k in ['?status=', 'stock_status', 'category_id', 'sn'])

print('UI_STOCK_FILTERS=' + ('1' if controls else '0'))
print('UI_STATUS_VOCAB=' + ('1' if vocab_ok else '0'))
print('UI_STOCK_URL_PARAMS=' + ('1' if url_params else '0'))

api_ok = all(k in api for k in ['`sn`（模糊）', '`category_id`（含后代）', '`stock_status`（等值）', '`exclude_status`'])
print('API_FILTER_PARAMS_DOC=' + ('1' if api_ok else '0'))
PY
)"
  printf '%s\n' "$DOC_OUT" | while IFS= read -r l; do [ -n "$l" ] && emit "$l"; done
  while IFS='=' read -r key value; do
    [ -n "${key:-}" ] || continue
    [ "$value" = "1" ] || RC=1
  done <<< "$DOC_OUT"
  require 'UI_STOCK_FILTERS=1'
  require 'UI_STATUS_VOCAB=1'
  require 'UI_STOCK_URL_PARAMS=1'
  require 'API_FILTER_PARAMS_DOC=1'
  finish R4-docs.txt || RC=1
  ;;

r5)
  LINT_LOG="$REPO/tmp/verify-$ISSUE-r5-lint.log"
  ( cd "$REPO/frontend-pc" && heavy npx eslint src/pages/InstrumentStock.jsx src/utils/instrumentStockFilters.js ) > "$LINT_LOG" 2>&1
  LINT_EXIT=$?
  emit "ESLINT_CMD=cd frontend-pc && npx eslint src/pages/InstrumentStock.jsx src/utils/instrumentStockFilters.js"
  emit "ESLINT_EXIT=$LINT_EXIT"
  if [ "$LINT_EXIT" = "0" ]; then emit "ESLINT_OK=1"; else RC=1; fi

  TESTS_LOG="$REPO/tmp/verify-$ISSUE-r5-tests.log"
  ( cd "$REPO/frontend-pc" && heavy node --test tests/instrumentStockFilters.test.mjs ) > "$TESTS_LOG" 2>&1
  TESTS_EXIT=$?
  emit "PC_TESTS_EXIT=$TESTS_EXIT"
  if [ "$TESTS_EXIT" = "0" ]; then emit "PC_TESTS_OK=1"; else RC=1; fi

  BUILD_LOG="$REPO/tmp/verify-$ISSUE-r5-build.log"
  ( cd "$REPO/frontend-pc" && heavy npm run build ) > "$BUILD_LOG" 2>&1
  BUILD_EXIT=$?
  emit "PC_BUILD_CMD=cd frontend-pc && npm run build"
  emit "PC_BUILD_EXIT=$BUILD_EXIT"
  if [ "$BUILD_EXIT" = "0" ]; then emit "PC_BUILD_OK=1"; else RC=1; fi

  GO_LOG="$REPO/tmp/verify-$ISSUE-r5-go-build.log"
  ( cd "$REPO/backend" && heavy go build ./... ) > "$GO_LOG" 2>&1
  GO_EXIT=$?
  emit "GO_BUILD_CMD=cd backend && go build ./..."
  emit "GO_BUILD_EXIT=$GO_EXIT"
  if [ "$GO_EXIT" = "0" ]; then emit "GO_BUILD_OK=1"; else RC=1; fi

  tail -3 "$BUILD_LOG" | sed 's/^/TAIL_PC_BUILD: /' | while IFS= read -r l; do emit "$l"; done

  require 'ESLINT_OK=1'
  require 'PC_TESTS_OK=1'
  require 'PC_BUILD_OK=1'
  require 'GO_BUILD_OK=1'
  finish R5-build.txt || RC=1
  ;;
esac

exit "$RC"
