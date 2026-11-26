#!/usr/bin/env bash
# verify-2527.sh — Evidence Contract runner for #2527
#   (归属缺失（nil UUID）用户的批量重置密码 / 网点调动 / IAM 归属推导)
#
# Usage:
#   bash scripts/verify-2527.sh r1 … r8
#
# Each row prints a provenance header, runs the row's commands, writes the
# artifact under docs/evidence/2527/ and fails (exit 1) when any expected
# assertion token is missing. Heavy commands (go build/test, npm build) go
# through scripts/heavy-run.sh when available so the host lock is honoured per
# command (never around the whole row).
#
# Requires: Go toolchain, psql reachable test DB (POSTGRES_*/TUNELOOP_DB, or
# backend/.env), node + frontend-pc dependencies for R4.
set -uo pipefail

ROW="${1:-}"
ISSUE=2527
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/docs/evidence/$ISSUE"
HEAVY_RUN="${HEAVY_RUN:-$HOME/.config/opencode/scripts/heavy-run.sh}"
LOCK_DIR="${HEAVY_LOCK_DIR:-/tmp/opencode/locks}"

mkdir -p "$OUT" "$ROOT/tmp"
cd "$ROOT" || exit 2

if [ -f "$ROOT/backend/.env" ]; then
  set -a
  # shellcheck disable=SC1091
  . "$ROOT/backend/.env"
  set +a
fi
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-5432}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export TUNELOOP_DB="${TUNELOOP_DB:-tuneloop_debug}"

# lib/pq mis-parses `password= dbname=…` when the value is empty (it silently
# connects to a DB named after the user), so fail loudly instead of producing
# misleading evidence.
if [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "ERROR: POSTGRES_PASSWORD is empty (backend/.env missing?). Tests need a real test-DB password." >&2
  exit 2
fi

COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short HEAD)"
BUILD="1.0.$SHORT"
# Evidence artifacts are regenerated after the fix commit; their own churn must
# not mark the tree dirty (they are committed separately as docs(evidence): issue-2527).
DIRTY="false"
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/' || true)" ]; then
  DIRTY="true"
fi

PROVENANCE="# build=$BUILD # commit=$COMMIT # dirty=$DIRTY"

FAILURES=0

heavy() {
  if [ -x "$HEAVY_RUN" ]; then
    bash "$HEAVY_RUN" --dir "$LOCK_DIR" -- "$@"
  else
    "$@"
  fi
}

assert_grep() { # artifact token...
  local artifact="$1"; shift
  local missing=0
  for token in "$@"; do
    if ! grep -qF -- "$token" "$artifact"; then
      echo "MISSING: $token (in ${artifact#$ROOT/})" >&2
      missing=1
      FAILURES=$((FAILURES + 1))
    fi
  done
  if [ "$missing" = "0" ]; then
    echo "ASSERT_OK: ${artifact#$ROOT/} ($# tokens)"
  fi
}

go_test_row() { # artifact run-regex packages...
  local artifact="$1"; shift
  local regex="$1"; shift
  local pkgs=("$@")
  {
    echo "$PROVENANCE"
    echo "# CMD go test -count=1 -run '$regex' ${pkgs[*]}"
    ( cd "$ROOT/backend" && heavy go test -v -count=1 -run "$regex" "${pkgs[@]}" )
  } >>"$artifact" 2>&1
  local rc=$?
  echo "GO_TEST_RC=$rc" >>"$artifact"
  if [ "$rc" != "0" ]; then
    echo "GO_TEST_FAIL: $regex (${artifact#$ROOT/})" >&2
    FAILURES=$((FAILURES + 1))
  fi
}

row_r1() {
  local a="$OUT/R1-reset-visible.txt"
  : >"$a"
  go_test_row "$a" '^TestResetPasswordZeroTenantVisible_2527$' ./handlers/
  assert_grep "$a" 'GO_TEST_RC=0' 'STATUS=200' 'SENT=1' 'IAM_IDS=1' 'SKIPPED_INVISIBLE=0'
}

row_r2() {
  local a="$OUT/R2-reset-mixed.txt"
  : >"$a"
  go_test_row "$a" '^TestResetPasswordMixedClassification_2527$' ./handlers/
  assert_grep "$a" 'GO_TEST_RC=0' 'STATUS=200' 'IAM_IDS=1' 'SKIPPED_INVISIBLE=1' 'IAM_INCLUDES_CROSS_TENANT=0'
}

row_r3() {
  local a="$OUT/R3-reset-all-invisible.txt"
  : >"$a"
  go_test_row "$a" '^TestResetPasswordAllInvisible_2527$' ./handlers/
  assert_grep "$a" 'GO_TEST_RC=0' 'STATUS=200' 'SENT=0' 'SKIPPED_INVISIBLE=1' 'IAM_CALLS=0'
}

row_r4() {
  local a="$OUT/R4-pc-wiring.txt"
  {
    echo "$PROVENANCE"
    echo "# R4 PC 提示接线：纯函数 node --test + 源码结构断言 + eslint + build"
  } >"$a"

  ( cd "$ROOT/frontend-pc" && node --test src/utils/resetPasswordSummary.test.js ) >>"$a" 2>&1
  local node_rc=$?
  echo "NODE_TEST_RC=$node_rc" >>"$a"

  # 新增分类字段必须有消费方（B7）：页面必须 import 纯函数并据其分支提示。
  local wired=0
  if grep -qF "resetPasswordSummary" "$ROOT/frontend-pc/src/pages/StaffManagement.jsx" &&
     grep -qF "from '../utils/resetPasswordSummary'" "$ROOT/frontend-pc/src/pages/StaffManagement.jsx" &&
     grep -qF "message.warning(summary.text)" "$ROOT/frontend-pc/src/pages/StaffManagement.jsx" &&
     grep -qF "skipped_invisible" "$ROOT/frontend-pc/src/utils/resetPasswordSummary.js"; then
    wired=1
  fi
  echo "RENDER_WIRED=$wired" >>"$a"

  ( cd "$ROOT/frontend-pc" && heavy npx eslint src/pages/StaffManagement.jsx src/utils/resetPasswordSummary.js src/utils/resetPasswordSummary.test.js ) >>"$a" 2>&1
  local lint_rc=$?
  echo "FRONTEND_LINT_RC=$lint_rc" >>"$a"

  ( cd "$ROOT/frontend-pc" && heavy npm run build ) >>"$a" 2>&1
  local build_rc=$?
  echo "FRONTEND_BUILD_RC=$build_rc" >>"$a"

  assert_grep "$a" 'NODE_TEST_RC=0' 'RENDER_WIRED=1' 'FRONTEND_LINT_RC=0' 'FRONTEND_BUILD_RC=0'
}

row_r5() {
  local a="$OUT/R5-role-derived-org.txt"
  : >"$a"
  go_test_row "$a" '^TestUpdateUserRoleUsesDerivedOrg_2527$' ./handlers/
  assert_grep "$a" 'GO_TEST_RC=0' 'STATUS=200' 'IAM_ORG=orgA' 'NIL_UUID_CALLS=0' 'ROLE_ERRORS=0'
}

row_r6() {
  local a="$OUT/R6-transfer-derived-org.txt"
  : >"$a"
  go_test_row "$a" '^TestSiteTransferUsesDerivedOrg_2527$' ./handlers/
  assert_grep "$a" 'GO_TEST_RC=0' 'STATUS=200' 'UNBIND_ORG=orgA' 'BIND_ORG=orgB' 'NIL_UUID_CALLS=0'
}

row_r7() {
  local a="$OUT/R7-no-attribution.txt"
  : >"$a"
  go_test_row "$a" '^TestNoAttributionRejectedExplicitly_2527$' ./handlers/
  assert_grep "$a" 'GO_TEST_RC=0' 'TRANSFER_STATUS=40002' 'ROLE_STATUS=200' 'ROLE_ERRORS_ATTRIBUTION=1' 'NIL_UUID_CALLS=0'
}

row_r8() {
  local a="$OUT/R8-regression.txt"
  {
    echo "$PROVENANCE"
    echo "# R8 go build ./... + 重置密码/调动/角色/可见性 既有用例定向回归"
  } >"$a"

  ( cd "$ROOT/backend" && heavy go build ./... ) >>"$a" 2>&1
  local build_rc=$?
  echo "BUILD_RC=$build_rc" >>"$a"

  local regression_regex='TestResetPassword|TestSiteTransfer_|TestUpdateUserRole|TestUpdateMerchantAdmin|TestPersonnel|TestStaffUser|TestResetPasswordInvisibleCustomerSkipped_2527'
  ( cd "$ROOT/backend" && heavy go test ./handlers/ -run "$regression_regex" -count=1 -v ) >"$ROOT/tmp/r8-2527-tests.log" 2>&1
  local regression_rc=$?
  local fail_count
  fail_count=$(grep -c '^--- FAIL' "$ROOT/tmp/r8-2527-tests.log" 2>/dev/null || true)
  fail_count=${fail_count:-0}
  grep -E '^(--- PASS|--- FAIL|INVISIBLE_CUSTOMER_SKIPPED=)' "$ROOT/tmp/r8-2527-tests.log" >>"$a" 2>/dev/null || true

  echo "REGRESSION_RC=$regression_rc" >>"$a"
  echo "FAIL_COUNT=$fail_count" >>"$a"
  local existing_2389=0
  if grep -q '^--- PASS: TestResetPasswordNoEmail_2389' "$ROOT/tmp/r8-2527-tests.log"; then
    existing_2389=1
  fi
  local invisible_customer=0
  if grep -q 'INVISIBLE_CUSTOMER_SKIPPED=1' "$ROOT/tmp/r8-2527-tests.log"; then
    invisible_customer=1
  fi
  if [ "$regression_rc" = "0" ] && [ "$build_rc" = "0" ] && [ "$fail_count" = "0" ]; then
    echo "REGRESSION_PASS=1" >>"$a"
  else
    echo "REGRESSION_PASS=0" >>"$a"
  fi
  echo "EXISTING_2389_PASS=$existing_2389" >>"$a"
  echo "INVISIBLE_CUSTOMER_SKIPPED=$invisible_customer" >>"$a"

  assert_grep "$a" 'BUILD_RC=0' 'REGRESSION_PASS=1' 'FAIL_COUNT=0' 'EXISTING_2389_PASS=1' 'INVISIBLE_CUSTOMER_SKIPPED=1' '--- PASS: TestSiteTransfer_Readback_2466'
}

case "$ROW" in
  r1) row_r1 ;;
  r2) row_r2 ;;
  r3) row_r3 ;;
  r4) row_r4 ;;
  r5) row_r5 ;;
  r6) row_r6 ;;
  r7) row_r7 ;;
  r8) row_r8 ;;
  *)
    echo "usage: bash scripts/verify-2527.sh {r1|r2|r3|r4|r5|r6|r7|r8}" >&2
    exit 2
    ;;
esac

if [ "$FAILURES" != "0" ]; then
  echo "RESULT: FAIL ($FAILURES assertion(s))"
  exit 1
fi
echo "RESULT: OK ($ROW)"
