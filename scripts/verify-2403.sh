#!/usr/bin/env bash
# #2403 InviteToMerchant 商户查找（merchantConfigDB + 可见组织授权）验收脚本。
# 用法：bash scripts/verify-2403.sh {r1-invite|r2-isolation|r3-dedup|build}
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  r1-invite|r2-isolation|r3-dedup|build) ;;
  *) echo "usage: $0 <r1-invite|r2-isolation|r3-dedup|build>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

if ! command -v go >/dev/null 2>&1; then
  export PATH="/usr/local/go/bin:${PATH}"
fi
# 构建临时目录固定在仓库 tmp/（系统 /tmp 可能是容量受限的 tmpfs）
mkdir -p "$REPO_ROOT/tmp/gobuild"
export TMPDIR="$REPO_ROOT/tmp/gobuild"
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"

cd "$REPO_ROOT/backend"
case "$MODE" in
  r1-invite)    go test ./handlers/ -run '^TestInviteToMerchantMerchantScopeFixed_2403$' -count=1 -v ;;
  r2-isolation) go test ./handlers/ -run '^TestInviteToMerchantCrossMerchant404_2403$' -count=1 -v ;;
  r3-dedup)     go test ./handlers/ -run '^TestInviteToMerchantDedupAndKind_2403$' -count=1 -v ;;
  build)        go build ./... ;;
esac
