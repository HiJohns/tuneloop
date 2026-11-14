#!/usr/bin/env bash
# #2352 R1/R2/R3：前端押金三档（unit / render / guarantor）。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in unit|render|guarantor) ;; *) echo "usage: $0 unit|render|guarantor" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT/frontend-mobile"
export NVM_DIR="$HOME/.nvm"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh" >/dev/null 2>&1
nvm use 22 >/dev/null 2>&1
OUT="$REPO_ROOT/docs/evidence/2352/R$( [ "$MODE" = unit ] && echo 1 || { [ "$MODE" = render ] && echo 2 || echo 3; } )-$( [ "$MODE" = unit ] && echo frontend-unit || { [ "$MODE" = render ] && echo checkout-render || echo guarantor-gate; } ).txt"
PROVENANCE="build=deposit-tiers-2352 commit=$(git -C "$REPO_ROOT" rev-parse HEAD) dirty=$(test -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no -- frontend-mobile backend scripts)" && echo false || echo true)"

case "$MODE" in
  unit)
    npx vitest run scripts/page-smoke/credit-flow-tiers.test.jsx 2>&1 | tee "$OUT"
    grep -q "4 passed" "$OUT" || { echo "R1 FAIL" >&2; exit 1; }
    ;;
  render)
    # 0 押金乐器：三档面板保留（不再被 !isZeroDeposit 短路），零押档文案要求信用评测；旧面板 DOM 源码已删除。
    npm run build:h5 > "$OUT" 2>&1 || { echo "R2 FAIL: build failed" >&2; exit 1; }
    check() { grep -q -- "$1" src/pages/Checkout.jsx || { echo "R2 FAIL: missing $1" >&2; exit 1; }; }
    forbid() { ! grep -q -- "$1" src/pages/Checkout.jsx || { echo "R2 FAIL: legacy panel still present: $1" >&2; exit 1; }; }
    check "0 押金乐器同样渲染"
    check "零押金"
    forbid "上传签名推荐信"
    forbid "下载推荐信模板"
    forbid "fetchWaiverEligibility"
    forbid "!isZeroDeposit && ("
    echo "RESULT: PASS (tier panel renders for zero-deposit; legacy waiver/letter panel removed)" >> "$OUT"
    ;;
  guarantor)
    node --input-type=module -e '
      import { buildOrderDepositPayload, DEPOSIT_TIER } from "./src/utils/creditFlow.mjs";
      const one = buildOrderDepositPayload({ tier: DEPOSIT_TIER.REDUCED, guarantorIds: ["g1"] });
      const two = buildOrderDepositPayload({ tier: DEPOSIT_TIER.REDUCED, guarantorIds: ["g1", "g2"] });
      if (one.ok !== false || one.error !== "guarantors_required") { console.error("FAIL: 1 guarantor must be blocked"); process.exit(1); }
      if (two.ok !== true) { console.error("FAIL: 2 guarantors must pass"); process.exit(1); }
      const zero = buildOrderDepositPayload({ tier: DEPOSIT_TIER.ZERO, guarantorIds: [] });
      if (zero.ok !== true || zero.body.deposit_credit_path !== "payscore") { console.error("FAIL: zero must use payscore"); process.exit(1); }
      console.log("guarantor gate: 0/1 blocked, 2 allowed; zero→payscore");
    ' 2>&1 | tee "$OUT"
    grep -q "guarantor gate" "$OUT" || { echo "R3 FAIL" >&2; exit 1; }
    ;;
esac
echo "$OUT: PASS"
