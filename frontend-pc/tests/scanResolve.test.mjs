// #2634 R2：resolveScanCandidates 表驱动用例（10 个 fixture，纯逻辑、无 DOM/DB）。
import assert from 'node:assert/strict'
import test from 'node:test'

import { resolveScanCandidates } from '../src/utils/scanResolve.js'

const orders = [
  { id: '1', order_no: 'ORD-1001', instrument_sn: 'SN-AAA', status: 'shipped', updated_at: '2026-11-01T10:00:00Z' },
  { id: '2', order_no: 'ORD-1002', instrument_sn: 'SN-BBB', status: 'returning', updated_at: '2026-11-02T10:00:00Z' },
  { id: '3', order_no: 'ORD-2001', instrument_sn: 'SN-CCC', status: 'in_lease', updated_at: '2026-11-03T10:00:00Z' },
  { id: '4', order_no: 'ORD-2002', instrument_sn: 'SN-DDD', status: 'shipped', updated_at: '2026-11-04T10:00:00Z' },
  { id: '5', order_no: 'ORD-3001', instrument_sn: 'SN-EEE', status: 'shipped', updated_at: '2026-11-05T10:00:00Z' },
  { id: '6', order_no: 'ORD-3002', instrument_sn: 'SN-FFF', status: 'shipped', updated_at: '2026-11-06T10:00:00Z' },
  { id: '7', order_no: 'ORD-3003', instrument_sn: 'SN-GGG', status: 'shipped', updated_at: '2026-11-07T10:00:00Z' },
]

test('EXACT_ORDER_NO: 订单号精确命中（单条）', () => {
  const r = resolveScanCandidates(orders, 'ORD-1001')
  assert.equal(r.kind, 'exact')
  assert.equal(r.list.length, 1)
  assert.equal(r.list[0].id, '1')
})

test('EXACT_SN: 乐器序列号精确命中', () => {
  const r = resolveScanCandidates(orders, 'SN-BBB')
  assert.equal(r.kind, 'exact')
  assert.deepEqual(r.list.map((o) => o.id), ['2'])
})

test('CASE_INSENSITIVE: 大小写不敏感', () => {
  const r = resolveScanCandidates(orders, 'ord-2001')
  assert.equal(r.kind, 'exact')
  assert.equal(r.list[0].id, '3')
  const sn = resolveScanCandidates(orders, 'sn-ccc')
  assert.equal(sn.kind, 'exact')
})

test('FUZZY_FALLBACK: 无精确命中时按包含匹配', () => {
  const r = resolveScanCandidates(orders, '300')
  assert.equal(r.kind, 'fuzzy')
  assert.equal(r.list.length, 3)
  assert.deepEqual(r.list.map((o) => o.id), ['7', '6', '5'])
})

test('EMPTY_CODE_NONE: 空串 / 纯空白不查', () => {
  assert.deepEqual(resolveScanCandidates(orders, ''), { kind: 'none', list: [], total: 0 })
  assert.deepEqual(resolveScanCandidates(orders, '   '), { kind: 'none', list: [], total: 0 })
})

test('NO_MATCH_NONE: 无匹配返回 none', () => {
  const r = resolveScanCandidates(orders, 'ORD-9999')
  assert.equal(r.kind, 'none')
  assert.equal(r.list.length, 0)
})

test('CANDIDATE_CAP_5: 候选上限 5 条', () => {
  const many = Array.from({ length: 9 }, (_, i) => ({
    id: String(i + 1),
    order_no: `ORD-X${i + 1}`,
    instrument_sn: `SN-X${i + 1}`,
    updated_at: `2026-11-0${(i % 9) + 1}T00:00:00Z`,
  }))
  const r = resolveScanCandidates(many, 'ORD-X')
  assert.equal(r.kind, 'fuzzy')
  assert.equal(r.list.length, 5)
})

test('TOTAL_BEFORE_CAP: total = 截断前命中数（页面据此提示「仅列出前 5 条」，list 不可用）', () => {
  const many = Array.from({ length: 8 }, (_, i) => ({
    id: String(i + 1),
    order_no: `ORD-Y${i + 1}`,
    instrument_sn: `SN-Y${i + 1}`,
    updated_at: `2026-11-0${(i % 9) + 1}T00:00:00Z`,
  }))
  const capped = resolveScanCandidates(many, 'ORD-Y')
  assert.equal(capped.list.length, 5)
  assert.equal(capped.total, 8)
  // 未超上限时 total 必须等于 list.length（不得恒等于上限，否则提示永远显示）
  const few = resolveScanCandidates(orders, 'ORD-300')
  assert.equal(few.list.length, 3)
  assert.equal(few.total, 3)
  // 精确命中同样带 total（弹窗提示不区分精确/模糊）
  const exact = resolveScanCandidates(orders, 'ORD-1001')
  assert.equal(exact.total, 1)
})

test('NULL_LIST_GUARD: orders 为 null / 非数组视为空列表', () => {
  assert.deepEqual(resolveScanCandidates(null, 'ORD-1'), { kind: 'none', list: [], total: 0 })
  assert.deepEqual(resolveScanCandidates(undefined, 'ORD-1'), { kind: 'none', list: [], total: 0 })
  assert.deepEqual(resolveScanCandidates('not-an-array', 'ORD-1'), { kind: 'none', list: [], total: 0 })
})

test('MISSING_FIELDS_SAFE: 缺 order_no/instrument_sn 的行不误命中', () => {
  const broken = [{ id: 'a' }, { id: 'b', order_no: null, instrument_sn: null }]
  const r = resolveScanCandidates(broken, 'ORD-1001')
  assert.equal(r.kind, 'none')
  assert.equal(r.list.length, 0)
})
