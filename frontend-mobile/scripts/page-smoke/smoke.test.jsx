import { describe, it, expect, afterEach } from 'vitest'
import { cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import { captureRender } from './harness'
import { criticalRoutes, discoverH5Routes, discoverWeappPages } from './routes'

describe('mobile page render smoke', () => {
  afterEach(() => cleanup())

  it('discovers H5 routes and weapp pages from app config', () => {
    expect(discoverH5Routes().length).toBeGreaterThan(10)
    expect(discoverWeappPages().length).toBeGreaterThan(10)
  })

  for (const route of criticalRoutes) {
    it(`${route.name} renders without fatal errors`, async () => {
      window.history.replaceState({}, '', route.entry)
      const { default: Page } = await route.load()
      const errors = await captureRender(
        <MemoryRouter initialEntries={[route.entry]}>
          <Routes>
            <Route path={route.pattern} element={<Page />} />
          </Routes>
        </MemoryRouter>
      )
      expect(errors, `${route.name} fatal render errors:\n${errors.join('\n\n')}`).toEqual([])
      for (const marker of (route.markers || [route.marker])) {
        expect(document.body.textContent, `${route.name} did not render marker "${marker}"`).toContain(marker)
      }
    })
  }


  // #2327 S4/R2: 支付页信用态——未 approved 时必须出现评测入口且支付按钮禁用文案可见。
  it('payment credit gate renders entry when credit is not approved', async () => {
    const originalFetch = globalThis.fetch
    const jsonResp = (body) => ({
      ok: true,
      status: 200,
      headers: { get: () => 'application/json' },
      json: async () => body,
    })
    globalThis.fetch = async (url) => {
      const u = String(url)
      if (u.includes('/pay/calculate')) {
        return jsonResp({
          code: 20000,
          data: {
            type: 'rent',
            title: '租赁支付',
            amount: 35400,
            details: { pricing_breakdown: '{}' },
            wallet: null,
          },
        })
      }
      if (u.includes('/credit-status')) {
        return jsonResp({
          code: 20000,
          data: {
            requires_assessment: true,
            credit_state: 'pending',
            deposit_tier: 'reduced',
            risk_amount_cents: 30000,
          },
        })
      }
      return jsonResp({ code: 20000, data: {} })
    }
    try {
      window.history.replaceState({}, '', '/payment?type=rent&id=order-1')
      const { default: Payment } = await import('../../src/pages/Payment')
      const errors = await captureRender(
        <MemoryRouter initialEntries={['/payment?type=rent&id=order-1']}>
          <Routes>
            <Route path="/payment" element={<Payment />} />
          </Routes>
        </MemoryRouter>,
        500
      )
      expect(errors).toEqual([])
      expect(document.body.textContent).toContain('微信信用评测')
      expect(document.body.textContent).toContain('请先完成微信信用评测')
    } finally {
      globalThis.fetch = originalFetch
    }
  })

  it('captures the #2298 t-shadow defect (red-green)', async () => {
    const { DefectiveTier } = await import('./red-green.fixture')
    const errors = await captureRender(<DefectiveTier />, 300, { silent: true })
    expect(errors.join('\n')).toMatch(/t is not a function|TypeError/)
  })
})
