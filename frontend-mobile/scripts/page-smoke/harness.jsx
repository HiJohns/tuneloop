import { render, act } from '@testing-library/react'

const FATAL_CONSOLE = /TypeError|ReferenceError|is not a function|The above error occurred|Cannot read propert/i

function sleep(ms) {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// Render a page/component and collect fatal render errors from:
// - React render throw (render() throws)
// - window error / unhandledrejection
// - console.error messages that indicate a fatal render failure
export async function captureRender(element, waitMs = 300, options = {}) {
  const errors = []
  const onError = (event) => errors.push(event?.error?.stack || event?.message || 'window error')
  const onRejection = (event) => errors.push(event?.reason?.stack || String(event?.reason || 'unhandled rejection'))
  const originalError = console.error
  console.error = (...args) => {
    const text = args.map((a) => (a && a.stack) || String(a)).join(' ')
    if (FATAL_CONSOLE.test(text)) errors.push(text)
    if (!options.silent) originalError(...args)
  }
  window.addEventListener('error', onError)
  window.addEventListener('unhandledrejection', onRejection)

  try {
    render(element)
    await act(async () => { await sleep(waitMs) })
  } catch (error) {
    errors.push(error.stack || String(error))
  } finally {
    window.removeEventListener('error', onError)
    window.removeEventListener('unhandledrejection', onRejection)
    console.error = originalError
  }
  return errors
}
