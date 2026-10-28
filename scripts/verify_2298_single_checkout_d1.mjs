// #2298 audit Round 3: verify the H5 SingleCheckout branch really renders D1.
//
// The audit rejected the previous evidence because only BatchCheckout
// (/checkout without ?id=) was rendered, while D1 lives in SingleCheckout
// (/checkout?id=...). This script drives the real Vite H5 app with mocked APIs
// and asserts the D1 logistics copy is visible.
//
// Run: node scripts/verify_2298_single_checkout_d1.mjs
//   BASE (optional) default http://localhost:5553 (H5 dev server)
// Exit code 0 = D1 visible and no page errors; 1 = failed.

import { chromium } from '../frontend-mobile/node_modules/playwright/index.mjs'
import { mkdirSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const BASE = process.env.BASE || 'http://localhost:5553'
const ID = '11111111-1111-4111-8111-111111111111'
const OUT_DIR = resolve(dirname(fileURLToPath(import.meta.url)), '../tmp')
mkdirSync(OUT_DIR, { recursive: true })

const json = (route, body, status = 200) =>
  route.fulfill({ status, contentType: 'application/json', body: JSON.stringify(body) })
const token = `h.${Buffer.from(JSON.stringify({ role: 'USER', sub: 'cust' })).toString('base64')}.s`

const browser = await chromium.launch({ headless: true })
const ctx = await browser.newContext({ viewport: { width: 420, height: 900 }, isMobile: true, hasTouch: true })
await ctx.addInitScript(t => {
  localStorage.setItem('token', t)
  localStorage.setItem('token_expiry', String(Date.now() + 86400000))
}, token)

const page = await ctx.newPage()
const pageErrors = []
page.on('pageerror', e => pageErrors.push(e.message))
await page.route('**/api/**', route => {
  const u = new URL(route.request().url())
  if (u.pathname === `/api/public/instruments/${ID}`) {
    return json(route, {
      code: 20000,
      data: {
        id: ID,
        name: '测试钢琴',
        sn: 'SN2298',
        images: [],
        cover_image: '',
        deposit: 0,
        pricing: JSON.stringify([{ deposit: 0, daily_rent: 10000 }]),
        base_daily_rate: 10000,
        stock_status: 'available',
        site_name: '测试网点',
      },
    })
  }
  if (u.pathname === `/api/public/instruments/${ID}/pricing-v2`) {
    return json(route, { code: 20000, data: { tiers: [], base_daily_rate: 10000, deposit: 0 } })
  }
  if (u.pathname === '/api/users/me') {
    return json(route, { code: 20000, data: { id: 'u', name: '顾客', phone: '13800000000', membership_level_name: '专业' } })
  }
  if (u.pathname.includes('addresses') || u.pathname.includes('guarantors') || u.pathname.includes('waiver')) {
    return json(route, { code: 20000, data: { list: [] } })
  }
  return json(route, { code: 20000, data: { list: [] } })
})

try {
  await page.goto(`${BASE}/checkout?id=${ID}`, { waitUntil: 'commit', timeout: 30000 })
  await page.waitForFunction(
    () => document.body.innerText.includes('确认订单'),
    undefined,
    { timeout: 30000 },
  )
  const body = await page.locator('body').innerText()
  const hasSingle = body.includes('确认订单') && body.includes('租期选择')
  const hasD1 = body.includes('顺丰快递到付') && body.includes('请注意检查乐器状态后方可支付运费')
  await page.screenshot({ path: resolve(OUT_DIR, '2298-smoke-single-checkout.png'), fullPage: true })

  console.log('BASE=', BASE)
  console.log('HAS_SINGLE_CHECKOUT=', hasSingle)
  console.log('HAS_D1=', hasD1)
  console.log('PAGE_ERRORS=', JSON.stringify(pageErrors))

  if (!hasSingle) {
    console.error('FAIL: SingleCheckout markers (确认订单 / 租期选择) not rendered')
    process.exitCode = 1
  } else if (!hasD1) {
    console.error('FAIL: D1 copy (顺丰快递到付 / 请注意检查乐器状态后方可支付运费) not visible')
    process.exitCode = 1
  } else if (pageErrors.length > 0) {
    console.error('FAIL: page errors detected')
    process.exitCode = 1
  } else {
    console.log('PASS: H5 SingleCheckout renders D1 copy without page errors')
  }
} finally {
  await browser.close()
}
