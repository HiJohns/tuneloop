#!/usr/bin/env bash
# #2348 R4: negative regression — the real #2343 evidence.json (pre-Rev.2 commands)
# must now be rejected by P12 with a "command drift" detail.
set -euo pipefail
TARGET="${1:-2343}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
PRE_READY="${PRE_READY_BIN:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
[ -f "$PRE_READY" ] || { echo "ERROR: pre-ready-check.sh not found: $PRE_READY" >&2; exit 2; }
mkdir -p "$REPO_ROOT/docs/evidence/2348" "$REPO_ROOT/tmp"
OUT="$REPO_ROOT/docs/evidence/2348/R4-regression-2343.txt"
PROVENANCE="build=p12-command-drift commit=$(git -C "$REPO_ROOT" rev-parse HEAD) dirty=$(test -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no -- scripts)" && echo false || echo true)"

PLAN_FILE="$REPO_ROOT/tmp/2348-plan-${TARGET}.md"
CHANGED_FILE="$REPO_ROOT/tmp/2348-changed-${TARGET}.txt"
SUMMARY_FILE="$REPO_ROOT/tmp/2348-summary-${TARGET}.md"

cd "$REPO_ROOT"
gh issue view "$TARGET" --json comments \
  --jq '[.comments[] | select(.body|startswith("## Accepted Plan"))] | last | .body' > "$PLAN_FILE"
gh issue view "$TARGET" --json comments \
  --jq '[.comments[] | select(.body|startswith("## Work Summary"))] | last | .body' > "$SUMMARY_FILE"
[ -s "$PLAN_FILE" ] || { echo "ERROR: no Accepted Plan on #$TARGET" >&2; exit 2; }
[ -s "$SUMMARY_FILE" ] || { echo "ERROR: no Work Summary on #$TARGET" >&2; exit 2; }
# #2348 修复轮：目标 Issue 的证据可能已被修正（漂移消失）。为保持负例可复现，
# 从目标证据派生一份「篡改副本」（仅改 command 尾部），断言 P12 仍能捕获。
TAMPERED="$REPO_ROOT/tmp/2348-regression-${TARGET}-tampered.json"
jq '(.rows[0].command) = ((.rows[0].command // "true") + " && true")' \
  "$REPO_ROOT/docs/evidence/${TARGET}/evidence.json" > "$TAMPERED"
[ -s "$TAMPERED" ] || { echo "ERROR: cannot build tampered evidence for #$TARGET" >&2; exit 2; }
printf 'docs/evidence/%s/evidence.json\n' "$TARGET" > "$CHANGED_FILE"

set +e
bash "$PRE_READY" "$TARGET" \
  --plan-file "$PLAN_FILE" \
  --summary-file "$SUMMARY_FILE" \
  --changed-files "$CHANGED_FILE" \
  --evidence-file "$TAMPERED" \
  --affected-cmd true \
  --dry-run --allow-local-only \
  --report "$REPO_ROOT/tmp/2348-regression-${TARGET}.json" > "$REPO_ROOT/tmp/2348-regression-${TARGET}.log" 2>&1
RC=$?
set -e

{
  echo "PROVENANCE $PROVENANCE"
  echo "TARGET #$TARGET"
  echo "CMD bash \"$PRE_READY\" $TARGET --plan-file <latest Accepted Plan> --summary-file <latest Work Summary> --dry-run"
  echo "exit=$RC"
  echo "--- P12 line ---"
  grep -E "^P12" "$REPO_ROOT/tmp/2348-regression-${TARGET}.log" || true
} > "$OUT"

# 篡改副本模拟 command drift（真实 #2343 证据已修正，故用派生副本保持负例可复现）。
[ "$RC" -ne 0 ] || { echo "R4 FAIL: gate passed despite command drift" >&2; exit 1; }
grep -q "P12" "$REPO_ROOT/tmp/2348-regression-${TARGET}.log" || { echo "R4 FAIL: P12 did not run" >&2; exit 1; }
grep -q "command drift" "$REPO_ROOT/tmp/2348-regression-${TARGET}.log" || { echo "R4 FAIL: command drift not reported" >&2; exit 1; }
echo "RESULT: FAIL-AS-EXPECTED (real #$TARGET drift detected)" >> "$OUT"
echo "$OUT: PASS"
