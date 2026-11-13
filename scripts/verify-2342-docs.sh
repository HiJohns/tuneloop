#!/usr/bin/env bash
# #2342 R5: the migration-shim contract must be documented in both languages.
# Requires: shim rules, failure handling, and the #2341 origin link.
set -euo pipefail
mkdir -p docs/evidence/2342
PROVENANCE="build=migrations-fresh-install commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- . ':!docs/evidence')" && echo false || echo true)"
ZH="docs/ops/test-db-lifecycle.md"
EN="docs/en/ops/test-db-lifecycle.md"

{
  echo "PROVENANCE $PROVENANCE"
  echo "=== R5: docs contract (zh + en) ==="
} > docs/evidence/2342/R5-docs.txt

fail=0
check() { # file needle label
  if grep -q -- "$2" "$1"; then
    echo "OK   $3: $1 contains '$2'" | tee -a docs/evidence/2342/R5-docs.txt
  else
    echo "MISS $3: $1 lacks '$2'" | tee -a docs/evidence/2342/R5-docs.txt
    fail=1
  fi
}

for f in "$ZH" "$EN"; do
  [ -s "$f" ] || { echo "MISS file: $f missing/empty" | tee -a docs/evidence/2342/R5-docs.txt; fail=1; }
done

# 链接与来源
check "$ZH" "#2341" "origin-link"
check "$EN" "#2341" "origin-link"
check "$ZH" "#2342" "issue-link"
check "$EN" "#2342" "issue-link"

# shim 规则
check "$ZH" "IF NOT EXISTS" "shim-rule"
check "$EN" "IF NOT EXISTS" "shim-rule"
check "$ZH" "to_regclass" "guard-rule"
check "$EN" "to_regclass" "guard-rule"
check "$ZH" "幂等" "idempotent"
check "$EN" "idempotent" "idempotent"

# 失败处理
check "$ZH" "失败处理" "failure-handling"
check "$EN" "Failure handling" "failure-handling"
check "$ZH" "禁止静默跳过" "no-silent-skip"
check "$EN" "no silent skips" "no-silent-skip"

# 旧口径必须消失（模板不再「不含 migrations」）
if grep -q "模板\*\*不应用\*\*" "$ZH" || grep -q "does \*\*not\*\* apply" "$EN"; then
  echo "MISS stale-doc: old 'template does not apply migrations' wording still present" | tee -a docs/evidence/2342/R5-docs.txt
  fail=1
fi

if [ "$fail" -ne 0 ]; then
  echo "RESULT: INCOMPLETE" | tee -a docs/evidence/2342/R5-docs.txt
  echo "R5 FAIL: docs contract incomplete" >&2
  exit 1
fi
echo "RESULT: COMPLETE" | tee -a docs/evidence/2342/R5-docs.txt
echo "R5 PASS: zh+en docs cover shim rules, failure handling and #2341"
