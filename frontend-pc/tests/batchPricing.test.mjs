// #2519：批量设价/批量状态纯函数回归（node --test；frontend-pc 无组件测试运行器）。
//
// #2519 Rev.5 把交互从「价格类型 + 操作 + 金额」三段式改为「三项并列 + 逐项开关」：
//   base_daily_rate（日租金）/ overdue_daily_fee（逾期日租金）/ deposit（押金）。
// 本文件覆盖必须失败与必须成功的边界：
//  1. 未开启任何项 → 前置拒绝（后端会把 no_fields 记进 failed[]）；
//  2. 只提交开启项，关闭项不出现于 items[]（后端据此「不改动」该字段）；
//  3. 日租金/逾期日租金必须 > 0；押金 0 合法（显式零押金）、负数非法；
//  4. 反馈必须优先消费后端 failed[]（D3：禁止把部分失败报成全部成功）。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  BATCH_PRICE_FIELDS,
  batchPricingFeedback,
  batchStatusFeedback,
  buildBatchPricingPayload,
  emptyBatchPricingValues,
} from '../src/utils/batchPricing.js'

test('字段清单与后端三项显式字段同源（不含周/月租、不含 overrides）', () => {
  assert.deepEqual(
    BATCH_PRICE_FIELDS.map((field) => field.key),
    ['base_daily_rate', 'overdue_daily_fee', 'deposit'],
  )
})

test('只提交开启项：关闭项不出现在 items[]', () => {
  const values = emptyBatchPricingValues()
  values.base_daily_rate = { enabled: true, value: 100 }
  values.overdue_daily_fee = { enabled: false, value: 99 }

  const built = buildBatchPricingPayload(['i1', 'i2'], values)
  assert.equal(built.error, undefined)
  assert.deepEqual(built.payload.items, [
    { id: 'i1', base_daily_rate: 100 },
    { id: 'i2', base_daily_rate: 100 },
  ])
  assert.equal('overdue_daily_fee' in built.payload.items[0], false)
})

test('三项全开：一条 item 携带全部字段（字符串金额按数值处理）', () => {
  const values = emptyBatchPricingValues()
  values.base_daily_rate = { enabled: true, value: '88.5' }
  values.overdue_daily_fee = { enabled: true, value: 120 }
  values.deposit = { enabled: true, value: 0 }

  const built = buildBatchPricingPayload(['i1'], values)
  assert.deepEqual(built.payload.items[0], {
    id: 'i1',
    base_daily_rate: 88.5,
    overdue_daily_fee: 120,
    deposit: 0,
  })
})

test('全关：前置拒绝且不产生 payload', () => {
  const built = buildBatchPricingPayload(['i1'], emptyBatchPricingValues())
  assert.equal(built.error, '请至少开启一项')
  assert.equal(built.payload, undefined)
})

test('空选择 / 非数组 ids：前置拒绝', () => {
  const values = emptyBatchPricingValues()
  values.deposit = { enabled: true, value: 0 }
  for (const ids of [[], undefined, null, 'i1']) {
    const built = buildBatchPricingPayload(ids, values)
    assert.equal(built.error, '请先选择乐器')
    assert.equal(built.payload, undefined)
  }
})

test('日租金/逾期日租金必须大于 0；押金可为 0、不可为负', () => {
  for (const key of ['base_daily_rate', 'overdue_daily_fee']) {
    for (const value of ['', null, undefined, 0, '0', -1, 'abc']) {
      const values = emptyBatchPricingValues()
      values[key] = { enabled: true, value }
      const built = buildBatchPricingPayload(['i1'], values)
      assert.ok(built.error, `${key}=${String(value)} 必须被拒绝`)
      assert.equal(built.payload, undefined)
    }
  }

  const zeroDeposit = emptyBatchPricingValues()
  zeroDeposit.deposit = { enabled: true, value: 0 }
  assert.deepEqual(buildBatchPricingPayload(['i1'], zeroDeposit).payload.items[0], {
    id: 'i1',
    deposit: 0,
  })

  const negativeDeposit = emptyBatchPricingValues()
  negativeDeposit.deposit = { enabled: true, value: -1 }
  assert.equal(buildBatchPricingPayload(['i1'], negativeDeposit).error, '押金不能为负数')
})

test('设价反馈优先消费 failed[]，其余按 updated/请求数比对', () => {
  const allOk = batchPricingFeedback({ data: { updated: 3, failed: [] } }, 3)
  assert.equal(allOk.level, 'success')
  assert.match(allOk.text, /3/)

  const partial = batchPricingFeedback(
    { data: { updated: 1, failed: [{ id: 'x', reason: 'no_fields' }] } },
    2,
  )
  assert.equal(partial.level, 'warning')
  assert.match(partial.text, /失败 1/)
  assert.match(partial.text, /no_fields/)

  const legacyPartial = batchPricingFeedback({ data: { updated: 1 } }, 3)
  assert.equal(legacyPartial.level, 'warning')
  assert.match(legacyPartial.text, /1\/3/)

  const nullRes = batchPricingFeedback(null, 2)
  assert.equal(nullRes.level, 'warning')
  assert.match(nullRes.text, /0\/2/)
})

test('批量状态反馈：失败逐条可见', () => {
  assert.deepEqual(batchStatusFeedback({ data: { updated: 2, failed: [] } }), {
    level: 'success',
    text: '已更新 2 个乐器状态',
  })

  const partial = batchStatusFeedback({
    data: { updated: 1, failed: [{ id: 'x', reason: 'not_found_or_forbidden' }] },
  })
  assert.equal(partial.level, 'warning')
  assert.match(partial.text, /失败 1/)
  assert.match(partial.text, /not_found_or_forbidden/)
})
