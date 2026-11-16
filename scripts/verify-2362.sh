#!/usr/bin/env bash
# #2362 Rev.2 支付端乐币额度与足额收款验收脚本。
# 用法：bash scripts/verify-2362.sh <r1-snapshot|r2-cap|r3-over-limit|r4-insufficient>
set -euo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-snapshot|r2-cap|r3-over-limit|r4-insufficient) ;;
  *) echo "usage: $0 <r1-snapshot|r2-cap|r3-over-limit|r4-insufficient>" >&2; exit 2 ;;
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
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2362}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"

case "$MODE" in
  r1-snapshot)   TEST='TestPointsRatioSnapshot_2362' ;;
  r2-cap)        TEST='TestPointsCapParity_2362' ;;
  r3-over-limit) TEST='TestPointsUsageLimits_2362' ;;
  r4-insufficient) TEST='TestInsufficientPointsPayment_2362' ;;
esac

cd "$REPO_ROOT/backend"
go test ./handlers/ -run "^${TEST}$" -count=1 -v
