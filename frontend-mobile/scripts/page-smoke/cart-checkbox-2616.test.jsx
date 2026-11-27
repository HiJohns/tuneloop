// #2616 R1：购物车勾选框渲染断言（H5 platform，jsdom 真实渲染 + fireEvent）。
//
// 断言的三种视觉态（Decision Lock D3）：
//   选中   → 填充 #B98E5F + 白色 ✓
//   未选中 → 2px #71717A 边框 + #FFFFFF 填充（对 #FDFBF7/白底可分辨）
//   禁用   → #E4E4E7 / #F4F4F5 + opacity 0.6，且点击不可选中（#1659 语义不回退）
// 另断言可点热区 ≥ 32px（视觉框仍是 20×20，D4）。
//
// 颜色断言一律经 DOM 归一化（D9），不硬编码 rgb() 字面量。
// 证据 token 走 process.stdout（page-smoke 的 setup 已 mock console.log）。
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { render, fireEvent, cleanup, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import { fixtures } from './fixtures'
import Cart from '../../src/pages/Cart'

const emit = (token) => process.stdout.write(`${token}\n`)

const norm = (color) => {
  const d = document.createElement('div')
  d.style.color = color
  return d.style.color
}

const seedCart = (items) => {
  window.localStorage.setItem('cart_user-1', JSON.stringify({ items }))
}

const rentedItem = {
  ...fixtures.cartItem,
  instrument_id: 'inst-2',
  name: '已租出乐器',
  sn: 'SN-SMOKE-2',
  stock_status: 'rented',
}

// checkStockStatus() 会用 /public/instruments/<id> 的响应覆盖本地 stock_status，
// 因此必须对 inst-2 打桩返回 rented，否则「已租出」形态在渲染前就被刷成 available。
const stubRentedInstrument = () => {
  const baseFetch = global.fetch
  vi.stubGlobal('fetch', vi.fn(async (input, init) => {
    if (String(input).includes('/public/instruments/inst-2')) {
      return {
        ok: true,
        status: 200,
        headers: { get: () => 'application/json' },
        json: async () => ({ code: 20000, data: { ...fixtures.instrument, stock_status: 'rented' }, message: 'success' }),
        text: async () => '',
        clone() { return this },
      }
    }
    return baseFetch(input, init)
  }))
}

const renderCart = () => render(
  <MemoryRouter initialEntries={['/cart']}>
    <Cart />
  </MemoryRouter>
)

describe('购物车勾选框视觉与热区 (#2616 R1)', () => {
  beforeEach(() => {
    seedCart([fixtures.cartItem, rentedItem])
    stubRentedInstrument()
  })
  afterEach(() => {
    cleanup()
    vi.unstubAllGlobals()
  })

  it('R1-1 选中态：填充主色 + 白色 ✓', async () => {
    const { container } = renderCart()
    const boxes = await waitFor(() => {
      const found = container.querySelectorAll('[data-testid="cart-select-box"]')
      expect(found.length).toBe(2)
      return found
    })
    const box = boxes[0]
    expect(box.style.borderColor).toBe(norm('#B98E5F'))
    expect(box.style.backgroundColor).toBe(norm('#B98E5F'))
    expect(box.textContent).toContain('✓')
    emit('CART_CHECKBOX_SELECTED_FILL=1')
  })

  it('R1-2 未选中态：加深边框 + 白底填充，且 ✓ 消失', async () => {
    const { container } = renderCart()
    const hit = await waitFor(() => {
      const found = container.querySelectorAll('[data-testid="cart-select-hit"]')
      expect(found.length).toBe(2)
      return found[0]
    })
    fireEvent.click(hit)
    const box = container.querySelectorAll('[data-testid="cart-select-box"]')[0]
    expect(box.style.borderColor).toBe(norm('#71717A'))
    expect(box.style.backgroundColor).toBe(norm('#FFFFFF'))
    expect(box.textContent).not.toContain('✓')
    emit('CART_CHECKBOX_UNSELECTED_CONTRAST=1')
    emit('CART_CHECKBOX_UNSELECTED_WHITE_BG=1')
  })

  it('R1-3 可点热区 ≥ 32px（实测 36，视觉框仍 20）', async () => {
    const { container } = renderCart()
    const hit = await waitFor(() => {
      const found = container.querySelectorAll('[data-testid="cart-select-hit"]')
      expect(found.length).toBe(2)
      return found[0]
    })
    const box = container.querySelectorAll('[data-testid="cart-select-box"]')[0]
    const side = Number.parseInt(hit.style.width, 10)
    expect(side).toBeGreaterThanOrEqual(32)
    expect(Number.parseInt(hit.style.height, 10)).toBeGreaterThanOrEqual(32)
    expect(Number.parseInt(box.style.width, 10)).toBe(20)
    emit(`CART_CHECKBOX_TAP_SIZE=${side}`)
  })

  it('R1-4 已租出项：灰阶禁用态与未选中态可区分，且点击不可选（#1659 不回退）', async () => {
    const { container } = renderCart()
    const hits = await waitFor(() => {
      const found = container.querySelectorAll('[data-testid="cart-select-hit"]')
      expect(found.length).toBe(2)
      return found
    })
    const disabledBox = container.querySelectorAll('[data-testid="cart-select-box"]')[1]
    expect(disabledBox.style.borderColor).toBe(norm('#E4E4E7'))
    expect(disabledBox.style.backgroundColor).toBe(norm('#F4F4F5'))
    expect(disabledBox.style.opacity).toBe('0.6')
    expect(disabledBox.textContent).not.toContain('✓')
    expect(disabledBox.style.borderColor).not.toBe(norm('#71717A'))
    emit('CART_CHECKBOX_DISABLED_DISTINCT=1')

    fireEvent.click(hits[1])
    const afterClick = container.querySelectorAll('[data-testid="cart-select-box"]')[1]
    expect(afterClick.textContent).not.toContain('✓')
    expect(afterClick.style.backgroundColor).toBe(norm('#F4F4F5'))
    emit('CART_CHECKBOX_DISABLED_NOT_SELECTABLE=1')
  })

  it('R1-5 渲染无致命错误', async () => {
    const { container } = renderCart()
    await waitFor(() => {
      expect(container.querySelectorAll('[data-testid="cart-select-box"]').length).toBe(2)
    })
    emit('CART_CHECKBOX_RENDER_FAIL=0')
  })
})
