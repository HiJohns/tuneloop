#!/usr/bin/env bash
# run-2331-provider.sh — R1 of Issue #2331 Evidence Contract.
# Provider tri-state: default mock / enabled-without-key explicit error /
# enabled-with-credentials real client. Regenerates docs/evidence/2331/R1-provider.txt.
set -euo pipefail

ISSUE_NUM="${ISSUE_NUM:-2331}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVIDENCE_DIR="$REPO_ROOT/docs/evidence/$ISSUE_NUM"
mkdir -p "$REPO_ROOT/tmp" "$EVIDENCE_DIR"
ARTIFACT="$EVIDENCE_DIR/R1-provider.txt"

COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then DIRTY="true"; else DIRTY="false"; fi

{
    echo "PROVENANCE build=s3a-payscore commit=$COMMIT dirty=$DIRTY"
    echo "# Command: go test ./services/wechatpay/ -run 'TestNewPayscoreProvider' -count=1 -v"
    cd "$REPO_ROOT/backend"
    go test ./services/wechatpay/ -run 'TestNewPayscoreProvider' -count=1 -v
} >"$ARTIFACT" 2>&1

grep -q "^--- PASS" "$ARTIFACT" || { echo "FAIL: no PASS subtest in $ARTIFACT" >&2; exit 1; }
if grep -q "^--- FAIL" "$ARTIFACT"; then
    echo "FAIL: failing subtest in $ARTIFACT" >&2
    exit 1
fi
echo "PASS: provider tri-state tests green -> $ARTIFACT"
