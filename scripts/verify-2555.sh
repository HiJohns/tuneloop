#!/usr/bin/env bash
# verify-2555.sh — Evidence Contract runner for #2555
#   (#2526 可见性口径回归：TestActivateUser2309_* 三个回滚用例恒 404，夹具缺员工角色)
#
# Usage:
#   bash scripts/verify-2555.sh r1 … r5
#
# Each row prints a provenance header, runs the row's commands, writes the
# artifact under docs/evidence/2555/ and fails (exit 1) when any expected
# assertion token is missing. Heavy commands go through scripts/heavy-run.sh
# when available so the host lock is honoured per command.
#
# Requires: Go toolchain + psql reachable test DB (POSTGRES_*/TUNELOOP_DB, or backend/.env).
set -uo pipefail

ROW="${1:-}"
ISSUE=2555
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

if [ -z "${POSTGRES_PASSWORD:-}" ]; then
  echo "ERROR: POSTGRES_PASSWORD is empty (backend/.env missing?). Tests need a real test-DB password." >&2
  exit 2
fi

COMMIT="$(git rev-parse HEAD)"
SHORT="$(git rev-parse --short HEAD)"
BUILD="1.0.$SHORT"
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

go_test_row() { # artifact run-regex
  local artifact="$1"; shift
  local regex="$1"
  {
    echo "$PROVENANCE"
    echo "# CMD go test -count=1 -v -run '$regex' ./handlers/"
    ( cd "$ROOT/backend" && heavy go test -v -count=1 -run "$regex" ./handlers/ )
  } >>"$artifact" 2>&1
  local rc=$?
  echo "GO_TEST_RC=$rc" >>"$artifact"
  if [ "$rc" != "0" ]; then
    echo "GO_TEST_FAIL: $regex (${artifact#$ROOT/})" >&2
    FAILURES=$((FAILURES + 1))
  fi
}

row_r1() {
  local a="$OUT/R1-rollback-paths.txt"
  : >"$a"
  go_test_row "$a" '^(TestActivateUser2309_BindFailurePurgesIAM|TestActivateUser2309_PermissionFailurePurgesIAM|TestActivateUser2309_LocalUpdateFailurePurgesIAM)$'
  local pass_count
  pass_count=$(grep -c '^--- PASS: TestActivateUser2309_' "$a" 2>/dev/null || true)
  pass_count=${pass_count:-0}
  echo "ACTIVATE2309_PASS_COUNT=$pass_count" >>"$a"
  assert_grep "$a" 'GO_TEST_RC=0' \
    'ACTIVATE2309_CASE=bind_failure HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=0 IAM_UNBIND=0' \
    'ACTIVATE2309_CASE=permission_failure HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=1 IAM_UNBIND=1' \
    'ACTIVATE2309_CASE=local_update_failure HTTP=500 PURGE=1 SOFT=0 IAM_CREATE=1 IAM_BIND=1 IAM_PERM=1 IAM_UNBIND=1 LOCAL_UPDATE_ATTEMPT=1' \
    'ACTIVATE2309_PASS_COUNT=3'
}

row_r2() {
  local a="$OUT/R2-mock-role-templates.txt"
  : >"$a"
  go_test_row "$a" '^TestActivateUser2309_IAMMockServesRoleTemplates$'
  assert_grep "$a" 'GO_TEST_RC=0' 'IAM_ROLE_TEMPLATE_COUNT=1' 'IAM_ROLE_TEMPLATE_CODE=site_member'
}

row_r3() {
  local a="$OUT/R3-fixture-precondition.txt"
  : >"$a"
  go_test_row "$a" '^TestActivateUser2309_FixtureVisibilityPrecondition$'
  assert_grep "$a" 'GO_TEST_RC=0' 'EMPTY_ROLE_STATUS=404' 'EMPTY_ROLE_IAM_CREATE=0' 'STAFF_ROLE_STATUS=500' 'STAFF_ROLE_PURGE=1'
}

row_r4() {
  local a="$OUT/R4-regression.txt"
  {
    echo "$PROVENANCE"
    echo "# R4 go build ./... + 定向回归（#2309 / #2526 / #2375）"
  } >"$a"

  ( cd "$ROOT/backend" && heavy go build ./... ) >>"$a" 2>&1
  local build_rc=$?
  echo "BUILD_RC=$build_rc" >>"$a"

  local regex='TestActivateUser2309|TestCreateUser2309|TestStaffDetail2526|TestStaffUpdate2526|TestStaffActivate2526|TestPersonnelAttributionFlag2526|TestUpdateUserRoleForbidden_2375|TestCreateUserRoleErrors_2375'
  ( cd "$ROOT/backend" && heavy go test -count=1 -v -run "$regex" ./handlers/ ) >"$ROOT/tmp/r4-2555.log" 2>&1
  local regression_rc=$?
  local fail_count
  fail_count=$(grep -c '^--- FAIL' "$ROOT/tmp/r4-2555.log" 2>/dev/null || true)
  fail_count=${fail_count:-0}
  grep -E '^--- (PASS|FAIL)' "$ROOT/tmp/r4-2555.log" >>"$a" 2>/dev/null || true

  echo "REGRESSION_RC=$regression_rc" >>"$a"
  echo "FAIL_COUNT=$fail_count" >>"$a"
  local existing_2526=0 existing_2375=0
  if grep -q '^--- PASS: TestStaffDetail2526_' "$ROOT/tmp/r4-2555.log"; then existing_2526=1; fi
  if grep -q '^--- PASS: TestUpdateUserRoleForbidden_2375' "$ROOT/tmp/r4-2555.log"; then existing_2375=1; fi
  if [ "$regression_rc" = "0" ] && [ "$build_rc" = "0" ] && [ "$fail_count" = "0" ]; then
    echo "REGRESSION_PASS=1" >>"$a"
  else
    echo "REGRESSION_PASS=0" >>"$a"
  fi
  echo "EXISTING_2526_PASS=$existing_2526" >>"$a"
  echo "EXISTING_2375_PASS=$existing_2375" >>"$a"

  assert_grep "$a" 'BUILD_RC=0' 'REGRESSION_PASS=1' 'FAIL_COUNT=0' 'EXISTING_2526_PASS=1' 'EXISTING_2375_PASS=1'
}

row_r5() {
  local a="$OUT/R5-static.txt"
  {
    echo "$PROVENANCE"
    echo "# R5 gofmt -l + go vet ./handlers"
  } >"$a"

  local gofmt_diff
  gofmt_diff=$(cd "$ROOT/backend" && gofmt -l handlers/ | wc -l | tr -d ' ')
  echo "GOFMT_DIFF=$gofmt_diff" >>"$a"

  ( cd "$ROOT/backend" && heavy go vet ./handlers/ ) >>"$a" 2>&1
  local vet_rc=$?
  echo "VET_RC=$vet_rc" >>"$a"

  assert_grep "$a" 'GOFMT_DIFF=0' 'VET_RC=0'
}

case "$ROW" in
  r1) row_r1 ;;
  r2) row_r2 ;;
  r3) row_r3 ;;
  r4) row_r4 ;;
  r5) row_r5 ;;
  *)
    echo "usage: bash scripts/verify-2555.sh {r1|r2|r3|r4|r5}" >&2
    exit 2
    ;;
esac

if [ "$FAILURES" != "0" ]; then
  echo "RESULT: FAIL ($FAILURES assertion(s))"
  exit 1
fi
echo "RESULT: OK ($ROW)"
