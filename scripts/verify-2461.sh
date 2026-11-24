#!/usr/bin/env bash
# #2461 证据生成器 —— IAM 组织角色更新改 JSON body + 调用点失败语义收口。
#
# 用法：bash scripts/verify-2461.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9|r10>
#
# 每行证据：
#   r1  R1-client-service.txt      服务凭据通路 JSON body（method/path/body/无 query）
#   r2  R2-client-token.txt        用户令牌通路 JSON body
#   r3  R3-client-400.txt          IAM 400 不被吞（错误串含状态码与原文）
#   r4  R4-site-failfast.txt       网点改角色主角色失败 → 502 + 本地不变
#   r5  R5-template-errors.txt     模板分配失败 → 200 + role_errors
#   r6  R6-merchant-compensate.txt 换管理员 demote 失败 → 502 + 补偿解绑 + 本地未提交
#   r7  R7-merchant-bind-pending.txt bind 失败 → 200 + admin_pending + 告警字段
#   r8  R8-staff-role-errors.txt   直属员工改角色 IAM 失败 → 200 + role_errors + role 已更新
#   r9  R9-failfast-regression.txt 既有 fail-fast 三入口回归（502/500/500）
#   r10 R10-build.txt              go build ./... + 定向测试（含 2375 翻转断言回归）
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
# 必须在 fix commit 之后、干净树（无未提交的已跟踪改动）上执行。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8|r9|r10) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9|r10>" >&2
    exit 2
    ;;
esac

ROOT="$(git rev-parse --show-toplevel)"
HEAD_FULL="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
EVID="$ROOT/docs/evidence/2461"
mkdir -p "$EVID"

DIRTY=false
if [ -n "$(git -C "$ROOT" status --porcelain --untracked-files=no)" ]; then
  DIRTY=true
fi

export PATH="/usr/local/go/bin:${PATH}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-25432}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-tune_secret_2026}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2461}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

provenance() {
  echo "# build=1.0.${HEAD_SHORT} # commit=${HEAD_FULL} # dirty=${DIRTY}"
}

run_pkg_test() { # package, regex, timeout(default 15m)
  local pkg="$1" regex="$2" to="${3:-15m}"
  ( cd "$ROOT/backend" && go test "./$pkg" -run "$regex" -count=1 -v -timeout "$to" )
}

emit() { # artifact_basename, log_file
  local artifact="$1" log="$2"
  provenance > "$EVID/$artifact"
  cat "$log" >> "$EVID/$artifact"
  echo "ARTIFACT=$EVID/$artifact"
}

require_tokens() { # log, token...
  local log="$1"; shift
  local missing=""
  local tok
  for tok in "$@"; do
    if ! grep -Fq -- "$tok" "$log"; then
      missing="$missing [$tok]"
    fi
  done
  if [ -n "$missing" ]; then
    echo "ASSERT_FAIL: missing tokens:$missing" >&2
    return 1
  fi
  return 0
}

LOG="$(mktemp "$ROOT/tmp/verify-2461-${MODE}.XXXXXX.log")" || exit 1
trap 'rm -f "$LOG"' EXIT

case "$MODE" in
  r1)
    run_pkg_test services '^TestUpdateUserRoleInOrgJSONBody_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'METHOD=PUT' 'PATH=/api/v1/organizations/ORG1/users/USER1/role' 'HAS_QUERY=0' 'BODY={"role":"STAFF"}' 'AUTH=service' || { cat "$LOG"; exit 1; }
    emit 'R1-client-service.txt' "$LOG"
    ;;
  r2)
    run_pkg_test services '^TestUpdateUserRoleInOrgWithTokenJSONBody_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'METHOD=PUT' 'HAS_QUERY=0' 'BODY={"role":"ADMIN"}' 'AUTH=user-token' || { cat "$LOG"; exit 1; }
    emit 'R2-client-token.txt' "$LOG"
    ;;
  r3)
    run_pkg_test services '^TestUpdateUserRoleInOrg400NotSwallowed_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'ERR_CONTAINS_400=1' 'ERR_CONTAINS_ROLE_REQUIRED=1' || { cat "$LOG"; exit 1; }
    emit 'R3-client-400.txt' "$LOG"
    ;;
  r4)
    run_pkg_test handlers '^TestUpdateMemberRoleIAMFailFast_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'STATUS=502' 'LOCAL_UNCHANGED=1' 'IAM_ROLE_CALLS=1' || { cat "$LOG"; exit 1; }
    emit 'R4-site-failfast.txt' "$LOG"
    ;;
  r5)
    run_pkg_test handlers '^TestUpdateMemberRoleTemplateErrorsVisible_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'STATUS=200' 'ROLE_ERRORS=1' 'LOCAL_ROLE=site_admin' || { cat "$LOG"; exit 1; }
    emit 'R5-template-errors.txt' "$LOG"
    ;;
  r6)
    run_pkg_test handlers '^TestUpdateMerchantAdminDemoteFailureCompensates_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'STATUS=502' 'MSG_CONTAINS=旧管理员降级失败' 'IAM_UNBIND_CALLS=1' 'ADMIN_UID_UNCHANGED=1' || { cat "$LOG"; exit 1; }
    emit 'R6-merchant-compensate.txt' "$LOG"
    ;;
  r7)
    run_pkg_test handlers '^TestUpdateMerchantAdminBindFailureVisible_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'STATUS=200' 'ADMIN_PENDING=true' 'ADMIN_SYNC_WARNING=1' || { cat "$LOG"; exit 1; }
    emit 'R7-merchant-bind-pending.txt' "$LOG"
    ;;
  r8)
    run_pkg_test handlers '^TestUpdateStaffRoleIAMFailRoleErrors_2461$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'STATUS=200' 'ROLE_ERRORS=1' 'USERS_ROLE_UPDATED=1' || { cat "$LOG"; exit 1; }
    emit 'R8-staff-role-errors.txt' "$LOG"
    ;;
  r9)
    run_pkg_test handlers '^(TestTransitSiteMemberRoleFailFast_2461|TestPermissionManageSetUserRoleFailFast_2461|TestMerchantMemberRoleFailFast_2461)$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'TRANSIT=502' 'PERMISSION=500' 'MERCHANT_MEMBER=500' || { cat "$LOG"; exit 1; }
    emit 'R9-failfast-regression.txt' "$LOG"
    ;;
  r10)
    BUILD_EXIT=0
    {
      echo "BUILD_CMD=cd backend && go build ./..."
      ( cd "$ROOT/backend" && go build ./... ) || BUILD_EXIT=$?
      echo "BUILD_EXIT=$BUILD_EXIT"
      TESTS_FAIL=0
      echo "TESTS_CMD=go test ./handlers -run '2461' ; go test ./handlers -run '2375' ; go test ./services -run '2461'"
      ( cd "$ROOT/backend" && go test ./handlers -run '2461' -count=1 -timeout 15m ) || TESTS_FAIL=$((TESTS_FAIL + 1))
      ( cd "$ROOT/backend" && go test ./handlers -run '2375' -count=1 -timeout 15m ) || TESTS_FAIL=$((TESTS_FAIL + 1))
      ( cd "$ROOT/backend" && go test ./services -run '2461' -count=1 -timeout 15m ) || TESTS_FAIL=$((TESTS_FAIL + 1))
      echo "TESTS_FAIL=$TESTS_FAIL"
    } >"$LOG" 2>&1
    require_tokens "$LOG" 'BUILD_EXIT=0' 'TESTS_FAIL=0' || { cat "$LOG"; exit 1; }
    emit 'R10-build.txt' "$LOG"
    ;;
esac

echo "RESULT: PASS ($MODE)"
