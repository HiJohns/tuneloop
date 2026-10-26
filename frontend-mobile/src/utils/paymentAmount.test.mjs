import test from 'node:test'
import assert from 'node:assert/strict'
import { computeDisplayedCashCents } from './paymentAmount.mjs'

test('coupon + gift：显示以优惠码折后金额为准，不再减乐币', () => {
  assert.equal(computeDisplayedCashCents({ amountCents: 10000, couponFinalCents: 1000, giftUsedCents: 3000, hasCoupon: true }), 1000)
})
test('无优惠码：金额减乐币，最低 0', () => {
  assert.equal(computeDisplayedCashCents({ amountCents: 10000, giftUsedCents: 3000, hasCoupon: false }), 7000)
  assert.equal(computeDisplayedCashCents({ amountCents: 1000, giftUsedCents: 3000 }), 0)
})
test('退款显示原金额', () => {
  assert.equal(computeDisplayedCashCents({ amountCents: 5000, couponFinalCents: 100, hasCoupon: true, isRefund: true }), 5000)
})
