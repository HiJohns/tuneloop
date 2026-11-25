// #2502: 移动端「我的」身份徽标 = 商户名 · 网点名 · 职位（H5 与 weapp 同口径）。
//
// 该文件同时是 R1–R5 的可执行断言源：每个用例通过 process.stdout.write 输出
// `KEY=value` 机器行（scripts/page-smoke/setup.js 已把 console.log 打桩），
// 由 scripts/verify-2502.sh 逐行 grep。

import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, waitFor, cleanup } from '@testing-library/react'
import { MemoryRouter } from 'react-router-dom'
import Profile from '../../src/pages/Profile'
import WeappProfile from '../../src/pages-weapp/Profile'
import {
  formatOrgIdentity,
  roleBadgeTextFromRoles,
  identityBadgeText,
} from '../../src/utils/identity'

const out = (key, value) => process.stdout.write(`${key}=${value}\n`)

const jsonResponse = (data) => ({
  ok: true,
  status: 200,
  headers: { get: () => 'application/json' },
  json: async () => ({ code: 20000, data }),
  text: async () => '',
  clone() { return this },
})

const makeToken = (payload) => `smoke.${btoa(JSON.stringify(payload))}.signature`

const staffToken = () => makeToken({
  sub: 'user-1',
  user_id: 'user-1',
  role: 'STAFF',
  oid: 'org-1',
  tid: 'tenant-1',
  exp: Math.floor(Date.now() / 1000) + 86400,
})

const customerToken = () => makeToken({
  sub: 'user-2',
  user_id: 'user-2',
  role: 'USER',
  exp: Math.floor(Date.now() / 1000) + 86400,
})

let originalFetch

function installFetch({ user, member = { roles: [], sites: [] } }) {
  originalFetch = globalThis.fetch
  globalThis.fetch = vi.fn(async (input) => {
    const url = String(input)
    if (url.includes('/site-members/me')) return jsonResponse(member)
    if (url.includes('/users/me')) return jsonResponse(user)
    if (url.includes('/notifications/unread-count')) return jsonResponse({ count: 0 })
    if (url.includes('/user/orders/counts')) return jsonResponse({ reserved: 0, in_lease: 0, returning: 0, completed: 0 })
    if (url.includes('/repair-services')) return jsonResponse({ list: [], total: 0 })
    if (url.includes('/user/invitations/pending')) return jsonResponse({ list: [] })
    if (url.includes('/user/pending-orders')) return jsonResponse({ list: [] })
    return jsonResponse({ list: [] })
  })
}

function leafTexts() {
  return [...document.querySelectorAll('body *')]
    .filter((el) => el.children.length === 0)
    .map((el) => (el.textContent || '').trim())
    .filter(Boolean)
}

function hasExactLeaf(text) {
  return leafTexts().includes(text)
}

const renderWeapp = () => render(<WeappProfile />)
const renderH5 = () => render(
  <MemoryRouter initialEntries={['/profile']}>
    <Profile />
  </MemoryRouter>,
)

const STAFF_FIXTURE = { tenant_name: 'cadenza', site_name: '卡丹萨乐器' }
const STAFF_MEMBER = { roles: ['site_admin'], sites: [] }
const EXPECTED = 'cadenza · 卡丹萨乐器 · 管理员'

describe('profile identity badge (#2502)', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
    if (originalFetch) {
      globalThis.fetch = originalFetch
      originalFetch = undefined
    }
  })

  it('TestProfileBadgeWeapp_2502 renders 商户名 · 网点名 · 职位', async () => {
    window.localStorage.setItem('token', staffToken())
    installFetch({ user: STAFF_FIXTURE, member: STAFF_MEMBER })

    renderWeapp()
    await waitFor(() => expect(hasExactLeaf(EXPECTED)).toBe(true))

    // 负例：旧表达式（网点名优先）产出「卡丹萨乐器 · 管理员」，不得再出现。
    expect(hasExactLeaf('卡丹萨乐器 · 管理员')).toBe(false)
    expect(document.body.textContent).not.toContain('undefined')

    out('BADGE_TEXT', EXPECTED)
    out('OLD_TEXT_ABSENT', '1')
  })

  it('TestProfileBadgeWeappFallback_2502 falls back to a single org segment', async () => {
    // 只有网点名 → 「卡丹萨乐器 · 管理员」
    window.localStorage.setItem('token', staffToken())
    installFetch({ user: { site_name: '卡丹萨乐器' }, member: STAFF_MEMBER })
    renderWeapp()
    await waitFor(() => expect(hasExactLeaf('卡丹萨乐器 · 管理员')).toBe(true))
    expect(document.body.textContent).not.toContain('undefined')
    expect(leafTexts().some((t) => t.startsWith(' · '))).toBe(false)
    out('BADGE_TEXT', '卡丹萨乐器 · 管理员')

    cleanup()

    // 两者都缺 → 只显示职位「管理员」
    installFetch({ user: {}, member: STAFF_MEMBER })
    renderWeapp()
    await waitFor(() => expect(hasExactLeaf('管理员')).toBe(true))
    expect(document.body.textContent).not.toContain('undefined')
    expect(document.body.textContent).not.toContain('null')
    expect(leafTexts().some((t) => t.startsWith(' · '))).toBe(false)

    out('BADGE_TEXT_ONLY_ROLE', '管理员')
    out('NO_UNDEFINED', '1')
    out('NO_LEADING_SEP', '1')
  })

  it('TestProfileBadgeH5_2502 renders the same text as weapp', async () => {
    window.localStorage.setItem('token', staffToken())
    installFetch({ user: STAFF_FIXTURE, member: STAFF_MEMBER })

    renderH5()
    await waitFor(() => expect(hasExactLeaf(EXPECTED)).toBe(true))

    const parity = hasExactLeaf(EXPECTED) &&
      identityBadgeText({ tenantName: 'cadenza', siteName: '卡丹萨乐器', roles: ['site_admin'], isStaff: true }) === EXPECTED
    expect(parity).toBe(true)

    out('H5_BADGE_TEXT', EXPECTED)
    out('PARITY', parity ? '1' : '0')
  })

  it('TestProfileBadgeCustomer_2502 hides org identity for customers', async () => {
    window.localStorage.setItem('token', customerToken())
    // 故意让 /users/me 带组织名：顾客上下文不得渲染
    installFetch({ user: STAFF_FIXTURE, member: { roles: [], sites: [] } })

    renderH5()
    await waitFor(() => expect(document.body.textContent).toContain('路人'))
    expect(document.body.textContent).not.toContain('cadenza')
    expect(document.body.textContent).not.toContain('卡丹萨乐器')

    cleanup()

    renderWeapp()
    await waitFor(() => expect(document.body.textContent).toContain('路人'))
    expect(document.body.textContent).not.toContain('cadenza')
    expect(document.body.textContent).not.toContain('卡丹萨乐器')

    out('CUSTOMER_BADGE_ABSENT', '1')
    out('NO_TENANT_LEAK', '1')
  })

  it('TestProfileBadgeLegacyCustomer_2502 hides org identity for legacy USER+org tokens', async () => {
    // #2502 R13：遗留 token（role=USER 但带 oid/tid）+ /users/me 带组织名。
    // H5 的 isStaffRole 以 oid/tid 判定员工 → 徽标必须仍按 role 判定，两端都不得展示组织名。
    const legacyToken = makeToken({
      sub: 'user-3',
      user_id: 'user-3',
      role: 'USER',
      oid: 'org-1',
      tid: 'tenant-1',
      exp: Math.floor(Date.now() / 1000) + 86400,
    })
    window.localStorage.setItem('token', legacyToken)
    installFetch({ user: STAFF_FIXTURE, member: { roles: [], sites: [] } })

    renderH5()
    await waitFor(() => expect(document.body.textContent).toContain('路人'))
    const h5Text = document.body.textContent || ''
    expect(h5Text).not.toContain('cadenza')
    expect(h5Text).not.toContain('卡丹萨乐器')

    cleanup()

    renderWeapp()
    await waitFor(() => expect(document.body.textContent).toContain('路人'))
    const weappText = document.body.textContent || ''
    expect(weappText).not.toContain('cadenza')
    expect(weappText).not.toContain('卡丹萨乐器')

    const absentBothEnds = !h5Text.includes('cadenza') &&
      !h5Text.includes('卡丹萨乐器') &&
      !weappText.includes('cadenza') &&
      !weappText.includes('卡丹萨乐器')
    expect(absentBothEnds).toBe(true)

    out('ORG_ABSENT_BOTH_ENDS', absentBothEnds ? '1' : '0')
  })

  it('TestIdentityHelperRedGreen_2502 proves old text dropped the merchant name', () => {
    expect(formatOrgIdentity('a', 'b')).toBe('a · b')
    expect(formatOrgIdentity('', 'b')).toBe('b')
    expect(formatOrgIdentity(null, null)).toBe('')
    expect(roleBadgeTextFromRoles(['site_admin', 'repair_technician'], true)).toBe('管理员 · 维修师傅')
    expect(roleBadgeTextFromRoles([], true)).toBe('员工')
    expect(roleBadgeTextFromRoles(['site_admin'], false)).toBe('顾客')

    const tenant = 'cadenza'
    const site = '卡丹萨乐器'
    // 旧表达式（#2455 之前 weapp 徽标）：site_name || tenant_name
    const oldOrg = [[site || tenant]].flat().filter(Boolean).join(' · ')
    const newOrg = formatOrgIdentity(tenant, site)
    expect(oldOrg).toBe('卡丹萨乐器')
    expect(oldOrg.includes(tenant)).toBe(false)
    expect(newOrg).toBe('cadenza · 卡丹萨乐器')
    expect(oldOrg).not.toBe(newOrg)

    out('FORMAT_FULL', formatOrgIdentity('a', 'b'))
    out('FORMAT_SINGLE', formatOrgIdentity('', 'b'))
    out('FORMAT_EMPTY', formatOrgIdentity(null, null))
    out('ROLE_MULTI', roleBadgeTextFromRoles(['site_admin', 'repair_technician'], true))
    out('ROLE_FALLBACK', roleBadgeTextFromRoles([], true))
    out('OLD_TEXT', oldOrg)
    out('NEW_TEXT', newOrg)
    out('RED_GREEN', '1')
  })
})
