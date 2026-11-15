import { getMinTierDailyRateYuan } from './pricing'

export const SORT_MODE = Object.freeze({
  DEFAULT: 'default',
  ASC: 'asc',
  DESC: 'desc',
})

export const SORT_LABEL = Object.freeze({
  [SORT_MODE.DEFAULT]: '价格⇅',
  [SORT_MODE.ASC]: '价格↑',
  [SORT_MODE.DESC]: '价格↓',
})

export const MENU_SIDE_PADDING = 16
export const SORT_PILL_WIDTH = 52
export const SORT_PAGE_SIZE = 100
export const MAX_SORT_PAGES = 100

export function nextSortMode(mode) {
  if (mode === SORT_MODE.DEFAULT) return SORT_MODE.ASC
  if (mode === SORT_MODE.ASC) return SORT_MODE.DESC
  return SORT_MODE.DEFAULT
}

export function computeMenuAvailWidth(windowWidth) {
  const width = Number(windowWidth)
  if (!Number.isFinite(width) || width <= 0) return 0
  return Math.max(0, width - MENU_SIDE_PADDING * 2 - SORT_PILL_WIDTH)
}

export function getHomeDisplayPrice(instrument) {
  const minTierRate = getMinTierDailyRateYuan(instrument)
  const showFrom = minTierRate !== null && minTierRate > 0
  const cents = Number(instrument && instrument.daily_rate_cents)
  const fallbackRate = Number.isFinite(cents) && cents > 0 ? cents / 100 : 0
  const raw = showFrom ? minTierRate : fallbackRate
  const value = raw > 0 ? Number(raw.toFixed(2)) : null
  return { value, showFrom: showFrom && value !== null }
}

export function shouldFetchNextPage({ page, pageSize, total, received }) {
  const currentPage = Number(page)
  const size = Number(pageSize)
  const totalCount = Number(total)
  const receivedCount = Number(received)
  if (!Number.isFinite(currentPage) || currentPage <= 0) return false
  if (!Number.isFinite(size) || size <= 0) return false
  if (!Number.isFinite(totalCount) || totalCount <= 0) return false
  if (!Number.isFinite(receivedCount) || receivedCount < 0) return false
  return receivedCount < totalCount && currentPage < MAX_SORT_PAGES
}

export function normalizeHomeInstrumentPages(pages) {
  const seen = new Set()
  const normalized = []
  for (const page of pages || []) {
    for (const instrument of page || []) {
      if (!instrument || instrument.stock_status === 'archived' || instrument.stock_status === 'lost') continue
      const id = instrument.id
      if (id !== undefined && id !== null) {
        if (seen.has(id)) continue
        seen.add(id)
      }
      normalized.push(instrument)
    }
  }
  return normalized
}

function stableKey(instrument) {
  return `${instrument && instrument.sn ? instrument.sn : ''}|${instrument && instrument.id != null ? instrument.id : ''}`
}

function compareStable(a, b) {
  const ka = stableKey(a)
  const kb = stableKey(b)
  if (ka < kb) return -1
  if (ka > kb) return 1
  return 0
}

export function sortHomeInstruments(list, mode) {
  if (mode !== SORT_MODE.ASC && mode !== SORT_MODE.DESC) return [...(list || [])]

  const available = []
  const unavailable = []
  for (const instrument of list || []) {
    if (instrument && instrument.stock_status === 'available') available.push(instrument)
    else unavailable.push(instrument)
  }

  const priced = []
  const unpriced = []
  for (const instrument of available) {
    const { value } = getHomeDisplayPrice(instrument)
    if (value === null) unpriced.push(instrument)
    else priced.push({ instrument, value })
  }

  const direction = mode === SORT_MODE.ASC ? 1 : -1
  priced.sort((a, b) => {
    if (a.value !== b.value) return (a.value - b.value) * direction
    return compareStable(a.instrument, b.instrument)
  })
  unpriced.sort(compareStable)
  unavailable.sort(compareStable)

  return [...priced.map(entry => entry.instrument), ...unpriced, ...unavailable]
}
