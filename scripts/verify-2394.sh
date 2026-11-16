#!/usr/bin/env bash
# #2394 成员移除审批流证据脚本。
# 逐条对应 Accepted Plan 的 Evidence Contract；从仓库根执行：
#   bash scripts/verify-2394.sh r1-request > docs/evidence/2394/R1-request.txt 2>&1
set -uo pipefail

cd "$(dirname "$0")/.." || exit 2

if ! command -v go >/dev/null 2>&1; then
  # 构建机常规安装路径（非 PATH 环境兜底）
  export PATH="/usr/local/go/bin:${PATH}"
fi

# 仓库内 tmp/ 作为构建临时目录（系统 /tmp 可能是容量受限的 tmpfs）
mkdir -p "${PWD}/tmp"
export TMPDIR="${PWD}/tmp"
# 迁移 SQL 为多语句，pgx 需 simple protocol
export TUNELOOP_DB_SIMPLE_PROTOCOL="${TUNELOOP_DB_SIMPLE_PROTOCOL:-1}"

mode="${1:-}"

run_test() {
  local name="$1"
  ( cd backend && go test ./handlers/ -run "^${name}$" -count=1 -v )
}

case "$mode" in
  r1-request)
    run_test 'TestMemberRemovalCreateRequest_2394'
    ;;
  r2-approve)
    run_test 'TestMemberRemovalApprove_2394'
    ;;
  r3-guard)
    run_test 'TestSiteRemoveRequiresApproval_2394'
    ;;
  r4-migration)
    run_test 'TestMemberRemovalMigration_2394' || exit 1
    ( cd backend && go build ./... ) || exit 1
    ;;
  *)
    echo "usage: $0 {r1-request|r2-approve|r3-guard|r4-migration}" >&2
    exit 2
    ;;
esac
