import { beforeAll, afterAll, beforeEach, vi } from 'vitest'
import { fixtures } from './fixtures'
import { installFetchMock, makeToken } from './mock-platform'

const token = makeToken()

beforeAll(() => {
  installFetchMock()

  Object.defineProperty(window, 'matchMedia', {
    writable: true,
    value: vi.fn((query) => ({
      matches: false,
      media: query,
      onchange: null,
      addListener: vi.fn(),
      removeListener: vi.fn(),
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  })

  global.ResizeObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  global.IntersectionObserver = class {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  window.scrollTo = vi.fn()
  vi.spyOn(console, 'log').mockImplementation(() => {})
  vi.spyOn(console, 'debug').mockImplementation(() => {})
})

afterAll(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

beforeEach(() => {
  window.localStorage.clear()
  window.sessionStorage.clear()
  window.localStorage.setItem('token', token)
  window.localStorage.setItem('token_expiry', String(Date.now() + 86400000))
  window.localStorage.setItem('cart_user-1', JSON.stringify({ items: [fixtures.cartItem] }))
  window.localStorage.setItem('cart', JSON.stringify({ items: [] }))
  window.history.replaceState({}, '', '/')
  global.IS_REACT_ACT_ENVIRONMENT = true
})
