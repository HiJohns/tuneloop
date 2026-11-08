import { describe, it, expect } from 'vitest'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import Page from '../../src/pages/CheckoutAuditDefect.jsx'
import { captureRender } from './harness'

// Deliberate red proof: this spec is excluded from `npm run smoke` (*.test.jsx)
// and is run explicitly via `npm run smoke:defect` (which generates the defect
// copy of the real H5 Checkout first). It MUST fail while the injected
// t-shadow is present, proving the smoke harness catches the real Checkout
// page with tiers non-empty.
describe('H5 /checkout injected t-shadow (red proof)', () => {
  it('fails the smoke on the real Checkout copy with tiers', async () => {
    window.history.replaceState({}, '', '/checkout?id=inst-1')
    const errors = await captureRender(
      <MemoryRouter initialEntries={['/checkout?id=inst-1']}>
        <Routes>
          <Route path="/checkout" element={<Page />} />
        </Routes>
      </MemoryRouter>,
      500,
      { silent: true }
    )
    expect(errors, 'expected the injected t-shadow to crash H5 /checkout').toEqual([])
  })
})
