#!/usr/bin/env bash
# #2462 证据生成器 —— 商户直属员工角色落域（users.role / merchant_members.role / 编辑预填）。
#
# 用法：bash scripts/verify-2462.sh <r1|r2|r3|r4|r5|r6|r7|r8|r9>
#   r1  R1-create-direct.txt    创建直属员工：users.role=merchant_staff、mm.role=merchant_staff、无 site_members
#   r2  R2-create-site.txt      创建网点员工：users.role=site_members.role=site_member
#   r3  R3-create-tech.txt      创建维修师傅：users.role=repair_technician、1 个师傅档案、无 merchant_members
#   r4  R4-list-role.txt        列表角色列（直属）：LIST_ROLE=merchant_staff、与 users.role 一致
#   r5  R5-noop-role.txt        编辑只改职位：不改角色、IAM 角色同步 0 次
#   r6  R6-role-change.txt      编辑显式改角色：双写 + IAM 同步 1 次（STAFF）
#   r7  R7-drift.txt            存量漂移只读清单（fixture）+ 未授权 --apply exit 2
#   r8  R8-prefill.txt          编辑页角色预填（源码静态 + 分支行为断言，evidence_type=static，D5 Owner 豁免）
#   r9  R9-build.txt            go build + 定向测试 + PC build
#
# 产物首行 provenance：`# build=1.0.<fix短码> # commit=<sha> # dirty=<bool>`
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1|r2|r3|r4|r5|r6|r7|r8|r9) ;;
  *)
    echo "usage: $0 <r1|r2|r3|r4|r5|r6|r7|r8|r9>" >&2
    exit 2
    ;;
esac

ROOT="$(git rev-parse --show-toplevel)"
HEAD_FULL="$(git rev-parse HEAD)"
HEAD_SHORT="$(git rev-parse --short HEAD)"
EVID="$ROOT/docs/evidence/2462"
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
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2462}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$ROOT/tmp/gocache}"
mkdir -p "$TMPDIR" "$GOCACHE"

provenance() { echo "# build=1.0.${HEAD_SHORT} # commit=${HEAD_FULL} # dirty=${DIRTY}"; }
emit() { # artifact log
  provenance > "$EVID/$1"
  cat "$2" >> "$EVID/$1"
  echo "ARTIFACT=$EVID/$1"
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
run_go() { # regex
  ( cd "$ROOT/backend" && go test ./handlers -run "$1" -count=1 -v -timeout 15m )
}

LOG="$(mktemp "$ROOT/tmp/verify-2462-${MODE}.XXXXXX.log")" || exit 1
trap 'rm -f "$LOG"' EXIT

case "$MODE" in
  r1)
    run_go '^TestCreateMerchantDirectRoleDomain_2462$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'USERS_ROLE=merchant_staff' 'MM_ROLE=merchant_staff' 'SITE_MEMBERS_ROWS=0' || { cat "$LOG"; exit 1; }
    emit 'R1-create-direct.txt' "$LOG"
    ;;
  r2)
    run_go '^TestCreateSiteStaffRoleDomain_2462$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'USERS_ROLE=site_member' 'SM_ROLE=site_member' 'ROLE_MATCH=1' || { cat "$LOG"; exit 1; }
    emit 'R2-create-site.txt' "$LOG"
    ;;
  r3)
    run_go '^TestCreateTechnicianRoleDomain_2462$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'USERS_ROLE=repair_technician' 'TECH_PROFILES=1' 'MM_ROWS=0' || { cat "$LOG"; exit 1; }
    emit 'R3-create-tech.txt' "$LOG"
    ;;
  r4)
    run_go '^TestListStaffDirectRole_2462$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'LIST_ROLE=merchant_staff' 'ROLE_MATCH=1' 'SITE_NAME_EMPTY=1' || { cat "$LOG"; exit 1; }
    emit 'R4-list-role.txt' "$LOG"
    ;;
  r5)
    run_go '^TestUpdateStaffRoleNoop_2462$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'USERS_ROLE_UNCHANGED=1' 'IAM_ROLE_CALLS=0' 'ROLE_ERRORS=0' 'POSITION=首席' || { cat "$LOG"; exit 1; }
    emit 'R5-noop-role.txt' "$LOG"
    ;;
  r6)
    run_go '^TestUpdateStaffRoleChanged_2462$' >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'USERS_ROLE=merchant_staff' 'MM_ROLE=merchant_staff' 'IAM_ROLE_CALLS=1' 'IAM_ROLE_VALUE=STAFF' || { cat "$LOG"; exit 1; }
    emit 'R6-role-change.txt' "$LOG"
    ;;
  r7)
    bash "$ROOT/scripts/tests/test_merchant_direct_role_drift_2462.sh" >"$LOG" 2>&1 || { cat "$LOG"; exit 1; }
    require_tokens "$LOG" 'ALIGN_ROWS=2' 'NONE_ROWS=1' 'READONLY=1' 'UNAUTHORIZED_APPLY_EXIT=2' 'DB_WRITES=0' || { cat "$LOG"; exit 1; }
    emit 'R7-drift.txt' "$LOG"
    ;;
  r8)
    # evidence_type=static（D5 Owner 豁免）：从真实源码抽取域判定函数并执行两个分支。
    node - "$ROOT/frontend-pc/src/pages/StaffEdit.jsx" >"$LOG" 2>&1 <<'NODE'
const fs = require('fs')
const path = process.argv[2]
const src = fs.readFileSync(path, 'utf8')
const m = src.match(/const SITE_DOMAIN_ROLES = \[[^\]]*\]\n\nfunction resolveInitialRole[\s\S]*?\n\}/)
if (!m) {
  console.log('DOMAIN_GUARD=0')
  process.exit(1)
}
const factory = new Function(`${m[0]}\nreturn resolveInitialRole`)
const resolve = factory()
const legacy = resolve({ role: 'site_member' })              // 无 site_id 的天量存量行
const legacyAdmin = resolve({ role: 'site_admin' })
const normal = resolve({ role: 'site_member', site_id: 's-1' }) // 正常网点员工
const direct = resolve({ role: 'merchant_staff' })
console.log('DOMAIN_GUARD=1')
console.log(`PREFILL_LEGACY=${legacy}`)
console.log(`PREFILL_LEGACY_ADMIN=${legacyAdmin}`)
console.log(`PREFILL_NORMAL=${normal}`)
console.log(`PREFILL_DIRECT=${direct}`)
if (legacy !== 'merchant_staff' || legacyAdmin !== 'merchant_staff' || normal !== 'site_member' || direct !== 'merchant_staff') {
  process.exit(1)
}
NODE
    require_tokens "$LOG" 'DOMAIN_GUARD=1' 'PREFILL_LEGACY=merchant_staff' 'PREFILL_NORMAL=site_member' || { cat "$LOG"; exit 1; }
    emit 'R8-prefill.txt' "$LOG"
    ;;
  r9)
    BUILD_EXIT=0
    TESTS_FAIL=0
    PC_BUILD=0
    {
      echo "BUILD_CMD=cd backend && go build ./..."
      ( cd "$ROOT/backend" && go build ./... ) || BUILD_EXIT=$?
      echo "BUILD_EXIT=$BUILD_EXIT"
      echo "TESTS_CMD=go test ./handlers -run 2462 (含 2375 回归)"
      ( cd "$ROOT/backend" && go test ./handlers -run '2462' -count=1 -timeout 15m ) || TESTS_FAIL=$((TESTS_FAIL + 1))
      ( cd "$ROOT/backend" && go test ./handlers -run '2375' -count=1 -timeout 15m ) || TESTS_FAIL=$((TESTS_FAIL + 1))
      echo "TESTS_FAIL=$TESTS_FAIL"
      echo "PC_BUILD_CMD=cd frontend-pc && npm run build"
      ( cd "$ROOT/frontend-pc" && npm run build >/dev/null 2>&1 ) || PC_BUILD=1
      echo "PC_BUILD=$PC_BUILD"
    } >"$LOG" 2>&1
    require_tokens "$LOG" 'BUILD_EXIT=0' 'TESTS_FAIL=0' 'PC_BUILD=0' || { cat "$LOG"; exit 1; }
    emit 'R9-build.txt' "$LOG"
    ;;
esac

echo "RESULT: PASS ($MODE)"
