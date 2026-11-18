#!/usr/bin/env bash
# #2389 无邮箱重设密码验收：后端显式 400。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r3-no-email) ;; *) echo "usage: $0 r3-no-email" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2389}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"

BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
# 证据产物在生成时被重定向创建/截断；排除 docs/evidence/2389/ 后 dirty 只反映源码变更。
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2389/' || true)" ]; then
  DIRTY=true
fi
echo "# build=$BUILD commit=$COMMIT dirty=$DIRTY"

cd "$REPO_ROOT/backend"
go test ./handlers/ -run '^TestResetPasswordNoEmail_2389$' -count=1 -v
