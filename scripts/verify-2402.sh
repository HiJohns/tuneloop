#!/usr/bin/env bash
# #2402 R4：非法 site_id 的邀请接口必须 400。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r4-backend) ;; *) echo "usage: $0 r4-backend" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2402}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"
cd "$REPO_ROOT/backend"
go test ./handlers/ -run '^TestSendStaffInviteInvalidSiteID_2402$' -count=1 -v
