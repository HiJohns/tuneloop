// #2436 Rev.4 R3b：BatchCheckout 专用渲染断言。
// 既有 4 条 smoke 路由都带 ?id= → SingleCheckout（合计标签「应付总额」）；
// 本单修复的「应付合计」只存在于 BatchCheckout（无 id + 购物车非空）。
// 因此单独渲染 BatchCheckout，断言 H5 / weapp 都出现「应付合计」，并补空购物车负例。
import { describe, it, expect, afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { captureRender } from './harness'

const CART_KEYS = ['cart_checkout', 'cart_user-1', 'cart']

async function renderCheckout(entry, pattern, load) {
  window.history.replaceState({}, '', entry)
  const { default: Page } = await load()
  const errors = await captureRender(
    <MemoryRouter initialEntries={[entry]}>
      <Routes>
        <Route path={pattern} element={<Page />} />
      </Routes>
    </MemoryRouter>
  )
  expect(errors, `fatal render errors:\n${errors.join('\n\n')}`).toEqual([])
  return document.body.textContent || ''
}

describe('BatchCheckout 合计标签 (#2436 Rev.4)', () => {
  afterEach(() => cleanup())

  it('R3b: H5 无 id + 购物车非空 → 渲染「应付合计」，且无 TOTAL PAYABLE', async () => {
    const text = await renderCheckout('/checkout', '/checkout', () => import('../../src/pages/Checkout'))
    expect(text).toContain('应付合计')
    expect(text).not.toContain('TOTAL PAYABLE')
  })

  it('R3b: weapp 无 id + 购物车非空 → 渲染「应付合计」', async () => {
    const text = await renderCheckout(
      '/weapp-checkout',
      '/weapp-checkout',
      () => import('../../src/pages-weapp/Checkout')
    )
    expect(text).toContain('应付合计')
    expect(text).not.toContain('TOTAL PAYABLE')
  })

  it('R3b 负例：空购物车 → 渲染「购物车为空」，不出现「应付合计」', async () => {
    for (const key of CART_KEYS) window.localStorage.removeItem(key)
    const text = await renderCheckout('/checkout', '/checkout', () => import('../../src/pages/Checkout'))
    expect(text).toContain('购物车为空')
    expect(text).not.toContain('应付合计')
  })
})
