#!/usr/bin/env bash
# #2375 角色分配/变更验收。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-role-errors|r2-role-update|r3-forbidden) ;; *) echo "usage: $0 <r1-role-errors|r2-role-update|r3-forbidden>" >&2; exit 2 ;; esac
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
mkdir -p "$TMPDIR" "$GOCACHE"
cd "$REPO_ROOT/backend"
case "$MODE" in
  r1-role-errors) go test ./handlers/ -run '^TestCreateUserRoleErrors_2375$' -count=1 -v ;;
  r2-role-update) go test ./handlers/ -run '^TestUpdateUserRole_2375$' -count=1 -v ;;
  r3-forbidden)   go test ./handlers/ -run '^TestUpdateUserRoleForbidden_2375$' -count=1 -v ;;
esac
