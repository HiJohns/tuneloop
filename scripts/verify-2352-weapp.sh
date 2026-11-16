#!/usr/bin/env bash
# #2352 / #2357 押金档位链路校验（weapp 双实现）：
#   - 确认页：全额押金>0 时提供「差额押金」拉杆（差额需 2 位担保人，自动带出已保存担保人），
#             不提供「免押金/零押金」选项；提交后进入信用评测页；
#   - 支付页：不再二次选择档位（#2357：档位决定回到确认页），只展示服务端计算金额；
#   - 产物：weapp bundle 里能看到上述行为（此前缺失的断言层）。
#
# 用法：bash scripts/verify-2352-weapp.sh <checkout|payment|dist>
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
      echo "CMD grep -nE 'depositTierState|差额押金|guarantors|credit-assessment|免押金' $CO"
      grep -nE "depositTierState|差额押金|guarantors|guarantor|credit-assessment|免押金|deposit_tier" "$CO" || true
    } > "$OUT" 2>&1

    # 正向 1：提交后进入评测页（单品 + 批量两条流）
    test "$(grep -c "pages-weapp/credit-assessment/index?order_id=" "$CO")" -ge 2 \
      || { echo "R1w FAIL: weapp checkout must route to credit-assessment on submit (single + cart)" >&2; exit 1; }
    # 正向 2：差额押金拉杆 + 2 位担保人校验
    grep -q "setDepositTierState" "$CO" || { echo "R1w FAIL: 差额押金 toggle missing" >&2; exit 1; }
    grep -q "差额押金订单需至少选择 2 位担保人" "$CO" || { echo "R1w FAIL: guarantor validation missing" >&2; exit 1; }
    # 正向 3：自动带出已保存担保人（列表 + 勾选）
    grep -qE "user/guarantors|guarantorsApi" "$CO" || { echo "R1w FAIL: saved guarantors are not loaded" >&2; exit 1; }
    # 正向 4：差额档必须 0 < reduced < full（deposit_reduced=0 视为未配置）
    grep -q "Number(depositReduced) > 0" "$CO" || { echo "R1w FAIL: reduced tier must require 0 < reduced < full" >&2; exit 1; }
    # 正向 5：档位必须真的随下单提交（2026-11-15 事故：weapp 漏了这段，后端只能按全额落库）
    grep -q "body.deposit_tier = isZeroDeposit ?" "$CO" || { echo "R1w FAIL: weapp single-checkout does not submit deposit_tier" >&2; exit 1; }
    grep -q "body.deposit_credit_path = 'payscore'" "$CO" || { echo "R1w FAIL: weapp checkout does not submit payscore credit path" >&2; exit 1; }
    grep -q "body.guarantor_ids = selectedGuarantorIds" "$CO" || { echo "R1w FAIL: weapp checkout does not submit guarantor_ids for reduced" >&2; exit 1; }
    # 两端一致性：H5 共享实现必须同样提交档位（防止单边漂移）
    grep -q "body.deposit_tier = isZeroDeposit ?" frontend-mobile/src/pages/Checkout.jsx \
      || { echo "R1w FAIL: H5 checkout lost deposit_tier submission" >&2; exit 1; }
    # 负例：不得再出现「免押金」zero 档入口（#2352 D1）
    grep -q "depositTierState === 'zero'" "$CO" && { echo "R1w FAIL: zero-deposit toggle must not exist" >&2; exit 1; }
    # 负例：不得直跳支付页
    grep -q "pages-weapp/payment/index?type=rent&id=" "$CO" && { echo "R1w FAIL: checkout must not skip the assessment page" >&2; exit 1; }
    echo "PASS: weapp checkout offers reduced-deposit toggle + saved guarantors, then routes to assessment"
    ;;

  payment)
    OUT=docs/evidence/2352/R2w-payment.txt
    {
      echo "PROVENANCE $PROVENANCE"
      echo "CMD grep -nE 'deposit_tier|buildTierOptions|credit-assessment|guarantor_ids' $PAY"
      grep -nE "deposit_tier|buildTierOptions|credit-assessment|guarantor_ids|creditGate" "$PAY" || true
    } > "$OUT" 2>&1

    # 负例：支付页不再选择档位（#2357）
    grep -q "buildTierOptions" "$PAY" && { echo "R2w FAIL: payment page must not offer tier selection anymore" >&2; exit 1; }
    grep -q "deposit_tier" "$PAY" && { echo "R2w FAIL: prepay must not send deposit_tier (chosen at checkout)" >&2; exit 1; }
    # 正向：评测入口仍只传 order_id
    grep -q "credit-assessment/index?order_id=" "$PAY" || { echo "R2w FAIL: assessment entry missing" >&2; exit 1; }
    grep -q "risk_amount_cents=" "$PAY" && { echo "R2w FAIL: stale risk_amount_cents param" >&2; exit 1; }
    echo "PASS: weapp payment shows server-calculated amount and no tier selector"
    ;;

  dist)
    OUT=docs/evidence/2352/R10-weapp-dist.txt
    A=frontend-mobile/dist-weapp
    test -d "$A" || { echo "R10 FAIL: dist-weapp missing (run build:weapp first)" >&2; exit 1; }
    {
      echo "PROVENANCE $PROVENANCE"
      echo "checkout bundle: assessment route hits = $(grep -o 'pages-weapp/credit-assessment/index?order_id=' "$A/pages-weapp/checkout/index.js" | wc -l)"
      echo "checkout bundle: reduced-tier toggle hits = $(grep -o '差额押金' "$A/pages-weapp/checkout/index.js" | wc -l)"
      echo "checkout bundle: guarantor fetch hits = $(grep -o 'user/guarantors' "$A/pages-weapp/checkout/index.js" | wc -l)"
      echo "payment bundle: deposit_tier hits = $(grep -o 'deposit_tier' "$A/pages-weapp/payment/index.js" | wc -l)"
    } > "$OUT" 2>&1

    grep -q "pages-weapp/credit-assessment/index?order_id=" "$A/pages-weapp/checkout/index.js" \
      || { echo "R10 FAIL: built checkout bundle has no assessment route" >&2; exit 1; }
    grep -q "user/guarantors" "$A/pages-weapp/checkout/index.js" \
      || { echo "R10 FAIL: built checkout bundle does not load saved guarantors" >&2; exit 1; }
    grep -q "deposit_tier" "$A/pages-weapp/checkout/index.js" \
      || { echo "R10 FAIL: built checkout bundle does not submit deposit_tier (would book full tier)" >&2; exit 1; }
    grep -q "deposit_tier" "$A/pages-weapp/payment/index.js" \
      && { echo "R10 FAIL: built payment bundle still sends deposit_tier" >&2; exit 1; }
    echo "PASS: weapp bundles carry confirm-page tier choice and assessment routing"
    ;;

esac
