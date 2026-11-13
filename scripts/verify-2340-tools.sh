#!/usr/bin/env bash
# verify-2340-tools.sh — R3 of Issue #2340 Evidence Contract.
#
# Proves the build-tag fix did not break the standalone tools:
#   1. `go build .` at backend root still passes (main build path).
#   2. Each tool in backend/scripts still compiles when named explicitly
#      (`go build -o <tmp>` <file.go>), which is how they are invoked.
#   3. `go vet ./scripts/...` stays clean (package patterns skip ignored files).
# Exit 0 only when every check passes.
set -euo pipefail

ISSUE_NUM="${ISSUE_NUM:-2340}"
REPO_ROOT="$(git rev-parse --show-toplevel)"
EVIDENCE_DIR="$REPO_ROOT/docs/evidence/$ISSUE_NUM"
mkdir -p "$EVIDENCE_DIR"
ARTIFACT="$EVIDENCE_DIR/R3-tools.txt"

COMMIT="$(git rev-parse HEAD)"
if [ -n "$(git status --porcelain)" ]; then DIRTY="true"; else DIRTY="false"; fi

TOOLS=(
    clean_instruments.go
    fix_dirty_migration.go
    fix_migration.go
    init_test_data.go
    migrate_media_keys.go
    migrate_photo_batches.go
)

OUT="$(mktemp "$REPO_ROOT/tmp/tools-2340.XXXXXX")"
FAILED=0

{
    echo "PROVENANCE build=backend-scripts-multimain commit=$COMMIT dirty=$DIRTY"
    echo "# Command: go build . ; per-tool go build -o <tmp> <file.go> ; go vet ./scripts/..."
} >"$ARTIFACT"

run_logged() {
    if "$@" >>"$ARTIFACT" 2>&1; then
        echo "OK: $*" >>"$ARTIFACT"
    else
        echo "FAIL: $*" >>"$ARTIFACT"
        FAILED=1
    fi
}

# 1. Main build path must stay green.
( cd "$REPO_ROOT/backend" && go build . )
echo "OK: go build ." >>"$ARTIFACT"

# 2. Every tool compiles when named explicitly (ignores its build tag).
for tool in "${TOOLS[@]}"; do
    run_logged go build -o "$OUT" "$REPO_ROOT/backend/scripts/$tool"
done

# 3. vet smoke on the scripts package tree.
( cd "$REPO_ROOT/backend" && go vet ./scripts/... )
echo "OK: go vet ./scripts/..." >>"$ARTIFACT"

if [ "$FAILED" -ne 0 ]; then
    echo "FAIL: tool verification failed; see $ARTIFACT" >&2
    exit 1
fi
echo "PASS: all tools compile and main build is green -> $ARTIFACT"
