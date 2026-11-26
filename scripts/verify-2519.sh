#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2519.sh — 证据生成器（#2519 批量操作能力缺口）
#
# 行定义（Accepted Plan #2519 Rev.5 / Decision Lock D1–D11）：
#   r1  批量状态 3 条成功            → docs/evidence/2519/R1-batch-status.txt
#   r2  跨租户阻断 + 无权限 403      → docs/evidence/2519/R2-cross-tenant.txt
#   r3  单条状态生产可用（去 Debug） → docs/evidence/2519/R3-status-route.txt
#   r4  批量状态部分失败上报         → docs/evidence/2519/R4-partial-fail.txt
#   r5  日租金 + 删除同名 override   → docs/evidence/2519/R5-daily-rate.txt
#   r6  逾期日租金落 pricing         → docs/evidence/2519/R6-overdue.txt
#   r7  押金列 + JSONB 双写（含 0）  → docs/evidence/2519/R7-deposit.txt
#   r8  三字段同时设置 + 缺失 id     → docs/evidence/2519/R8-multi-field.txt
#   r9  前端开关模型纯函数（node）   → docs/evidence/2519/R9-toggles.txt
#   r10 前端接线（无旧选择器）       → docs/evidence/2519/R10-frontend-wiring.txt
#   r11 API 文档同步                 → docs/evidence/2519/R11-docs.txt
#   r12 构建 + 定向测试              → docs/evidence/2519/R12-build.txt
#   r13 legacy overrides 拒绝        → docs/evidence/2519/R13-legacy-overrides.txt
#
# 用法：bash scripts/verify-2519.sh <r1..r13>
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|r11|r12|r13) ;;
  *) echo "usage: $0 <r1..r13>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel)"; cd "$REPO" || exit 2
ISSUE=2519
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
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR")" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"

WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap 'rm -rf "$WORK"' EXIT

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" --timeout 3600 -- "$@"; else "$@"; fi
}

NAME=""; RC=0
LOG="$WORK/$MODE.log"; : > "$LOG"
ART="$OUT_DIR"

# run_go_row <file> <run-regex> : runs `go test -run` and returns exit code, log at $WORK/go.log
run_go_row() {
  ( cd "$REPO/backend" && heavy go test ./handlers -run "$2" -count=1 -v -timeout 10m ) > "$1" 2>&1
}

# emit <artifact-name> [extra lines already in $LOG]
finish() {
  local name=$1
  local art="$ART/$name"
  { printf '%s\n%s\n' "$PROV" "# generated_at=$(date -u +%Y-%m-%dT%H:%M:%SZ) HEAD=$HEAD_SHA"; cat "$LOG"; } > "$art"
  echo "ARTIFACT=$art"
  if [ "$RC" != "0" ]; then echo "RESULT: FAIL ($MODE)" >&2; exit 1; fi
  echo "RESULT: PASS ($MODE)"
}

require_tokens() { # file tokens...
  local f=$1; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$f" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then echo "ASSERT_FAIL: missing tokens:$missing" >&2; RC=1; fi
}

case "$MODE" in
r1)
  NAME="R1-batch-status.txt"
  run_go_row "$WORK/go.log" '^TestBatchUpdateInstrumentStatus_Success_2519$'
  T=$?
  grep -E 'R1 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'STATUS=20000' 'UPDATED=3' 'FAILED=0' 'DB_CHANGED=3'
  ;;
r2)
  NAME="R2-cross-tenant.txt"
  run_go_row "$WORK/go.log" '^TestBatchUpdateInstrumentStatus_CrossTenantAndPermission_2519$'
  T=$?
  grep -E 'R2 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'CROSS_TENANT_BLOCKED=1' 'NO_PERM_STATUS=403' 'DB_UNCHANGED=1'
  ;;
r3)
  NAME="R3-status-route.txt"
  # 静态面：路由行不得再挂 RequireDebugMode
  ROUTE_LINE="$(grep -n '"/instruments/:id/status"' "$REPO/backend/main.go" | head -1)"
  HAS_DEBUG=0
  echo "$ROUTE_LINE" | grep -q 'RequireDebugMode' && HAS_DEBUG=1
  # 运行时面：同形中间件链（不含 DebugMode）下单条状态可用
  run_go_row "$WORK/go.log" '^TestUpdateInstrumentStatusRoute_2519$'
  T=$?
  echo "MIDDLEWARE_HAS_DEBUG=$HAS_DEBUG" >> "$LOG"
  grep -E 'R3 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  echo "ROUTE=$ROUTE_LINE" >> "$LOG"
  [ "$HAS_DEBUG" = "0" ] || RC=1
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'MIDDLEWARE_HAS_DEBUG=0' 'STATUS=20000'
  ;;
r4)
  NAME="R4-partial-fail.txt"
  run_go_row "$WORK/go.log" '^TestBatchUpdateInstrumentStatus_PartialFail_2519$'
  T=$?
  grep -E 'R4 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'FAILED=1' 'REASON_PRESENT=1' 'DB_CHANGED=1'
  ;;
r5)
  NAME="R5-daily-rate.txt"
  run_go_row "$WORK/go.log" '^TestBatchSetInstrumentPricing_DailyRate_2519$'
  T=$?
  grep -E 'R5 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'BASE_DAILY_RATE=5000' 'PRICING_DAILY_RENT=50' \
    'OVERRIDE_DAILY_RENT_REMOVED=1' 'PRICING_V2_RATE=5000'
  ;;
r6)
  NAME="R6-overdue.txt"
  run_go_row "$WORK/go.log" '^TestBatchSetInstrumentPricing_ExplicitOverdue_2519$'
  T=$?
  grep -E 'R6 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'PRICING_OVERDUE=120' 'OVERRIDE_OVERDUE_REMOVED=1' 'PRICING_V2_OVERDUE=120'
  ;;
r7)
  NAME="R7-deposit.txt"
  run_go_row "$WORK/go.log" '^TestBatchSetInstrumentPricing_Deposit_2519$'
  T=$?
  grep -E 'R7 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'DEPOSIT_COL=10000' 'PRICING_DEPOSIT=100' \
    'OVERRIDE_DEPOSIT_REMOVED=1' 'ZERO_CASE_COL=0' 'ZERO_CASE_TIER=zero'
  ;;
r8)
  NAME="R8-multi-field.txt"
  run_go_row "$WORK/go.log" '^TestBatchSetInstrumentPricing_MultiFieldPartialFail_2519$'
  T=$?
  grep -E 'R8 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'UPDATED=1' 'FAILED=1' 'BASE=5000' 'OVERDUE=120' 'DEPOSIT=10000'
  ;;
r13)
  NAME="R13-legacy-overrides.txt"
  run_go_row "$WORK/go.log" '^TestBatchSetInstrumentPricing_LegacyOverridesRejected_2519$'
  T=$?
  grep -E 'R13 STATUS=' "$WORK/go.log" >> "$LOG"
  echo "GO_TEST_EXIT=$T" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  require_tokens "$LOG" 'UPDATED=0' 'FAILED=1' 'FAILED_REASON=no_fields' 'OVERRIDES_STORED=0' 'PRICING_V2_OVERDUE=0'
  ;;
r9)
  NAME="R9-toggles.txt"
  ( cd "$REPO/frontend-pc" && heavy node --test --test-reporter=tap tests/batchPricingToggles.test.mjs tests/batchPricing.test.mjs ) > "$WORK/node.log" 2>&1
  T=$?
  cat "$WORK/node.log" >> "$LOG"
  PASS_COUNT="$(grep -c '^ok ' "$WORK/node.log" || true)"
  echo "TESTS_PASS=$PASS_COUNT" >> "$LOG"
  echo "NODE_TEST_EXIT=$T" >> "$LOG"
  # 旧三段式用例不得再断言废弃 API
  LEGACY=1
  grep -q 'BATCH_PRICE_TYPE_OPTIONS\|BATCH_PRICE_OPERATOR_OPTIONS' "$REPO/frontend-pc/tests/batchPricing.test.mjs" && LEGACY=0
  echo "LEGACY_TEST_REWRITTEN=$LEGACY" >> "$LOG"
  ONLY_ENABLED=0; grep -q '^ok .*only enabled fields are submitted' "$WORK/node.log" && ONLY_ENABLED=1
  ALL_OFF=0; grep -q '^ok .*all toggles off is rejected' "$WORK/node.log" && ALL_OFF=1
  DEP_ZERO=0; grep -q '^ok .*deposit accepts 0 but daily rent rejects 0' "$WORK/node.log" && DEP_ZERO=1
  printf 'ONLY_ENABLED_SUBMITTED=%s\nALL_OFF_ERROR=%s\nDEPOSIT_ZERO_OK=%s\n' \
    "$ONLY_ENABLED" "$ALL_OFF" "$DEP_ZERO" >> "$LOG"
  [ "$T" = "0" ] || RC=1
  [ "${PASS_COUNT:-0}" -ge 6 ] || RC=1
  require_tokens "$LOG" 'LEGACY_TEST_REWRITTEN=1' 'ONLY_ENABLED_SUBMITTED=1' 'ALL_OFF_ERROR=1' 'DEPOSIT_ZERO_OK=1'
  ;;
r10)
  NAME="R10-frontend-wiring.txt"
  STOCK="$REPO/frontend-pc/src/pages/InstrumentStock.jsx"
  LIST="$REPO/frontend-pc/src/pages/admin/instrument/List.jsx"
  LEGACY_SELECTORS=0
  for f in "$STOCK" "$LIST"; do
    n="$(grep -cE 'name="price_type"|name="operator"|BATCH_PRICE_TYPE_OPTIONS|BATCH_PRICE_OPERATOR_OPTIONS' "$f" || true)"
    LEGACY_SELECTORS=$((LEGACY_SELECTORS + n))
  done
  BATCH_STATUS_CALL="$(grep -h -c "/instruments/batch/status" "$STOCK" "$LIST" | paste -sd+ - | bc)"
  FIELDS_CONST="$(sed -n '/export const BATCH_PRICE_FIELDS/,/^]/p' "$REPO/frontend-pc/src/utils/batchPricing.js" | grep -c "key: '" || true)"
  echo "LEGACY_SELECTORS=$LEGACY_SELECTORS" >> "$LOG"
  echo "BATCH_STATUS_CALL=$BATCH_STATUS_CALL" >> "$LOG"
  echo "FIELDS_CONST=$FIELDS_CONST" >> "$LOG"
  echo "BATCH_PRICE_FIELDS_USES=$(grep -h -c 'BATCH_PRICE_FIELDS' "$STOCK" "$LIST" | paste -sd+ - | bc)" >> "$LOG"
  [ "$LEGACY_SELECTORS" = "0" ] || RC=1
  [ "${BATCH_STATUS_CALL:-0}" -ge 1 ] || RC=1
  [ "${FIELDS_CONST:-0}" = "3" ] || RC=1
  require_tokens "$LOG" 'LEGACY_SELECTORS=0' 'FIELDS_CONST=3'
  ;;
r11)
  NAME="R11-docs.txt"
  DOC="$REPO/docs/spec/api/api.md"
  TOKENS=0
  for tok in 'batch/status' 'deposit' 'overdue_daily_fee' 'failed'; do
    if grep -q -- "$tok" "$DOC"; then TOKENS=$((TOKENS + 1)); echo "DOC_HAS=$tok"; fi
  done
  echo "DOC_TOKENS=$TOKENS" >> "$LOG"
  [ "$TOKENS" = "4" ] || RC=1
  require_tokens "$LOG" 'DOC_TOKENS=4'
  ;;
r12)
  NAME="R12-build.txt"
  ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/build.log" 2>&1
  BUILD=$?
  ( cd "$REPO/backend" && heavy go test ./handlers -run '2519|PricingOverdue' -count=1 -timeout 10m ) > "$WORK/gotest.log" 2>&1
  GTEST=$?
  ( cd "$REPO/frontend-pc" && heavy npm run build ) > "$WORK/npm.log" 2>&1
  NPM=$?
  ( cd "$REPO/frontend-pc" && heavy npx eslint src/utils/batchPricing.js src/pages/InstrumentStock.jsx src/pages/admin/instrument/List.jsx ) > "$WORK/eslint.log" 2>&1
  LINT=$?
  NO_UNDEF="$(grep -c 'no-undef' "$WORK/eslint.log" || true)"
  echo "BUILD_EXIT=$BUILD"; echo "GO_TEST_EXIT=$GTEST"; echo "NPM_BUILD_EXIT=$NPM"; echo "ESLINT_EXIT=$LINT"; echo "ESLINT_NO_UNDEF=$NO_UNDEF"
  {
    echo "BUILD_EXIT=$BUILD"
    echo "GO_TEST_EXIT=$GTEST"
    echo "NPM_BUILD_EXIT=$NPM"
    echo "ESLINT_EXIT=$LINT"
    echo "ESLINT_NO_UNDEF=$NO_UNDEF"
    tail -3 "$WORK/gotest.log"
    tail -5 "$WORK/npm.log"
    tail -10 "$WORK/eslint.log"
  } >> "$LOG"
  [ "$BUILD" = "0" ] && [ "$GTEST" = "0" ] && [ "$NPM" = "0" ] && [ "$LINT" = "0" ] && [ "$NO_UNDEF" = "0" ] || RC=1
  require_tokens "$LOG" 'BUILD_EXIT=0' 'GO_TEST_EXIT=0' 'NPM_BUILD_EXIT=0' 'ESLINT_NO_UNDEF=0'
  ;;
esac

finish "$NAME"
