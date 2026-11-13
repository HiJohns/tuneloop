#!/usr/bin/env bash
# #2349 R3: hygiene — no new skips and no weakened assertions in this Issue's diff.
set -euo pipefail
mkdir -p docs/evidence/2349
PROVENANCE="build=strict-repair-fixtures commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
BASE="${HYGIENE_BASE:-}"
if [ -z "$BASE" ]; then
  FIRST="$(git log --format=%H --grep='issue-2349' origin/develop 2>/dev/null | tail -1)"
  if [ -n "$FIRST" ]; then
    BASE="$FIRST^"
  else
    BASE="HEAD"
  fi
fi
OUT=docs/evidence/2349/R3-hygiene.txt
{
  echo "PROVENANCE $PROVENANCE"
  echo "BASE $BASE"
  echo "CMD git diff $BASE -- backend/handlers"
  DIFF="$(git diff "$BASE" -- backend/handlers)"
  ADDED_SKIP="$(printf '%s\n' "$DIFF" | grep -cE '^\+.*t\.Skip' || true)"
  REMOVED_ASSERT="$(printf '%s\n' "$DIFF" | grep -cE '^\-.*(assert\.|require\.)' || true)"
  echo "added_t_skip=$ADDED_SKIP (expected 0)"
  echo "removed_assertion_lines=$REMOVED_ASSERT (expect 0: fixtures only add seeds/codes)"
  echo "--- added t.Skip lines (must be empty) ---"
  printf '%s\n' "$DIFF" | grep -E '^\+.*t\.Skip' || true
  echo "--- removed assertion lines ---"
  printf '%s\n' "$DIFF" | grep -E '^\-.*(assert\.|require\.)' || true
} > "$OUT" 2>&1

grep -q "added_t_skip=0" "$OUT" || { echo "R3 FAIL: new t.Skip introduced" >&2; exit 1; }
grep -q "removed_assertion_lines=0" "$OUT" || { echo "R3 FAIL: assertions were removed" >&2; exit 1; }
echo "R3 PASS: 0 new t.Skip, 0 removed assertions"
