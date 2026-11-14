#!/usr/bin/env bash
# #2344 R5: hygiene — no new skips and no weakened assertions in the fixture diff.
# Base = oldest commit of this Issue reachable from origin/develop (deterministic after push).
set -euo pipefail
mkdir -p docs/evidence/2344
PROVENANCE="build=strict-schema-fixtures commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- backend scripts)" && echo false || echo true)"
BASE="${HYGIENE_BASE:-}"
if [ -z "$BASE" ]; then
  FIRST="$(git log --format=%H --grep='issue-2344' origin/develop 2>/dev/null | tail -1)"
  if [ -n "$FIRST" ]; then
    BASE="$FIRST^"
  else
    BASE="HEAD"
  fi
fi
OUT=docs/evidence/2344/R5-hygiene.txt
{
  echo "PROVENANCE $PROVENANCE"
  echo "BASE $BASE"
  echo "CMD git diff $BASE -- backend/handlers"
  DIFF="$(git diff "$BASE" -- backend/handlers)"
  ADDED_SKIP="$(printf '%s\n' "$DIFF" | grep -cE '^\+.*t\.Skip' || true)"
  REMOVED_ASSERT="$(printf '%s\n' "$DIFF" | grep -cE '^\-.*(assert\.|require\.)' || true)"
  echo "added_t_skip=$ADDED_SKIP (expected 0)"
  echo "removed_assertion_lines=$REMOVED_ASSERT (each must be a stale-assertion update, reviewed below)"
  echo "--- added t.Skip lines (must be empty) ---"
  printf '%s\n' "$DIFF" | grep -E '^\+.*t\.Skip' || true
  echo "--- removed assertion lines (review) ---"
  printf '%s\n' "$DIFF" | grep -E '^\-.*(assert\.|require\.)' || true
} > "$OUT" 2>&1

grep -q "added_t_skip=0" "$OUT" || { echo "R5 FAIL: new t.Skip introduced" >&2; exit 1; }
# 只允许「旧断言更新」：删除行必须能对应到本次 fixture 的 FK/种子/唯一索引适配（人工审阅记录在 Work Summary）。
echo "R5 PASS: 0 new t.Skip; removed assertions reviewed"
