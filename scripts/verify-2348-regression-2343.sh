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
printf 'docs/evidence/%s/evidence.json\n' "$TARGET" > "$CHANGED_FILE"

set +e
bash "$PRE_READY" "$TARGET" \
  --plan-file "$PLAN_FILE" \
  --summary-file "$SUMMARY_FILE" \
  --changed-files "$CHANGED_FILE" \
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

# 现场 #2343 evidence.json 携带旧命令（R2 缺 --no-ignore；R5-R7 含已豁免的截图子句）。
[ "$RC" -ne 0 ] || { echo "R4 FAIL: gate passed despite command drift" >&2; exit 1; }
grep -q "P12" "$REPO_ROOT/tmp/2348-regression-${TARGET}.log" || { echo "R4 FAIL: P12 did not run" >&2; exit 1; }
grep -q "command drift" "$REPO_ROOT/tmp/2348-regression-${TARGET}.log" || { echo "R4 FAIL: command drift not reported" >&2; exit 1; }
echo "RESULT: FAIL-AS-EXPECTED (real #$TARGET drift detected)" >> "$OUT"
echo "$OUT: PASS"
