#!/usr/bin/env bash
# #2348 R1/R2/R3: P12 command-cell consistency (drift detection + normalization).
set -euo pipefail
MODE="${1:-}"
case "$MODE" in
  drift|consistent|full|snapshot) ;;
  *) echo "usage: $0 drift|consistent|full|snapshot" >&2; exit 2 ;;
esac
REPO_ROOT="$(git rev-parse --show-toplevel)"
PRE_READY="${PRE_READY_BIN:-$HOME/.config/opencode/scripts/pre-ready-check.sh}"
[ -f "$PRE_READY" ] || { echo "ERROR: pre-ready-check.sh not found: $PRE_READY" >&2; exit 2; }
mkdir -p "$REPO_ROOT/docs/evidence/2348" "$REPO_ROOT/tmp"
PROVENANCE="build=p12-command-drift commit=$(git -C "$REPO_ROOT" rev-parse HEAD) dirty=$(test -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no -- scripts)" && echo false || echo true)"

# #2348 修复轮：共享配置不在 git，先验证当前主机确实已落盘 P12 改动。
if [ "$MODE" != "snapshot" ]; then
  if ! grep -q "normalize_contract_text" "$PRE_READY" || ! grep -q "command drift" "$PRE_READY"; then
    echo "ERROR: P12 command-drift check missing in $PRE_READY" >&2
    echo "REMEDIATION: bash $REPO_ROOT/scripts/apply-2348-p12.sh  (idempotent, backs up .bak-2348)" >&2
    exit 3
  fi
fi

if [ "$MODE" = "snapshot" ]; then
  OUT="$REPO_ROOT/docs/evidence/2348/R5-config-snapshot.txt"
  TESTS="$HOME/.config/opencode/scripts/tests/test_pre_ready_check.sh"
  {
    echo "PROVENANCE $PROVENANCE"
    echo "TARGET $PRE_READY"
    echo "SHA256 $(sha256sum "$PRE_READY" | awk '{print $1}')"
    echo "MTIME $(date -r "$PRE_READY" -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || stat -c %y "$PRE_READY")"
    echo "--- P12 symbols ---"
    grep -n "normalize_contract_text\|command drift" "$PRE_READY"
    echo "--- self-test fixtures ---"
    grep -n "T73 command whitespace\|T74 substantive command drift" "$TESTS"
    echo "--- syntax check ---"
    bash -n "$PRE_READY" && echo "pre-ready bash -n: OK"
    bash -n "$TESTS" && echo "self-test bash -n: OK"
    echo "--- diff vs backups (durable change snapshot) ---"
    diff -u "$HOME/.config/opencode/scripts/.pre-ready-check.sh.bak-2348" "$PRE_READY" || true
    diff -u "$HOME/.config/opencode/scripts/tests/test_pre_ready_check.sh.bak-2348" "$TESTS" || true
  } > "$OUT"
  echo "$OUT: PASS"
  exit 0
fi

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
