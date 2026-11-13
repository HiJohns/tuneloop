#!/usr/bin/env bash
# verify-2331-remote.sh — R5 of Issue #2331 Evidence Contract.
# P13 traceability: the services gate (+ its test) exists on origin/develop and
# the deleted handlers copy is gone from the remote tree.
set -euo pipefail

ISSUE_NUM="${ISSUE_NUM:-2331}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVIDENCE_DIR="$REPO_ROOT/docs/evidence/$ISSUE_NUM"
mkdir -p "$REPO_ROOT/tmp" "$EVIDENCE_DIR"
ARTIFACT="$EVIDENCE_DIR/R5-remote.txt"

COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then DIRTY="true"; else DIRTY="false"; fi

git fetch origin --prune >/dev/null 2>&1 || true

SERVICES_GATE=$(git ls-tree -r --name-only origin/develop -- backend/services/payscore_gate.go | grep -c 'payscore_gate' || true)
SERVICES_TEST=$(git ls-tree -r --name-only origin/develop -- backend/services/payscore_gate_test.go | grep -c 'payscore_gate' || true)
HANDLERS_GATE=$(git ls-tree -r --name-only origin/develop -- backend/handlers/ | grep -c 'payscore_gate' || true)

{
    echo "PROVENANCE build=s3a-payscore commit=$COMMIT dirty=$DIRTY"
    echo "# Command: git fetch origin; git ls-tree -r --name-only origin/develop -- <gate paths>"
    echo "services_gate_on_remote=$SERVICES_GATE"
    echo "services_gate_test_on_remote=$SERVICES_TEST"
    echo "handlers_gate_on_remote=$HANDLERS_GATE"
} >"$ARTIFACT"

if [ "$SERVICES_GATE" -lt 1 ] || [ "$SERVICES_TEST" -lt 1 ]; then
    echo "FAIL: services gate/test missing on origin/develop" >&2
    exit 1
fi
if [ "$HANDLERS_GATE" -ne 0 ]; then
    echo "FAIL: handlers gate still exists on origin/develop" >&2
    exit 1
fi
echo "PASS: remote traceability proven -> $ARTIFACT"
