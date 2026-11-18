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

const jsonResponse = (body) => ({ json: async () => body })

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

describe('Bind page needs_merge (#2410)', () => {
  it('shows merge guidance and never claims success on 409 needs_merge', async () => {
    window.history.replaceState({}, '', '/?token=abcd1234abcd1234')
    platform.request.mockImplementation(async (url) => {
      const target = String(url)
      if (target.includes('/auth/wx-accounts?code=code-1')) {
        return jsonResponse({
          code: 20000,
          data: { openid: 'o-1', accounts: [{ user_id: 'u-y' }], exchange_token: 'ex-1' },
        })
      }
      if (target.includes('/wechat-bind/confirm')) {
        return jsonResponse({
          code: 40900,
          message: '该微信已绑定其他账户，需要合并',
          data: {
            needs_merge: true,
            existing_user_id: 'u-y',
            target_user_id: 'u-x',
            openid: 'o-1',
          },
        })
      }
      throw new Error(`unexpected request: ${target}`)
    })

    render(<Bind />)

    await waitFor(() => expect(document.body.textContent).toContain('确认合并'))
    expect(document.body.textContent).toContain('编辑资料')
    expect(document.body.textContent).not.toContain('绑定成功')

    const confirmCall = platform.request.mock.calls.find(([url]) =>
      String(url).includes('/wechat-bind/confirm'))
    expect(confirmCall, 'must call confirm with the exchange token').toBeTruthy()
    const body = JSON.parse(confirmCall[1].body)
    expect(body.exchange_token).toBe('ex-1')
    expect(body.token).toBe('abcd1234abcd1234')
  })
})
