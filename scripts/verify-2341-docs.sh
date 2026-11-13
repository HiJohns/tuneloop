#!/usr/bin/env bash
# R5: zh/en docs exist with the four required sections.
set -euo pipefail
echo "PROVENANCE build=test-db-lifecycle commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no)" && echo false || echo true)"
for f in docs/ops/test-db-lifecycle.md docs/en/ops/test-db-lifecycle.md; do
  test -f "$f" || { echo "missing $f"; exit 1; }
  for k in "初始化顺序" "fail-fast" "清理" "排障"; do
    grep -q "$k" "$f" || { echo "$f missing section: $k"; exit 1; }
  done
done
echo "R5 DOCS PASS"
