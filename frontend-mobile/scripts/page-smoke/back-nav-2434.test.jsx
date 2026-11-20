// #2434：扫码/分享直达页没有页面栈 → navigateBack 静默失败。
// 统一出口 decideBackAction：栈深 > 1 才返回上一页，否则回首页（tabBar 必须 switchTab）。
import { describe, it, expect } from 'vitest'
import { decideBackAction, WEAPP_HOME_ROUTE } from '../../src/utils/backNav.mjs'

describe('decideBackAction (#2434)', () => {
  it('栈深 1（扫码直达）→ switchTab 首页', () => {
    expect(decideBackAction({ stackDepth: 1 })).toEqual({ type: 'switchTab', url: WEAPP_HOME_ROUTE })
  })

  it('栈深 0 / 未定义 → 仍回首页（负例：裸 navigateBack 会静默失败）', () => {
    expect(decideBackAction({ stackDepth: 0 })).toEqual({ type: 'switchTab', url: WEAPP_HOME_ROUTE })
    expect(decideBackAction({})).toEqual({ type: 'switchTab', url: WEAPP_HOME_ROUTE })
  })

  it('栈深 > 1 → navigateBack（行为不变）', () => {
    expect(decideBackAction({ stackDepth: 2 })).toEqual({ type: 'navigateBack' })
    expect(decideBackAction({ stackDepth: 5 })).toEqual({ type: 'navigateBack' })
  })

  it('支持自定义兜底路由（如「我的」）', () => {
    expect(decideBackAction({ stackDepth: 1, fallbackRoute: '/pages-weapp/profile/index' }))
      .toEqual({ type: 'switchTab', url: '/pages-weapp/profile/index' })
  })
})

// #2434 R4：返回判定表（审计挑战：同级重复打开 / redirectTo 直达 / 正常返回）。
describe('R4 判定表 (#2434)', () => {
  it('depth 1（扫码/redirectTo）→ switchTab 首页', () => {
    expect(decideBackAction({ stackDepth: 1, currentRoute: 'pages-weapp/bind/index' }))
      .toEqual({ type: 'switchTab', url: WEAPP_HOME_ROUTE })
  })

  it('depth 2，上一页不同 → navigateBack', () => {
    expect(decideBackAction({
      stackDepth: 2,
      previousRoute: 'pages-weapp/home/index',
      currentRoute: 'pages-weapp/bind/index',
    })).toEqual({ type: 'navigateBack' })
  })

  it('depth 2，上一页同名（重复打开）→ switchTab 首页', () => {
    expect(decideBackAction({
      stackDepth: 2,
      previousRoute: 'pages-weapp/login/index',
      currentRoute: 'pages-weapp/login/index',
    })).toEqual({ type: 'switchTab', url: WEAPP_HOME_ROUTE })
  })

  it('depth 5，上一页不同 → navigateBack', () => {
    expect(decideBackAction({
      stackDepth: 5,
      previousRoute: 'pages-weapp/order-detail/index',
      currentRoute: 'pages-weapp/payment/index',
    })).toEqual({ type: 'navigateBack' })
  })
})
