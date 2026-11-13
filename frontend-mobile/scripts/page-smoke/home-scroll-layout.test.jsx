import { describe, it, expect } from 'vitest'

import {
  NAV_FALLBACK_HEIGHT,
  LOAD_MORE_THRESHOLD,
  computeBottomInset,
  computeContentMinHeight,
  shouldLoadMore,
} from '../../src/utils/scrollLayout'

describe('home scroll layout (#2343)', () => {
  it('computes the nav inset from the measured top', () => {
    expect(computeBottomInset({ windowHeight: 667, navTop: 607 })).toBe(60)
    expect(computeBottomInset({ windowHeight: 812, navTop: 741 })).toBe(71)
  })

  it('falls back to 60 for missing/invalid/nav-out-of-range input (never negative)', () => {
    expect(computeBottomInset({ windowHeight: 667 })).toBe(NAV_FALLBACK_HEIGHT)
    expect(computeBottomInset({ windowHeight: NaN, navTop: 600 })).toBe(NAV_FALLBACK_HEIGHT)
    expect(computeBottomInset({ windowHeight: 667, navTop: -1 })).toBe(NAV_FALLBACK_HEIGHT)
    expect(computeBottomInset({ windowHeight: 667, navTop: 668 })).toBe(NAV_FALLBACK_HEIGHT)
    expect(computeBottomInset({ windowHeight: 667, navTop: 667 })).toBe(NAV_FALLBACK_HEIGHT)
  })

  it('keeps the content-taller-than-viewport invariant without a 2000px spacer', () => {
    expect(computeContentMinHeight(600)).toBe(601)
    expect(computeContentMinHeight(0)).toBe(0)
    expect(computeContentMinHeight(undefined)).toBe(0)
    // 旧模型的可滚动余量 = 2000 + viewport - viewport = 2000px；新模型必须不等于它。
    const oldMaxScroll = 2000 + 600 - 600
    expect(oldMaxScroll).toBe(2000)
    expect(computeContentMinHeight(600) - 600).not.toBe(oldMaxScroll)
  })

  it('triggers load-more exactly at the 300px threshold', () => {
    const base = { viewportHeight: 600, scrollHeight: 1201 }
    expect(shouldLoadMore({ ...base, scrollTop: 300 })).toBe(false)
    expect(shouldLoadMore({ ...base, scrollTop: 301 })).toBe(true)
    expect(shouldLoadMore({ ...base, scrollTop: 0, scrollHeight: 0 })).toBe(false)
    expect(shouldLoadMore({ ...base, scrollTop: 400, threshold: LOAD_MORE_THRESHOLD })).toBe(true)
  })
})
