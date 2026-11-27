#!/usr/bin/env bash
# #2597 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2597.sh r1   # 配置不变量违反（0 个 / 2 个主组织）→ 500/50014 + 可行动文案
#   bash scripts/verify-2597.sh r2   # IAM 不可用（瞬时）→ 500/50000 +「稍后再试」（不误判配置类）
#   bash scripts/verify-2597.sh r3   # 存量断言翻转（#2552 主组织用例）+ 停用文案去内部 Issue 号
#   bash scripts/verify-2597.sh r4   # 前端解析链（新码映射 / 内部串不外溢 / 真实抛出链）
#   bash scripts/verify-2597.sh r5   # 页面接线（统一出口 + try/catch + 编辑判码 + 双端码表）
#   bash scripts/verify-2597.sh r6   # 文档同步（api.md 新码口径 + ui.md 失败提示口径）
#   bash scripts/verify-2597.sh r7   # 构建 / gofmt / eslint(no-undef)
#
# Capture a row into its artifact:
#   bash scripts/verify-2597.sh r1 > docs/evidence/2597/R1-primary-config.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
#   GENERATED_AT_HEAD=<full-sha>
# The anchor is the issue's fix commit (FIX_COMMIT overrides), never HEAD after the
# evidence commit, so a later re-run keeps pointing at the same source.
#
# Heavy commands (go test / go build / node --test / eslint) go through
# scripts/heavy-run.sh so the host build lock is held only for the command (D8).
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|r7) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|r7" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Backend integration tests use the isolated test database created/cloned by
# backend/handlers/main_test.go (see docs/ops/test-db-lifecycle.md).
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2597)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
echo "GENERATED_AT_HEAD=$(git rev-parse HEAD)"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { if [ -x "$HEAVY" ]; then bash "$HEAVY" -- "$@"; else "$@"; fi; }

TOKEN_RE='^(PRIMARY_CONFIG_|PRIMARY_ORG_|TRANSIENT_|DEACTIVATE_|E50014_|HTTP_THROW_|ASCII_LEAK=|HUMAN_40002=|MOBILE_MAP_MATCH=|RESOLVE_HELPER=|RAW_MESSAGE_BRANCHES=|TRY_CATCH_HANDLERS=|EDIT_CODE_CHECK=|PC_MAP_50014=|MOBILE_MAP_50014=|API_CODE_50014=|API_TEXT_ACTIONABLE=|UI_ERROR_LAYER=|GO_BUILD_OK=|GOFMT_DIFF=|ESLINT_NOUNDEF=|NODE_TEST_FAIL=|HARNESS_FLIPPED=)'

go_test_row() { # pkg test-pattern...
  local pkg="$1" pattern="$2" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test "$pkg" -run "$pattern" -count=1 -v -timeout 15m ) > "$out" 2>&1
  rc=$?
  grep -E "$TOKEN_RE" "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -40 "$out"
    rm -f "$out"
    fail "go test ${pkg} -run ${pattern} exit=${rc}"
  fi
  rm -f "$out"
}

run_row() { # pkg pattern expected-token...
  local pkg="$1" pattern="$2"; shift 2
  local out; out="$(mktemp)"
  go_test_row "$pkg" "$pattern" > "$out" 2>&1
  cat "$out"
  local tok
  for tok in "$@"; do
    grep -qx -- "$tok" "$out" || { rm -f "$out"; fail "missing expected token '$tok'"; }
  done
  rm -f "$out"
}

case "$ROW" in
  r1)
    echo "== R1 配置类失败分流（0 个 / 2 个 primary）→ 500/50014 + 可行动文案 =="
    run_row ./handlers 'TestTransitSiteCreate2597_PrimaryConfigInvariant' \
      "PRIMARY_CONFIG_CODE=50014" \
      "PRIMARY_CONFIG_MSG_HAS_ACTIONABLE=1" \
      "PRIMARY_CONFIG_MSG_NO_RETRY=0" \
      "PRIMARY_CONFIG_ORGS_CREATED=0" \
      "PRIMARY_CONFIG_SITES_CREATED=0" \
      "PRIMARY_CONFIG_LOG_DETAIL=1"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 瞬时失败不分流（IAM 组织列表不可用）→ 500/50000 +「稍后再试」 =="
    run_row ./handlers 'TestTransitSiteCreate2597_PrimaryTransient' \
      "TRANSIENT_STATUS=500" \
      "TRANSIENT_CODE=50000" \
      "TRANSIENT_MSG_RETRY=1" \
      "TRANSIENT_NOT_CONFIG_CODE=1" \
      "TRANSIENT_SITES_CREATED=0"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 存量断言翻转（#2552 主组织用例）+ 停用文案去内部 Issue 号 =="
    # 翻转必须两端一致：测试 token 与既有 harness 期望都改为 50014。
    HARNESS_EXPECT_50014="$(grep -c 'PRIMARY_ORG_UNRESOLVED=50014' scripts/verify-2552.sh || true)"
    HARNESS_EXPECT_50000="$(grep -c 'PRIMARY_ORG_UNRESOLVED=50000' scripts/verify-2552.sh || true)"
    echo "VERIFY_2552_EXPECTS_50014=${HARNESS_EXPECT_50014:-0}"
    echo "VERIFY_2552_EXPECTS_50000=${HARNESS_EXPECT_50000:-0}"
    [ "${HARNESS_EXPECT_50000:-0}" = "0" ] || fail "scripts/verify-2552.sh 仍期望旧 token 50000"
    [ "${HARNESS_EXPECT_50014:-0}" -ge 1 ] || fail "scripts/verify-2552.sh 未翻转 PRIMARY_ORG_UNRESOLVED 期望值"

    run_row ./handlers 'TestTransitSiteCreate2552_PrimaryUnresolved' \
      "PRIMARY_ORG_UNRESOLVED=50014"
    # 既有 #1935 回归（关键词「路由引用」保留、内部 Issue 号移除）：exit 0 即通过。
    go_test_row ./handlers 'TestDeleteTransitSiteRejectsWhenReferenced' >/dev/null
    echo "DEACTIVATE_REFERENCED_REGRESSION=1"
    run_row ./handlers 'TestTransitSiteDeactivate2597_MessageNoInternalRef' \
      "DEACTIVATE_MSG_NO_INTERNAL_REF=1"
    if [ "${HARNESS_EXPECT_50014:-0}" -ge 1 ] && [ "${HARNESS_EXPECT_50000:-0}" = "0" ]; then
      echo "HARNESS_FLIPPED=1"
    else
      echo "HARNESS_FLIPPED=0"
    fi
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 前端解析链（新码映射 / 内部串不外溢 / 真实抛出链） =="
    OUT="$(mktemp)"
    TOK="$(mktemp)"
    ( cd frontend-pc && heavy node --test --test-reporter=tap tests/transitErrorText-2597.test.mjs ) \
      > "$OUT" 2>&1
    RC=$?
    # TAP 把 console.log 渲染为 `# TOKEN=...` 注释；去掉前缀后与 go 侧同口径断言。
    sed 's/^# //' "$OUT" | grep -E "$TOKEN_RE" > "$TOK" || true
    cat "$TOK"
    grep -E '^# (tests|pass|fail) ' "$OUT" || true
    NODE_FAIL="$(grep -cE '^# fail [1-9]' "$OUT" || true)"
    if [ "$RC" -ne 0 ]; then
      echo "--- node --test output (tail) ---"
      tail -40 "$OUT"
    fi
    rm -f "$OUT"
    echo "NODE_TEST_FAIL=${NODE_FAIL:-0}"
    [ "$RC" -eq 0 ] || { rm -f "$TOK"; fail "node --test tests/transitErrorText-2597.test.mjs exit=${RC}"; }
    [ "${NODE_FAIL:-0}" = "0" ] || { rm -f "$TOK"; fail "node --test reported failures"; }
    for tok in "E50014_LAYER=code_map" "E50014_RETRY_HINT=0" "HTTP_THROW_CODE=50014" \
               "ASCII_LEAK=0" "HUMAN_40002=1" "MOBILE_MAP_MATCH=1"; do
      grep -qx -- "$tok" "$TOK" || { rm -f "$TOK"; fail "missing expected token '$tok'"; }
    done
    rm -f "$TOK"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 页面接线（统一出口 + try/catch + 编辑判码 + 双端码表） =="
    PAGE="frontend-pc/src/pages/admin/transit/TransitCenter.jsx"

    RESOLVE_HELPER="$(grep -c 'const showError = ' "$PAGE" || true)"
    RESOLVE_CALLS="$(grep -c 'resolveErrorMessage(' "$PAGE" || true)"
    RAW_BRANCHES="$(grep -cE 'message\.error\((r|e)\.message' "$PAGE" || true)"
    TRY_CATCH="$(grep -c 'try {' "$PAGE" || true)"
    EDIT_CODE_CHECK="$(sed -n '/if (editingSite) {/,/已更新/p' "$PAGE" | grep -c 'code !== 20000' || true)"
    PC_MAP="$(grep -c "^  50014: '命名空间主组织配置异常（应为 1 个），请联系平台管理员处理'," frontend-pc/src/services/errorMessages.js || true)"
    MOBILE_MAP="$(grep -c "^  50014: '命名空间主组织配置异常（应为 1 个），请联系平台管理员处理'," frontend-mobile/src/services/errorMessages.js || true)"

    [ "${RESOLVE_HELPER:-0}" = "1" ] || fail "showError 统一出口未接线"
    [ "${RESOLVE_CALLS:-0}" -ge 1 ] || fail "resolveErrorMessage 未被调用"
    [ "${RAW_BRANCHES:-0}" = "0" ] || fail "仍有 r.message / e.message 直出分支"
    [ "${TRY_CATCH:-0}" = "5" ] || fail "try/catch 数量应为 5（fetchAll + 4 个 handler），实际 ${TRY_CATCH:-0}"
    [ "${EDIT_CODE_CHECK:-0}" -ge 1 ] || fail "编辑分支未判 r.code（404/409 会误报成功）"
    [ "${PC_MAP:-0}" = "1" ] || fail "PC ERROR_CODE_MAP 缺 50014"
    [ "${MOBILE_MAP:-0}" = "1" ] || fail "mobile ERROR_CODE_MAP 缺 50014"

    echo "RESOLVE_HELPER=${RESOLVE_HELPER:-0}"
    echo "RAW_MESSAGE_BRANCHES=${RAW_BRANCHES:-0}"
    echo "TRY_CATCH_HANDLERS=${TRY_CATCH:-0}"
    echo "EDIT_CODE_CHECK=${EDIT_CODE_CHECK:-0}"
    echo "PC_MAP_50014=${PC_MAP:-0}"
    echo "MOBILE_MAP_50014=${MOBILE_MAP:-0}"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 文档同步（api.md 新码 + ui.md 失败提示口径） =="
    API="docs/spec/api/api.md"
    UI="docs/spec/ui/ui.md"

    API_CODE_50014="$(grep -cE '^\| 50014 \|' "$API" || true)"
    API_TEXT_ACTIONABLE="$(grep -E '^\| 50014 \|' "$API" | grep -c '不可重试' || true)"
    API_RULE="$(grep -c '配置类失败用专用码' "$API" || true)"
    UI_SECTION="$(awk '/^### 3\.20 中转中心/{f=1;next} f && /^### /{exit} f{print}' "$UI")"
    UI_ERROR_LAYER="$(printf '%s\n' "$UI_SECTION" | grep -c 'resolveErrorMessage' || true)"

    [ "${API_CODE_50014:-0}" -ge 1 ] || fail "api.md §14.4 缺 50014 行"
    [ "${API_TEXT_ACTIONABLE:-0}" -ge 1 ] || fail "api.md 50014 行未写「不可重试」语义"
    [ "${API_RULE:-0}" -ge 1 ] || fail "api.md 缺「配置类失败用专用码」口径"
    [ "${UI_ERROR_LAYER:-0}" -ge 1 ] || fail "ui.md §3.20 缺失败提示（统一错误层）口径"

    echo "API_CODE_50014=${API_CODE_50014:-0}"
    echo "API_TEXT_ACTIONABLE=${API_TEXT_ACTIONABLE:-0}"
    echo "UI_ERROR_LAYER=${UI_ERROR_LAYER:-0}"
    echo "R6_OK"
    ;;

  r7)
    echo "== R7 构建 / gofmt / eslint(no-undef) =="
    OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$OUT"
    rm -f "$OUT"

    GOFMT_DIFF="$(gofmt -l backend/services/iam_client.go backend/handlers/transit_site_admin.go \
      backend/handlers/transit_site_admin_2597_test.go backend/handlers/transit_site_admin_2552_test.go \
      backend/handlers/transit_site_admin_test.go 2>/dev/null | wc -l | tr -d ' ')"

    OUT="$(mktemp)"
    ( cd frontend-pc && heavy npx eslint src/pages/admin/transit/TransitCenter.jsx \
        src/services/errorMessages.js ) > "$OUT" 2>&1
    LINT_RC=$?
    tail -10 "$OUT"
    ESLINT_NOUNDEF="$(grep -c 'no-undef' "$OUT" || true)"
    rm -f "$OUT"

    if [ "$BUILD_RC" -eq 0 ]; then echo "GO_BUILD_OK=1"; else echo "GO_BUILD_OK=0"; fi
    echo "GOFMT_DIFF=${GOFMT_DIFF:-0}"
    echo "ESLINT_NOUNDEF=${ESLINT_NOUNDEF:-0}"

    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... exit=${BUILD_RC}"
    [ "${GOFMT_DIFF:-0}" = "0" ] || fail "gofmt 有差异（gofmt -l 非空）"
    [ "${ESLINT_NOUNDEF:-0}" = "0" ] || fail "eslint 报 no-undef（exit=${LINT_RC}）"
    echo "R7_OK"
    ;;
esac

echo "RESULT: PASS ($ROW)"
