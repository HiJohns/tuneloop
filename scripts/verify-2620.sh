#!/usr/bin/env bash
# scripts/verify-2620.sh — 证据生成器（#2620 身份自查询跳过自动租户 scope）。
#
# 用法：bash scripts/verify-2620.sh <r1|r2>
#   r1  docs/evidence/2620/R1-self-query-tests.txt   4 端点自查询 + 跨租户负例（handler 直调，一次性 PG）
#   r2  docs/evidence/2620/R2-build-and-scan.txt     句柄收口静态扫描 + go build + 编译冒烟 + gofmt
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`；锚点 = 本 Issue fix commit（FIX_REF 可覆盖）。
# 重命令（go test / go build / gofmt）一律经 heavy-run.sh 取锁；handler 测试用一次性 PG
# （127.0.0.1:15433，#2341 口径），不使用 jobmaster-postgres。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2) ;;
  *) echo "usage: $0 <r1|r2>" >&2; exit 2 ;;
esac

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

ISSUE=2620
EVIDENCE_DIR="docs/evidence/$ISSUE"
mkdir -p "$EVIDENCE_DIR"

export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2620}"
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
[ -n "$(git status --porcelain --untracked-files=no -- . ":(exclude)$EVIDENCE_DIR" ':(exclude)tmp')" ] && DIRTY=true
PROV="# build=1.0.$FIX_SHORT # commit=$FIX_SHA # dirty=$DIRTY"

HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
WORK="$(mktemp -d "$REPO/tmp/verify-$ISSUE-XXXXXX")" || exit 2
trap '[ -n "${KEEP_WORK:-}" ] || rm -rf "$WORK"' EXIT

heavy() {
  if [ -x "$HEAVY_RUN" ]; then bash "$HEAVY_RUN" -- "$@"; else "$@"; fi
}

RC=0
LOG="$WORK/$MODE.log"
: > "$LOG"

require_tokens() { # log tokens...
  local log="$1"; shift
  local missing="" tok
  for tok in "$@"; do
    grep -Fq -- "$tok" "$log" || missing="$missing [$tok]"
  done
  if [ -n "$missing" ]; then
    echo "ASSERT_FAIL: missing tokens:$missing" >&2
    RC=1
  fi
}

count_in() { # pattern files...
  local pat="$1"; shift
  grep -cE -- "$pat" "$@" 2>/dev/null | awk -F: '{s+=$NF} END {print s+0}'
}

GO_FILES=(
  backend/handlers/wechat_bind.go
  backend/handlers/referral.go
  backend/handlers/self_query_tenant_scope_2620_test.go
)
PROD_FILES=(backend/handlers/wechat_bind.go backend/handlers/referral.go)
TEST_PKG='TestSelfQueryTenantScope2620'

case "$MODE" in
r1)
  ( cd "$REPO/backend" && heavy go test ./handlers -run "$TEST_PKG" -count=1 -v -timeout 15m ) > "$WORK/r1.test.log" 2>&1
  TEST_RC=$?
  CASES_TOTAL="$(grep -cE "^[[:space:]]+--- PASS: ${TEST_PKG}/" "$WORK/r1.test.log" 2>/dev/null || true)"
  CASES_FAIL="$(grep -cE "^[[:space:]]+--- FAIL: ${TEST_PKG}/" "$WORK/r1.test.log" 2>/dev/null || true)"
  {
    printf '%s\n' "$PROV"
    echo "GENERATED_AT_HEAD=$HEAD_SHA"
    echo "# 4 个 /users/me/* 端点自查询 + 跨租户负例（users.tenant_id=cacheTenant ≠ JWT tid）"
    grep -E 'SELF2620_[A-Z_]+=' "$WORK/r1.test.log" | sed 's/^[[:space:]]*//'
    printf 'SELF2620_CASES_TOTAL=%s\n' "${CASES_TOTAL:-0}"
    printf 'SELF2620_CASES_FAIL=%s\n' "${CASES_FAIL:-0}"
    printf 'GO_TEST_EXIT=%s\n' "$TEST_RC"
    echo "--- go test subtest results ---"
    grep -E "^[[:space:]]+--- (PASS|FAIL): ${TEST_PKG}/" "$WORK/r1.test.log" || true
    if [ "$TEST_RC" != "0" ]; then
      echo "--- go test output (tail) ---"
      tail -40 "$WORK/r1.test.log"
    fi
  } > "$LOG" 2>&1
  require_tokens "$LOG" 'GO_TEST_EXIT=0' 'SELF2620_CASES_TOTAL=5' 'SELF2620_CASES_FAIL=0' \
    'SELF2620_BIND_GEN_HTTP=200' 'SELF2620_UNBIND_HTTP=200' 'SELF2620_PROMO_QR_HTTP=200' \
    'SELF2620_REFERRALS_TOTAL=1' 'SELF2620_REFEREE_NAME=1' 'SELF2620_CROSS_TENANT_HIDDEN=1'
  for sub in bind_gen unbind promo_qr referrals cross_tenant_scoped; do
    require_tokens "$LOG" "--- PASS: ${TEST_PKG}/${sub}"
  done
  ;;

r2)
  ( cd "$REPO/backend" && heavy go build ./... ) > "$WORK/r2.build.log" 2>&1
  GO_BUILD_EXIT=$?
  ( cd "$REPO/backend" && heavy go test ./... -run '^$' -count=1 -timeout 15m ) > "$WORK/r2.compile.log" 2>&1
  TEST_COMPILE_EXIT=$?
  # heavy-run 会在同一 stdout 打印锁日志（[heavy-lock] ...）；gofmt 结果需先落文件再过滤。
  ( cd "$REPO" && heavy gofmt -l "${GO_FILES[@]}" ) > "$WORK/r2.gofmt.log" 2>&1
  GOFMT_OUT="$(grep -v '^\[heavy-lock\]' "$WORK/r2.gofmt.log" 2>/dev/null | sed '/^$/d')"
  GOFMT_DIFF="$(printf '%s' "$GOFMT_OUT" | sed '/^$/d' | wc -l | tr -d ' ')"
  IDENTITY_CTX_SITES="$(count_in 'WithContext\(database\.IdentityCtx\(ctx\)\)' "${PROD_FILES[@]}")"
  RESIDUAL_PLAIN_CTX_SITES="$(count_in 'WithContext\(ctx\)' "${PROD_FILES[@]}")"
  {
    printf '%s\n' "$PROV"
    echo "GENERATED_AT_HEAD=$HEAD_SHA"
    echo "# 句柄收口静态扫描 + go build ./... + 编译冒烟（go test ./... -run '^$'）+ gofmt"
    printf 'GO_BUILD_EXIT=%s\n' "$GO_BUILD_EXIT"
    printf 'TEST_COMPILE_EXIT=%s\n' "$TEST_COMPILE_EXIT"
    printf 'GOFMT_DIFF=%s\n' "$GOFMT_DIFF"
    printf 'IDENTITY_CTX_SITES=%s\n' "$IDENTITY_CTX_SITES"
    printf 'RESIDUAL_PLAIN_CTX_SITES=%s\n' "$RESIDUAL_PLAIN_CTX_SITES"
    echo "--- per-file IdentityCtx counts ---"
    for f in "${PROD_FILES[@]}"; do
      printf '%s=%s\n' "$f" "$(grep -cE 'WithContext\(database\.IdentityCtx\(ctx\)\)' "$f" 2>/dev/null || echo 0)"
    done
    if [ "$GO_BUILD_EXIT" != "0" ]; then
      echo "--- go build output (tail) ---"; tail -20 "$WORK/r2.build.log"
    fi
    if [ "$TEST_COMPILE_EXIT" != "0" ]; then
      echo "--- compile smoke output (tail) ---"; tail -30 "$WORK/r2.compile.log"
    fi
    if [ -n "$GOFMT_OUT" ]; then echo "--- gofmt -l ---"; printf '%s\n' "$GOFMT_OUT"; fi
  } > "$LOG" 2>&1
  require_tokens "$LOG" 'GO_BUILD_EXIT=0' 'TEST_COMPILE_EXIT=0' 'GOFMT_DIFF=0' \
    'IDENTITY_CTX_SITES=6' 'RESIDUAL_PLAIN_CTX_SITES=0'
  ;;
esac

cat "$LOG"
[ "$RC" = "0" ] || { echo "RESULT: FAIL" >&2; exit 1; }
exit 0
