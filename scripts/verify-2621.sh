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
# 用法：bash scripts/verify-2621.sh <r1..r5>
#   r1 docs/evidence/2621/R1-resolver.txt                共享解析器（EnsureLocalUser / LocalUserID）
#   r2 docs/evidence/2621/R2-handler-sites.txt           9 个端点 / 11 子用例（handler 直调）
#   r3 docs/evidence/2621/R3-isolation-and-build.txt     隔离未放松 + 构建/编译冒烟 + 残留扫描
#   r4 docs/evidence/2621/R4-staff-site-order-scope.txt  员工订单按当前网点（fail-closed，Rev.4 R4）
#   r5 docs/evidence/2621/R5-self-query-scan.txt         语义级自查询扫描 + 5 处 defer 白名单（Rev.4 R5）
#
# 每个 artifact 首行 provenance（P14）：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`，
# 其后 `GENERATED_AT_HEAD=<sha>`。锚点 = 本 Issue 的 fix commit（FIX_REF 可覆盖）。
# 重命令一律经 heavy-run.sh 取锁；测试库用一次性 PostgreSQL（127.0.0.1:15433），
# 禁止使用 jobmaster-postgres 的 dev 库。
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

REPO="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$REPO" || exit 2

ISSUE=2621
OUT_DIR="${OUT_DIR:-$REPO/docs/evidence/$ISSUE}"; mkdir -p "$OUT_DIR"
EVIDENCE_DIR="docs/evidence/$ISSUE"
TEST_FILE="backend/handlers/self_query_tenant_scope_2621_test.go"
VIS_TEST_FILE="backend/handlers/order_visibility_2621_test.go"
# R5 defer 白名单：显示面/运维同步面的同类自查询（Rev.4 D10 裁定 defer 到 follow-up）。
DEFERRED_ISSUE="${DEFERRED_ISSUE:-2644}"

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

  r4)
    NAME="R4-staff-site-order-scope.txt"
    ( cd "$REPO/backend" && heavy go test ./handlers \
        -run '^TestOrderVisibility2621$' -count=1 -v -timeout 20m ) > "$WORK/r4.test.log" 2>&1
    TEST_RC=$?
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 员工订单可见范围 = 当前组织上下文（JWT oid）；自查询失败 / oid 空 → fail-closed（空列表）"
      echo "BACKEND_CMD=cd backend && go test ./handlers -run '^TestOrderVisibility2621\$' -count=1 -v"
      grep -E '(STAFF_ORDER_SITE_SCOPED|CROSS_SITE_ORDER_HIDDEN|SELF_QUERY_FAIL_CLOSED|SELF_QUERY_FAIL_LIST_LEN|NO_OID_EMPTY_LIST|NO_OID_LIST_LEN|TENANT_WIDE_FALLBACK)=' \
        "$WORK/r4.test.log" | sed 's/^[[:space:]]*//'
      printf 'TEST_EXIT=%s\n' "$TEST_RC"
      if [ "$TEST_RC" != "0" ]; then
        echo "--- go test output ---"
        tail -40 "$WORK/r4.test.log"
      fi
    } > "$LOG" 2>&1
    require_tokens "$LOG" 'TEST_EXIT=0' 'STAFF_ORDER_SITE_SCOPED=1' 'CROSS_SITE_ORDER_HIDDEN=1' \
      'SELF_QUERY_FAIL_CLOSED=1' 'NO_OID_EMPTY_LIST=1' 'TENANT_WIDE_FALLBACK=0' || RC=1
    ;;

  r5)
    NAME="R5-self-query-scan.txt"
    # 语义级扫描（不靠单点正则）：逐个 `Where("iam_sub` 语句判定是否处于身份句柄链上
    # （同语句链出现 IdentityCtx，或调用的是 `x := ...IdentityCtx(...)` 派生的句柄），
    # 排除 UPDATE 型自写（GORM Update 无自动 scope，#2621 D5）。
    cat > "$WORK/r5_scan.py" <<'R5PY'
import re
import sys

# 在册收口点（本单 9 个直连点 + 共享解析器）+ defer 白名单（显示面，带 Issue 号）。
IN_SCOPE = [
    "backend/middleware/iam.go",
    "backend/handlers/user_points.go",
    "backend/handlers/pending_order.go",
    "backend/handlers/face_capture.go",
    "backend/handlers/user_address.go",
    "backend/handlers/payment_calculate.go",
    "backend/handlers/deposit_waiver.go",
    "backend/handlers/customer_role.go",
]
DEFERRED = {
    "backend/handlers/order.go": 2,      # 取消订单操作人名 / 状态历史操作人名
    "backend/handlers/api.go": 1,        # resolveUserNamesByIAMSub
    "backend/handlers/audit_log.go": 1,  # enrichAuditLogs
    "backend/handlers/iam_proxy.go": 1,  # SyncUsers 既有用户匹配
}


def classify(path):
    src = open(path, encoding='utf-8').read()
    lines = src.split("\n")
    id_handles = set(re.findall(r'(\w+)\s*:=\s*[^\n]*IdentityCtx\(', src))
    plain, ident, writes = [], [], []
    for i, line in enumerate(lines):
        if 'Where("iam_sub' not in line:
            continue
        window = "\n".join(lines[max(0, i - 4):i + 1])
        if "Updates(" in line or ".Update(" in line:
            writes.append(i + 1)
            continue
        m = re.search(r'(\w+)\.Where\("iam_sub', line)
        if "IdentityCtx(" in window or (m and m.group(1) in id_handles):
            ident.append(i + 1)
        else:
            plain.append(i + 1)
    return plain, ident, writes


exit_code = 0
in_scope_plain = 0
deferred_total = 0
for path in IN_SCOPE:
    plain, ident, _writes = classify(path)
    print("IN_SCOPE %s identity_sites=%d plain_sites=%d %s" % (path, len(ident), len(plain), plain if plain else ""))
    in_scope_plain += len(plain)
    if len(ident) == 0:
        print("ASSERT_FAIL: %s 无任何身份句柄（IdentityCtx）站点" % path)
        exit_code = 1
for path, expected in DEFERRED.items():
    plain, ident, writes = classify(path)
    print("DEFERRED %s expected=%d plain_sites=%d %s identity_sites=%d write_sites=%d %s"
          % (path, expected, len(plain), plain if plain else "", len(ident), len(writes), writes if writes else ""))
    deferred_total += len(plain)
    if len(plain) != expected:
        print("ASSERT_FAIL: %s defer 站点数 %d != 登记值 %d（新增/移除了同类点，须更新白名单或先修复）"
              % (path, len(plain), expected))
        exit_code = 1

print("SELF_QUERY_WITH_TENANT_CTX=%d" % in_scope_plain)
print("DEFERRED_SITES=%d" % deferred_total)
sys.exit(exit_code)
R5PY
    SCAN_RC=0
    python3 "$WORK/r5_scan.py" > "$WORK/r5.scan.out" 2>&1 || SCAN_RC=$?
    GH_BIN="${GH_BIN:-gh}"
    ISSUE_OK=0
    ISSUE_STATE=""
    if ISSUE_STATE="$("$GH_BIN" issue view "$DEFERRED_ISSUE" --json state,title -q '.state + " " + .title' 2>/dev/null)"; then
      case "$ISSUE_STATE" in
        OPEN*) ISSUE_OK=1 ;;
      esac
    fi
    {
      printf '%s\n' "$PROV"
      echo "GENERATED_AT_HEAD=$HEAD_SHA"
      echo "# 语义级自查询扫描：每个 iam_sub 查询语句是否处于身份句柄链（IdentityCtx）；defer 白名单 5 处带 Issue 号"
      echo "SCAN_CMD=python3 tmp/r5_scan.py（逐语句分类 identity/plain/write；UPDATE 型自写按 D5 豁免）"
      cat "$WORK/r5.scan.out"
      printf 'SCAN_EXIT=%s\n' "$SCAN_RC"
      printf 'DEFERRED_ISSUE=#%s\n' "$DEFERRED_ISSUE"
      printf 'DEFERRED_ISSUE_STATE=%s\n' "${ISSUE_STATE:-unavailable}"
      printf 'DEFERRED_ISSUE_LINKED=%s\n' "$ISSUE_OK"
    } > "$LOG" 2>&1
    [ "$SCAN_RC" = "0" ] || RC=1
    require_tokens "$LOG" 'SELF_QUERY_WITH_TENANT_CTX=0' 'DEFERRED_SITES=5' 'DEFERRED_ISSUE_LINKED=1' || RC=1
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
