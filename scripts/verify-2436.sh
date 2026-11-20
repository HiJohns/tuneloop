#!/usr/bin/env bash
# #2436：确认订单页合计标签中文 + i18n + 中文排版字距口径。
# 用法：bash scripts/verify-2436.sh <r1-scan|r2-smoke|r3-typography>
# 每个模式把带 provenance 头的输出写入 docs/evidence/2436/ 对应产物。
set -uo pipefail

MODE="${1:-}"
case "$MODE" in
  r1-scan) ART="docs/evidence/2436/R1-scan.txt" ;;
  r2-smoke) ART="docs/evidence/2436/R2-smoke.txt" ;;
  r3-typography) ART="docs/evidence/2436/R3-typography.md" ;;
  *) echo "usage: $0 <r1-scan|r2-smoke|r3-typography>" >&2; exit 2 ;;
esac

REPO_ROOT="$(git rev-parse --show-toplevel)"; cd "$REPO_ROOT"
BUILD="1.0.$(git rev-parse --short HEAD)"
COMMIT="$(git rev-parse HEAD)"
DIRTY=false
if [ -n "$(git status --porcelain --untracked-files=no | grep -v 'docs/evidence/2436/' || true)" ]; then
  DIRTY=true
fi
PROVENANCE="# build=$BUILD commit=$COMMIT dirty=$DIRTY"
mkdir -p "$(dirname "$ART")"

case "$MODE" in
  r1-scan)
    {
      echo "$PROVENANCE"
      echo "## 裸英文扫描"
      if grep -rn "TOTAL PAYABLE" frontend-mobile/src frontend-pc/src 2>/dev/null; then
        echo "SCAN_FAIL: TOTAL PAYABLE 残留"
        exit 1
      fi
      echo "SCAN_OK: 无 TOTAL PAYABLE"
      echo "## t() 调用"
      for f in frontend-mobile/src/pages/Checkout.jsx frontend-mobile/src/pages-weapp/Checkout.jsx; do
        grep -nF "t('应付合计')" "$f" || { echo "T_FAIL: $f"; exit 1; }
      done
      echo "## page-smoke 静态用例"
      ( cd frontend-mobile && npx vitest run --config vitest.config.js scripts/page-smoke/no-uppercase-label-2436.test.jsx )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r2-smoke)
    {
      echo "$PROVENANCE"
      ( cd frontend-mobile && npm run smoke )
    } 2>&1 | tee "$ART"
    exit "${PIPESTATUS[0]}"
    ;;
  r3-typography)
    {
      echo "$PROVENANCE"
      echo "# R3 — 中文合计标签字距口径"
      echo
      echo "- 结论：中文标签不保留英文大写排版字距（uppercase/tracking-widest/letterSpacing）。"
      fail=0
      h5="$(grep -nF "应付合计" frontend-mobile/src/pages/Checkout.jsx || true)"
      if printf '%s' "$h5" | grep -qE "uppercase|tracking-widest"; then
        echo "- FAIL: H5 标签仍含 uppercase/tracking-widest"; fail=1
      else
        echo "- PASS: H5 标签无 uppercase/tracking-widest"
      fi
      weapp="$(grep -nF "应付合计" frontend-mobile/src/pages-weapp/Checkout.jsx || true)"
      if printf '%s' "$weapp" | grep -q "letterSpacing"; then
        echo "- FAIL: weapp 标签仍含 letterSpacing"; fail=1
      else
        echo "- PASS: weapp 标签无 letterSpacing"
      fi
      if grep -rn "TOTAL PAYABLE" frontend-mobile/src frontend-pc/src 2>/dev/null; then
        echo "- FAIL: TOTAL PAYABLE 残留"; fail=1
      else
        echo "- PASS: 无 TOTAL PAYABLE"
      fi
      echo
      echo "## 证据"
      printf '%s\n' "$h5" "$weapp"
      [ "$fail" = "0" ] && echo "TYPOGRAPHY_OK" || echo "TYPOGRAPHY_FAIL"
    } 2>&1 | tee "$ART"
    if grep -q "TYPOGRAPHY_FAIL" "$ART"; then exit 1; fi
    exit 0
    ;;
esac
