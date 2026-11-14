#!/usr/bin/env bash
# #2349 R2: strict-schema repair fixtures — targeted 4-test suite.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2349}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2349 tmp
PROVENANCE="build=strict-repair-fixtures commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"

REGEX='^(TestListMerchantRepairServices_SharedTenantNoLeak_2248|TestRepairService_StatusGuardsAndTenantIsolation|TestRepairService_TaskListScopes|TestRepairService_ListMineStatusFilter)$'

{
  echo "PROVENANCE $PROVENANCE"
  echo "CMD (cd backend && go test ./handlers/ -run '$REGEX' -count=1 -v)"
  ( cd backend && go test ./handlers/ -run "$REGEX" -count=1 -v )
} 2>&1 | tee docs/evidence/2349/R2-targeted.txt

grep -q "FAIL" docs/evidence/2349/R2-targeted.txt && { echo "R2 FAIL: failures present" >&2; exit 1; }
PASS_COUNT="$(grep -c '^--- PASS:' docs/evidence/2349/R2-targeted.txt || true)"
[ "$PASS_COUNT" -ge 4 ] || { echo "R2 FAIL: expected >=4 PASS, got $PASS_COUNT" >&2; exit 1; }
echo "R2 PASS: 4/4 targeted"
