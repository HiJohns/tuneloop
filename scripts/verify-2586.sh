#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2586.sh — 证据生成器（#2586 乐器数据进/出端点 cus_perm 守卫对齐）
#
# 口径（docs/spec/permissions.md §3.2.1「乐器数据进/出 端点-权限对照」）：
#   导出 = instrument:read；导入族（import / import-template / batch-import /
#   batch-import template / preview / media）= instrument:create。
#
# 用法：bash scripts/verify-2586.sh <r1..r6>
#   r1 docs/evidence/2586/R1-export-perm.txt        导出守卫（无 read 位 403/40305；有 read 位 200 且 SN=2；legacy/system 放行）
#   r2 docs/evidence/2586/R2-import-family-perm.txt 导入族 6 端点守卫（无 create 位 403×6；有 create 位非 403×6）
#   r3 docs/evidence/2586/R3-route-wiring.txt       路由表接线结构断言 + fix^ 红样
#   r4 docs/evidence/2586/R4-pc-gates.txt           PC 入口门控（导入按钮 / 批量导入路由 / 库存监控既有 gate）
#   r5 docs/evidence/2586/R5-build-lint.txt         go build + 全包编译冒烟 + 定向测试 + eslint + vite build
#   r6 docs/evidence/2586/R6-docs.txt               api.md 权限口径 + permissions.md 端点对照表
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`；另起一行 GENERATED_AT_HEAD。
# 重命令一律经 heavy-run.sh 取锁（D8）。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6) ;;
  *)
    echo "usage: $0 <r1..r6>" >&2
    exit 2
    ;;
esac

# 本机 Go 位于 /usr/local/go/bin（不在默认 PATH）→ 脚本自带兜底，避免 GO_TEST_EXIT=127 的假失败。
case ":$PATH:" in
  *":/usr/local/go/bin:"*) ;;
  *) [ -d /usr/local/go/bin ] && PATH="/usr/local/go/bin:$PATH" && export PATH ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2586
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
MAIN_GO="backend/main.go"
LIST_JSX="frontend-pc/src/pages/admin/instrument/List.jsx"
APP_JSX="frontend-pc/src/App.jsx"
STOCK_JSX="frontend-pc/src/pages/InstrumentStock.jsx"
API_DOC="docs/spec/api/api.md"
PERM_DOC="docs/spec/permissions.md"

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

go_test_2586() { # package, run-regex, out
  local pkg="$1" re="$2" out="$3"
  ( cd "$REPO/backend" && heavy go test "$pkg" -run "$re" -count=1 -v -timeout 10m ) > "$out" 2>&1
  return $?
}

# route_lines <main.go content file> <path> → 注册行
route_line() { # file path method
  local f="$1" path="$2" method="$3"
  grep -F "authRequired.$method(\"$path\"" "$f" | head -1
}

NAME=""
RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-export-perm.txt"
    go_test_2586 ./handlers 'TestExportPerm2586' "$WORK/r1.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# GET /api/instruments/export + RequireCusPerm(instrument:read)"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'EXPORT_[A-Z_]+=' "$WORK/r1.test.log" | sed 's/^ *//'
      [ "$TEST_RC" = "0" ] && echo "R1_OK" || echo "R1_FAIL"
    } > "$LOG" 2>&1
    grep -q '^R1_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'EXPORT_NO_READ_PERM_STATUS=403' 'EXPORT_NO_READ_PERM_CODE=40305' \
      'EXPORT_NO_READ_PERM_XLSX=0' 'EXPORT_WITH_READ_PERM_STATUS=200' 'EXPORT_WITH_READ_PERM_SNS=2' \
      'EXPORT_ZERO_CUSPERM_STATUS=200' 'EXPORT_SYSTEM_ADMIN_STATUS=200' || RC=1
    ;;
  r2)
    NAME="R2-import-family-perm.txt"
    go_test_2586 ./handlers 'TestImportFamilyPerm2586' "$WORK/r2.test.log"; TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 导入族 6 端点 + RequireCusPerm(instrument:create)"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'IMPORT_FAMILY_[A-Z0-9_]+=' "$WORK/r2.test.log" | sed 's/^ *//'
      [ "$TEST_RC" = "0" ] && echo "R2_OK" || echo "R2_FAIL"
    } > "$LOG" 2>&1
    grep -q '^R2_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'IMPORT_FAMILY_ENDPOINTS=6' 'IMPORT_FAMILY_403=6' \
      'IMPORT_FAMILY_40305=6' 'IMPORT_FAMILY_WITH_CREATE_NOT403=6' || RC=1
    ;;
  r3)
    NAME="R3-route-wiring.txt"
    go_test_2586 . 'TestInstrumentPermRoutes2586' "$WORK/r3.test.log"; TEST_RC=$?
    # 红样：fix 前 main.go 的未守卫数量（绿样由上面的结构断言给出）
    if [ -z "${RED_REF:-}" ]; then
      FIRST_FIX="$(git log --format=%H --grep="^fix(issue-$ISSUE)" --reverse | head -1 2>/dev/null || true)"
      RED_REF="${FIRST_FIX:-$FIX_SHA}^"
    fi
    git show "$RED_REF:$MAIN_GO" > "$WORK/main.before.go" 2>/dev/null || {
      echo "FATAL: cannot read pre-fix main.go ($RED_REF)" >&2; exit 3; }
    RED_EXPORT=0
    [ -z "$(route_line "$WORK/main.before.go" /instruments/export GET | grep -F 'RequireCusPerm')" ] && RED_EXPORT=1
    RED_IMPORT=0
    for p in /instruments/import /instruments/import/template /instruments/batch-import \
             /instruments/batch-import/template /instruments/batch-import/preview /instruments/batch-import/media; do
      m=POST
      case "$p" in *template) m=GET ;; esac
      [ -z "$(route_line "$WORK/main.before.go" "$p" "$m" | grep -F 'RequireCusPerm')" ] && RED_IMPORT=$((RED_IMPORT + 1))
    done
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 路由注册行结构断言（backend/instrument_route_guard_2586_test.go）+ 首个 fix 的父提交红样"
      printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
      grep -E 'ROUTE_[A-Z_]+=' "$WORK/r3.test.log" | sed 's/^ *//'
      printf 'RED_SOURCE=%s:%s\n' "$RED_REF" "$MAIN_GO"
      printf 'RED_PRE_FIX_UNGUARDED_EXPORT=%s\n' "$RED_EXPORT"
      printf 'RED_PRE_FIX_UNGUARDED_IMPORT=%s\n' "$RED_IMPORT"
      [ "$TEST_RC" = "0" ] && echo "R3_OK" || echo "R3_FAIL"
    } > "$LOG" 2>&1
    grep -q '^R3_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_TEST_EXIT=0' 'ROUTE_EXPORT_GUARDED=1' 'ROUTE_IMPORT_FAMILY_GUARDED=6' \
      'ROUTE_INSTRUMENT_IO_UNGUARDED=0' 'RED_PRE_FIX_UNGUARDED_EXPORT=1' 'RED_PRE_FIX_UNGUARDED_IMPORT=6' || RC=1
    ;;
  r4)
    NAME="R4-pc-gates.txt"
    # 导入按钮：PermissionGate 必须直接包住 navigate('/instruments/batch-import') 的按钮
    BTN_BLOCK="$(awk "/onClick=\{\(\) => navigate\('\/instruments\/batch-import'\)\}/{f=1} f{print} f&&/导入/{exit}" "$REPO/$LIST_JSX")"
    BTN_LINE="$(grep -n "navigate('/instruments/batch-import')" "$REPO/$LIST_JSX" | head -1 | cut -d: -f1)"
    GATE_BEFORE=0
    if [ -n "$BTN_LINE" ]; then
      FROM=$((BTN_LINE > 6 ? BTN_LINE - 6 : 1))
      sed -n "${FROM},${BTN_LINE}p" "$REPO/$LIST_JSX" | grep -Fq '<PermissionGate code="instrument:create">' && GATE_BEFORE=1
    fi
    ROUTE_GATED=0
    grep -F '/instruments/batch-import" element={<ProtectedRoute requiredPermission={{ cusPermCodes: [' "$REPO/$APP_JSX" \
      | grep -Fq "instrument:create" && ROUTE_GATED=1
    STOCK_EXPORT_GATE=0
    STOCK_IMPORT_GATE=0
    STOCK_BLOCK="$(awk '/导出 Excel|批量导入/{print}' "$REPO/$STOCK_JSX")"
    awk "/<PermissionGate code=\"instrument:read\">/{f=1;next} f&&/导出 Excel/{print \"EXPORT\"; exit}" "$REPO/$STOCK_JSX" > "$WORK/stock-export.txt"
    grep -q EXPORT "$WORK/stock-export.txt" && STOCK_EXPORT_GATE=1
    awk "/<PermissionGate code=\"instrument:create\">/{f=1;next} f&&/批量导入/{print \"IMPORT\"; exit}" "$REPO/$STOCK_JSX" > "$WORK/stock-import.txt"
    grep -q IMPORT "$WORK/stock-import.txt" && STOCK_IMPORT_GATE=1
    UNGATED=0
    [ "$GATE_BEFORE" = "1" ] || UNGATED=1
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# PC 入口门控：List 导入按钮 / batch-import 路由 / 库存监控既有 gate（回归）"
      printf 'PC_LIST_IMPORT_BUTTON_GATED=%s\n' "$GATE_BEFORE"
      printf 'PC_LIST_IMPORT_BUTTON_UNGATED=%s\n' "$UNGATED"
      printf 'PC_BATCH_IMPORT_ROUTE_GATED=%s\n' "$ROUTE_GATED"
      printf 'PC_STOCK_EXPORT_GATE=%s\n' "$STOCK_EXPORT_GATE"
      printf 'PC_STOCK_BATCH_IMPORT_GATE=%s\n' "$STOCK_IMPORT_GATE"
      echo "--- import button block ---"
      printf '%s\n' "$BTN_BLOCK"
      echo "--- stock gates ---"
      grep -nE '<PermissionGate code="instrument:(read|create)">|导出 Excel|批量导入</Button>' "$REPO/$STOCK_JSX" | head -6
      if [ "$GATE_BEFORE" = "1" ] && [ "$UNGATED" = "0" ] && [ "$ROUTE_GATED" = "1" ] && \
         [ "$STOCK_EXPORT_GATE" = "1" ] && [ "$STOCK_IMPORT_GATE" = "1" ]; then
        echo "R4_OK"
      else
        echo "R4_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R4_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'PC_LIST_IMPORT_BUTTON_GATED=1' 'PC_LIST_IMPORT_BUTTON_UNGATED=0' \
      'PC_BATCH_IMPORT_ROUTE_GATED=1' 'PC_STOCK_EXPORT_GATE=1' 'PC_STOCK_BATCH_IMPORT_GATE=1' || RC=1
    ;;
  r5)
    NAME="R5-build-lint.txt"
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "GO_BUILD_CMD=cd backend && go build ./..."
      ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/gobuild.out" 2>&1
      GOB=$?
      tail -3 "$WORK/gobuild.out"
      printf 'GO_BUILD_EXIT=%s\n' "$GOB"
      echo "GO_COMPILE_SMOKE_CMD=cd backend && go test ./... -run '^$' -count=1"
      ( cd "$REPO/backend" && heavy go test ./... -run '^$' -count=1 ) > "$WORK/gosmoke.out" 2>&1
      GOS=$?
      tail -3 "$WORK/gosmoke.out"
      printf 'GO_COMPILE_SMOKE_EXIT=%s\n' "$GOS"
      echo "GO_TEST_2586_CMD=cd backend && go test ./handlers . -run 'TestExportPerm2586|TestImportFamilyPerm2586|TestInstrumentPermRoutes2586' -count=1"
      ( cd "$REPO/backend" && heavy go test ./handlers . -run 'TestExportPerm2586|TestImportFamilyPerm2586|TestInstrumentPermRoutes2586' -count=1 ) > "$WORK/gotest.out" 2>&1
      GOT=$?
      tail -3 "$WORK/gotest.out"
      printf 'GO_TEST_2586_EXIT=%s\n' "$GOT"
      echo "ESLINT_CMD=cd frontend-pc && npx eslint src/pages/admin/instrument/List.jsx src/App.jsx src/pages/InstrumentStock.jsx"
      ( cd "$REPO/frontend-pc" && npx eslint src/pages/admin/instrument/List.jsx src/App.jsx src/pages/InstrumentStock.jsx ) > "$WORK/eslint.out" 2>&1
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
      if [ "$GOB" = "0" ] && [ "$GOS" = "0" ] && [ "$GOT" = "0" ] && [ "$NO_UNDEF" = "0" ] && [ "$PCB" = "0" ]; then
        echo "GO_BUILD_OK=1"
        echo "GO_COMPILE_SMOKE_OK=1"
        echo "GO_TEST_2586_OK=1"
        echo "ESLINT_NOUNDEF=0"
        echo "PC_BUILD_OK=1"
        echo "R5_OK"
      else
        echo "GO_BUILD_OK=0"
        echo "PC_BUILD_OK=0"
        echo "R5_FAIL"
      fi
    } > "$LOG" 2>&1
    grep -q '^R5_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'GO_BUILD_OK=1' 'GO_COMPILE_SMOKE_OK=1' 'GO_TEST_2586_OK=1' \
      'ESLINT_NOUNDEF=0' 'PC_BUILD_OK=1' || RC=1
    ;;
  r6)
    NAME="R6-docs.txt"
    API_EXPORT=0; grep -A 6 '^#### 5\.8\.2 导出乐器列表' "$REPO/$API_DOC" | grep -Fq '`instrument:read`' && API_EXPORT=1
    API_IMPORT=0; grep -A 6 '^#### 5\.8\.1 导入乐器信息' "$REPO/$API_DOC" | grep -Fq '`instrument:create`' && API_IMPORT=1
    API_TEMPLATE=0; grep -A 8 '^#### 5\.8\.3 下载导入模板' "$REPO/$API_DOC" | grep -Fq '`instrument:create`' && API_TEMPLATE=1
    # Rev.3（Owner 2026-11-27）：一行一端点 = 8 行，逐端点首列必须是端点路径
    PERM_ROWS="$(grep -cE '^\| `/api/instruments' "$REPO/$PERM_DOC" || true)"
    has_perm_row() { grep -Fq -- "$1" "$REPO/$PERM_DOC" && echo 1 || echo 0; }
    PERM_HAS_LIST="$(has_perm_row '| `/api/instruments`（列表） | GET |')"
    PERM_HAS_EXPORT="$(has_perm_row '| `/api/instruments/export` | GET |')"
    PERM_HAS_IMPORT="$(has_perm_row '| `/api/instruments/import` | POST |')"
    PERM_HAS_IMPORT_TEMPLATE="$(has_perm_row '| `/api/instruments/import/template` | GET |')"
    PERM_HAS_BATCH="$(has_perm_row '| `/api/instruments/batch-import` | POST |')"
    PERM_HAS_BATCH_TEMPLATE="$(has_perm_row '| `/api/instruments/batch-import/template` | GET |')"
    PERM_HAS_BATCH_PREVIEW="$(has_perm_row '| `/api/instruments/batch-import/preview` | POST |')"
    PERM_HAS_BATCH_MEDIA="$(has_perm_row '| `/api/instruments/batch-import/media` | POST |')"
    # §3.2.1 原有「操作-权限矩阵」必须完整（8 行），不得被端点数插入切断
    PERM_321_ROWS="$(awk '/^### 3\.2\.1 /{f=1} f && /^\*\*乐器数据进\/出/{exit} f && /^\| [^|-]/ && !/^\| 操作 \|/{c++} END{print c+0}' "$REPO/$PERM_DOC")"
    PERM_321_TABLE_INTACT=0
    if [ "$PERM_321_ROWS" = "8" ]; then PERM_321_TABLE_INTACT=1; fi
    {
      printf '%s\n' "$PROV"
      printf 'GENERATED_AT_HEAD=%s\n' "$HEAD_SHA"
      echo "# 文档：api.md 三段权限口径 + permissions.md 端点-权限对照表（8 行，一行一端点）"
      printf 'API_EXPORT_PERM=%s\n' "$API_EXPORT"
      printf 'API_IMPORT_PERM=%s\n' "$API_IMPORT"
      printf 'API_TEMPLATE_PERM=%s\n' "$API_TEMPLATE"
      printf 'PERM_ENDPOINT_ROWS=%s\n' "$PERM_ROWS"
      printf 'PERM_HAS_LIST=%s\n' "$PERM_HAS_LIST"
      printf 'PERM_HAS_EXPORT=%s\n' "$PERM_HAS_EXPORT"
      printf 'PERM_HAS_IMPORT=%s\n' "$PERM_HAS_IMPORT"
      printf 'PERM_HAS_IMPORT_TEMPLATE=%s\n' "$PERM_HAS_IMPORT_TEMPLATE"
      printf 'PERM_HAS_BATCH=%s\n' "$PERM_HAS_BATCH"
      printf 'PERM_HAS_BATCH_TEMPLATE=%s\n' "$PERM_HAS_BATCH_TEMPLATE"
      printf 'PERM_HAS_BATCH_PREVIEW=%s\n' "$PERM_HAS_BATCH_PREVIEW"
      printf 'PERM_HAS_BATCH_MEDIA=%s\n' "$PERM_HAS_BATCH_MEDIA"
      printf 'PERM_321_ROWS=%s\n' "$PERM_321_ROWS"
      printf 'PERM_321_TABLE_INTACT=%s\n' "$PERM_321_TABLE_INTACT"
      ALL_OK=1
      for v in "$API_EXPORT" "$API_IMPORT" "$API_TEMPLATE" "$PERM_HAS_LIST" "$PERM_HAS_EXPORT" \
               "$PERM_HAS_IMPORT" "$PERM_HAS_IMPORT_TEMPLATE" "$PERM_HAS_BATCH" \
               "$PERM_HAS_BATCH_TEMPLATE" "$PERM_HAS_BATCH_PREVIEW" "$PERM_HAS_BATCH_MEDIA" \
               "$PERM_321_TABLE_INTACT"; do
        [ "$v" = "1" ] || ALL_OK=0
      done
      [ "$PERM_ROWS" = "8" ] || ALL_OK=0
      if [ "$ALL_OK" = "1" ]; then echo "R6_OK"; else echo "R6_FAIL"; fi
    } > "$LOG" 2>&1
    grep -q '^R6_OK$' "$LOG" || RC=1
    require_tokens "$LOG" 'API_EXPORT_PERM=1' 'API_IMPORT_PERM=1' 'API_TEMPLATE_PERM=1' \
      'PERM_ENDPOINT_ROWS=8' 'PERM_321_TABLE_INTACT=1' \
      'PERM_HAS_LIST=1' 'PERM_HAS_EXPORT=1' 'PERM_HAS_IMPORT=1' 'PERM_HAS_BATCH=1' \
      'PERM_HAS_BATCH_MEDIA=1' 'PERM_HAS_BATCH_PREVIEW=1' 'PERM_HAS_BATCH_TEMPLATE=1' \
      'PERM_HAS_IMPORT_TEMPLATE=1' || RC=1
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
