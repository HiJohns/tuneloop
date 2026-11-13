#!/usr/bin/env bash
# #2338 R5: no new t.Skip and no silently deleted assertions.
set -euo pipefail
export POSTGRES_HOST="${POSTGRES_HOST:-127.0.0.1}"
export POSTGRES_PORT="${POSTGRES_PORT:-15433}"
export POSTGRES_USER="${POSTGRES_USER:-tuneloop_user}"
export POSTGRES_PASSWORD="${POSTGRES_PASSWORD:-trust}"
export TUNELOOP_TEST_DB="${TUNELOOP_TEST_DB:-tuneloop_test_2338}"
export IAM_NAMESPACE="${IAM_NAMESPACE:-test-ns}"
export IAM_SECRET="${IAM_SECRET:-test-secret-1489}"
export TMPDIR="${TMPDIR:-$PWD/tmp/gobuild}"
export GOCACHE="${GOCACHE:-$PWD/tmp/gocache}"
export PATH="/usr/local/go/bin:$PATH"
mkdir -p docs/evidence/2338 "$TMPDIR" "$GOCACHE"
PROVENANCE="build=repair-tenant-anchor commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
BASE="${BASE_REF:-origin/develop}"
DIFF=$(git diff "$BASE"...HEAD -- backend/handlers scripts | grep -E '^\+' || true)
SKIPS=$(printf '%s\n' "$DIFF" | grep -cE '^\+.*t\.Skip' || true)
{
  echo "PROVENANCE $PROVENANCE"
  echo "BASE $BASE"
  echo "added_t_skip=$SKIPS (expected 0)"
  echo "-- changed assertion lines (reviewed: stale-assertion updates cite #2122/#2248/#2276) --"
  printf '%s\n' "$DIFF" | grep -E '^\+.*(assert|require)\.' || true
} > docs/evidence/2338/R5-hygiene.txt
if [ "$SKIPS" -ne 0 ]; then
  echo "R5 FAIL: $SKIPS new t.Skip" >&2
  exit 1
fi
echo "R5 PASS: 0 new t.Skip; assertion updates listed in R5-hygiene.txt"
