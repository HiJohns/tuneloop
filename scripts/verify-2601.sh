#!/usr/bin/env bash
# #2601 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2601.sh r1   # ConfirmBind 失败四类三元组（400/403/404/409/瞬时/不可达）+ needs_merge 回归
#   bash scripts/verify-2601.sh r2   # Unbind 失败四类三元组 + IAMClient.UpdateUser 类型化（403 文本保持 / 404 可 errors.As）
#   bash scripts/verify-2601.sh r3   # MergeBind token 状态前置 400/40002 + merged 幂等回归
#   bash scripts/verify-2601.sh r4   # Bind 页失败态渲染（jsdom + @testing-library/react）
#   bash scripts/verify-2601.sh r5   # 契约文档同步（端点清单 + 失败三元组 + Bind 页失败提示）
#   bash scripts/verify-2601.sh r6   # 构建 / gofmt / eslint（no-undef）
#   bash scripts/verify-2601.sh r8   # Bind 页过期/失效分支文案（ASCII 过期串 → 可行动中文）
#   bash scripts/verify-2601.sh gate # 供 pre-ready --affected-cmd 复用（build + 三条定向测试）
#
# Capture a row into its artifact:
#   bash scripts/verify-2601.sh r1 > docs/evidence/2601/R1-confirm-triples.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
#   GENERATED_AT_HEAD=<full-sha>
# The anchor is the issue's fix commit (FIX_COMMIT overrides), never HEAD after
# the evidence commit, so a later re-run keeps pointing at the same source.
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|r8|gate) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|r8|gate" >&2; exit 2 ;;
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

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2601)' -1 2>/dev/null)}"
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
heavy() { bash "$HEAVY" -- "$@"; }

TOKEN_RE='^(CONFIRMBIND_|UNBIND_|UPDATEUSER_|MERGEBIND_|BIND_UI_|API_CONFIRM_|API_UNBIND_|API_MERGE_|DOC_BIND_|GO_BUILD_|GOFMT_|ESLINT_|--- (PASS|FAIL))'

go_test_row() { # pkg test-pattern...
  local pkg="$1" pattern="$2" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test "$pkg" -run "$pattern" -count=1 -v ) > "$out" 2>&1
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

# node/npx for the render row (Taro v4 needs Node 22).
ensure_node22() {
  if [ -s "$HOME/.nvm/nvm.sh" ]; then
    # shellcheck disable=SC1091
    source "$HOME/.nvm/nvm.sh"
    nvm use 22 >/dev/null 2>&1 || true
  fi
  if ! command -v node >/dev/null 2>&1; then
    echo "ERROR:node-unavailable" >&2
    exit 2
  fi
  if ! command -v npx >/dev/null 2>&1; then
    echo "ERROR:npx-unavailable" >&2
    exit 2
  fi
}

case "$ROW" in
  r1)
    echo "== R1 ConfirmBind 失败四类三元组 + needs_merge 回归 =="
    run_row ./handlers 'TestConfirmBind2601_Triples' \
      "CONFIRMBIND_400_STATUS=400" \
      "CONFIRMBIND_400_CODE=40002" \
      "CONFIRMBIND_403_STATUS=500" \
      "CONFIRMBIND_403_CODE=50015" \
      "CONFIRMBIND_404_STATUS=500" \
      "CONFIRMBIND_404_CODE=50015" \
      "CONFIRMBIND_409_STATUS=409" \
      "CONFIRMBIND_409_CODE=40900" \
      "CONFIRMBIND_TRANSIENT_STATUS=502" \
      "CONFIRMBIND_TRANSIENT_CODE=50200" \
      "CONFIRMBIND_DEAD_STATUS=502" \
      "CONFIRMBIND_PERSISTENT_NO_RETRY=0" \
      "CONFIRMBIND_RAW_LEAK=0" \
      "CONFIRMBIND_NEEDS_MERGE_KEPT=1"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 Unbind 失败四类三元组 + UpdateUser 类型化 =="
    run_row ./handlers 'TestUnbind2601_Triples' \
      "UNBIND_400_STATUS=400" \
      "UNBIND_400_CODE=40002" \
      "UNBIND_403_STATUS=500" \
      "UNBIND_403_CODE=50015" \
      "UNBIND_404_STATUS=500" \
      "UNBIND_404_CODE=50015" \
      "UNBIND_TRANSIENT_STATUS=502" \
      "UNBIND_TRANSIENT_CODE=50200" \
      "UNBIND_DEAD_STATUS=502" \
      "UNBIND_PERSISTENT_NO_RETRY=0" \
      "UNBIND_RAW_LEAK=0" \
      "UPDATEUSER_403_TEXT_KEPT=1" \
      "UPDATEUSER_404_TYPED=1"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 MergeBind token 状态前置（400/40002） + merged 幂等 =="
    run_row ./handlers 'TestMergeBind2601_StateMismatch' \
      "MERGEBIND_STATE_STATUS=400" \
      "MERGEBIND_STATE_CODE=40002" \
      "MERGEBIND_STATE_MSG_ACTIONABLE=1" \
      "MERGEBIND_STATE_INTERNAL_LEAK=0" \
      "MERGEBIND_IDEMPOTENT_KEPT=1"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 Bind 页失败态渲染（render + waitFor） =="
    ensure_node22
    OUT="$(mktemp)"
    ( cd frontend-mobile && heavy npx vitest run --config vitest.config.js \
        scripts/page-smoke/bind-failure-2601.test.jsx ) > "$OUT" 2>&1
    RC=$?
    grep -E "$TOKEN_RE" "$OUT" || true
    if [ "$RC" -ne 0 ]; then
      tail -40 "$OUT"
      rm -f "$OUT"
      fail "vitest bind-failure-2601 exit=${RC}"
    fi
    for tok in \
      "BIND_UI_NEEDS_MERGE_RENDER=1" \
      "BIND_UI_50015_CONFIG_TEXT=1" \
      "BIND_UI_50015_NO_RETRY_HINT=1" \
      "BIND_UI_50200_RETRY_TEXT=1" \
      "BIND_UI_40002_SERVER_TEXT=1" \
      "BIND_UI_RENDER_FAIL=0"; do
      grep -qx -- "$tok" "$OUT" || { rm -f "$OUT"; fail "missing expected token '$tok'"; }
    done
    rm -f "$OUT"
    echo "R4_OK"
    ;;

  r8)
    echo "== R8 Bind 页过期/失效分支文案（render + waitFor，ASCII 过期串） =="
    ensure_node22
    OUT="$(mktemp)"
    ( cd frontend-mobile && heavy npx vitest run --config vitest.config.js \
        scripts/page-smoke/bind-failure-2601.test.jsx ) > "$OUT" 2>&1
    RC=$?
    grep -E "$TOKEN_RE" "$OUT" || true
    if [ "$RC" -ne 0 ]; then
      tail -40 "$OUT"
      rm -f "$OUT"
      fail "vitest bind-failure-2601 (expired rows) exit=${RC}"
    fi
    for tok in \
      "BIND_UI_EXPIRED_TOKEN_TEXT=1" \
      "BIND_UI_EXPIRED_NO_CODE_MAP_MISMATCH=1" \
      "BIND_UI_EXPIRED_RENDER_FAIL=0"; do
      grep -qx -- "$tok" "$OUT" || { rm -f "$OUT"; fail "missing expected token '$tok'"; }
    done
    rm -f "$OUT"
    echo "R8_OK"
    ;;

  r5)
    echo "== R5 契约文档同步（端点清单 + 失败三元组 + Bind 页失败提示） =="
    API="docs/spec/api/api.md"
    UI="docs/spec/ui/ui.md"

    APPLICABLE_LINE="$(grep -m1 '^> 适用端点：' "$API" || true)"
    API_CONFIRM_ENDPOINT=0
    case "$APPLICABLE_LINE" in *'`POST /api/wechat-bind/confirm`'*) API_CONFIRM_ENDPOINT=1 ;; esac
    API_UNBIND_ENDPOINT=0
    case "$APPLICABLE_LINE" in *'`POST /api/users/me/wechat-unbind`'*) API_UNBIND_ENDPOINT=1 ;; esac

    CONFIRM_SECTION="$(mktemp)"
    awk '/^#### `POST \/api\/wechat-bind\/confirm`/{f=1} /^#### `POST \/api\/wechat-bind\/merge`/{f=0} f' "$API" > "$CONFIRM_SECTION"
    ROW_400=0; grep -q '| 400 | 40002 |' "$CONFIRM_SECTION" && ROW_400=1
    ROW_409=0; grep -q '| 409 | 40900 |' "$CONFIRM_SECTION" && ROW_409=1
    ROW_500=0; grep -q '| 500 | 50015 |' "$CONFIRM_SECTION" && ROW_500=1
    ROW_502=0; grep -q '| 502 | 50200 |' "$CONFIRM_SECTION" && ROW_502=1
    rm -f "$CONFIRM_SECTION"
    API_CONFIRM_FAILURE_ROWS=0
    if [ "$ROW_400$ROW_409$ROW_500$ROW_502" = "1111" ]; then API_CONFIRM_FAILURE_ROWS=1; fi

    MERGE_SECTION="$(mktemp)"
    awk '/^#### `POST \/api\/wechat-bind\/merge`/{f=1} /^#### `POST \/api\/auth\/wx-login-select`/{f=0} f' "$API" > "$MERGE_SECTION"
    API_MERGE_STATE_ROW=0
    if grep -q 'token 状态非 `needs_merge`' "$MERGE_SECTION" \
       && grep -q '绑定二维码已失效，请让管理员重新生成二维码后重试' "$MERGE_SECTION" \
       && grep -q '| 400 | 40002 |' "$MERGE_SECTION"; then
      API_MERGE_STATE_ROW=1
    fi
    rm -f "$MERGE_SECTION"

    DOC_BIND_FAILURE_TEXT=0
    if grep -q 'Bind 页（扫码绑定）失败提示（#2601）' "$UI" \
       && grep -q '身份服务配置异常，请联系平台管理员处理' "$UI" \
       && grep -q '`500 / 50015`' "$UI"; then
      DOC_BIND_FAILURE_TEXT=1
    fi

    echo "API_CONFIRM_ENDPOINT=${API_CONFIRM_ENDPOINT}"
    echo "API_UNBIND_ENDPOINT=${API_UNBIND_ENDPOINT}"
    echo "API_CONFIRM_FAILURE_ROWS=${API_CONFIRM_FAILURE_ROWS}"
    echo "API_MERGE_STATE_ROW=${API_MERGE_STATE_ROW}"
    echo "DOC_BIND_FAILURE_TEXT=${DOC_BIND_FAILURE_TEXT}"

    [ "$API_CONFIRM_ENDPOINT" = "1" ] || fail "$API §14.4 适用端点缺 POST /api/wechat-bind/confirm"
    [ "$API_UNBIND_ENDPOINT" = "1" ] || fail "$API §14.4 适用端点缺 POST /api/users/me/wechat-unbind"
    [ "$API_CONFIRM_FAILURE_ROWS" = "1" ] || fail "$API confirm 响应表缺失败三元组行（400/40002、409/40900、500/50015、502/50200）"
    [ "$API_MERGE_STATE_ROW" = "1" ] || fail "$API merge 段缺 token 状态前置 400/40002 行"
    [ "$DOC_BIND_FAILURE_TEXT" = "1" ] || fail "$UI 缺 Bind 页失败提示（#2601）表"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 构建 / gofmt / eslint（no-undef） =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"
    GO_BUILD_OK=0
    [ "$BUILD_RC" -eq 0 ] && GO_BUILD_OK=1

    GOFMT_FILES=(
      backend/handlers/wechat_bind.go
      backend/services/iam_client.go
      backend/handlers/wechat_bind_failure_2601_test.go
    )
    GOFMT_OUT="$(mktemp)"
    gofmt -l "${GOFMT_FILES[@]}" > "$GOFMT_OUT" 2>&1
    GOFMT_DIFF="$(grep -c . "$GOFMT_OUT" || true)"
    GOFMT_DIFF="${GOFMT_DIFF:-0}"
    cat "$GOFMT_OUT"
    rm -f "$GOFMT_OUT"

    ensure_node22
    LINT_OUT="$(mktemp)"
    LINT_RC=0
    ( cd frontend-mobile && heavy npx --no-install eslint \
        src/pages-weapp/bind/Bind.jsx src/services/errorMessages.js \
        scripts/page-smoke/bind-failure-2601.test.jsx ) > "$LINT_OUT" 2>&1 || LINT_RC=$?
    ESLINT_NOUNDEF="$(grep -c 'no-undef' "$LINT_OUT" || true)"
    ESLINT_NOUNDEF="${ESLINT_NOUNDEF:-0}"
    if [ "$LINT_RC" -ne 0 ]; then
      tail -30 "$LINT_OUT"
    fi
    rm -f "$LINT_OUT"

    echo "GO_BUILD_OK=${GO_BUILD_OK}"
    echo "GOFMT_DIFF=${GOFMT_DIFF}"
    echo "ESLINT_NOUNDEF=${ESLINT_NOUNDEF}"

    [ "$GO_BUILD_OK" = "1" ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "$GOFMT_DIFF" = "0" ] || fail "gofmt diff present (${GOFMT_DIFF} files)"
    [ "$ESLINT_NOUNDEF" = "0" ] || fail "eslint no-undef present (${ESLINT_NOUNDEF})"
    [ "$LINT_RC" -eq 0 ] || fail "eslint failed (rc=${LINT_RC})"
    echo "R6_OK"
    ;;

  gate)
    echo "== GATE #2601：go build ./... + 三条定向测试 =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"
    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... failed (rc=${BUILD_RC})"
    go_test_row ./handlers 'TestConfirmBind2601_Triples|TestUnbind2601_Triples|TestMergeBind2601_StateMismatch'
    echo "GATE_OK"
    ;;
esac
