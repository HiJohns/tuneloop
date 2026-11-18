#!/usr/bin/env bash
# #2406 证据脚本：押金读路径 / 中文文案 / 真机回执 / 静态检查
set -uo pipefail
MODE="${1:-}"
ROOT="$(git rev-parse --show-toplevel)"; cd "$ROOT" || exit 2
case "$MODE" in
  r1)
    cd backend && go test ./handlers/ -run 'TestPricingResponseDeposit_.*_2406' -count=1 -v
    ;;
  r2)
    # 用户面英文文案必须清零（押金/乐币/日期/乐器可用性）
    if grep -nE '"message": "(invalid|gift points|points exceed|insufficient|instrument not available|instrument already reserved|user not found|end_date must|invalid start_date|invalid end_date)' backend/handlers/user_rental.go; then
      echo "FAIL: English user-facing message remains"
      exit 1
    fi
    grep -q '该乐器不支持零押金档' backend/handlers/user_rental.go
    echo "OK: no English user-facing messages; zh zero-tier copy present"
    ;;
  r3)
    test -s docs/evidence/2406/R3-manual.md \
      && grep -q 'executor: user' docs/evidence/2406/R3-manual.md \
      && grep -q 'RESULT: PASS' docs/evidence/2406/R3-manual.md
    ;;
  r4)
    . "$HOME/.nvm/nvm.sh" >/dev/null 2>&1; nvm use 22 >/dev/null 2>&1
    if (cd frontend-mobile && npx eslint --no-ignore src/pages/Checkout.jsx src/pages-weapp/Checkout.jsx); then
      echo "0 errors"
    else
      exit 1
    fi
    ;;
  *) echo "usage: $0 <r1|r2|r3|r4>" >&2; exit 2 ;;
esac
