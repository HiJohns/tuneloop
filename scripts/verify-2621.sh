#!/usr/bin/env bash
# =============================================================================
# scripts/verify-2621.sh — 证据生成器（#2621 身份自查询绕开自动租户 scope）
#
# 缺陷：`users.tenant_id` 是 IAM 关系的**本地缓存**，可与登录 JWT 的 `tid` 不一致；
# 「按 JWT sub 查我自己那行」若用带 tid 的句柄，会被 `addTenantScope` 追加
# `tenant_id = tid` → 404 / 静默空 / 误报 profile_not_ready / 建 shadow user 冲突 500。
# 修复：共享解析器（`middleware.EnsureLocalUser` / `LocalUserID`）与 7 处直连自查询
# 改用身份句柄 `database.IdentityCtx(ctx)`；**域查询句柄不变**（隔离不放宽）。
#
# 用法：bash scripts/verify-2621.sh <r1..r3>
#   r1 docs/evidence/2621/R1-resolver.txt            共享解析器（EnsureLocalUser / LocalUserID）
#   r2 docs/evidence/2621/R2-handler-sites.txt       9 个端点 / 11 子用例（handler 直调）
#   r3 docs/evidence/2621/R3-isolation-and-build.txt 隔离未放松 + 构建/编译冒烟 + 残留扫描
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`。锚点 = 本 Issue 的 fix commit（FIX_REF 可覆盖）。
# 重命令一律经 heavy-run.sh 取锁；测试库用一次性 PostgreSQL（127.0.0.1:15433），
# 禁止使用 jobmaster-postgres 的 dev 库。
# =============================================================================
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3) ;;
  *)
    echo "usage: $0 <r1..r3>" >&2
    exit 2
    ;;
esac

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

ISSUE=2621
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
TEST_FILE="backend/handlers/self_query_tenant_scope_2621_test.go"

export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"
if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi
TMPDIR="${TMPDIR:-$REPO/tmp/gobuild}"
GOCACHE="${GOCACHE:-$REPO/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"
export TMPDIR GOCACHE

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
trap '[ -n "${KEEP_WORK:-}" ] || rm -rf "$WORK"' EXIT

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

count_in() { # <pattern> <files...>
  local pat="$1"; shift
  grep -cE -- "$pat" "$@" 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'
}

RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

case "$MODE" in
  r1)
    NAME="R1-resolver.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers \
        -run 'TestSelfQueryTenantScope2621/(ensure_local_user|local_user_id)$' -count=1 -v -timeout 20m ) \
      > "$WORK/r1.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 共享解析器：users.tenant_id=cacheTenant ≠ JWT tid 时仍命中本行且不新建 shadow user"
      echo "BACKEND_CMD=cd backend && go test ./handlers -run 'TestSelfQueryTenantScope2621/(ensure_local_user|local_user_id)\$' -count=1 -v"
      grep -E 'ENSURE_[A-Z_]+=|LOCAL_USER_ID_HIT=|USERS_ROW_COUNT_DELTA=' "$WORK/r1.test.log" | sed 's/^[[:space:]]*//'
      printf 'TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -40 "$WORK/r1.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'TEST_EXIT=0' 'ENSURE_HIT=1' 'ENSURE_NO_NEW_ROW=1' \
      'USERS_ROW_COUNT_DELTA=0' 'LOCAL_USER_ID_HIT=1' || RC=1
    ;;

  r2)
    NAME="R2-handler-sites.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers \
        -run '^TestSelfQueryTenantScope2621$' -count=1 -v -timeout 20m ) > "$WORK/r2.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 9 个直连点 + 共享解析器 + 跨租户负例 = 11 子用例（handler 直调 + 一次性 PG）"
      echo "BACKEND_CMD=cd backend && go test ./handlers -run '^TestSelfQueryTenantScope2621$' -count=1 -v"
      grep -E '(CASES_TOTAL|CASES_FAIL|POINTS_BALANCE_HTTP|POINTS_TX_HTTP|PENDING_ORDER_HTTP|FACE_CAPTURE_NOT_404|ADDRESSES_HTTP|PAY_CALC_NO_USER_ERROR|DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY|CUSTOMER_ROLE_HTTP|CROSS_TENANT_HIDDEN|DOMAIN_QUERY_STILL_SCOPED)=' \
        "$WORK/r2.test.log" | sed 's/^[[:space:]]*//'
      printf 'TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -60 "$WORK/r2.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'TEST_EXIT=0' 'CASES_TOTAL=11' 'CASES_FAIL=0' \
      'POINTS_BALANCE_HTTP=200' 'POINTS_TX_HTTP=200' 'PENDING_ORDER_HTTP=200' \
      'FACE_CAPTURE_NOT_404=1' 'ADDRESSES_HTTP=200' 'PAY_CALC_NO_USER_ERROR=1' \
      'DEPOSIT_WAIVER_NOT_PROFILE_NOT_READY=1' 'CUSTOMER_ROLE_HTTP=200' || RC=1
    ;;

  r3)
    NAME="R3-isolation-and-build.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers \
        -run 'TestSelfQueryTenantScope2621/(cross_tenant_scoped|points_balance)$' -count=1 -v -timeout 20m ) \
      > "$WORK/r3.test.log" 2>&1
    ISO_RC=$?
    ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/build.out" 2>&1
    BUILD=$?
    ( cd "$REPO/backend" && heavy go test ./... -run '^$' -count=1 -timeout 30m ) > "$WORK/compile.out" 2>&1
    COMPILE=$?
    RESIDUAL="$(count_in 'WithContext\(ctx\)\.Where\("iam_sub' backend/handlers/*.go)"
    RESIDUAL_MW="$(count_in 'WithContext\(ctx\)\.Where\("iam_sub' backend/middleware/*.go)"
    RESIDUAL=$((RESIDUAL + RESIDUAL_MW))
    IDENTITY_SITES="$(count_in 'database\.IdentityCtx\(' backend/middleware/iam.go)"
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 隔离未放松（他人行仍隐藏 / 域查询仍有租户过滤）+ 构建/编译冒烟 + 残留静态扫描"
      echo "ISOLATION_CMD=cd backend && go test ./handlers -run 'TestSelfQueryTenantScope2621/(cross_tenant_scoped|points_balance)\$' -count=1 -v"
      grep -E '(CROSS_TENANT_HIDDEN|DOMAIN_QUERY_STILL_SCOPED|POINTS_BALANCE_HTTP)=' "$WORK/r3.test.log" | sed 's/^[[:space:]]*//'
      printf 'ISOLATION_TEST_EXIT=%s\n' "$ISO_RC"
      echo "BUILD_CMD=cd backend && go build ./..."
      tail -3 "$WORK/build.out"
      printf 'GO_BUILD_EXIT=%s\n' "$BUILD"
      echo "TEST_COMPILE_CMD=cd backend && go test ./... -run '^\$' -count=1"
      tail -5 "$WORK/compile.out"
      printf 'TEST_COMPILE_EXIT=%s\n' "$COMPILE"
      echo "RESIDUAL_SCAN_CMD=grep -rn 'WithContext(ctx).Where(\"iam_sub' backend --include=*.go | grep -v _test | wc -l"
      printf 'IDENTITY_CTX_SITES_IAM_GO=%s\n' "$IDENTITY_SITES"
      printf 'RESIDUAL_PLAIN_CTX_SELF_QUERY=%s\n' "$RESIDUAL"
      echo "--- residual hits (must be empty) ---"
      grep -rn 'WithContext(ctx)\.Where("iam_sub' backend --include=*.go | grep -v '_test.go' || echo "(none)"
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'CROSS_TENANT_HIDDEN=1' 'DOMAIN_QUERY_STILL_SCOPED=1' \
      'GO_BUILD_EXIT=0' 'TEST_COMPILE_EXIT=0' 'RESIDUAL_PLAIN_CTX_SELF_QUERY=0' || RC=1
    ;;
esac

ART="$OUT_DIR/$NAME"
# Documented usage redirects stdout into $ART; copying then would race with the
# shell's write offset, so only copy when stdout is a different file.
STDOUT_PATH="$(readlink /proc/self/fd/1 2>/dev/null || true)"
ART_PATH="$(readlink -f "$ART" 2>/dev/null || true)"
if [ -z "$STDOUT_PATH" ] || [ "$STDOUT_PATH" != "$ART_PATH" ]; then
  cp "$LOG" "$ART"
fi
cat "$LOG"
echo "ARTIFACT=$ART" >&2
if [ "$RC" != "0" ]; then
  echo "RESULT: FAIL ($MODE)" >&2
  exit 1
fi
echo "RESULT: PASS ($MODE)" >&2
