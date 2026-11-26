#!/usr/bin/env bash
# #2505 S1 evidence generator — IAM 功能角色「集合语义」+ 人员管理改角色主诉路径。
#
# 用法：
#   bash scripts/verify-2505-s1.sh <r1|r2|...|r14|all>
#
# 每行把机器可判定的 marker 写入 docs/evidence/2506/R<N>-*.txt（首行 PROVENANCE）。
# 契约：Accepted Plan `## Evidence Contract`；marker 名与该表逐字一致。
# 退出码：0 = 全部 marker 命中；1 = 任一 marker 缺失/值不符；2 = 基础设施错误。
set -uo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND="$ROOT/backend"
EVID="$ROOT/docs/evidence/2506"
LOG_DIR="$ROOT/tmp"
ROW="${1:-all}"

mkdir -p "$EVID" "$LOG_DIR"

die() { echo "FAIL:$*" >&2; exit 2; }
[ -d "$BACKEND" ] || die "missing backend dir: $BACKEND"

COMMIT="$(git -C "$ROOT" rev-parse HEAD 2>/dev/null || echo unknown)"
SHORT="$(git -C "$ROOT" rev-parse --short=12 HEAD 2>/dev/null || echo unknown)"
DIRTY="false"
if [ -n "$(git -C "$ROOT" status --porcelain 2>/dev/null | grep -v '^?? docs/evidence/2506/' | head -1)" ]; then
  DIRTY="true"
fi

provenance() { # artifact
  printf '# build=1.0.%s # commit=%s # dirty=%s\n' "$SHORT" "$COMMIT" "$DIRTY"
}

# emit_markers <log> <keys-regex>：把 `key=value` marker 行抽取为独立行（去重排序）。
emit_markers() {
  local log=$1 keys=$2
  grep -E ": ($keys)=" "$log" 2>/dev/null \
    | sed -E 's/^.*: //' \
    | sort -u
}

# assert_markers <artifact> <line>...
assert_markers() {
  local artifact=$1; shift
  local missing=0
  for want in "$@"; do
    if ! grep -Fxq -- "$want" "$artifact"; then
      echo "MISSING_MARKER: $want ($artifact)" >&2
      missing=1
    fi
  done
  return $missing
}

go_row() { # row artifact test-regex keys m1 m2 ...
  local row=$1 artifact=$2 pattern=$3 keys=$4; shift 4
  local log="$LOG_DIR/verify-2505-s1-${row}.log"
  ( cd "$BACKEND" && go test ./handlers -run "$pattern" -count=1 -v ) >"$log" 2>&1
  local rc=$?
  {
    provenance "$artifact"
    printf '# row=%s command=go test ./handlers -run %s -count=1 -v\n' "$row" "$pattern"
    emit_markers "$log" "$keys"
  } >"$EVID/$artifact"
  if [ "$rc" != "0" ]; then
    echo "FAIL:${row^^}_TEST_EXIT=$rc (see $log)" >&2
    tail -20 "$log" >&2
    return 1
  fi
  if ! assert_markers "$EVID/$artifact" "$@"; then
    echo "FAIL:${row^^}_MARKERS" >&2
    return 1
  fi
  echo "PASS:$row -> docs/evidence/2506/$artifact"
}

KEYS_ROLE='CASE_ADMIN_TO_MEMBER|CASE_MERCHANT_TO_REPAIR|CASE_PRESERVE_CUSTOMER|CASE_DEDUP_ORDER|PRESERVE_CUSTOMER|ORDER_STABLE|WITHOUT_OK|PUT_ROLES|STALE_SITE_ADMIN|STALE_MERCHANT_STAFF|DROPPED_REPAIR|CUSTOMER_REMOVED|UNMANAGED_KEPT|GET_FAIL_ROLE_ERRORS|PUT_SKIPPED_ON_GET_FAIL|PUT_FAIL_ROLE_ERRORS|SILENT_200|PUT_CALLS|INACTIVE_DROPPED|CUSTOMER_KEPT|PUT_NO_INACTIVE|INVALID_ROLE_400|ROLE_ERRORS|SITE_ORG_NE_TENANT|USERS_ORG_IS_TENANT|PUT_ORG|OPERATOR_ORG|LEGACY_TARGET|NEW_TARGET|LEGACY_STALE_SITE_ADMIN|NEW_STALE_SITE_ADMIN|RED_GREEN_FALSIFIABLE|LEGACY_ASSIGN_CALLS|SET_ROLES_CALLS|RED_GREEN|LOCAL_ROLE'

row_r1() {
  go_row r1 R1-target-set.txt '^TestFunctionalRoleTargetSet_2505$' "$KEYS_ROLE" \
    'CASE_ADMIN_TO_MEMBER=["site_member"]' \
    'CASE_MERCHANT_TO_REPAIR=["repair_technician"]' \
    'PRESERVE_CUSTOMER=1' \
    'ORDER_STABLE=1'
}

row_r2() {
  local artifact=R2-iam-contract.txt log="$LOG_DIR/verify-2505-s1-r2.log"
  ( cd "$BACKEND" && go test ./services -run '^TestFunctionalRolesContract_2505$' -count=1 -v ) >"$log" 2>&1
  local rc=$?
  {
    provenance "$artifact"
    printf '# row=r2 command=go test ./services -run ^TestFunctionalRolesContract_2505$ -count=1 -v\n'
    emit_markers "$log" 'GET_PATH|GET_QUERY|PUT_PATH|PUT_BODY|PERM_VERSION_CALLS|FORBIDDEN_ERR|NOTFOUND_ERR|BADREQUEST_ERR|GET_ERROR_ERR|TEMPLATE_INACTIVE_PARSED|EMPTY_ORG_REJECTED'
  } >"$EVID/$artifact"
  if [ "$rc" != "0" ]; then
    echo "FAIL:R2_TEST_EXIT=$rc (see $log)" >&2
    tail -20 "$log" >&2
    return 1
  fi
  assert_markers "$EVID/$artifact" \
    'GET_QUERY=org_id=ORG' \
    'PUT_PATH=/api/v1/organizations/ORG/users/USER/functional-roles' \
    'PUT_BODY={"roles":["site_member"]}' \
    'FORBIDDEN_ERR=1' || { echo "FAIL:R2_MARKERS" >&2; return 1; }
  echo "PASS:r2 -> docs/evidence/2506/$artifact"
}

row_r3() {
  go_row r3 R3-demote-admin.txt '^TestUpdateUserDemoteSiteAdmin_2505$' "$KEYS_ROLE" \
    'PUT_ROLES=["site_member"]' \
    'STALE_SITE_ADMIN=0' \
    'RED_GREEN=1'
}

row_r4() {
  go_row r4 R4-merchant-to-repair.txt '^TestUpdateUserMerchantStaffToRepair_2505$' "$KEYS_ROLE" \
    'PUT_ROLES=["repair_technician"]' \
    'STALE_MERCHANT_STAFF=0'
}

row_r5() {
  go_row r5 R5-dual-role.txt '^TestUpdateUserDualRolePreserved_2505$' "$KEYS_ROLE" \
    'PUT_ROLES=["site_admin","repair_technician"]' \
    'DROPPED_REPAIR=0'
}

row_r6() {
  go_row r6 R6-unmanaged-preserve.txt '^TestUpdateUserPreservesUnmanagedRole_2505$' "$KEYS_ROLE" \
    'CUSTOMER_REMOVED=0' \
    'UNMANAGED_KEPT=1'
}

row_r7() {
  local artifact=R7-failure-visible.txt log="$LOG_DIR/verify-2505-s1-r7.log"
  ( cd "$BACKEND" && go test ./handlers -run '^TestUpdateUserRoleSync(Get|Put)Failure_2505$' -count=1 -v ) >"$log" 2>&1
  local rc=$?
  {
    provenance "$artifact"
    printf '# row=r7 command=go test ./handlers -run ^TestUpdateUserRoleSync(Get|Put)Failure_2505$ -count=1 -v\n'
    emit_markers "$log" "$KEYS_ROLE"
  } >"$EVID/$artifact"
  if [ "$rc" != "0" ]; then
    echo "FAIL:R7_TEST_EXIT=$rc (see $log)" >&2
    tail -20 "$log" >&2
    return 1
  fi
  assert_markers "$EVID/$artifact" \
    'GET_FAIL_ROLE_ERRORS=1' \
    'PUT_SKIPPED_ON_GET_FAIL=1' \
    'PUT_FAIL_ROLE_ERRORS=1' \
    'SILENT_200=0' || { echo "FAIL:R7_MARKERS" >&2; return 1; }
  echo "PASS:r7 -> docs/evidence/2506/$artifact"
}

row_r8() {
  local artifact=R8-build.txt log="$LOG_DIR/verify-2505-s1-r8.log"
  : >"$log"
  ( cd "$BACKEND" && go build ./... ) >>"$log" 2>&1
  local build_rc=$?
  ( cd "$BACKEND" && go test ./handlers -run '2505|2506|2482|2461|2475|2375|2462|2466' -count=1 -timeout 15m ) >>"$log" 2>&1
  local test_rc=$?
  local panics
  panics=$(grep -ciE 'panic:|runtime error' "$log" 2>/dev/null || true)
  panics=${panics:-0}
  local fails=0
  [ "$test_rc" != "0" ] && fails=1
  {
    provenance "$artifact"
    printf '# row=r8 command=go build ./... + go test ./handlers -run "2505|2506|2482|2461|2475|2375|2462|2466"\n'
    printf 'BUILD_EXIT=%s\n' "$build_rc"
    printf 'TESTS_EXIT=%s\n' "$test_rc"
    printf 'TESTS_FAIL=%s\n' "$fails"
    printf 'PANIC=%s\n' "$panics"
    grep -E '^(ok|FAIL|---)' "$log" | tail -20
  } >"$EVID/$artifact"
  if [ "$build_rc" != "0" ] || [ "$test_rc" != "0" ] || [ "$panics" != "0" ]; then
    echo "FAIL:R8_BUILD_EXIT=$build_rc TESTS_EXIT=$test_rc PANIC=$panics (see $log)" >&2
    tail -30 "$log" >&2
    return 1
  fi
  assert_markers "$EVID/$artifact" 'BUILD_EXIT=0' 'TESTS_FAIL=0' 'PANIC=0' \
    || { echo "FAIL:R8_MARKERS" >&2; return 1; }
  echo "PASS:r8 -> docs/evidence/2506/$artifact"
}

row_r9() {
  go_row r9 R9-legacy-assign.txt '^TestUpdateUserLegacyAssignRetired_2505$' "$KEYS_ROLE" \
    'LEGACY_ASSIGN_CALLS=0'
}

# R10：对端（beaconiam）授权例外契约。在临时 worktree 上跑对端单测，不改对端工作树。
row_r10() {
  local artifact=R10-beaconiam-authz.txt log="$LOG_DIR/verify-2505-s1-r10.log"
  local repo="${BEACONIAM_REPO:-$HOME/beaconiam}"
  if [ ! -d "$repo/.git" ]; then
    echo "SKIP:R10 beaconiam repo not found ($repo)" >&2
    return 2
  fi
  local ref
  ref="$(git -C "$repo" rev-parse origin/main 2>/dev/null)" || die "R10: cannot resolve $repo origin/main"
  local wt="$LOG_DIR/beaconiam-2505-authz-wt"
  rm -rf "$wt"
  git -C "$repo" worktree add --detach "$wt" "$ref" >"$log" 2>&1 || {
    echo "FAIL:R10_WORKTREE (see $log)" >&2; return 2; }
  ( cd "$wt" && go test ./internal/handler -run 'TestUpdateUserFunctionalRoles_' -count=1 -v ) >>"$log" 2>&1
  local rc=$?
  git -C "$repo" worktree remove --force "$wt" >/dev/null 2>&1 || true

  local parent_ok=0 sibling=0 grandparent=0 inactive=0 crossns=0
  grep -qE '^--- PASS: TestUpdateUserFunctionalRoles_ParentOrgOwnerAllowed' "$log" && \
    grep -qE '^--- PASS: TestUpdateUserFunctionalRoles_ParentOrgAdminAllowed' "$log" && parent_ok=1
  grep -qE '^--- PASS: TestUpdateUserFunctionalRoles_SiblingDenied' "$log" && sibling=1
  grep -qE '^--- PASS: TestUpdateUserFunctionalRoles_GrandparentDenied' "$log" && grandparent=1
  grep -qE '^--- PASS: TestUpdateUserFunctionalRoles_InactiveParentRelationDenied' "$log" && inactive=1
  grep -qE '^--- PASS: TestUpdateUserFunctionalRoles_CrossNamespaceParentDenied' "$log" && crossns=1

  {
    provenance "$artifact"
    printf '# row=r10 command=git -C %s worktree add --detach + go test ./internal/handler -run TestUpdateUserFunctionalRoles_\n' "$repo"
    printf '# beaconiam_ref=%s\n' "$ref"
    printf 'AUTHZ_PARENT_CHILD_OK=%s\n' "$parent_ok"
    printf 'AUTHZ_SIBLING_DENIED=%s\n' "$sibling"
    printf 'AUTHZ_GRANDPARENT_DENIED=%s\n' "$grandparent"
    printf 'AUTHZ_INACTIVE_DENIED=%s\n' "$inactive"
    printf 'AUTHZ_CROSS_NAMESPACE_DENIED=%s\n' "$crossns"
    grep -E '^(--- (PASS|FAIL): TestUpdateUserFunctionalRoles_)' "$log" | sort
  } >"$EVID/$artifact"

  if [ "$rc" != "0" ]; then
    echo "FAIL:R10_TEST_EXIT=$rc (see $log)" >&2
    return 1
  fi
  assert_markers "$EVID/$artifact" \
    'AUTHZ_PARENT_CHILD_OK=1' \
    'AUTHZ_SIBLING_DENIED=1' \
    'AUTHZ_GRANDPARENT_DENIED=1' \
    'AUTHZ_INACTIVE_DENIED=1' || { echo "FAIL:R10_MARKERS" >&2; return 1; }
  echo "PASS:r10 -> docs/evidence/2506/$artifact"
}

row_r11() {
  go_row r11 R11-inactive-filter.txt '^TestUpdateUserInactiveTemplateFiltered_2505$' "$KEYS_ROLE" \
    'INACTIVE_DROPPED=1' \
    'CUSTOMER_KEPT=1'
}

row_r12() {
  go_row r12 R12-fixture-fidelity.txt '^TestUpdateUserFixtureFidelity_2505$' "$KEYS_ROLE" \
    'SITE_ORG_NE_TENANT=1' \
    'PUT_ORG=site_org'
}

row_r13() {
  go_row r13 R13-local-read-failure.txt '^TestUpdateUserLocalRolesReadFailure_2505$' "$KEYS_ROLE" \
    'PUT_CALLS=0' \
    'SILENT_200=0'
}

row_r14() {
  go_row r14 R14-red-green.txt '^TestUpdateUserRedGreenFalsifiable_2505$' "$KEYS_ROLE" \
    'LEGACY_STALE_SITE_ADMIN=1' \
    'NEW_STALE_SITE_ADMIN=0' \
    'RED_GREEN_FALSIFIABLE=1'
}

run_row() { # row
  case "$1" in
    r1) row_r1 ;; r2) row_r2 ;; r3) row_r3 ;; r4) row_r4 ;; r5) row_r5 ;;
    r6) row_r6 ;; r7) row_r7 ;; r8) row_r8 ;; r9) row_r9 ;; r10) row_r10 ;;
    r11) row_r11 ;; r12) row_r12 ;; r13) row_r13 ;; r14) row_r14 ;;
    *) die "unknown row: $1" ;;
  esac
}

if [ "$ROW" = "all" ]; then
  rc=0
  for r in r1 r2 r3 r4 r5 r6 r7 r8 r9 r10 r11 r12 r13 r14; do
    run_row "$r" || rc=1
  done
  exit "$rc"
fi
run_row "$ROW"
