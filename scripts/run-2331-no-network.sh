#!/usr/bin/env bash
# run-2331-no-network.sh — R3 of Issue #2331 Evidence Contract.
# PAYSCORE_ENABLED=false + counting HTTP doer -> zero outbound requests.
# Regenerates docs/evidence/2331/R3-no-network.txt.
set -euo pipefail

ISSUE_NUM="${ISSUE_NUM:-2331}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVIDENCE_DIR="$REPO_ROOT/docs/evidence/$ISSUE_NUM"
mkdir -p "$REPO_ROOT/tmp" "$EVIDENCE_DIR"
ARTIFACT="$EVIDENCE_DIR/R3-no-network.txt"

COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain --untracked-files=no)" ]; then DIRTY="true"; else DIRTY="false"; fi

{
    echo "PROVENANCE build=s3a-payscore commit=$COMMIT dirty=$DIRTY"
    echo "# Command: go test ./services/wechatpay/ -run 'TestProviderNoNetworkWhenDisabled' -count=1 -v"
    cd "$REPO_ROOT/backend"
    go test ./services/wechatpay/ -run 'TestProviderNoNetworkWhenDisabled' -count=1 -v
} >"$ARTIFACT" 2>&1

grep -q "^--- PASS" "$ARTIFACT" || { echo "FAIL: no-network test did not pass" >&2; exit 1; }
if grep -q "^--- FAIL" "$ARTIFACT"; then
    echo "FAIL: failing subtest" >&2
    exit 1
fi
grep -q "^--- PASS: TestProviderNoNetworkWhenDisabled" "$ARTIFACT" || {
    echo "FAIL: zero-network proof test did not run" >&2
    exit 1
}
echo "PASS: zero-network test green -> $ARTIFACT"
