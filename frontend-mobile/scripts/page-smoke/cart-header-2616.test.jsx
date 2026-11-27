// #2616 R2：weapp 自定义导航头部胶囊避让 + 长文案不裁切（jsdom 真实渲染）。
//
// 覆盖三种形态（Decision Lock D1/D2/D5）：
//   weapp（胶囊 left=300, windowWidth=375）→ paddingRight = 375-300+8 = 83，paddingTop = 20+8 = 28
//   H5（无胶囊）                          → 16 / 12（不得套用胶囊内边距）
//   胶囊 API 抛错                          → 96 兜底（不得退化为 0，那正是缺陷形态）
// 另断言「删除所选(12)」完整且不压缩、标题 flex:1 1 0% + minWidth:0 先让位。
// 证据 token 走 process.stdout（page-smoke 的 setup 已 mock console.log）。
import { describe, it, expect, afterEach, beforeEach, vi } from 'vitest'
import { render, cleanup, waitFor } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Taro from '@tarojs/taro'
import * as platformMock from '../../src/platform'
import Cart from '../../src/pages/Cart'
import { fixtures } from './fixtures'

// weapp 平台实现 + 可切换的 env.isMiniProgram（同一文件覆盖 weapp / H5 两形态）。
vi.mock('../../src/platform', async () => {
  const actual = await import('../../src/platform/index.weapp')
  const state = { miniProgram: true }
  const env = { ...actual.env, apiBaseUrl: '/api' }
  Object.defineProperty(env, 'isMiniProgram', { get: () => state.miniProgram, configurable: true })
  return { ...actual, env, __envState: state }
})

const emit = (token) => process.stdout.write(`${token}\n`)

const renderCart = () => render(
  <MemoryRouter initialEntries={['/cart']}>
    <Cart />
  </MemoryRouter>
)

const header = (container) => container.querySelector('[data-testid="cart-header"]')

describe('购物车自定义导航头部胶囊避让 (#2616 R2)', () => {
  beforeEach(() => {
    platformMock.__envState.miniProgram = true
    window.localStorage.setItem('cart_user-1', JSON.stringify({ items: [fixtures.cartItem] }))
  })
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('R2-1 weapp：右内边距 = windowWidth - menu.left + 8 = 83，上内边距 = 28', async () => {
    const { container } = renderCart()
    const el = await waitFor(() => {
      const found = header(container)
      expect(found).toBeTruthy()
      return found
    })
    expect(el.style.paddingRight).toBe('83px')
    expect(el.style.paddingTop).toBe('28px')
    expect(el.style.boxSizing).toBe('border-box')
    emit('CART_HEADER_WEAPP_PADDING_RIGHT=83')
    emit('CART_HEADER_WEAPP_PADDING_TOP=28')
  })

  it('R2-2 H5：无胶囊 → 16 / 12（负例：不得把胶囊内边距套到 H5）', async () => {
    platformMock.__envState.miniProgram = false
    const { container } = renderCart()
    const el = await waitFor(() => {
      const found = header(container)
      expect(found).toBeTruthy()
      return found
    })
    expect(el.style.paddingRight).toBe('16px')
    expect(el.style.paddingTop).toBe('12px')
    emit('CART_HEADER_H5_PADDING_RIGHT=16')
    emit('CART_HEADER_H5_PADDING_TOP=12')
  })

  it('R2-3 胶囊 API 抛错：96 兜底（负例：兜底不得为 0）', async () => {
    vi.spyOn(Taro, 'getMenuButtonBoundingClientRect').mockImplementation(() => {
      throw new Error('no capsule')
    })
    const { container } = renderCart()
    const el = await waitFor(() => {
      const found = header(container)
      expect(found).toBeTruthy()
      return found
    })
    expect(el.style.paddingRight).toBe('96px')
    expect(el.style.paddingRight).not.toBe('0px')
    emit('CART_HEADER_FALLBACK_PADDING_RIGHT=96')
  })

  it('R2-4 长计数不裁切：删除所选(12) 完整、不压缩、标题先让位', async () => {
    const items = Array.from({ length: 12 }, (_, i) => ({
      ...fixtures.cartItem,
      instrument_id: `inst-bulk-${i}`,
      sn: `SN-BULK-${i}`,
      name: `批量乐器${i}`,
    }))
    window.localStorage.setItem('cart_user-1', JSON.stringify({ items }))
    const { container } = renderCart()
    const del = await waitFor(() => {
      const found = container.querySelector('[data-testid="cart-batch-delete"]')
      expect(found).toBeTruthy()
      return found
    })
    expect(del.textContent).toBe('删除所选(12)')
    expect(del.style.flexShrink).toBe('0')
    expect(del.style.whiteSpace).toBe('nowrap')
    emit('CART_HEADER_DELETE_TEXT_FULL=1')
    emit('CART_HEADER_DELETE_NO_SHRINK=1')

    const title = container.querySelector('[data-testid="cart-header-title"]')
    expect(title.textContent).toBe('购物车')
    expect(title.style.flex).toContain('1 1 0%')
    // jsdom 对无单位 0 归一化为 '0'（真机/浏览器为 '0px'），两者都表示「可收缩到 0」
    expect(['0px', '0']).toContain(title.style.minWidth)
    emit('CART_HEADER_TITLE_YIELDS=1')
  })

  it('R2-5 渲染无致命错误', async () => {
    const { container } = renderCart()
    await waitFor(() => {
      expect(header(container)).toBeTruthy()
    })
    emit('CART_HEADER_RENDER_FAIL=0')
  })
})
