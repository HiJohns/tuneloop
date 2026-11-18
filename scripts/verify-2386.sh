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
  build|r4-build)      ART="docs/evidence/2386/R4-build.txt"; TEST="" ;;
  r5-default-scope)    ART="docs/evidence/2386/R5-default-scope.txt"; TEST='^TestStaffMembersDefaultScope_2386$' ;;
  r6-identity-roles)   ART="docs/evidence/2386/R6-identity-roles.txt"; TEST='^TestStaffMembersIdentityRoles_2386$' ;;
  r7-merchant-role)    ART="docs/evidence/2386/R7-merchant-role.txt"; TEST='^TestStaffMembersMerchantRole_2386$' ;;
  r8-direct-isolation) ART="docs/evidence/2386/R8-direct-isolation.txt"; TEST='^TestStaffMembersDirectIsolation_2386$' ;;
  *) echo "usage: $0 <r1-aggregate|r2-scope|r3-platform|build|r5-default-scope|r6-identity-roles|r7-merchant-role|r8-direct-isolation>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  export PATH="/usr/local/go/bin:${PATH}"
fi
export TMPDIR="$REPO_ROOT/tmp/gobuild"
mkdir -p "$TMPDIR" "$(dirname "$ART")"

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
  case "$MODE" in
    build|r4-build)
      go build ./...
      ;;
    *)
      go test ./handlers/ -run "$TEST" -count=1 -v
      ;;
  esac
} 2>&1 | tee "$ART"

exit "${PIPESTATUS[0]}"
