#!/usr/bin/env bash
# #2411 证据脚本：return_path 解码 / 真机回执 / 静态检查
set -uo pipefail
MODE="${1:-}"
ROOT="$(git rev-parse --show-toplevel)"; cd "$ROOT" || exit 2
case "$MODE" in
  r1)
    . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1; nvm use 22 >/dev/null 2>&1
    cd frontend-mobile && npx vitest run scripts/page-smoke/credit-return-path-2411.test.jsx
    ;;
  r2)
    test -s docs/evidence/2411/R2-manual.md \
      && grep -q 'executor: user' docs/evidence/2411/R2-manual.md \
      && grep -q 'RESULT: PASS' docs/evidence/2411/R2-manual.md
    ;;
  r3)
    . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1; nvm use 22 >/dev/null 2>&1
    cd frontend-mobile && npx eslint --no-ignore src/pages/CreditAssessment.jsx src/utils/creditFlow.mjs src/pages-weapp/payment/Payment.jsx
    ;;
  *) echo "usage: $0 <r1|r2|r3>" >&2; exit 2 ;;
esac
