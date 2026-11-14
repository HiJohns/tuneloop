#!/usr/bin/env bash
# #2336 R9: order-dependence red/green pair.
# RED  = pre-fix full-suite excerpts captured in docs/evidence/2336/R3-handlers.txt.
# GREEN = the same two tests run isolated on current code.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2336}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p "$TMPDIR" "$GOCACHE" docs/evidence/2336 tmp
PROVENANCE="build=handlers-test-infra commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" && echo false || echo true)"
RED=docs/evidence/2336/R3-handlers.txt
OUT=docs/evidence/2336/R9-order-fix.txt
set +e
{
  echo "PROVENANCE $PROVENANCE"
  echo "== RED: pre-fix full-suite excerpts from $RED =="
  for t in TestStaffOrderActions TestUpdateUser_SiteIDNoChange; do
    echo "--- $t (pre-fix, full suite) ---"
    grep -A 8 -- "--- FAIL: $t " "$RED" || echo "(no pre-fix FAIL excerpt found)"
  done
  echo "== GREEN: isolated current code =="
  ( cd backend && go test ./handlers/ -run '^(TestStaffOrderActions|TestUpdateUser_SiteIDNoChange)$' -count=1 -v -timeout 10m )
} > "$OUT" 2>&1
rc=$?
set -e
for t in TestStaffOrderActions TestUpdateUser_SiteIDNoChange; do
  grep -q -- "--- $t (pre-fix, full suite) ---" "$OUT" || { echo "R9 missing RED section for $t" >&2; exit 1; }
  grep -q -- "--- PASS: $t" "$OUT" || { echo "R9 missing GREEN PASS for $t" >&2; exit 1; }
done
[ "$rc" -eq 0 ] || { echo "R9 FAIL: isolated run exit=$rc" >&2; exit 1; }
echo "R9 PASS: red/green pair present for both order-dependent tests"
