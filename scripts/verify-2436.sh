#!/usr/bin/env bash
# #2436：确认订单页不得硬编码英文标签，必须经 t()。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in r1-scan|r2-smoke) ;; *) echo "usage: $0 {r1-scan|r2-smoke}" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
echo "# build=1.0.$(git rev-parse --short HEAD) commit=$(git rev-parse HEAD) dirty=$([ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2436/' || true)" ] && echo true || echo false)"
cd frontend-mobile
case "$MODE" in
  r1-scan) npx vitest run --config vitest.config.js scripts/page-smoke/no-uppercase-label-2436.test.jsx ;;
  r2-smoke) npm run smoke ;;
esac
