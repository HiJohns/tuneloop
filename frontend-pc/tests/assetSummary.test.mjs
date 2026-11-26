// #2538: 乐器详情页页头摘要纯函数单测（`node --test`）。
// PC 端无组件测试运行器（frontend-pc/package.json 仅 dev/build/lint/preview），
// 按 Decision Lock D6 以纯函数 + 源码结构断言提供证据（evidence_type: static/unit）。

import test from 'node:test'
import assert from 'node:assert/strict'

import { buildAssetSummary } from '../src/utils/assetSummary.js'

const byKey = (rows) => Object.fromEntries(rows.map((row) => [row.key, row]))

test('buildAssetSummary returns the six summary items in a fixed order', () => {
  const rows = buildAssetSummary({}, {})
  assert.equal(rows.length, 6)
  assert.deepEqual(
    rows.map((row) => row.label),
    ['标准日租金', '逾期租金', '全额押金', '差额押金', '估值', '加入日期']
  )
})

test('buildAssetSummary converts every money field from cents', () => {
  const rows = byKey(
    buildAssetSummary(
      { total_price: 1200000, created_at: '2026-01-02T03:04:05Z' },
      {
        base_daily_rate: 10000,
        overdue_daily_fee: 3600,
        deposit: 50000,
        deposit_reduced: 20000,
      }
    )
  )
  assert.equal(rows.daily_rate.value, '¥100.00/天')
  assert.equal(rows.overdue_daily_fee.value, '¥36.00/天')
  assert.equal(rows.deposit.value, '¥500.00')
  assert.equal(rows.deposit_reduced.value, '¥200.00')
  assert.equal(rows.total_price.value, '¥12000.00')
  assert.match(rows.created_at.value, /月/)
})

test('buildAssetSummary keeps explicit zero deposit / reduced visible', () => {
  const rows = byKey(buildAssetSummary({}, { deposit: 0, deposit_reduced: 0 }))
  assert.equal(rows.deposit.value, '¥0.00')
  assert.equal(rows.deposit_reduced.value, '¥0.00')
})

test('buildAssetSummary shows 未配置 for a missing reduced deposit', () => {
  const rows = byKey(buildAssetSummary({}, { deposit: 50000 }))
  assert.equal(rows.deposit_reduced.value, '未配置')
})

test('buildAssetSummary degrades to - when pricing-v2 is unavailable', () => {
  const rows = byKey(buildAssetSummary({ total_price: 90000 }, null))
  assert.equal(rows.daily_rate.value, '-')
  assert.equal(rows.overdue_daily_fee.value, '-')
  assert.equal(rows.deposit.value, '-')
  assert.equal(rows.deposit_reduced.value, '未配置')
  assert.equal(rows.total_price.value, '¥900.00')
})

test('buildAssetSummary takes 估值 from total_price (cents), never daily rate', () => {
  const rows = byKey(buildAssetSummary({ total_price: 360000 }, { base_daily_rate: 10000 }))
  assert.equal(rows.total_price.value, '¥3600.00')
  assert.notEqual(rows.total_price.value, rows.daily_rate.value)
})

test('buildAssetSummary shows - when created_at is missing or unparsable', () => {
  const missing = byKey(buildAssetSummary({}, {}))
  assert.equal(missing.created_at.value, '-')
  const invalid = byKey(buildAssetSummary({ created_at: 'not-a-date' }, {}))
  assert.equal(invalid.created_at.value, 'not-a-date')
})

test('buildAssetSummary tolerates a null instrument without throwing', () => {
  const rows = byKey(buildAssetSummary(null, { base_daily_rate: 10000, overdue_daily_fee: 0 }))
  assert.equal(rows.daily_rate.value, '¥100.00/天')
  assert.equal(rows.overdue_daily_fee.value, '¥0.00/天')
  assert.equal(rows.total_price.value, '-')
  assert.equal(rows.created_at.value, '-')
})
