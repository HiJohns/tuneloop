// #2469 R1: 孤儿面板展示判定的纯函数用例（node --test）。
import test from 'node:test'
import assert from 'node:assert/strict'
import {
  shouldShowWarning, orphanTitle, orphanEmptyText, isEmptyState,
} from '../src/utils/orphanPanel.js'

const EMPTY = []
const ONE = [{ id: 'a', username: 'orphan-1' }]

test('空列表不渲染告警', () => {
  assert.equal(shouldShowWarning(EMPTY), false)
  assert.equal(shouldShowWarning(undefined), false)
  console.log(`SHOW_EMPTY=${shouldShowWarning(EMPTY)}`)
})

test('有 1 条命中时渲染告警', () => {
  assert.equal(shouldShowWarning(ONE), true)
  console.log(`SHOW_ONE=${shouldShowWarning(ONE)}`)
})

test('标题：空态固定、有数据含命中数', () => {
  assert.equal(orphanTitle(EMPTY, 12), 'IAM 孤儿账号')
  assert.match(orphanTitle(ONE, 12), /命中 1 个/)
  assert.match(orphanTitle(ONE, 12), /扫描 12 个/)
  assert.equal(orphanTitle(ONE, undefined), '命中 1 个')
  console.log('TITLE_HIT=命中 1 个')
})

test('空态文案带扫描数与挂起门槛，缺失时降级', () => {
  const text = orphanEmptyText(12, 60)
  assert.match(text, /本次扫描 12 个/)
  assert.match(text, /挂起 ≥ 60 分钟/)
  assert.match(orphanEmptyText(undefined, undefined), /本次扫描 — 个/)
  assert.doesNotMatch(orphanEmptyText(12, undefined), /挂起/)
  console.log('EMPTY_TEXT_HAS_SCANNED=1')
})

test('加载中不算空态（避免先闪「未发现」）', () => {
  assert.equal(isEmptyState({ loading: true, error: '', list: EMPTY }), false)
  assert.equal(isEmptyState({ loading: true, error: 'x', list: EMPTY }), false)
  console.log(`ISEMPTY_LOADING=${isEmptyState({ loading: true, error: '', list: EMPTY })}`)
})

test('请求失败不算空态', () => {
  assert.equal(isEmptyState({ loading: false, error: 'boom', list: EMPTY }), false)
  console.log(`ISEMPTY_ERROR=${isEmptyState({ loading: false, error: 'boom', list: EMPTY })}`)
})

test('稳定空态为真', () => {
  assert.equal(isEmptyState({ loading: false, error: '', list: EMPTY }), true)
  assert.equal(isEmptyState({ loading: false, error: '', list: ONE }), false)
  assert.equal(isEmptyState({ loading: false, error: '', list: undefined }), true)
})
