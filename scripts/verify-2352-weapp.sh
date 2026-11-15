#!/usr/bin/env bash
# #2352 weapp 端校验：确认页去档位 UI + 提交跳评测页；支付页档位自选 + prepay 带 tier。
#
# 用法：bash scripts/verify-2352-weapp.sh <checkout|payment|dist>
# 产物：docs/evidence/2352/R1w-checkout.txt / R2w-payment.txt / R10-weapp-dist.txt
#
# 背景（2026-11-15 发现）：Rev.2 的改动只落在 H5 共享实现（pages/Checkout.jsx、
# pages/Payment.jsx），weapp 的两份双实现（pages-weapp/Checkout.jsx、
# pages-weapp/payment/Payment.jsx）未跟进 → weapp 真机上确认页仍选档位、提交后
# 直接进支付页，评测页永不出现。本脚本把这条链路的断言固定下来。
set -euo pipefail

MODE="${1:-}"
case "$MODE" in checkout|payment|dist) ;; *) echo "usage: $0 checkout|payment|dist" >&2; exit 2 ;; esac

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"
mkdir -p docs/evidence/2352
PROVENANCE="build=credit-flow-2352-weapp commit=$(git rev-parse HEAD) dirty=$(test -z "$(git status --porcelain --untracked-files=no -- frontend-mobile/src/pages-weapp scripts)" && echo false || echo true)"

CO="frontend-mobile/src/pages-weapp/Checkout.jsx"
PAY="frontend-mobile/src/pages-weapp/payment/Payment.jsx"

case "$MODE" in
  checkout)
    OUT=docs/evidence/2352/R1w-checkout.txt
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep -nE 'credit-assessment|免押金|buildOrderDepositPayload|deposit_tier' $CO"
      grep -nE "credit-assessment|免押金|buildOrderDepositPayload|deposit_tier|担保人" "$CO" || true
    } > "$OUT" 2>&1

    # 正向：单品流与批量流都必须跳评测页
    test "$(grep -c "pages-weapp/credit-assessment/index?order_id=" "$CO")" -ge 2 \
      || { echo "R1w FAIL: weapp checkout must route to credit-assessment on submit (single + cart)" >&2; exit 1; }
    grep -q "pages-weapp/payment/index?type=rent&id=" "$CO" \
      && { echo "R1w FAIL: weapp checkout still redirects straight to payment" >&2; exit 1; }
    # 负例：确认页不得再有档位/免押交互或档位提交（注释提及不算）
    grep -qE "setDepositTierState|depositTierState|setDepositWaived\(" "$CO" \
      && { echo "R1w FAIL: legacy deposit-tier toggle state still present" >&2; exit 1; }
    grep -q "buildOrderDepositPayload" "$CO" && { echo "R1w FAIL: checkout still submits deposit tier" >&2; exit 1; }
    grep -q "body.deposit_tier" "$CO" && { echo "R1w FAIL: batch body still sends deposit_tier" >&2; exit 1; }
    echo "PASS: weapp checkout has no tier UI and routes both flows to credit-assessment"
    ;;

  payment)
    OUT=docs/evidence/2352/R2w-payment.txt
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep -nE 'buildTierOptions|deposit_tier|guarantor_ids|buildAssessmentRoute|credit-assessment' $PAY"
      grep -nE "buildTierOptions|deposit_tier|guarantor_ids|buildAssessmentRoute|credit-assessment|selectedGuarantorIds" "$PAY" || true
    } > "$OUT" 2>&1

    grep -q "buildTierOptions" "$PAY"      || { echo "R2w FAIL: payment page has no tier options" >&2; exit 1; }
    grep -q "deposit_tier" "$PAY"          || { echo "R2w FAIL: prepay body lacks deposit_tier" >&2; exit 1; }
    grep -q "guarantor_ids" "$PAY"         || { echo "R2w FAIL: prepay body lacks guarantor_ids" >&2; exit 1; }
    grep -q "selectedGuarantorIds.length < 2" "$PAY" \
      || { echo "R2w FAIL: reduced tier does not require 2 guarantors" >&2; exit 1; }
    # 负例：评测入口不得再拼旧参数（页面已只读 order_id）
    grep -q "risk_amount_cents=" "$PAY" \
      && { echo "R2w FAIL: stale risk_amount_cents param still passed to assessment page" >&2; exit 1; }
    echo "PASS: weapp payment offers full/reduced tiers, requires 2 guarantors, sends tier on prepay"
    ;;

  dist)
    OUT=docs/evidence/2352/R10-weapp-dist.txt
    A=frontend-mobile/dist-weapp
    test -d "$A" || { echo "R10 FAIL: dist-weapp missing (run build:weapp first)" >&2; exit 1; }
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep credit-assessment + 免押金 in $A/pages-weapp/{checkout,payment}/index.js"
      echo "--- checkout bundle: assessment route hits ---"
      grep -o "pages-weapp/credit-assessment/index?order_id=" "$A/pages-weapp/checkout/index.js" | wc -l || true
      echo "--- checkout bundle: legacy zero-deposit toggle hits ---"
      grep -o "免押金" "$A/pages-weapp/checkout/index.js" | wc -l || true
      echo "--- payment bundle: deposit_tier hits ---"
      grep -o "deposit_tier" "$A/pages-weapp/payment/index.js" | wc -l || true
    } > "$OUT" 2>&1

    grep -q "pages-weapp/credit-assessment/index?order_id=" "$A/pages-weapp/checkout/index.js" \
      || { echo "R10 FAIL: built weapp checkout bundle has no assessment route" >&2; exit 1; }
    # 陷阱：文案在产物里可能是 unicode 转义，用 \u 形式再查一次
    if grep -q "免押金" "$A/pages-weapp/checkout/index.js" || grep -q 'u514d\\u62bc\\u91d1' "$A/pages-weapp/checkout/index.js"; then
      echo "R10 FAIL: built weapp checkout bundle still contains 免押金 toggle copy" >&2
      exit 1
    fi
    grep -q "deposit_tier" "$A/pages-weapp/payment/index.js" \
      || { echo "R10 FAIL: built weapp payment bundle does not send deposit_tier" >&2; exit 1; }
    echo "PASS: weapp bundles route to assessment and carry tier choice"
    ;;

esac
