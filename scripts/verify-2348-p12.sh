#!/usr/bin/env bash
# #2348 R1/R2/R3: P12 command-cell consistency (drift detection + normalization).
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  drift|consistent|full) ;;
  *) echo "usage: $0 drift|consistent|full" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"
PRE_READY="${PRE_READY_BIN:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
[ -f "$PRE_READY" ] || { echo "ERROR: pre-ready-check.sh not found: $PRE_READY" >&2; exit 2; }
mkdir -p "$REPO_ROOT/docs/evidence/2348" "$REPO_ROOT/tmp"
PROVENANCE="build=p12-command-drift commit=$(git -C "$REPO_ROOT" rev-parse HEAD) dirty=$(test -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no -- scripts)" && echo false || echo true)"

case "$MODE" in
  drift) OUT="docs/evidence/2348/R1-drift-fail.txt" ;;
  consistent) OUT="docs/evidence/2348/R2-consistent-pass.txt" ;;
  full) OUT="docs/evidence/2348/R3-selftest.txt" ;;
esac

set +e
bash "$PRE_READY" --self-test > "$REPO_ROOT/tmp/2348-selftest.log" 2>&1
SELF_RC=$?
set -e

{
  echo "PROVENANCE $PROVENANCE"
  echo "MODE $MODE"
  echo "CMD bash \"$PRE_READY\" --self-test"
  echo "exit=$SELF_RC"
  echo "--- summary ---"
  grep -E "^PASS=|^FAIL=" "$REPO_ROOT/tmp/2348-selftest.log" || true
  echo "--- #2348 fixtures ---"
  grep -E "T73|T74|command drift" "$REPO_ROOT/tmp/2348-selftest.log" || true
} > "$REPO_ROOT/$OUT"

case "$MODE" in
  drift)
    grep -q "PASS: T74 substantive command drift fails P12" "$REPO_ROOT/tmp/2348-selftest.log" \
      || { echo "R1 FAIL: drift fixture did not report command drift" >&2; exit 1; }
    echo "RESULT: FAIL-AS-EXPECTED (command drift detected)" >> "$REPO_ROOT/$OUT"
    ;;
  consistent)
    grep -q "PASS: T73 command whitespace normalization passes P12" "$REPO_ROOT/tmp/2348-selftest.log" \
      || { echo "R2 FAIL: whitespace-only difference was reported as drift" >&2; exit 1; }
    echo "RESULT: NO-FALSE-POSITIVE" >> "$REPO_ROOT/$OUT"
    ;;
  full)
    [ "$SELF_RC" -eq 0 ] || { echo "R3 FAIL: self-test exited $SELF_RC" >&2; exit 1; }
    grep -qE "FAIL=0($| )" "$REPO_ROOT/tmp/2348-selftest.log" \
      || { echo "R3 FAIL: self-test has failures" >&2; exit 1; }
    echo "RESULT: SELFTEST-OK" >> "$REPO_ROOT/$OUT"
    ;;
esac
echo "$OUT: PASS"
