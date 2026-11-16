#!/usr/bin/env bash
# #2352 Rev.2 R1/R2：确认页去档位/信用 UI + 提交后跳评测页。
set -euo pipefail
MODE="${1:-}"
case "$MODE" in checkout|route) ;; *) echo "usage: $0 checkout|route" >&2; exit 2 ;; esac
REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT/frontend-mobile"
export NVM_DIR="$HOME/.nvm"
# shellcheck disable=SC1091
. "$NVM_DIR/nvm.sh" >/dev/null 2>&1
nvm use 22 >/dev/null 2>&1
PROVENANCE="build=credit-flow-2352 commit=$(git -C "$REPO_ROOT" rev-parse HEAD) dirty=$(test -z "$(git -C "$REPO_ROOT" status --porcelain --untracked-files=no -- frontend-mobile backend scripts)" && echo false || echo true)"

if [ "$MODE" = checkout ]; then
  OUT="$REPO_ROOT/docs/evidence/2352/R1-checkout-ui.txt"
  {
    echo "PROVENANCE $PROVENANCE"
    echo "CMD static UI assertions on src/pages/Checkout.jsx (#2352 Rev.2 D3/D5)"
    check() { grep -q -- "$1" src/pages/Checkout.jsx || { echo "FAIL: missing $1"; exit 1; }; }
    forbid() { ! grep -q -- "$1" src/pages/Checkout.jsx || { echo "FAIL: legacy control present: $1"; exit 1; }; }
    check "押金"
    forbid "押金档位"
    forbid "担保人信息"
    forbid "buildTierOptions"
    forbid "tierOptions"
    forbid "上传签名推荐信"
    forbid "下载推荐信模板"
    forbid "depositWaived &&"
    echo "PASS: checkout renders rent + full deposit only (no tier/credit/guarantor UI)"
  } > "$OUT" 2>&1
  tail -1 "$OUT"
else
  OUT="$REPO_ROOT/docs/evidence/2352/R2-route.txt"
  {
    echo "PROVENANCE $PROVENANCE"
    echo "CMD route assertions + creditFlow unit"
    grep -n "credit-assessment?order_id=" src/pages/Checkout.jsx
    npx vitest run scripts/page-smoke/credit-flow-tiers.test.jsx 2>&1 | tail -6
  } > "$OUT" 2>&1
  grep -q "credit-assessment?order_id=" "$OUT" || { echo "R2 FAIL: no assessment route" >&2; exit 1; }
  grep -q "Tests  4 passed" "$OUT" || { echo "R2 FAIL: unit tests" >&2; exit 1; }
  echo "PASS: submit navigates to credit-assessment with order_id"
fi
