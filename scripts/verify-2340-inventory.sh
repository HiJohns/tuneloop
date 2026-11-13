#!/usr/bin/env bash
# verify-2340-inventory.sh — R1 of Issue #2340 Evidence Contract.
#
# Regenerates the PRE-FIX `go build ./...` failure inventory deterministically:
# builds a throwaway worktree at HEAD^ (the state before the build-tag fix),
# runs `go build ./...` there, and captures the raw compiler output.
# Exit 0 requires the inventory to prove the multi-main conflict existed.
#
# PROVENANCE: stamps the artifact with the CURRENT HEAD (fix commit) and
# dirty state at generation time; the build itself runs on HEAD^ content.
set -euo pipefail

ISSUE_NUM="${ISSUE_NUM:-2340}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVIDENCE_DIR="$REPO_ROOT/docs/evidence/$ISSUE_NUM"
mkdir -p "$REPO_ROOT/tmp" "$EVIDENCE_DIR"
ARTIFACT="$EVIDENCE_DIR/R1-inventory.txt"

COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain)" ]; then DIRTY="true"; else DIRTY="false"; fi

# Locate the ORIGINAL pre-fix state: the first (oldest) commit that introduced
# the //go:build ignore tag into clean_instruments.go — a file that was
# untagged until the fix. Pickaxe over that single file avoids false anchors
# from migrate_photo_batches.go, which carried the tag long before the fix.
TAG_COMMIT="$(git log --format=%H -S '//go:build ignore' -- backend/scripts/clean_instruments.go | tail -1)"
if [ -n "$TAG_COMMIT" ] && git rev-parse --verify --quiet "$TAG_COMMIT^" >/dev/null; then
    PRE_FIX_SHA="$TAG_COMMIT^"
elif git rev-parse --verify --quiet HEAD^ >/dev/null; then
    PRE_FIX_SHA="HEAD^"
else
    echo "ERROR: cannot locate a pre-fix commit." >&2
    exit 1
fi

TMP_DIR="$(mktemp -d "$REPO_ROOT/tmp/inventory-2340.XXXXXX")"
WORKTREE="$TMP_DIR/wt"
LOG="$TMP_DIR/build.log"
cleanup() { git worktree remove --force "$WORKTREE" >/dev/null 2>&1 || true; }
trap cleanup EXIT

git worktree add --detach "$WORKTREE" "$PRE_FIX_SHA" >/dev/null
( cd "$WORKTREE/backend" && go build ./... ) >"$LOG" 2>&1 || true

{
    echo "PROVENANCE build=backend-scripts-multimain commit=$COMMIT dirty=$DIRTY"
    echo "# Pre-fix state: worktree at $PRE_FIX_SHA; command: go build ./..."
    echo "# (R2 verifies the post-fix build is green; this file proves the conflict existed.)"
    cat "$LOG"
} >"$ARTIFACT"

# Assertions: the pre-fix build must have failed with main redeclared in the
# scripts package.
REDECLARED=$(grep -c "main redeclared" "$ARTIFACT" || true)
if [ "$REDECLARED" -lt 1 ]; then
    echo "FAIL: pre-fix build log has no 'main redeclared' (hits=$REDECLARED)" >&2
    exit 1
fi
grep -q "backend/scripts" "$ARTIFACT" || {
    echo "FAIL: inventory does not mention backend/scripts" >&2
    exit 1
}
echo "PASS: pre-fix inventory captured (main redeclared hits=$REDECLARED) -> $ARTIFACT"
