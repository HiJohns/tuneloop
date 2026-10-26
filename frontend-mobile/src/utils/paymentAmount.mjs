// #2291：支付页显示金额（分）。优惠码与乐币互斥：
// 有优惠码时以服务端折后金额为准，不再减乐币（prepay 也传 gift_used=0）。
export function computeDisplayedCashCents({ amountCents, couponFinalCents, giftUsedCents = 0, hasCoupon = false, isRefund = false }) {
  const amount = Number(amountCents) || 0
  if (isRefund) return amount
  if (hasCoupon) return Number(couponFinalCents) || 0
  return Math.max(0, amount - (Number(giftUsedCents) || 0))
}
