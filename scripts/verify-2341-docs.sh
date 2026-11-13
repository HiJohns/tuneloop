#!/usr/bin/env bash
# R5: zh/en docs exist with the four required sections.
set -euo pipefail
echo "PROVENANCE build=test-db-lifecycle commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no)" && echo false || echo true)"
check() { local f=$1; shift; test -f "$f" || { echo "missing $f"; exit 1; }; for k in "$@"; do grep -qi "$k" "$f" || { echo "$f missing section: $k"; exit 1; }; done; }
check docs/ops/test-db-lifecycle.md "初始化顺序" "fail-fast" "清理" "排障"
check docs/en/ops/test-db-lifecycle.md "Initialization order" "fail-fast" "Cleanup" "Troubleshooting"
echo "R5 DOCS PASS"
