#!/usr/bin/env bash
# #2332 R2: prepay/pay credit gate wiring.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_local}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p "$TMPDIR" "$GOCACHE" docs/evidence/2332
PROVENANCE="build=payscore-s3b-rev3 commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD go test ./handlers/ -run 'TestPrepayGate|TestPayGate|TestCreditGate' -count=1 -v"
  ( cd backend && go test ./handlers/ -run 'TestPrepayGate|TestPayGate|TestCreditGate' -count=1 -v )
} 2>&1 | tee docs/evidence/2332/R2-gate.txt
