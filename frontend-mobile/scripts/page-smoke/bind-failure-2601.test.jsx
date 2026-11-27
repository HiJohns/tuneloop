import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { render, waitFor, cleanup } from '@testing-library/react'

// #2601 R4：Bind 页失败态文案必须来自解析层（services/errorMessages.js），
// 不得把持久失败（500/50015 配置异常）表达成可重试的「请重试」。
//
// 契约 token 由 console.info 输出（setup.js 只静音 console.log/debug），
// scripts/verify-2601.sh r4 逐行校验：
//   BIND_UI_NEEDS_MERGE_RENDER / BIND_UI_50015_CONFIG_TEXT / BIND_UI_50015_NO_RETRY_HINT /
//   BIND_UI_50200_RETRY_TEXT / BIND_UI_40002_SERVER_TEXT / BIND_UI_RENDER_FAIL

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
let renderFailed = 0

const TOKEN = '2601abcd2601abcd'

beforeEach(async () => {
  vi.clearAllMocks()
  platform.wxLogin.mockResolvedValue('code-2601')
  const mod = await import('../../src/pages-weapp/bind/Bind')
  Bind = mod.default
})

afterEach(() => {
  cleanup()
})

function mockConfirm(confirmBody) {
  platform.request.mockImplementation(async (url) => {
    const target = String(url)
    if (target.includes('/auth/wx-accounts')) {
      return jsonResponse({ code: 20000, data: { openid: 'o-2601', exchange_token: 'ex-2601' } })
    }
    if (target.includes('/wechat-bind/confirm')) {
      return jsonResponse(confirmBody)
    }
    throw new Error(`unexpected request: ${target}`)
  })
}

async function renderWith(confirmBody) {
  window.history.replaceState({}, '', `/?token=${TOKEN}`)
  mockConfirm(confirmBody)
  try {
    const view = render(<Bind />)
    await waitFor(() => expect(document.body.textContent).not.toContain('正在绑定'))
    return view
  } catch (err) {
    renderFailed = 1
    throw err
  }
}

describe('Bind page failure texts (#2601)', () => {
  it('keeps needs_merge as merge guidance, never as a failure', async () => {
    await renderWith({
      code: 40900,
      message: '该微信已绑定其他账户，需要合并',
      data: {
        needs_merge: true,
        existing_user_id: 'u-y-2601',
        target_user_id: 'u-x-2601',
        openid: 'o-2601',
        merge_ticket: 'ticket-2601',
      },
    })
    expect(document.body.textContent).toContain('合并到我的个人账户')
    expect(document.body.textContent).not.toContain('绑定失败')
    console.info('BIND_UI_NEEDS_MERGE_RENDER=1')
  })

  it('shows the persistent config text for 500/50015 and no retry hint', async () => {
    await renderWith({ code: 50015, message: '身份服务配置异常，请联系平台管理员处理' })
    await waitFor(() => expect(document.body.textContent).toContain('绑定失败'))
    expect(document.body.textContent).toContain('身份服务配置异常，请联系平台管理员处理')
    expect(document.body.textContent).not.toContain('请重试')
    expect(document.body.textContent).not.toContain('请稍后')
    console.info('BIND_UI_50015_CONFIG_TEXT=1')
    console.info('BIND_UI_50015_NO_RETRY_HINT=1')
  })

  it('shows a retry hint for the transient 502/50200', async () => {
    await renderWith({ code: 50200, message: '绑定失败，请稍后再试' })
    await waitFor(() => expect(document.body.textContent).toContain('绑定失败'))
    expect(document.body.textContent).toContain('请稍后')
    expect(document.body.textContent).not.toContain('身份服务配置异常')
    console.info('BIND_UI_50200_RETRY_TEXT=1')
  })

  it('passes through the backend CJK message for 400/40002 and stays out of merge flow', async () => {
    await renderWith({ code: 40002, message: '微信授权已失效，请重新进入页面重试' })
    await waitFor(() => expect(document.body.textContent).toContain('绑定失败'))
    expect(document.body.textContent).toContain('微信授权已失效，请重新进入页面重试')
    expect(document.body.textContent).not.toContain('合并到我的个人账户')
    console.info('BIND_UI_40002_SERVER_TEXT=1')
  })

  it('never crashes while rendering a failure state', () => {
    expect(renderFailed).toBe(0)
    console.info('BIND_UI_RENDER_FAIL=0')
  })
})
