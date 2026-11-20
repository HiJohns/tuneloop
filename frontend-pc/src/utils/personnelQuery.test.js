// #2427 R1：buildPersonnelQuery 空值省略 + direct 语义（node --test）
import test from 'node:test'
import assert from 'node:assert/strict'

import { buildPersonnelQuery, personnelQueryString } from './personnelQuery.js'

test('R1: siteId=null 省略 site_id，查询串不含 null', () => {
  const params = buildPersonnelQuery({ name: '', siteId: null, direct: false, page: 1, pageSize: 10 })
  assert.equal('site_id' in params, false)
  const qs = personnelQueryString(params)
  assert.equal(qs.includes('site_id'), false)
  assert.equal(qs.includes('null'), false)
  assert.equal(qs, 'page=1&page_size=10')
})

test('R1: siteId="" 省略 site_id', () => {
  const params = buildPersonnelQuery({ name: '张三', siteId: '', page: 2, pageSize: 20 })
  assert.equal('site_id' in params, false)
  assert.equal(params.name, '张三')
  assert.equal(personnelQueryString(params), 'name=%E5%BC%A0%E4%B8%89&page=2&page_size=20')
})

test('R1: 有效 siteId 原样保留', () => {
  const siteId = '4c1f9f0e-2a3b-4c5d-8e9f-0a1b2c3d4e5f'
  const params = buildPersonnelQuery({ siteId })
  assert.equal(params.site_id, siteId)
  assert.equal(personnelQueryString(params), 'site_id=' + siteId)
})

test('R1: direct 仅 true 时输出，false/undefined 省略', () => {
  assert.equal(buildPersonnelQuery({ direct: true }).direct, 'true')
  assert.equal(buildPersonnelQuery({ direct: 'true' }).direct, 'true')
  assert.equal('direct' in buildPersonnelQuery({ direct: false }), false)
  assert.equal('direct' in buildPersonnelQuery({}), false)
})

test('R1: name 去空白，空串省略；page/page_size 保留', () => {
  assert.equal('name' in buildPersonnelQuery({ name: '   ' }), false)
  const params = buildPersonnelQuery({ name: '  李四  ', page: 3, pageSize: 50 })
  assert.equal(params.name, '李四')
  assert.equal(params.page, 3)
  assert.equal(params.page_size, 50)
  assert.equal(buildPersonnelQuery({ page: 1 }).page, 1, 'page=1 不得被当作空值丢弃')
})
