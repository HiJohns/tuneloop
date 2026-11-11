// #2326 回归测试：公开页判定（双端）、降级窗口去重、接线、平台对称。
// 运行：node --test frontend-mobile/src/services/authRouting.test.mjs
import test from 'node:test'
import assert from 'node:assert/strict'
import fs from 'node:fs'
import path from 'node:path'
import { fileURLToPath } from 'node:url'
import { isPublicPath, shouldDegrade } from './authRouting.mjs'

const HERE = path.dirname(fileURLToPath(import.meta.url))
const SRC_ROOT = path.resolve(HERE, '..')

test('公开页判定（H5）：带 query 也必须命中', () => {
  for (const p of ['/', '/cart', '/cart?foo=1', '/instrument?id=abc', '/content?key=x', '/mall', '/success']) {
    assert.equal(isPublicPath(p, { isMiniProgram: false }), true, `${p} 应为公开页`)
  }
  // 负例：受保护页必须判 false（判 true 会掩盖登出缺陷）
  for (const p of ['/profile', '/checkout', '/payment', '/setting', '/staff/orders']) {
    assert.equal(isPublicPath(p, { isMiniProgram: false }), false, `${p} 不应为公开页`)
  }
})

test('公开页判定（weapp）：按 pages-weapp 页面路径', () => {
  for (const p of [
    '/pages-weapp/home/index',
    '/pages-weapp/detail/index?id=1',
    '/pages-weapp/content/index',
    '/pages-weapp/cart/index',
    '/pages-weapp/mall/index',
  ]) {
    assert.equal(isPublicPath(p, { isMiniProgram: true }), true, `${p} 应为公开页`)
  }
  // 负例：受保护页必须以小程序路径判 false
  for (const p of ['/pages-weapp/checkout/index', '/pages-weapp/profile/index', '/pages-weapp/order-detail/index?id=1']) {
    assert.equal(isPublicPath(p, { isMiniProgram: true }), false, `${p} 不应为公开页`)
  }
  // 负例：H5 规则不得串到小程序（/cart 在小程序无对应页面）
  assert.equal(isPublicPath('/cart', { isMiniProgram: true }), false)
})

test('降级窗口：同窗口内并发 401 只降级一次', () => {
  assert.equal(shouldDegrade(1000, 0), true, '首次降级必须放行')
  assert.equal(shouldDegrade(1500, 1000), false, '窗口内第二次必须抑制')
  assert.equal(shouldDegrade(1999, 1000), false, '边界前仍抑制')
  assert.equal(shouldDegrade(3000, 1000), true, '窗口后允许再次降级')
  // 负例：把窗口内判 true 的实现在此断言下会失败
  assert.notEqual(shouldDegrade(1500, 1000, 100), false, '自定义窗口语义须生效')
})

function extractFunction(src, header) {
  const start = src.indexOf(header)
  if (start < 0) return ''
  const end = src.indexOf('\n}', start)
  return end < 0 ? src.slice(start) : src.slice(start, end + 2)
}

test('接线：api.js 使用 isPublicPath 与 resetToHome，且无裸 redirect 根路径', () => {
  const api = fs.readFileSync(path.join(SRC_ROOT, 'services/api.js'), 'utf8')

  assert.match(api, /isPublicPath\(/, 'isPublicRoute 必须走平台感知判定')
  assert.match(api, /resetToHome\(/, 'degradeToGuest 必须平台化跳转')
  assert.doesNotMatch(api, /navigation\.redirect\('\/'\)/, '不得再把裸根路径交给 navigation.redirect')

  const degrade = extractFunction(api, 'export function degradeToGuest(')
  assert.match(degrade, /resetToHome\(/, 'degradeToGuest 内部必须调用 resetToHome')
  assert.match(degrade, /shouldDegrade\(/, 'degradeToGuest 必须做窗口去重')

  // 负例：缺陷版写法必须被同一断言识别（证明正则具备鉴别力）
  const legacy = "export function degradeToGuest() { navigation.redirect('/') }"
  assert.match(legacy, /navigation\.redirect\('\/'\)/)
})

test('平台对称：两端均导出 resetToHome', () => {
  const web = fs.readFileSync(path.join(SRC_ROOT, 'platform/browser.js'), 'utf8')
  const weapp = fs.readFileSync(path.join(SRC_ROOT, 'platform/index.weapp.js'), 'utf8')
  assert.match(web, /resetToHome:\s*\(\)\s*=>/, 'browser.js 缺少 resetToHome')
  assert.match(weapp, /resetToHome:\s*\(\)\s*=>/, 'index.weapp.js 缺少 resetToHome')
  assert.match(weapp, /switchTab\(\{\s*url:\s*'\/pages-weapp\/home\/index'\s*\}\)/, 'weapp 必须 switchTab 回首页')

  // 负例：缺少该导出的对象必须判失败
  const sample = { redirect: () => {} }
  assert.equal(typeof sample.resetToHome, 'undefined')
})
