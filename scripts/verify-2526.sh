#!/usr/bin/env bash
# #2526 verification harness — one subcommand per Evidence Contract row.
#
#   bash scripts/verify-2526.sh r1   # 零租户成员详情可加载（红绿主线）
#   bash scripts/verify-2526.sh r2   # site_admin 详情 + 跨组织负例
#   bash scripts/verify-2526.sh r3   # system_admin 详情口径
#   bash scripts/verify-2526.sh r4   # 越权负例（纯顾客 / 他商户）
#   bash scripts/verify-2526.sh r5   # 列表 ↔ 详情逐行一致性（不变量）
#   bash scripts/verify-2526.sh r6   # 保存零租户行真正落库
#   bash scripts/verify-2526.sh r7   # 激活行操作
#   bash scripts/verify-2526.sh r8   # 归属缺失标记（三视图）
#   bash scripts/verify-2526.sh r9   # PC 列渲染接线（纯函数 + 静态接线）
#   bash scripts/verify-2526.sh r10  # 构建 + 既有用例回归
#
# Capture a row into its artifact:
#   bash scripts/verify-2526.sh r1 > docs/evidence/2526/R1-detail-zero-tenant.txt
#
# Every artifact starts with the provenance header consumed by
# scripts/evidence-artifacts.sh (P14):
#   # build=1.0.<short-sha> # commit=<full-sha> # dirty=<true|false>
# The anchor is the issue's fix commit (FIX_COMMIT overrides), never HEAD after
# the evidence commit, so a later re-run keeps pointing at the same source.
set -uo pipefail

ROW="${1:-}"
case "$ROW" in
  r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|gate) ;;
  *) echo "usage: $0 r1|r2|r3|r4|r5|r6|r7|r8|r9|r10|gate" >&2; exit 2 ;;
esac

ROOT="$(git rev-parse --show-toplevel 2>/dev/null)" || { echo "not a git repository" >&2; exit 2; }
cd "$ROOT" || exit 2

if ! command -v go >/dev/null 2>&1 && [ -x /usr/local/go/bin/go ]; then
  export PATH="/usr/local/go/bin:$PATH"
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

FIX_SHA="${FIX_COMMIT:-$(git log --format=%H --grep='^fix(issue-2526)' -1 2>/dev/null)}"
ANCHOR_SHA="${FIX_SHA:-$(git rev-parse HEAD)}"
SHORT_SHA="$(git rev-parse --short "$ANCHOR_SHA")"
BUILD="1.0.${SHORT_SHA}"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence' ':!tmp')" ]; then
  DIRTY=true
fi
echo "# build=${BUILD} # commit=${ANCHOR_SHA} # dirty=${DIRTY}"

fail() {
  echo "FAIL(${ROW}): $*" >&2
  exit 1
}

HEAVY="$HOME/.config/opencode/scripts/heavy-run.sh"
heavy() { bash "$HEAVY" -- "$@"; }

go_test_row() { # test-name → echoes its token lines
  local name="$1" out rc
  out="$(mktemp)"
  ( cd backend && heavy go test ./handlers -run "${name}" -count=1 -v ) > "$out" 2>&1
  rc=$?
  grep -E '^(DETAIL_|OLD_PREDICATE_ROWS|MEMBERSHIP_ROWS|RED_CONFIRMED|SITE_DETAIL_|SYSTEM_|MERCHANT_|PARITY_|UPDATE_|ACTIVATE_|FLAG_)' "$out" || true
  grep -E '^--- (PASS|FAIL)' "$out" || true
  if [ "$rc" -ne 0 ]; then
    echo "--- go test output (tail) ---"
    tail -30 "$out"
    rm -f "$out"
    fail "go test ${name} exit=${rc}"
  fi
  rm -f "$out"
}

need_token() { # expected-line file
  grep -qx -- "$1" "$2" || fail "missing expected token '$1'"
}

run_row() { # test-name expected-token...
  local name="$1"; shift
  local out; out="$(mktemp)"
  go_test_row "$name" > "$out" 2>&1
  cat "$out"
  local tok
  for tok in "$@"; do
    need_token "$tok" "$out"
  done
  rm -f "$out"
}

REG_PATTERN='TestPersonnel2065|TestPersonnel_|TestPersonnel2067|TestPersonnel2072|TestStaffDetail2526|TestStaffUpdate2526|TestStaffActivate2526|TestPersonnelAttributionFlag2526|TestGetStaffUser_|TestUpdateUser_|TestSiteTransfer_|TestUpdateUserRoleForbidden_2375'

case "$ROW" in
  r1)
    echo "== R1 零租户成员详情可加载（红绿主线） =="
    run_row TestStaffDetail2526_ZeroTenantMember \
      "DETAIL_ZERO_TENANT_STATUS=200" \
      "DETAIL_ZERO_TENANT_USER_ROLE_STATUS=200" \
      "OLD_PREDICATE_ROWS=0" \
      "MEMBERSHIP_ROWS=1" \
      "DETAIL_SITE_NAME=网点A" \
      "RED_CONFIRMED=1"
    echo "R1_OK"
    ;;

  r2)
    echo "== R2 site_admin 详情 + 跨组织负例 =="
    run_row TestStaffDetail2526_SiteAdmin \
      "SITE_DETAIL_ZERO=200" \
      "SITE_DETAIL_CROSS_ORG=404"
    echo "R2_OK"
    ;;

  r3)
    echo "== R3 system_admin 详情口径 =="
    run_row TestStaffDetail2526_SystemAdmin \
      "SYSTEM_PLATFORM=200" \
      "SYSTEM_TRANSIT=200" \
      "SYSTEM_ZERO_TENANT=404"
    echo "R3_OK"
    ;;

  r4)
    echo "== R4 越权负例（不泄漏存在性） =="
    run_row TestStaffDetail2526_IsolationNegative \
      "MERCHANT_CUSTOMER=404" \
      "MERCHANT_CUSTOMER_BODY_CODE=40400" \
      "MERCHANT_OTHER_TENANT=404" \
      "MERCHANT_OTHER_TENANT_BODY_CODE=40400"
    echo "R4_OK"
    ;;

  r5)
    echo "== R5 列表 ↔ 详情逐行一致性 =="
    run_row TestStaffDetail2526_ListDetailParity \
      "PARITY_LIST_ROWS=5" \
      "PARITY_DETAIL_OK=5" \
      "PARITY_MISMATCH=0"
    echo "R5_OK"
    ;;

  r6)
    echo "== R6 保存：零租户行真正落库 + 跨租户不可写 =="
    run_row TestStaffUpdate2526_ZeroTenantPersists \
      "UPDATE_ZERO_STATUS=200" \
      "UPDATE_ZERO_NAME_PERSISTED=1" \
      "UPDATE_CROSS_TENANT_STATUS=404" \
      "UPDATE_CROSS_TENANT_UNCHANGED=1"
    echo "R6_OK"
    ;;

  r7)
    echo "== R7 激活行操作：零租户行不再 404 =="
    run_row TestStaffActivate2526_RowAction \
      "ACTIVATE_ZERO_STATUS=40001" \
      "ACTIVATE_ZERO_HTTP=400" \
      "ACTIVATE_CROSS_TENANT_STATUS=404"
    echo "R7_OK"
    ;;

  r8)
    echo "== R8 归属缺失标记（三视图一致） =="
    run_row TestPersonnelAttributionFlag2526 \
      "FLAG_ZERO_TENANT=1" \
      "FLAG_NORMAL_MEMBER=0" \
      "FLAG_SITE_VIEW_ZERO=1"
    echo "R8_OK"
    ;;

  r9)
    echo "== R9 PC 列渲染接线（纯函数 + 静态接线 + lint/build） =="
    OUT="$(mktemp)"
    node --test frontend-pc/src/utils/personnelQuery.test.js > "$OUT" 2>&1
    NODE_TEST_RC=$?
    tail -8 "$OUT"
    rm -f "$OUT"

    PAGE="frontend-pc/src/pages/StaffManagement.jsx"
    UTIL="frontend-pc/src/utils/personnelQuery.js"
    [ -f "$PAGE" ] || fail "missing $PAGE"
    grep -q 'attributionMissingBadge' "$UTIL" || fail "$UTIL must define attributionMissingBadge"
    IMPORTED="$(grep -c "attributionMissingBadge.*from '../utils/personnelQuery'" "$PAGE" || true)"
    RENDERED="$(grep -c 'attributionMissingBadge(record)' "$PAGE" || true)"
    RENDER_WIRED=0
    if [ "${IMPORTED:-0}" = "1" ] && [ "${RENDERED:-0}" = "1" ]; then RENDER_WIRED=1; fi

    LINT_OUT="$(mktemp)"
    ( cd frontend-pc && heavy npx eslint src/pages/StaffManagement.jsx src/utils/personnelQuery.js src/utils/personnelQuery.test.js ) > "$LINT_OUT" 2>&1
    FRONTEND_LINT_RC=$?
    tail -5 "$LINT_OUT"
    rm -f "$LINT_OUT"

    BUILD_OUT="$(mktemp)"
    ( cd frontend-pc && heavy npm run build ) > "$BUILD_OUT" 2>&1
    FRONTEND_BUILD_RC=$?
    tail -8 "$BUILD_OUT"
    rm -f "$BUILD_OUT"

    echo "NODE_TEST_RC=${NODE_TEST_RC}"
    echo "RENDER_WIRED=${RENDER_WIRED}"
    echo "FRONTEND_LINT_RC=${FRONTEND_LINT_RC}"
    echo "FRONTEND_BUILD_RC=${FRONTEND_BUILD_RC}"
    [ "$NODE_TEST_RC" -eq 0 ] || fail "node --test failed (rc=${NODE_TEST_RC})"
    [ "$RENDER_WIRED" -eq 1 ] || fail "attribution_missing has no consumer in $PAGE (dead field)"
    [ "$FRONTEND_LINT_RC" -eq 0 ] || fail "eslint failed (rc=${FRONTEND_LINT_RC})"
    [ "$FRONTEND_BUILD_RC" -eq 0 ] || fail "frontend build failed (rc=${FRONTEND_BUILD_RC})"
    echo "R9_OK"
    ;;

  r10|gate)
    echo "== R10 构建 + 既有用例回归 =="
    BUILD_OUT="$(mktemp)"
    ( cd backend && heavy go build ./... ) > "$BUILD_OUT" 2>&1
    BUILD_RC=$?
    tail -5 "$BUILD_OUT"
    rm -f "$BUILD_OUT"

    REG_OUT="$(mktemp)"
    ( cd backend && heavy go test ./handlers -count=1 -v -run "$REG_PATTERN" ) > "$REG_OUT" 2>&1
    REG_RC=$?
    FAIL_COUNT="$(grep -c '^--- FAIL' "$REG_OUT" || true)"
    PASS_COUNT="$(grep -c '^--- PASS' "$REG_OUT" || true)"
    grep -E '^--- (PASS|FAIL)' "$REG_OUT" | tail -30 || true
    tail -3 "$REG_OUT"
    REGRESSION_PASS=0
    if [ "$REG_RC" -eq 0 ] && [ "${FAIL_COUNT:-1}" = "0" ]; then REGRESSION_PASS=1; fi
    HAVE_2429="$(grep -c -- '--- PASS: TestGetStaffUser_ReturnsSiteAndDirectFlag_2429' "$REG_OUT" || true)"
    HAVE_2375="$(grep -c -- '--- PASS: TestUpdateUserRoleForbidden_2375' "$REG_OUT" || true)"
    rm -f "$REG_OUT"

    echo "BUILD_RC=${BUILD_RC}"
    echo "REGRESSION_PASS=${REGRESSION_PASS}"
    echo "FAIL_COUNT=${FAIL_COUNT:-0}"
    echo "REGRESSION_PASS_COUNT=${PASS_COUNT:-0}"
    echo "EXISTING_2429_PASS=${HAVE_2429:-0}"
    echo "EXISTING_2375_PASS=${HAVE_2375:-0}"

    [ "$BUILD_RC" -eq 0 ] || fail "go build ./... failed (rc=${BUILD_RC})"
    [ "$REGRESSION_PASS" -eq 1 ] || fail "regression suite failed (rc=${REG_RC}, fails=${FAIL_COUNT:-?})"
    [ "${HAVE_2429:-0}" -ge 1 ] || fail "existing #2429 detail case did not PASS"
    [ "${HAVE_2375:-0}" -ge 1 ] || fail "existing #2375 role-whitelist case did not PASS"
    echo "R10_OK"
    ;;
esac
