#!/usr/bin/env bash
# #2567 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2567.sh r1   # 合并后缓存镜像（A 继承组织身份 / C 废止回落顾客态）
#   bash scripts/verify-2567.sh r2   # 成员列表 + 人员管理三视图可见性
#   bash scripts/verify-2567.sh r3   # 合并审计（ACCOUNT_MERGE + from/to iam_sub + qr_bind）
#   bash scripts/verify-2567.sh r4   # 合并确认页摘要 + 密码步骤接线（static）
#   bash scripts/verify-2567.sh r5   # 构建/测试：go build + 定向 go test + weapp build + eslint
#   bash scripts/verify-2567.sh r6   # C 的本地待办/邀请关闭（不触碰 IAM 库）
#
# Capture a row into its artifact:
#   bash scripts/verify-2567.sh r1 > docs/evidence/2567/R1-merge-reconcile.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
# The anchor is the issue's fix commit (FIX_COMMIT overrides), never HEAD after
# the evidence commit, so a later re-run keeps pointing at the same source.
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
fi

# Taro v4 does not build on Node 24 (AGENTS.md §Node.js 版本要求): pin Node 22.
if [ -d "$HOME/.nvm/versions/node/v22.22.3/bin" ]; then
  export PATH="$HOME/.nvm/versions/node/v22.22.3/bin:$PATH"
fi

# Backend integration tests need the Postgres test database
# (docs/ops/test-db-lifecycle.md). Overridable for other hosts.
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2567)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"
echo "GENERATED_AT_HEAD=${ANCHOR_SHA}"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

# has_token TOKEN FILE — the test logs one line carrying several machine
# tokens (`A_ROLE=… C_STATUS=…`), so match inside the line, not whole-line.
has_token() {
  grep -q -- "$1" "$2" || fail "missing expected token '$1'"
}

run_go_case() { # test-name expected-token...
  local name="$1"; shift
  local out; out="$(mktemp)"
  ( cd backend && heavy go test ./handlers -run "^${name}$" -count=1 -v ) > "$out" 2>&1
  local rc=$?
  grep -E '^--- (PASS|FAIL)' "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -30 "$out"
    rm -f "$out"
    fail "go test ${name} exit=${rc}"
  fi
  local tok
  for tok in "$@"; do
    has_token "$tok" "$out" || { tail -30 "$out"; rm -f "$out"; fail "token '$tok' missing"; }
  done
  grep -E '^[[:space:]]+[a-z0-9_]+\.go:[0-9]+: ' "$out" || true
  rm -f "$out"
}

case "$ROW" in
  r1)
    echo "== R1 合并后缓存镜像（A 继承组织身份 / C 废止回落顾客态） =="
    run_go_case TestAccountMergeReconcilesCache_2567 \
      "A_ROLE=merchant_staff" \
      "A_TENANT_NON_NIL=1" \
      "A_ORG=" \
      "C_STATUS=obsolete" \
      "C_ROLE=USER" \
      "MEMBERSHIP_ROWS=1"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 成员列表（/api/staff）+ 人员管理（/api/admin/personnel）可见性 =="
    run_go_case TestListStaffHidesObsolete_2567 \
      "A_IN_LIST=1" \
      "C_IN_LIST=0" \
      "OBSOLETE_ROWS=0" \
      "DISABLED_ROWS=0" \
      "PENDING_ROWS=1"
    run_go_case TestPersonnelMerchantViewHidesObsolete_2567 \
      "PERSONNEL_A_IN_LIST=1" \
      "PERSONNEL_C_IN_LIST=0" \
      "PERSONNEL_OBSOLETE_ROWS=0"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 合并审计（ACCOUNT_MERGE + from/to iam_sub + 来源 qr_bind） =="
    run_go_case TestAccountMergeWritesAudit_2567 \
      "AUDIT_ACTION=ACCOUNT_MERGE" \
      "AUDIT_HAS_FROM_TO=1" \
      "AUDIT_SOURCE=qr_bind" \
      "AUDIT_RESOURCE=user"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 合并确认页摘要 + 密码步骤接线（static） =="
    BIND="frontend-mobile/src/pages-weapp/bind/Bind.jsx"
    CRED="frontend-mobile/src/pages-weapp/credentials/Credentials.jsx"
    WB="backend/handlers/wechat_bind.go"
    for f in "$BIND" "$CRED" "$WB"; do
      [ -f "$f" ] || fail "missing $f"
    done

    # 后端能力字段：确认页摘要（组织名 / 原员工名 / 「将废止」语义）
    ORG_NAME_FIELD="$(grep -c '"org_name"' "$WB" || true)"
    STAFF_NAME_FIELD="$(grep -c '"staff_name"' "$WB" || true)"
    RETIRES_FIELD="$(grep -c '"retires_staff"' "$WB" || true)"
    # 前端确认页摘要 + 二次确认
    SUMMARY_RENDERED="$(grep -c 'mergeInfo?.org_name' "$BIND" || true)"
    STAFF_RENDERED="$(grep -c 'mergeInfo?.staff_name' "$BIND" || true)"
    CONFIRM_DIALOG="$(grep -c 'dialog.confirm' "$BIND" || true)"
    RETIRE_WORDING="$(grep -c '废止' "$BIND" || true)"
    # 密码步骤：无密码才出现（SKIP_WHEN_EXISTS）+ 密码 + 确认密码字段
    SKIP_WIRED="$(grep -c 'mergeResult?.needs_credentials' "$BIND" || true)"
    CRED_NAV="$(grep -c 'pages-weapp/credentials/index' "$BIND" || true)"
    PASSWORD_FIELD="$(grep -c 'value={password}' "$CRED" || true)"
    CONFIRM_FIELD="$(grep -c 'value={confirmPassword}' "$CRED" || true)"
    CONFIRM_VALIDATED="$(grep -c 'password !== confirmPassword' "$CRED" || true)"
    # 文案禁止「删除员工账号」（D6：统一「废止」）
    DELETE_WORDING="$(grep -c '删除员工账号' "$BIND" || true)"

    SKIP_WHEN_EXISTS=0
    if [ "${SKIP_WIRED:-0}" -ge 1 ] && [ "${CRED_NAV:-0}" -ge 1 ]; then
      SKIP_WHEN_EXISTS=1
    fi
    PASSWORD_STEP=0
    if [ "${PASSWORD_FIELD:-0}" -ge 1 ] && [ "${CONFIRM_FIELD:-0}" -ge 1 ] && [ "${CONFIRM_VALIDATED:-0}" -ge 1 ]; then
      PASSWORD_STEP=1
    fi
    SUMMARY_WIRED=0
    if [ "${ORG_NAME_FIELD:-0}" -ge 1 ] && [ "${STAFF_NAME_FIELD:-0}" -ge 1 ] && \
       [ "${SUMMARY_RENDERED:-0}" -ge 1 ] && [ "${STAFF_RENDERED:-0}" -ge 1 ] && \
       [ "${CONFIRM_DIALOG:-0}" -ge 1 ]; then
      SUMMARY_WIRED=1
    fi

    echo "ORG_NAME_FIELD=${ORG_NAME_FIELD:-0}"
    echo "STAFF_NAME_FIELD=${STAFF_NAME_FIELD:-0}"
    echo "RETIRES_FIELD=${RETIRES_FIELD:-0}"
    echo "SUMMARY_WIRED=${SUMMARY_WIRED}"
    echo "RETIRE_WORDING=${RETIRE_WORDING:-0}"
    echo "DELETE_WORDING=${DELETE_WORDING:-0}"
    echo "PASSWORD_FIELD=${PASSWORD_FIELD:-0}"
    echo "CONFIRM_FIELD=${CONFIRM_FIELD:-0}"
    echo "PASSWORD_STEP=${PASSWORD_STEP}"
    echo "SKIP_WHEN_EXISTS=${SKIP_WHEN_EXISTS}"

    [ "${SUMMARY_WIRED:-0}" -eq 1 ] || fail "合并确认页摘要未接线（org_name/staff_name/二次确认）"
    [ "${RETIRE_WORDING:-0}" -ge 1 ] || fail "确认页缺少「废止」措辞（D6）"
    [ "${DELETE_WORDING:-0}" -eq 0 ] || fail "确认页出现「删除员工账号」，违反 D6"
    [ "${PASSWORD_STEP:-0}" -eq 1 ] || fail "密码步骤缺「密码 + 确认密码」字段或校验"
    [ "${SKIP_WHEN_EXISTS:-0}" -eq 1 ] || fail "密码步骤未按 has-credentials 跳过（SKIP_WHEN_EXISTS）"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 构建/测试（go build + 定向 go test + weapp build + eslint） =="
    GO_TEST_OK=0
    GO_BUILD_OK=0
    MOBILE_BUILD_OK=0
    ESLINT_OK=0

    T_OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -run "^$" -count=1 ) > "$T_OUT" 2>&1
    COMPILE_RC=$?
    ( cd backend && heavy go test ./handlers -count=1 -v -run '2567|TestListStaff|TestPersonnel2065|TestPersonnel_|TestPersonnel2067|TestPersonnel2072|TestAccountMerge' ) >> "$T_OUT" 2>&1
    T_RC=$?
    FAIL_COUNT="$(grep -c '^--- FAIL' "$T_OUT" || true)"
    PASS_COUNT="$(grep -c '^--- PASS' "$T_OUT" || true)"
    grep -E '^--- (PASS|FAIL)' "$T_OUT" | tail -20 || true
    tail -3 "$T_OUT"
    if [ "$COMPILE_RC" -eq 0 ] && [ "$T_RC" -eq 0 ] && [ "${FAIL_COUNT:-1}" = "0" ]; then
      GO_TEST_OK=1
    fi
    rm -f "$T_OUT"

    B_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$B_OUT" 2>&1
    B_RC=$?
    tail -5 "$B_OUT"
    [ "$B_RC" -eq 0 ] && GO_BUILD_OK=1
    rm -f "$B_OUT"

    L_OUT="$(mktemp)"
    ( cd frontend-mobile && heavy npx eslint src/pages-weapp/bind/Bind.jsx src/pages-weapp/credentials/Credentials.jsx ) > "$L_OUT" 2>&1
    L_RC=$?
    tail -10 "$L_OUT"
    [ "$L_RC" -eq 0 ] && ESLINT_OK=1
    rm -f "$L_OUT"

    M_OUT="$(mktemp)"
    ( cd frontend-mobile && heavy npm run build:weapp ) > "$M_OUT" 2>&1
    M_RC=$?
    tail -8 "$M_OUT"
    if [ "$M_RC" -eq 0 ] && [ -f frontend-mobile/dist-weapp/app.js ]; then
      MOBILE_BUILD_OK=1
    fi
    # weapp 可达性探针（AGENTS.md §前端流程可达性）：bind 页不得引入原生 HTML 交互标签
    NATIVE_HITS=0
    if [ -d frontend-mobile/dist-weapp ]; then
      NATIVE_HITS="$(grep -rlE '<(div|span|img|button|input|textarea|p|select|a)\b' \
        frontend-mobile/dist-weapp/pages-weapp/bind --include='*.wxml' 2>/dev/null | wc -l | tr -d ' ')"
    fi
    rm -f "$M_OUT"

    echo "GO_TEST_OK=${GO_TEST_OK}"
    echo "GO_TEST_PASS_COUNT=${PASS_COUNT:-0}"
    echo "GO_TEST_FAIL_COUNT=${FAIL_COUNT:-0}"
    echo "GO_BUILD_OK=${GO_BUILD_OK}"
    echo "MOBILE_BUILD_OK=${MOBILE_BUILD_OK}"
    echo "ESLINT_OK=${ESLINT_OK}"
    echo "WEAPP_NATIVE_TAG_HITS=${NATIVE_HITS:-0}"

    [ "$GO_TEST_OK" -eq 1 ] || fail "go test failed"
    [ "$GO_BUILD_OK" -eq 1 ] || fail "go build ./... failed"
    [ "$ESLINT_OK" -eq 1 ] || fail "eslint failed"
    [ "$MOBILE_BUILD_OK" -eq 1 ] || fail "weapp build failed"
    [ "${NATIVE_HITS:-0}" -eq 0 ] || fail "bind 页 wxml 含原生 HTML 标签（weapp 不渲染）"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 C 的本地待办/邀请关闭（不触碰 IAM 库） =="
    run_go_case TestAccountMergeClosesLocalTodo_2567 \
      "LOCAL_INVITES_CLOSED=1" \
      "CONFIRM_SESSIONS_CLOSED=1" \
      "IAM_DB_TOUCHED=0"
    # 静态佐证：tuneloop 不得出现针对 user_task_queues 的 SQL（注释/说明常量允许）
    IAM_SQL_HITS="$(grep -rniE '(select|insert|update|delete|from|join|table\()[^"]*user_task_queues' \
      backend --include='*.go' | wc -l | tr -d ' ')"
    echo "IAM_SQL_HITS=${IAM_SQL_HITS}"
    [ "${IAM_SQL_HITS:-1}" -eq 0 ] || fail "backend 出现 user_task_queues 的非注释引用（跨库红线）"
    echo "R6_OK"
    ;;
esac
