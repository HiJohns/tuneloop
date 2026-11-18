#!/usr/bin/env bash
# #2386 全体成员聚合视图验收脚本（Rev.2）。
# 用法：bash scripts/verify-2386.sh <r1-aggregate|r2-scope|r3-platform|build
#        |r5-default-scope|r6-identity-roles|r7-merchant-role|r8-direct-isolation>
# 每个模式将带 provenance 头的输出写入 docs/evidence/2386/ 对应产物（计划命令无重定向）。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-aggregate)        ART="docs/evidence/2386/R1-aggregate.txt"; TEST='^TestStaffMembersAggregate_2386$' ;;
  r2-scope)            ART="docs/evidence/2386/R2-scope.txt"; TEST='^TestStaffMembersScope_2386$' ;;
  r3-platform)         ART="docs/evidence/2386/R3-platform.txt"; TEST='^TestStaffMembersPlatform_2386$' ;;
  build)               ART="docs/evidence/2386/R4-build.txt"; TEST="" ;;
  r5-default-scope)    ART="docs/evidence/2386/R5-default-scope.txt"; TEST='^TestStaffMembersDefaultScope_2386$' ;;
  r6-identity-roles)   ART="docs/evidence/2386/R6-identity-roles.txt"; TEST='^TestStaffMembersIdentityRoles_2386$' ;;
  r7-merchant-role)    ART="docs/evidence/2386/R7-merchant-role.txt"; TEST='^TestStaffMembersMerchantRole_2386$' ;;
  r8-direct-isolation) ART="docs/evidence/2386/R8-direct-isolation.txt"; TEST='^TestStaffMembersDirectIsolation_2386$' ;;
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
mkdir -p "$TMPDIR" "$GOCACHE" "$(dirname "$ART")"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
# 生成时 shell 会创建/截断产物；排除 docs/evidence/2386/ 后 dirty 只反映源码变更。
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2386/' || true)" ]; then
  DIRTY=true
fi

{
  echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"
  cd "$REPO_ROOT/backend"
  if [ "$MODE" = "build" ]; then
    go build ./...
  else
    go test ./handlers/ -run "$TEST" -count=1 -v
  fi
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
