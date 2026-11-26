// #2577: 库存监控筛选纯函数单测（`node --test`；frontend-pc 无组件测试运行器）。
//
// 覆盖 Decision Lock 的每条裁定（D1/D3/D4/D5/D6）：
//   - URL 读取：`status` 优先 + `stock_status` 兼容读；
//   - 列表参数：sn / category_id / stock_status，显式 lost 不叠加 exclude_status；
//   - 筛选变化：回第 1 页 + 清空行选择；
//   - 词表与选项：非管理级剔除 lost/archived/sold，URL 带入的当前值保留。
import test from 'node:test'
import assert from 'node:assert/strict'

import {
  HIDDEN_STOCK_STATUSES,
  applyFilterPatch,
  buildStockFilterQuery,
  buildStockListParams,
  readStockFilters,
  stockStatusLabel,
  stockStatusOptions,
} from '../src/utils/instrumentStockFilters.js'

const params = (query) => new URLSearchParams(query)

test('readStockFilters 空 URL → 全空筛选', () => {
  assert.deepEqual(readStockFilters(params('')), { sn: '', categoryId: '', status: '' })
  assert.deepEqual(readStockFilters(null), { sn: '', categoryId: '', status: '' })
})

test('readStockFilters 读取 sn / category_id / status', () => {
  const filters = readStockFilters(params('sn=CVZ-104&category_id=c1&status=available'))
  assert.equal(filters.sn, 'CVZ-104')
  assert.equal(filters.categoryId, 'c1')
  assert.equal(filters.status, 'available')
})

test('readStockFilters 兼容读 stock_status（乐器列表写入的别名）', () => {
  assert.equal(readStockFilters(params('stock_status=rented')).status, 'rented')
})

test('readStockFilters status 优先于 stock_status', () => {
  assert.equal(readStockFilters(params('status=available&stock_status=lost')).status, 'available')
})

test('buildStockListParams 透传分页与识别码', () => {
  const built = buildStockListParams({
    filters: { sn: 'CVZ-104', categoryId: '', status: '' },
    page: 2,
    pageSize: 20,
    isManagerLike: true,
  })
  assert.equal(built.page, 2)
  assert.equal(built.pageSize, 20)
  assert.equal(built.sn, 'CVZ-104')
  assert.equal(built.category_id, undefined)
  assert.equal(built.stock_status, undefined)
  assert.equal(built.exclude_status, undefined)
})

test('buildStockListParams 分类筛选走 category_id（服务端后代展开）', () => {
  const built = buildStockListParams({
    filters: { sn: '', categoryId: 'c1', status: '' },
    page: 1,
    pageSize: 20,
    isManagerLike: true,
  })
  assert.equal(built.category_id, 'c1')
})

test('buildStockListParams 状态筛选用 stock_status', () => {
  const built = buildStockListParams({
    filters: { status: 'available' },
    page: 1,
    pageSize: 20,
    isManagerLike: true,
  })
  assert.equal(built.stock_status, 'available')
})

test('buildStockListParams 非管理级追加 exclude_status', () => {
  const built = buildStockListParams({
    filters: { status: 'available' },
    page: 1,
    pageSize: 20,
    isManagerLike: false,
  })
  assert.equal(built.exclude_status, HIDDEN_STOCK_STATUSES.join(','))
})

test('buildStockListParams 非管理级显式 lost 时不叠加 exclude_status（D5）', () => {
  const built = buildStockListParams({
    filters: { status: 'lost' },
    page: 1,
    pageSize: 20,
    isManagerLike: false,
  })
  assert.equal(built.stock_status, 'lost')
  assert.equal(built.exclude_status, undefined)
})

test('buildStockListParams 管理级不追加 exclude_status', () => {
  const built = buildStockListParams({
    filters: { status: 'lost' },
    page: 1,
    pageSize: 20,
    isManagerLike: true,
  })
  assert.equal(built.exclude_status, undefined)
})

test('buildStockFilterQuery 写回参数名用 status 并剔除空值', () => {
  assert.deepEqual(buildStockFilterQuery({ sn: '', categoryId: '', status: 'available' }), {
    status: 'available',
  })
  assert.deepEqual(
    buildStockFilterQuery({ sn: 'CVZ-104', categoryId: 'c1', status: '' }),
    { sn: 'CVZ-104', category_id: 'c1' }
  )
  assert.deepEqual(buildStockFilterQuery({}), {})
})

test('applyFilterPatch 合并 patch 并回第 1 页、清空行选择', () => {
  const next = applyFilterPatch(
    { filters: { sn: 'old', categoryId: '', status: 'available' }, page: 3, selectedRowKeys: ['a', 'b'] },
    { sn: 'CVZ-105' }
  )
  assert.equal(next.filters.sn, 'CVZ-105')
  assert.equal(next.filters.status, 'available')
  assert.equal(next.page, 1)
  assert.deepEqual(next.selectedRowKeys, [])
})

test('applyFilterPatch 允许显式清空某一项筛选', () => {
  const next = applyFilterPatch(
    { filters: { sn: 'old', categoryId: 'c1', status: 'available' }, page: 2, selectedRowKeys: ['a'] },
    { categoryId: '' }
  )
  assert.equal(next.filters.categoryId, '')
  assert.equal(next.filters.sn, 'old')
  assert.equal(next.page, 1)
})

test('stockStatusLabel 词表（D4）与未知值原样', () => {
  assert.equal(stockStatusLabel('available'), '待租')
  assert.equal(stockStatusLabel('rented'), '在租')
  assert.equal(stockStatusLabel('maintenance'), '维修中')
  assert.equal(stockStatusLabel('reserved'), '已预订')
  assert.equal(stockStatusLabel('shipping'), '配送中')
  assert.equal(stockStatusLabel('lost'), '已丢失')
  assert.equal(stockStatusLabel('archived'), '已下架')
  assert.equal(stockStatusLabel('sold'), '已售出')
  assert.equal(stockStatusLabel('unknown-state'), 'unknown-state')
  assert.equal(stockStatusLabel(''), '')
})

const ALL_STATUSES = [
  { value: 'available' },
  { value: 'rented' },
  { value: 'maintenance' },
  { value: 'reserved' },
  { value: 'shipping' },
  { value: 'lost' },
  { value: 'archived' },
  { value: 'sold' },
]

test('stockStatusOptions 非管理级剔除 lost/archived/sold（8 → 5）', () => {
  const options = stockStatusOptions({ statuses: ALL_STATUSES, isManagerLike: false })
  assert.equal(options.length, 5)
  assert.deepEqual(
    options.map((option) => option.value),
    ['available', 'rented', 'maintenance', 'reserved', 'shipping']
  )
  assert.equal(options[0].label, '待租')
})

test('stockStatusOptions 管理级保留全部并去重', () => {
  const options = stockStatusOptions({
    statuses: [{ value: 'available' }, { value: 'lost' }, 'available', { value: 'sold' }],
    isManagerLike: true,
  })
  assert.deepEqual(options.map((option) => option.value), ['available', 'lost', 'sold'])
})

test('stockStatusOptions 保留 URL 带入但不在选项集合中的当前值', () => {
  const options = stockStatusOptions({
    statuses: [{ value: 'available' }],
    isManagerLike: true,
    current: 'lost',
  })
  assert.deepEqual(options.map((option) => option.value), ['available', 'lost'])
})

test('stockStatusOptions 当前值已存在时不重复追加', () => {
  const options = stockStatusOptions({
    statuses: [{ value: 'available' }],
    isManagerLike: true,
    current: 'available',
  })
  assert.deepEqual(options.map((option) => option.value), ['available'])
})

test('stockStatusOptions 空输入不抛错', () => {
  assert.deepEqual(stockStatusOptions(), [])
  assert.deepEqual(stockStatusOptions({ statuses: null }), [])
})
