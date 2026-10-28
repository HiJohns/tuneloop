import test from 'node:test'
import assert from 'node:assert/strict'
import { mergeLocale } from './i18n-merge.mjs'

test('merge 保留既有 key 并追加新 key', () => {
  const out = mergeLocale({ '旧文案': '旧文案', '共同': '自定义值' }, ['共同', '新文案'])
  assert.equal(out['旧文案'], '旧文案')
  assert.equal(out['共同'], '自定义值', '既有 value 不被覆盖')
  assert.equal(out['新文案'], '新文案')
  assert.deepEqual(Object.keys(out), Object.keys(out).slice().sort(), 'keys 稳定排序')
})
