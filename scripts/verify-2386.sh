#!/usr/bin/env bash
# #2386 全体成员聚合视图验收脚本。
# 用法：bash scripts/verify-2386.sh <r1-aggregate|r2-scope|r3-platform|build
#        |r5-default-scope|r6-identity-roles|r7-merchant-role|r8-direct-isolation>
set -euo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-aggregate|r2-scope|r3-platform|build|\
  r5-default-scope|r6-identity-roles|r7-merchant-role|r8-direct-isolation) ;;
  *) echo "usage: $0 <r1-aggregate|r2-scope|r3-platform|build|r5-default-scope|r6-identity-roles|r7-merchant-role|r8-direct-isolation>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2386}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"

cd "$REPO_ROOT/backend"
case "$MODE" in
  r1-aggregate)        go test ./handlers/ -run '^TestStaffMembersAggregate_2386$' -count=1 -v ;;
  r2-scope)            go test ./handlers/ -run '^TestStaffMembersScope_2386$' -count=1 -v ;;
  r3-platform)         go test ./handlers/ -run '^TestStaffMembersPlatform_2386$' -count=1 -v ;;
  r5-default-scope)    go test ./handlers/ -run '^TestStaffMembersDefaultScope_2386$' -count=1 -v ;;
  r6-identity-roles)   go test ./handlers/ -run '^TestStaffMembersIdentityRoles_2386$' -count=1 -v ;;
  r7-merchant-role)    go test ./handlers/ -run '^TestStaffMembersMerchantRole_2386$' -count=1 -v ;;
  r8-direct-isolation) go test ./handlers/ -run '^TestStaffMembersDirectIsolation_2386$' -count=1 -v ;;
  build)               go build ./... ;;
esac
