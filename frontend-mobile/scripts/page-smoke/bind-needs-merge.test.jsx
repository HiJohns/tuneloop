import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, waitFor, cleanup, fireEvent } from '@testing-library/react'

const platform = vi.hoisted(() => ({
  wxLogin: vi.fn(),
  request: vi.fn(),
  storage: { getItem: vi.fn(), setItem: vi.fn(), removeItem: vi.fn() },
  session: { getItem: vi.fn(), setItem: vi.fn(), removeItem: vi.fn() },
  navigation: { safeBack: vi.fn(), navigateTo: vi.fn(), redirect: vi.fn(), getOrigin: vi.fn() },
}))

vi.mock('../../src/platform', () => ({
  wxLogin: platform.wxLogin,
  request: platform.request,
  storage: platform.storage,
  session: platform.session,
  navigation: platform.navigation,
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

const needsMergeConfirm = (ticket) => ({
  code: 40900,
  message: '该微信已绑定其他账户，需要合并',
  data: {
    needs_merge: true,
    existing_user_id: 'u-y',
    target_user_id: 'u-x',
    openid: 'o-1',
    merge_ticket: ticket,
  },
})

describe('Bind page needs_merge (#2445)', () => {
  it('shows the in-page merge confirmation and never claims success before merging', async () => {
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
        return jsonResponse(needsMergeConfirm('ticket-2445'))
      }
      throw new Error(`unexpected request: ${target}`)
    })

    render(<Bind />)

    await waitFor(() => expect(document.body.textContent).toContain('合并到我的个人账户'))
    expect(document.body.textContent).toContain('员工账户作废')
    expect(document.body.textContent).not.toContain('绑定成功')

    const confirmCall = platform.request.mock.calls.find(([url]) =>
      String(url).includes('/wechat-bind/confirm'))
    expect(confirmCall, 'must call confirm with the exchange token').toBeTruthy()
    const body = JSON.parse(confirmCall[1].body)
    expect(body.exchange_token).toBe('ex-1')
    expect(body.token).toBe('abcd1234abcd1234')
  })

  it('shows success after merge (and does not show the old unreachable 编辑资料 path)', async () => {
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
        return jsonResponse(needsMergeConfirm('ticket-2445'))
      }
      if (target.includes('/wechat-bind/merge')) {
        return jsonResponse({
          code: 20000,
          data: { merged: true, to_user_id: 'u-y', needs_credentials: true, suggested_username: '13900002445' },
        })
      }
      throw new Error(`unexpected request: ${target}`)
    })

    render(<Bind />)
    await waitFor(() => expect(document.body.textContent).toContain('合并到我的个人账户'))
    fireEvent.click(document.body.querySelector('button') || document.body)

    await waitFor(() => expect(document.body.textContent).toContain('合并成功'))
    expect(document.body.textContent).toContain('切换身份')
    expect(document.body.textContent).not.toContain('编辑资料')

    const mergeCall = platform.request.mock.calls.find(([url]) =>
      String(url).includes('/wechat-bind/merge'))
    expect(mergeCall, 'must call merge with token + ticket').toBeTruthy()
    const body = JSON.parse(mergeCall[1].body)
    expect(body.token).toBe('abcd1234abcd1234')
    expect(body.merge_ticket).toBe('ticket-2445')
  })
})
