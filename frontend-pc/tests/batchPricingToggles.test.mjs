// #2519 R9：批量设价「三项并列 + 逐项开关」纯逻辑用例（node --test）。
//
// PC 无组件测试运行器（frontend-pc/package.json 只有 dev/build/lint/preview），
// 因此交互语义收敛到 utils/batchPricing.js 用纯函数覆盖（D10：evidence_type=static）。

import test from 'node:test'
import assert from 'node:assert/strict'

import {
  BATCH_PRICE_FIELDS,
  batchPricingFeedback,
  batchStatusFeedback,
  buildBatchPricingPayload,
  emptyBatchPricingValues,
} from '../src/utils/batchPricing.js'

test('field list is exactly the three backend fields', () => {
  assert.deepEqual(
    BATCH_PRICE_FIELDS.map((field) => field.key),
    ['base_daily_rate', 'overdue_daily_fee', 'deposit'],
  )
})

test('only enabled fields are submitted', () => {
  const values = emptyBatchPricingValues()
  values.base_daily_rate = { enabled: true, value: 50 }
  values.deposit = { enabled: true, value: 0 }

  const built = buildBatchPricingPayload(['a', 'b'], values)
  assert.equal(built.error, undefined)
  assert.deepEqual(built.payload.items, [
    { id: 'a', base_daily_rate: 50, deposit: 0 },
    { id: 'b', base_daily_rate: 50, deposit: 0 },
  ])
  assert.equal('overdue_daily_fee' in built.payload.items[0], false)
})

test('single enabled field submits only that field', () => {
  const values = emptyBatchPricingValues()
  values.overdue_daily_fee = { enabled: true, value: 120 }
  const built = buildBatchPricingPayload(['x'], values)
  assert.deepEqual(built.payload.items, [{ id: 'x', overdue_daily_fee: 120 }])
})

test('all toggles off is rejected', () => {
  const built = buildBatchPricingPayload(['x'], emptyBatchPricingValues())
  assert.equal(built.payload, undefined)
  assert.match(built.error, /请至少开启一项/)
})

test('deposit accepts 0 but daily rent rejects 0', () => {
  const zeroDeposit = emptyBatchPricingValues()
  zeroDeposit.deposit = { enabled: true, value: 0 }
  assert.equal(buildBatchPricingPayload(['x'], zeroDeposit).error, undefined)

  const zeroRent = emptyBatchPricingValues()
  zeroRent.base_daily_rate = { enabled: true, value: 0 }
  assert.match(buildBatchPricingPayload(['x'], zeroRent).error, /必须大于 0/)

  const negativeDeposit = emptyBatchPricingValues()
  negativeDeposit.deposit = { enabled: true, value: -1 }
  assert.match(buildBatchPricingPayload(['x'], negativeDeposit).error, /不能为负数/)
})

test('empty selection and non numeric values are rejected', () => {
  const values = emptyBatchPricingValues()
  values.base_daily_rate = { enabled: true, value: 50 }
  assert.match(buildBatchPricingPayload([], values).error, /请先选择乐器/)

  const bad = emptyBatchPricingValues()
  bad.base_daily_rate = { enabled: true, value: 'abc' }
  assert.match(buildBatchPricingPayload(['x'], bad).error, /必须填写数字/)
})

test('feedback surfaces failed[] reasons', () => {
  const feedback = batchPricingFeedback({ data: { updated: 1, failed: [{ id: 'x', reason: 'no_fields' }] } }, 2)
  assert.equal(feedback.level, 'warning')
  assert.match(feedback.text, /失败 1 个/)
  assert.match(feedback.text, /no_fields×1/)
})

test('feedback is success when everything updated', () => {
  const feedback = batchPricingFeedback({ data: { updated: 2, failed: [] } }, 2)
  assert.equal(feedback.level, 'success')
})

test('batch status feedback surfaces failed[]', () => {
  const feedback = batchStatusFeedback({ data: { updated: 3, failed: [] } })
  assert.equal(feedback.level, 'success')
  assert.match(feedback.text, /已更新 3 个乐器状态/)

  const partial = batchStatusFeedback({ data: { updated: 1, failed: [{ id: 'y', reason: 'not_found_or_forbidden' }] } })
  assert.equal(partial.level, 'warning')
  assert.match(partial.text, /not_found_or_forbidden×1/)
})
