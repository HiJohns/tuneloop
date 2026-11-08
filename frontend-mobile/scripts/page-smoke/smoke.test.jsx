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

  it('captures the #2298 t-shadow defect (red-green)', async () => {
    const { DefectiveTier } = await import('./red-green.fixture')
    const errors = await captureRender(<DefectiveTier />, 300, { silent: true })
    expect(errors.join('\n')).toMatch(/t is not a function|TypeError/)
  })
})
