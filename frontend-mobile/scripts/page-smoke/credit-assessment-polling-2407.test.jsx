// #2407 回归：评测页在 pending 必须持续轮询，直到状态推进为 approved。
//
// 缺陷版（effect 依赖 credit_state/service_order_no + setCredit 续排）只会轮询 1 次，
// 之后 effect 不再重跑 → mock provider 30s 后通过也看不到，页面永远停在「评测进行中」。
import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, act, cleanup } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'

const jsonResp = (body) => ({
  ok: true,
  status: 200,
  headers: { get: () => 'application/json' },
  json: async () => body,
})

const creditPayload = (state) => ({
  code: 20000,
  data: {
    order_id: 'o-2407',
    deposit_tier: 'full',
    deposit_credit_path: 'payscore',
    credit_state: state,
    risk_amount_cents: 10000,
    service_order_no: 's-2407',
    mock: true,
    requires_assessment: true,
    credit_package: '',
    order_status: 'reserved',
    payment_deadline: null,
    credit_package_expire_at: null,
  },
})

describe('credit assessment polling (#2407)', () => {
  afterEach(() => {
    cleanup()
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('keeps polling pending and renders 评测通过 once approved', async () => {
    vi.useFakeTimers()
    let calls = 0
    globalThis.fetch = async (url) => {
      const u = String(url)
      if (u.includes('/credit-status')) {
        calls += 1
        // 前 2 次 pending，第 3 次通过（模拟 mock provider 的延迟出结果）
        return jsonResp(creditPayload(calls <= 2 ? 'pending' : 'approved'))
      }
      throw new Error(`unexpected fetch: ${u}`)
    }

    window.history.replaceState({}, '', '/credit-assessment?order_id=o-2407')
    const { default: Page } = await import('../../src/pages/CreditAssessment')

    await act(async () => {
      render(
        <MemoryRouter initialEntries={['/credit-assessment?order_id=o-2407']}>
          <Routes>
            <Route path="/credit-assessment" element={<Page />} />
          </Routes>
        </MemoryRouter>
      )
      await vi.advanceTimersByTimeAsync(50)
    })

    // 依次越过 1.5s / 3s 的轮询间隔（缺陷版在第 1 次轮询后就再无请求）
    for (let i = 0; i < 4; i += 1) {
      await act(async () => { await vi.advanceTimersByTimeAsync(4000) })
    }

    expect(calls, 'must keep polling while pending (defect: stops after 1 poll)').toBeGreaterThanOrEqual(3)
    expect(document.body.textContent).toContain('评测通过')
    expect(document.body.textContent).toContain('返回继续支付')
  })
})
