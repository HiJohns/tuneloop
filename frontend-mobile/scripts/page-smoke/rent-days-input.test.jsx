import { describe, it, expect, afterEach } from 'vitest'
import { cleanup, fireEvent, render, waitFor } from '@testing-library/react'
import { MemoryRouter, Routes, Route } from 'react-router-dom'
import Checkout from '../../src/pages/Checkout'
import Renewal from '../../src/pages/Renewal'

async function renderDaysInput(path, routePath, element, expectedValue = '30') {
  window.history.replaceState({}, '', path)
  render(
    <MemoryRouter initialEntries={[path]}>
      <Routes>
        <Route path={routePath} element={element} />
      </Routes>
    </MemoryRouter>
  )
  return await waitFor(() => {
    const inputs = document.querySelectorAll('input[type="number"]')
    if (inputs.length !== 1) {
      throw new Error(`expected 1 number input on ${path}, got ${inputs.length}`)
    }
    if (inputs[0].value !== expectedValue) {
      throw new Error(`expected initial days ${expectedValue} on ${path}, got ${inputs[0].value}`)
    }
    return inputs[0]
  })
}

describe('mobile rent days input direct editing (#2351)', () => {
  afterEach(() => cleanup())

  it('H5 Checkout keeps the field empty while clearing and retyping', async () => {
    const input = await renderDaysInput('/checkout?id=inst-1', '/checkout', <Checkout />)

    fireEvent.change(input, { target: { value: '' } })
    expect(input.value, 'cleared input must not be written back to 1').toBe('')

    fireEvent.change(input, { target: { value: '6' } })
    expect(input.value).toBe('6')

    fireEvent.change(input, { target: { value: '60' } })
    expect(input.value).toBe('60')

    fireEvent.blur(input)
    expect(input.value).toBe('60')
  })

  it('H5 Checkout normalizes empty and over-limit values on blur', async () => {
    const input = await renderDaysInput('/checkout?id=inst-1', '/checkout', <Checkout />)

    fireEvent.change(input, { target: { value: '' } })
    fireEvent.blur(input)
    expect(input.value).toBe('1')

    fireEvent.change(input, { target: { value: '999' } })
    expect(input.value).toBe('999')
    fireEvent.blur(input)
    expect(input.value).toBe('730')
  })

  it('H5 Renewal keeps transient text and normalizes empty on blur', async () => {
    const input = await renderDaysInput('/renewal?order_id=order-1', '/renewal', <Renewal />)

    fireEvent.change(input, { target: { value: '' } })
    expect(input.value, 'cleared renewal input must not be written back to 1').toBe('')

    fireEvent.change(input, { target: { value: '6' } })
    expect(input.value).toBe('6')
    fireEvent.change(input, { target: { value: '60' } })
    expect(input.value).toBe('60')
    fireEvent.blur(input)
    expect(input.value).toBe('60')

    fireEvent.change(input, { target: { value: '' } })
    fireEvent.blur(input)
    expect(input.value).toBe('1')
  })
})
