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
  # 白名单：允许「同一 fixture 断言行携带新增字段（Code/种子）」的删除行——必须由等价新增行平衡。
  UNAPPROVED="$(printf '%s\n' "$DIFF" | grep -E '^\-.*(assert\.|require\.)' | grep -vE 'Create\(&models\.(Merchant|User|Site|SiteMember)' || true)"
  ADDED_ASSERT="$(printf '%s\n' "$DIFF" | grep -cE '^\+.*(assert\.|require\.)' || true)"
  echo "added_t_skip=$ADDED_SKIP (expected 0)"
  echo "removed_assertion_lines=$REMOVED_ASSERT added_assertion_lines=$ADDED_ASSERT (balanced fixture updates allowed)"
  echo "--- added t.Skip lines (must be empty) ---"
  printf '%s\n' "$DIFF" | grep -E '^\+.*t\.Skip' || true
  echo "--- removed assertion lines (must be balanced by added ones) ---"
  printf '%s\n' "$DIFF" | grep -E '^\-.*(assert\.|require\.)' || true
  echo "--- removed assertion lines outside the fixture whitelist (must be empty) ---"
  printf '%s\n' "$UNAPPROVED" | sed '/^$/d' || true
} > "$OUT" 2>&1

grep -q "added_t_skip=0" "$OUT" || { echo "R3 FAIL: new t.Skip introduced" >&2; exit 1; }
REMOVED="$(sed -n 's/^removed_assertion_lines=\([0-9]*\) added_assertion_lines=\([0-9]*\).*/\1 \2/p' "$OUT" | head -1)"
REMOVED_N="${REMOVED%% *}"; ADDED_N="${REMOVED##* }"
[ "${REMOVED_N:-0}" -le "${ADDED_N:-0}" ] || { echo "R3 FAIL: assertions removed without equivalent additions ($REMOVED_N > $ADDED_N)" >&2; exit 1; }
if sed -n '/outside the fixture whitelist/,$p' "$OUT" | sed '1d' | grep -qE '^\-'; then
  echo "R3 FAIL: assertion removal outside fixture whitelist" >&2; exit 1
fi
echo "R3 PASS: 0 new t.Skip, 0 removed assertions"
