#!/usr/bin/env bash
# #2379 technician_profile 商户解析（merchantConfigDB）验收。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-fixed|r2-red|build) ;; *) echo "usage: $0 <r1-fixed|r2-red|build>" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
export PATH="/usr/local/go/bin:${PATH}"
export TMPDIR="${TMPDIR:-$REPO_ROOT/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$REPO_ROOT/tmp/gocache}"
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_verify2379}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
mkdir -p "$TMPDIR" "$GOCACHE"
cd "$REPO_ROOT/backend"
case "$MODE" in
  r1-fixed) go test ./handlers/ -run '^TestTechnicianProfileMerchantScopeFixed_2379$' -count=1 -v ;;
  r2-red)   go test ./handlers/ -run '^TestTechnicianProfileMerchantScopeRed_2379$' -count=1 -v ;;
  build)    go build ./... ;;
esac
