import { vi } from 'vitest'
import { fixtures } from './fixtures'

const json = (data, status = 200) => {
  const payload = { code: status === 200 ? 20000 : 40000, data, message: status === 200 ? 'success' : 'mock error' }
  return {
    ok: status < 400,
    status,
    headers: { get: () => 'application/json' },
    json: async () => payload,
    text: async () => JSON.stringify(payload),
    clone() { return this },
  }
}

export function installFetchMock() {
  const fetchMock = vi.fn(async (input) => {
    const url = String(input)
    if (url.includes('/public/instruments/') && url.includes('/pricing-v2')) return json(fixtures.pricingV2)
    if (url.includes('/public/instruments/')) return json(fixtures.instrument)
    if (url.includes('/user/addresses')) return json({ list: fixtures.addresses })
    if (url.includes('/user/guarantors')) return json({ list: fixtures.guarantors })
    if (url.includes('/user/deposit-waiver/eligibility')) return json({ eligible: false, reasons: [] })
    if (url.includes('/users/me')) return json(fixtures.user)
    if (url.includes('/user/orders/counts')) return json(fixtures.counts)
    if (url.includes('/site-members/me')) return json(fixtures.siteMember)
    if (url.includes('/user/invitations/pending')) return json({ list: [] })
    if (url.includes('/user/pending-orders')) return json({ list: [] })
    if (url.includes('/repair-services')) return json({ list: [], total: 0 })
    if (url.includes('/pay/calculate')) return json(fixtures.payment)
    if (url.includes('/pay/prepay')) return json({ prepay: {}, amount: fixtures.payment.amount })
    if (url.includes('/pay/query')) return json({ status: 'paid' })
    if (url.includes('/orders/') && url.includes('/logs')) return json({ logs: [], total: 0 })
    if (url.includes('/orders/')) return json(fixtures.order)
    return json({ list: [] })
  })
  vi.stubGlobal('fetch', fetchMock)
  return fetchMock
}

export function makeToken() {
  const payload = {
    sub: 'user-1',
    user_id: 'user-1',
    id: 'user-1',
    role: 'STAFF',
    oid: 'org-1',
    tid: 'tenant-1',
    exp: Math.floor(Date.now() / 1000) + 86400,
  }
  return `smoke.${btoa(JSON.stringify(payload))}.signature`
}
