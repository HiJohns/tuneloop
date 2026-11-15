import test from 'node:test'
import assert from 'node:assert/strict'

import { pendingOrderHint, PENDING_HINT_FALLBACK } from './pendingOrder.mjs'

test('无截止时间 → 通用提示（不误报倒计时）', () => {
  assert.equal(pendingOrderHint(null), PENDING_HINT_FALLBACK)
  assert.equal(pendingOrderHint(undefined), PENDING_HINT_FALLBACK)
  assert.equal(pendingOrderHint('not-a-date'), PENDING_HINT_FALLBACK)
})

test('剩余时间 → 显示截止时刻与剩余分钟', () => {
  const now = new Date('2026-11-15T12:26:30Z').getTime()
  const hint = pendingOrderHint('2026-11-15T12:36:30Z', now)
  assert.match(hint, /约 10 分钟/)
  assert.match(hint, /前完成信用评测与支付/)
})

test('已过截止时间 → 提示可能已自动取消', () => {
  const now = new Date('2026-11-15T12:40:00Z').getTime()
  assert.match(pendingOrderHint('2026-11-15T12:36:30Z', now), /已超过支付截止时间/)
})

test('不足 1 分钟仍显示 1 分钟（不出现 0 分钟）', () => {
  const now = new Date('2026-11-15T12:36:00Z').getTime()
  assert.match(pendingOrderHint('2026-11-15T12:36:30Z', now), /约 1 分钟/)
})
