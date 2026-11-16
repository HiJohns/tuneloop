import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { act, cleanup, fireEvent, screen } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import Taro from '@tarojs/taro'

import H5Home from '../../src/pages/Home'
import WeappHome from '../../src/pages-weapp/Home'
import {
  SORT_MODE,
  SORT_LABEL,
  nextSortMode,
  computeMenuAvailWidth,
  getHomeDisplayPrice,
  normalizeHomeInstrumentPages,
  shouldFetchNextPage,
  sortHomeInstruments,
} from '../../src/utils/homeSort'
import { captureRender } from './harness'

const CATEGORIES = [
  { id: 'cat-guitar', name: '吉他', parent_id: null, sort: 1 },
  { id: 'cat-piano', name: '钢琴', parent_id: null, sort: 2 },
]

const INSTRUMENTS = [
  {
    id: 'high',
    sn: 'SN-B',
    name: '高价琴',
    category_name: '吉他',
    stock_status: 'available',
    daily_rate_cents: 3000,
    pricing: JSON.stringify({ tiers: [{ days_max: 7, daily_rate: 30 }] }),
    images: [],
  },
  {
    id: 'rented',
    sn: 'SN-D',
    name: '租赁琴',
    category_name: '吉他',
    stock_status: 'rented',
    daily_rate_cents: 900,
    pricing: JSON.stringify({ tiers: [{ days_max: 7, daily_rate: 9 }] }),
    images: [],
  },
  {
    id: 'low',
    sn: 'SN-A',
    name: '低价琴',
    category_name: '吉他',
    stock_status: 'available',
    daily_rate_cents: 1000,
    pricing: JSON.stringify({ tiers: [{ days_max: 7, daily_rate: 6 }] }),
    images: [],
  },
  {
    id: 'unpriced',
    sn: 'SN-C',
    name: '无价琴',
    category_name: '吉他',
    stock_status: 'available',
    daily_rate_cents: 0,
    pricing: '',
    images: [],
  },
  {
    id: 'tie',
    sn: 'SN-E',
    name: '同价琴',
    category_name: '吉他',
    stock_status: 'available',
    daily_rate_cents: 600,
    pricing: JSON.stringify({ tiers: [{ days_max: 7, daily_rate: 6 }] }),
    images: [],
  },
]

const DEFAULT_ORDER = ['高价琴', '租赁琴', '低价琴', '无价琴', '同价琴']
const ASC_ORDER = ['低价琴', '同价琴', '高价琴', '无价琴', '租赁琴']
const DESC_ORDER = ['高价琴', '低价琴', '同价琴', '无价琴', '租赁琴']

let instrumentRequests = []
let failPriceFetch = false

function response(data) {
  return {
    ok: true,
    status: 200,
    headers: { get: () => 'application/json' },
    json: async () => ({ code: 20000, data, message: 'success' }),
    text: async () => JSON.stringify({ code: 20000, data }),
    clone() { return this },
  }
}

function installHomeFetch() {
  instrumentRequests = []
  failPriceFetch = false
  globalThis.fetch = vi.fn(async (input) => {
    const url = String(input)
    if (url.includes('/public/categories')) return response({ list: CATEGORIES })
    if (url.includes('/public/banners')) return response({ list: [] })
    if (url.includes('/public/instruments?')) {
      instrumentRequests.push(url)
      const parsed = new URL(url, 'http://localhost')
      const category = parsed.searchParams.get('category_id')
      const pageSize = Number(parsed.searchParams.get('pageSize')) || 0
      if (failPriceFetch && pageSize === 100) {
        return { ...response({}), json: async () => ({ code: 50000, message: 'failed' }) }
      }
      const list = category ? INSTRUMENTS.filter(item => item.id === 'high') : INSTRUMENTS
      return response({ list, total: list.length, page: 1, pageSize })
    }
    return response({ list: [] })
  })
}

function setViewportWidth(width) {
  Object.defineProperty(window, 'innerWidth', { writable: true, configurable: true, value: width })
  Object.defineProperty(window, 'innerHeight', { writable: true, configurable: true, value: 667 })
}

function sleep(ms) {
  return new Promise(resolve => setTimeout(resolve, ms))
}

async function renderHome(Page) {
  window.history.replaceState({}, '', '/')
  const errors = await captureRender(
    <MemoryRouter initialEntries={['/']}>
      <Routes>
        <Route path="/" element={<Page />} />
      </Routes>
    </MemoryRouter>,
    100
  )
  expect(errors, errors.join('\n\n')).toEqual([])
}

function visibleOrder() {
  const names = new Set([...DEFAULT_ORDER, ...ASC_ORDER, ...DESC_ORDER])
  return Array.from(document.querySelectorAll('span'))
    .map(node => node.textContent)
    .filter(text => names.has(text))
}

function sortPill(label = SORT_LABEL[SORT_MODE.DEFAULT]) {
  const text = screen.getByText(label)
  const pill = text.closest('div')
  expect(pill).toBeTruthy()
  return pill
}

async function clickSort(label) {
  const pill = sortPill(label)
  await act(async () => {
    fireEvent.click(pill)
    await sleep(80)
  })
}

describe('homeSort', () => {
  it('cycles default → asc → desc → default', () => {
    expect(nextSortMode(SORT_MODE.DEFAULT)).toBe(SORT_MODE.ASC)
    expect(nextSortMode(SORT_MODE.ASC)).toBe(SORT_MODE.DESC)
    expect(nextSortMode(SORT_MODE.DESC)).toBe(SORT_MODE.DEFAULT)
  })

  it('computes the category viewport for 375/320 widths', () => {
    expect(computeMenuAvailWidth(375)).toBe(291)
    expect(computeMenuAvailWidth(320)).toBe(236)
    expect(computeMenuAvailWidth(0)).toBe(0)
  })

  it('uses the tier minimum first and falls back to daily_rate_cents', () => {
    expect(getHomeDisplayPrice(INSTRUMENTS[2])).toEqual({ value: 6, showFrom: true })
    expect(getHomeDisplayPrice({ daily_rate_cents: 750 })).toEqual({ value: 7.5, showFrom: false })
    expect(getHomeDisplayPrice(INSTRUMENTS[3])).toEqual({ value: null, showFrom: false })
  })

  it('fetches the next page only while total remains and below the cap', () => {
    expect(shouldFetchNextPage({ page: 1, pageSize: 100, total: 250, received: 100 })).toBe(true)
    expect(shouldFetchNextPage({ page: 2, pageSize: 100, total: 250, received: 200 })).toBe(true)
    expect(shouldFetchNextPage({ page: 3, pageSize: 100, total: 250, received: 250 })).toBe(false)
    expect(shouldFetchNextPage({ page: 100, pageSize: 100, total: 20000, received: 10000 })).toBe(false)
    expect(shouldFetchNextPage({ page: 1, pageSize: 100, total: 0, received: 0 })).toBe(false)
  })

  it('deduplicates and filters archived/lost without mutating pages', () => {
    const pages = [
      [INSTRUMENTS[0], INSTRUMENTS[2]],
      [INSTRUMENTS[2], { id: 'archived', stock_status: 'archived' }, { id: 'lost', stock_status: 'lost' }, INSTRUMENTS[4]],
    ]
    const before = pages.map(page => [...page])
    expect(normalizeHomeInstrumentPages(pages).map(item => item.id)).toEqual(['high', 'low', 'tie'])
    expect(pages).toEqual(before)
  })

  it('sorts available priced first, invalid prices before unavailable, ties by SN', () => {
    const input = [...INSTRUMENTS]
    const asc = sortHomeInstruments(input, SORT_MODE.ASC)
    const desc = sortHomeInstruments(input, SORT_MODE.DESC)
    expect(asc.map(item => item.name)).toEqual(ASC_ORDER)
    expect(desc.map(item => item.name)).toEqual(DESC_ORDER)
    expect(sortHomeInstruments(input, SORT_MODE.DEFAULT).map(item => item.name)).toEqual(DEFAULT_ORDER)
    expect(input.map(item => item.name)).toEqual(DEFAULT_ORDER)
  })
})

describe('H5 home price sort', () => {
  beforeEach(() => {
    setViewportWidth(375)
    installHomeFetch()
  })

  afterEach(() => {
    cleanup()
    setViewportWidth(375)
  })

  it('toggles all three states, preserves mode across categories, and resets on remount', async () => {
    await renderHome(H5Home)
    expect(visibleOrder()).toEqual(DEFAULT_ORDER)
    expect(instrumentRequests.length).toBe(1)
    expect(instrumentRequests[0]).toContain('pageSize=50')

    await clickSort(SORT_LABEL[SORT_MODE.DEFAULT])
    expect(screen.getByText(SORT_LABEL[SORT_MODE.ASC])).toBeTruthy()
    expect(visibleOrder()).toEqual(ASC_ORDER)
    expect(instrumentRequests[instrumentRequests.length - 1]).toContain('pageSize=100')

    const beforeDesc = instrumentRequests.length
    await clickSort(SORT_LABEL[SORT_MODE.ASC])
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DESC])).toBeTruthy()
    expect(visibleOrder()).toEqual(DESC_ORDER)
    expect(instrumentRequests.length).toBe(beforeDesc)

    await act(async () => {
      fireEvent.click(screen.getByText('钢琴'))
      await sleep(80)
    })
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DESC])).toBeTruthy()
    expect(instrumentRequests[instrumentRequests.length - 1]).toContain('category_id=cat-piano')

    await clickSort(SORT_LABEL[SORT_MODE.DESC])
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DEFAULT])).toBeTruthy()
    expect(instrumentRequests[instrumentRequests.length - 1]).toContain('pageSize=50')

    cleanup()
    await renderHome(H5Home)
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DEFAULT])).toBeTruthy()
    expect(visibleOrder()).toEqual(DEFAULT_ORDER)
  })

  it('rolls back the label and list when the full price load fails', async () => {
    const alertSpy = vi.spyOn(window, 'alert').mockImplementation(() => {})
    await renderHome(H5Home)
    failPriceFetch = true

    await clickSort(SORT_LABEL[SORT_MODE.DEFAULT])

    expect(screen.getByText(SORT_LABEL[SORT_MODE.DEFAULT])).toBeTruthy()
    expect(visibleOrder()).toEqual(DEFAULT_ORDER)
    expect(alertSpy).toHaveBeenCalled()
    alertSpy.mockRestore()
  })
})

describe('weapp home price sort', () => {
  beforeEach(() => {
    setViewportWidth(375)
    installHomeFetch()
  })

  afterEach(() => {
    cleanup()
    setViewportWidth(375)
  })

  it('toggles all three states and keeps mode across categories', async () => {
    await renderHome(WeappHome)
    expect(visibleOrder()).toEqual(DEFAULT_ORDER)
    expect(instrumentRequests.length).toBe(1)
    expect(instrumentRequests[0]).toContain('pageSize=50')

    await clickSort(SORT_LABEL[SORT_MODE.DEFAULT])
    expect(screen.getByText(SORT_LABEL[SORT_MODE.ASC])).toBeTruthy()
    expect(visibleOrder()).toEqual(ASC_ORDER)
    expect(instrumentRequests[instrumentRequests.length - 1]).toContain('pageSize=100')

    const beforeDesc = instrumentRequests.length
    await clickSort(SORT_LABEL[SORT_MODE.ASC])
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DESC])).toBeTruthy()
    expect(visibleOrder()).toEqual(DESC_ORDER)
    expect(instrumentRequests.length).toBe(beforeDesc)

    await act(async () => {
      fireEvent.click(screen.getByText('钢琴'))
      await sleep(80)
    })
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DESC])).toBeTruthy()
    expect(instrumentRequests[instrumentRequests.length - 1]).toContain('category_id=cat-piano')

    await clickSort(SORT_LABEL[SORT_MODE.DESC])
    expect(screen.getByText(SORT_LABEL[SORT_MODE.DEFAULT])).toBeTruthy()
    expect(instrumentRequests[instrumentRequests.length - 1]).toContain('pageSize=50')
  })
})

describe('home price sort layout', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    setViewportWidth(375)
  })

  it('keeps the 52px pill clear of the category viewport at 375 and 320', async () => {
    installHomeFetch()
    setViewportWidth(375)
    await renderHome(H5Home)
    const wideScroller = Array.from(document.querySelectorAll('div')).find(node => node.style.width === '291px')
    const widePill = Array.from(document.querySelectorAll('div')).find(node => node.style.width === '52px' && node.style.position === 'absolute')
    expect(wideScroller).toBeTruthy()
    expect(widePill).toBeTruthy()
    expect(16 + 291).toBeLessThanOrEqual(375 - 8 - 52)

    cleanup()
    installHomeFetch()
    setViewportWidth(320)
    await renderHome(H5Home)
    const narrowScroller = Array.from(document.querySelectorAll('div')).find(node => node.style.width === '236px')
    const narrowPill = Array.from(document.querySelectorAll('div')).find(node => node.style.width === '52px' && node.style.position === 'absolute')
    expect(narrowScroller).toBeTruthy()
    expect(narrowPill).toBeTruthy()
    expect(16 + 236).toBeLessThanOrEqual(320 - 8 - 52)
  })

  it('keeps the 52px pill clear of the category viewport in weapp at 375 and 320', async () => {
    const renderWeappAt = async (width) => {
      vi.spyOn(Taro, 'getSystemInfoSync').mockReturnValue({
        windowWidth: width,
        windowHeight: 667,
        statusBarHeight: 20,
        platform: 'devtools',
      })
      vi.spyOn(Taro, 'getMenuButtonBoundingClientRect').mockReturnValue({
        top: 10,
        height: 32,
        right: width - 10,
        left: width - 75,
        width: 65,
      })
      installHomeFetch()
      await renderHome(WeappHome)
    }
    const findWidth = (width) => Array.from(document.querySelectorAll('div')).find(node => node.style.width === width)
    const findPill = () => Array.from(document.querySelectorAll('div')).find(node => node.style.width === '52px' && node.style.position === 'absolute')

    await renderWeappAt(375)
    expect(findWidth('291px')).toBeTruthy()
    expect(findPill()).toBeTruthy()
    expect(16 + 291).toBeLessThanOrEqual(375 - 8 - 52)

    cleanup()
    await renderWeappAt(320)
    expect(findWidth('236px')).toBeTruthy()
    expect(findPill()).toBeTruthy()
    expect(16 + 236).toBeLessThanOrEqual(320 - 8 - 52)
  })
})
