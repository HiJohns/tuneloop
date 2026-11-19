#!/usr/bin/env bash
# #2375 Rev.3：角色分配/变更 + 非阻断告警 + 角色列。
# 用法：bash scripts/verify-2375.sh <r1-role-errors|r2-role-update|r3-forbidden|r4-list-role|r5-pending|r6-iam-params|r7-role-column>
set -uo pipefail
MODE="${1:-}"
case "$MODE" in
  r1-role-errors) ART="docs/evidence/2375/R1-role-errors.txt"; TEST='^TestCreateUserRoleErrors_2375$' ;;
  r2-role-update) ART="docs/evidence/2375/R2-role-update.txt"; TEST='^TestUpdateUserRole_2375$' ;;
  r3-forbidden) ART="docs/evidence/2375/R3-forbidden.txt"; TEST='^TestUpdateUserRoleForbidden_2375$' ;;
  r4-list-role) ART="docs/evidence/2375/R4-list-role.txt"; TEST="" ;;
  r5-pending) ART="docs/evidence/2375/R5-update-pending.txt"; TEST='^TestUpdateUserPendingRoleErrors_2375$' ;;
  r6-iam-params) ART="docs/evidence/2375/R6-iam-mapping.txt"; TEST='^TestUpdateUserRoleIAMPayload_2375$' ;;
  r7-role-column) ART="docs/evidence/2375/R7-role-column.md"; TEST="" ;;
  *) echo "usage: $0 <r1-role-errors|r2-role-update|r3-forbidden|r4-list-role|r5-pending|r6-iam-params|r7-role-column>" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2375}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"
BUILD="1.0.$(git rev-parse --short HEAD)"; COMMIT="$(git rev-parse HEAD)"; DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2375/' || true)" ]; then DIRTY=true; fi

if [ "$MODE" = "r7-role-column" ]; then
  f="docs/evidence/2375/R7-role-column.md"
  if grep -q 'executor: user' "$f" && grep -q 'RESULT: PASS' "$f"; then echo "MANUAL_OK: $f"; exit 0; fi
  echo "MANUAL_PENDING: $f（Owner 角色列回执未签）"; exit 1
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  case "$MODE" in
    r1-role-errors|r2-role-update|r3-forbidden|r5-pending|r6-iam-params)
      cd "$REPO_ROOT/backend"
      go test ./handlers/ -run "$TEST" -count=1 -v
      ;;
    r4-list-role)
      cd "$REPO_ROOT/frontend-pc"
      npm run build
      ;;
  esac
} 2>&1 | tee "$ART"
exit "${PIPESTATUS[0]}"
