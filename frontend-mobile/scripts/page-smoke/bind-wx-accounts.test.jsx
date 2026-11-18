import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, waitFor, cleanup } from '@testing-library/react'

const platform = vi.hoisted(() => ({
  wxLogin: vi.fn(),
  request: vi.fn(),
}))

vi.mock('../../src/platform', () => ({
  wxLogin: platform.wxLogin,
  request: platform.request,
  storage: { getItem: vi.fn(), setItem: vi.fn(), removeItem: vi.fn() },
  env: { apiBaseUrl: '/api' },
}))

const jsonResponse = (body) => ({
  json: async () => body,
})

let Bind

beforeEach(async () => {
  vi.clearAllMocks()
  platform.wxLogin.mockResolvedValue('code-1')
  const mod = await import('../../src/pages-weapp/bind/Bind')
  Bind = mod.default
})

afterEach(() => {
  cleanup()
})

describe('Bind page wx-accounts (#2409)', () => {
  it.each([
    ['no bound accounts', []],
    ['one bound account', [{ user_id: 'u-1' }]],
  ])('uses wx-accounts openid with %s and confirms', async (_label, accounts) => {
    window.history.replaceState({}, '', '/?scene=bind_abcd1234abcd1234')
    platform.request.mockImplementation(async (url) => {
      const target = String(url)
      if (target.includes('/auth/wx-accounts?code=code-1')) {
        return jsonResponse({
          code: 20000,
          data: { openid: 'o-1', accounts, exchange_token: 'ex-1' },
        })
      }
      if (target.includes('/wechat-bind/confirm')) {
        return jsonResponse({ code: 20000 })
      }
      throw new Error(`unexpected request: ${target}`)
    })

    render(<Bind />)

    await waitFor(() => expect(document.body.textContent).toContain('绑定成功'))

    const accountsCall = platform.request.mock.calls.find(([url]) =>
      String(url).includes('/auth/wx-accounts?code=code-1'))
    expect(accountsCall, 'must call wx-accounts with the wx.login code').toBeTruthy()

    const confirmCall = platform.request.mock.calls.find(([url]) =>
      String(url).includes('/wechat-bind/confirm'))
    expect(confirmCall, 'must call confirm after obtaining openid').toBeTruthy()
    expect(JSON.parse(confirmCall[1].body)).toEqual({
      token: 'abcd1234abcd1234',
      wx_openid: 'o-1',
      exchange_token: 'ex-1',
    })
  })

  it.each([
    ['non-20000 response', { code: 50000, message: 'failed' }],
    ['missing openid', { code: 20000, data: { accounts: [] } }],
  ])('shows failure and skips confirm on %s', async (_label, accountsBody) => {
    window.history.replaceState({}, '', '/?token=abcd1234abcd1234')
    platform.request.mockResolvedValue(jsonResponse(accountsBody))

    render(<Bind />)

    await waitFor(() => expect(document.body.textContent).toContain('绑定失败'))
    const confirmCall = platform.request.mock.calls.find(([url]) =>
      String(url).includes('/wechat-bind/confirm'))
    expect(confirmCall, 'must not call confirm without a real openid').toBeFalsy()
  })
})
