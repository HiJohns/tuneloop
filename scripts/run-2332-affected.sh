#!/usr/bin/env bash
# #2332 R6 (Rev.3): affected packages + build only.
#
# Scope (D15): S3b is a sub-task, so this row runs the affected packages and a
# build. The full handlers gate belongs to S5 (#2334) after #2336 is fixed.
#
# The -run regex below is the exact affected list (S3b tests + gate/tier tests).
# TestSettlementFlow is deliberately excluded: it is a pre-existing baseline
# failure owned by #2339 (Group B), not an S3b regression.
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
AFFECTED='TestPayscore|TestDepositTier|TestSettlementCashVsPayscore|TestRefundNotForPayscore|TestCreditGate'
{
  echo "PROVENANCE $PROVENANCE"
  echo "NOTE TestSettlementFlow excluded: pre-existing baseline failure owned by #2339 (Group B)."
  echo "CMD go build . && go test ./services/ ./database/ -count=1 && go test ./handlers/ -run '$AFFECTED' -count=1"
  ( cd backend && go build . && go test ./services/ ./database/ -count=1 && go test ./handlers/ -run "$AFFECTED" -count=1 )
} 2>&1 | tee docs/evidence/2332/R6-affected.txt
