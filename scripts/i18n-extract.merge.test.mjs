import test from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { mergeLocale } from './i18n-merge.mjs'

const ROOT = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..')

test('merge 保留既有 key 并追加新 key', () => {
  const out = mergeLocale({ '旧文案': '旧文案', '共同': '自定义值' }, ['共同', '新文案'])
  assert.equal(out['旧文案'], '旧文案')
  assert.equal(out['共同'], '自定义值', '既有 value 不被覆盖')
  assert.equal(out['新文案'], '新文案')
  assert.deepEqual(Object.keys(out), Object.keys(out).slice().sort(), 'keys 稳定排序')
})

test('extract 跳过被局部变量遮蔽的 t（#2318）', () => {
  const out = execFileSync(
    process.execPath,
    ['scripts/i18n-extract.mjs', 'scripts/fixtures/i18n-shadow'],
    { cwd: ROOT, encoding: 'utf8' }
  )
  assert.match(out, /命中 1 处/, '未遮蔽文案仍应被抽取')
  assert.match(out, /跳过 2 处（t 被局部变量遮蔽，未改写）/, 'map 参数 t 遮蔽的 JSXText 应跳过')
  assert.match(out, /shadowed\t.*shadowed\.jsx/)
})
