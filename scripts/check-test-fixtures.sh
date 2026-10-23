#!/usr/bin/env bash
# #2271: 禁止 testfixtures 包外的局部 Drop/Create 表（测试顺序耦合源头）。
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT/backend"
MATCHES=$(grep -rln "Migrator()\.\(DropTable\|CreateTable\)" handlers/*_test.go 2>/dev/null | xargs -r grep -L "fixture-guard:allow" || true)
if [ -n "$MATCHES" ]; then
  echo "ERROR: 发现局部 Drop/Create（请改用 testfixtures.SetupTestDB / ResetTestDB）：" >&2
  echo "$MATCHES" >&2
  exit 1
fi
echo "OK: 无局部 Drop/Create"
