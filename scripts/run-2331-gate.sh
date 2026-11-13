#!/usr/bin/env bash
# run-2331-gate.sh — R2 (and, on rerun, R4) of Issue #2331 Evidence Contract.
# Services-layer deposit gate pure functions. Artifacts:
#   R2 -> docs/evidence/2331/R2-order.txt        (first run)
#   R4 -> docs/evidence/2331/R4-order-rerun.txt  (rerun with ARTIFACT_SUFFIX=rerun)
set -euo pipefail

ISSUE_NUM="${ISSUE_NUM:-2331}"
SUFFIX="${ARTIFACT_SUFFIX:-}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVIDENCE_DIR="$REPO_ROOT/docs/evidence/$ISSUE_NUM"
mkdir -p "$REPO_ROOT/tmp" "$EVIDENCE_DIR"

if [ "$SUFFIX" = "rerun" ]; then
    ARTIFACT="$EVIDENCE_DIR/R4-order-rerun.txt"
else
    ARTIFACT="$EVIDENCE_DIR/R2-order.txt"
fi

COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then DIRTY="true"; else DIRTY="false"; fi

{
    echo "PROVENANCE build=s3a-payscore commit=$COMMIT dirty=$DIRTY"
    echo "# Command: go test ./services/ -run 'TestAssertTierPayable|TestResolveCreditRiskAmount' -count=1 -v"
    cd "$REPO_ROOT/backend"
    go test ./services/ -run 'TestAssertTierPayable|TestResolveCreditRiskAmount' -count=1 -v
} >"$ARTIFACT" 2>&1

grep -q "^--- PASS" "$ARTIFACT" || { echo "FAIL: no PASS subtest in $ARTIFACT" >&2; exit 1; }
if grep -q "^--- FAIL" "$ARTIFACT"; then
    echo "FAIL: failing subtest in $ARTIFACT" >&2
    exit 1
fi
# R4/R2 断言：gate 必须位于 services（handlers 版已删除）。
[ -f "$REPO_ROOT/backend/services/payscore_gate.go" ] || { echo "FAIL: services gate missing" >&2; exit 1; }
[ ! -e "$REPO_ROOT/backend/handlers/payscore_gate.go" ] || { echo "FAIL: handlers gate must not exist" >&2; exit 1; }
echo "PASS: gate tests green (services location) -> $ARTIFACT"
