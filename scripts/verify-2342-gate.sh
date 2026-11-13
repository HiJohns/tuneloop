#!/usr/bin/env bash
# #2342 R6: build + services/database regression guard.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2342}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2342
PROVENANCE="build=migrations-fresh-install commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
PGBIN="${PGBIN:-/home/coder/opt/pg/bin}"
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go build . && go test ./services/ ./database/ -count=1"
  ( cd backend && go build . && go test ./services/ ./database/ -count=1 )
} 2>&1 | tee docs/evidence/2342/R6-gate.txt
echo "R6 PASS"
