// #2514：批量设价纯函数回归（node --test；frontend-pc 无组件测试运行器）。
//
// 覆盖三类必须失败的方向：
//  1. 空选择 / 非本轮支持的 price_type → 前置拒绝，不产生请求；
//  2. 金额缺失 / 0 / NaN / 负数 → 前置拒绝（后端 <=0 静默跳过，会产生「假成功」）；
//  3. updated 少于请求条数 → warning（禁止报全部成功）。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  BATCH_PRICE_OPERATOR_OPTIONS,
  BATCH_PRICE_TYPE_OPTIONS,
  batchPricingFeedback,
  buildBatchPricingPayload,
} from '../src/utils/batchPricing.js'

test('合法输入映射为后端契约 items[{id, base_daily_rate}]（元）', () => {
  const result = buildBatchPricingPayload(['i1', 'i2'], { price_type: 'daily_rate', amount: 100 })

  assert.equal(result.error, undefined)
  assert.equal(result.payload.items.length, 2)
  assert.deepEqual(Object.keys(result.payload.items[0]), ['id', 'base_daily_rate'])
  assert.deepEqual(result.payload.items[0], { id: 'i1', base_daily_rate: 100 })
  assert.deepEqual(result.payload.items[1], { id: 'i2', base_daily_rate: 100 })

  // 字符串金额（InputNumber 可能给出字符串）同样按数值处理
  const fromString = buildBatchPricingPayload(['i1'], { price_type: 'daily_rate', amount: '88.5' })
  assert.deepEqual(fromString.payload.items[0], { id: 'i1', base_daily_rate: 88.5 })
})

test('空选择 / 非数组 ids：前置拒绝且不产生 payload', () => {
  for (const ids of [[], undefined, null, 'i1']) {
    const result = buildBatchPricingPayload(ids, { price_type: 'daily_rate', amount: 100 })
    assert.equal(result.error, '请先选择乐器')
    assert.equal(result.payload, undefined)
  }
})

test('非本轮支持的 price_type：前置拒绝（后端无周/月租与押金语义）', () => {
  for (const priceType of ['weekly_rate', 'monthly_rate', 'deposit', '', undefined]) {
    const result = buildBatchPricingPayload(['i1'], { price_type: priceType, amount: 100 })
    assert.equal(result.error, '批量设价当前仅支持「日租金」+「设置为」')
    assert.equal(result.payload, undefined)
  }
})

test('金额缺失/0/NaN/负数：前置拒绝（后端 <=0 会静默跳过）', () => {
  for (const amount of ['', null, undefined, 0, '0', NaN, -1, 'abc']) {
    const result = buildBatchPricingPayload(['i1'], { price_type: 'daily_rate', amount })
    assert.equal(result.error, '金额必须大于 0', `amount=${String(amount)} 必须被拒绝`)
    assert.equal(result.payload, undefined)
  }
})

test('updated 与请求条数：全部更新 → success，部分/缺失 → warning', () => {
  assert.deepEqual(batchPricingFeedback({ data: { updated: 3 } }, 3), {
    level: 'success',
    text: '已修改 3 个乐器价格',
  })

  const partial = batchPricingFeedback({ data: { updated: 1 } }, 3)
  assert.equal(partial.level, 'warning')
  assert.match(partial.text, /1\/3/)

  const none = batchPricingFeedback({ code: 20000 }, 2)
  assert.equal(none.level, 'warning')
  assert.match(none.text, /0\/2/)

  const nullRes = batchPricingFeedback(null, 2)
  assert.equal(nullRes.level, 'warning')
  assert.match(nullRes.text, /0\/2/)
})

test('选项与后端能力同源：只暴露日租金与设置为', () => {
  assert.deepEqual(BATCH_PRICE_TYPE_OPTIONS, [{ value: 'daily_rate', label: '日租金' }])
  assert.deepEqual(BATCH_PRICE_OPERATOR_OPTIONS, [{ value: 'set', label: '设置为' }])
})
