import { describe, it, expect, vi, afterEach } from 'vitest'
import { render, waitFor, cleanup, fireEvent } from '@testing-library/react'
import Credentials from '../../src/pages-weapp/credentials/Credentials'
import { dialog } from '../../src/platform'

const jsonResponse = (body) => ({
  ok: true,
  status: 200,
  headers: { get: () => 'application/json' },
  json: async () => body,
  clone() { return this },
})

describe('credentials page (#2446)', () => {
  afterEach(() => {
    cleanup()
    vi.restoreAllMocks()
  })

  it('prefills phone, validates and submits username/password', async () => {
    const originalFetch = globalThis.fetch
    const calls = []
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      calls.push({ url: String(url), options })
      if (String(url).includes('/users/me') && (options.method || 'GET') === 'GET') {
        return jsonResponse({ code: 20000, data: { username: '13800002446', phone: '13800002446' } })
      }
      if (String(url).includes('/users/me') && options.method === 'PUT') {
        return jsonResponse({ code: 20000, data: { relogin: true } })
      }
      return jsonResponse({ code: 20000, data: {} })
    })
    vi.spyOn(dialog, 'confirm').mockResolvedValue(true)
    vi.spyOn(dialog, 'alert').mockResolvedValue(undefined)
    window.history.replaceState({}, '', '/credentials')

    render(<Credentials />)
    await waitFor(() => expect(document.body.textContent).toContain('保存登录账号'))

    const inputs = document.querySelectorAll('input')
    expect(inputs.length).toBeGreaterThanOrEqual(3)
    expect(inputs[0].value).toBe('13800002446')

    fireEvent.input(inputs[0], { target: { value: 'newuser2446' } })
    fireEvent.input(inputs[1], { target: { value: 'Abcd1234' } })
    fireEvent.input(inputs[2], { target: { value: 'Abcd1234' } })
    const button = [...document.querySelectorAll('button')].find(b => String(b.textContent).includes('保存登录账号'))
    expect(button).toBeTruthy()
    fireEvent.click(button)

    await waitFor(() => {
      const put = calls.find(c => c.options.method === 'PUT')
      expect(put, 'must PUT /users/me').toBeTruthy()
      const body = JSON.parse(put.options.body)
      expect(body.username).toBe('newuser2446')
      expect(body.password).toBe('Abcd1234')
    })
    expect(dialog.alert).toHaveBeenCalled()
    globalThis.fetch = originalFetch
  })

  it('rejects username equal to phone before PUT', async () => {
    const originalFetch = globalThis.fetch
    const calls = []
    globalThis.fetch = vi.fn(async (url, options = {}) => {
      calls.push({ url: String(url), options })
      return jsonResponse({ code: 20000, data: { username: '13800002446', phone: '13800002446' } })
    })
    vi.spyOn(dialog, 'confirm').mockResolvedValue(true)
    vi.spyOn(dialog, 'alert').mockResolvedValue(undefined)
    window.history.replaceState({}, '', '/credentials')

    render(<Credentials />)
    await waitFor(() => expect(document.body.textContent).toContain('保存登录账号'))
    const inputs = document.querySelectorAll('input')
    fireEvent.input(inputs[1], { target: { value: 'Abcd1234' } })
    fireEvent.input(inputs[2], { target: { value: 'Abcd1234' } })
    const button = [...document.querySelectorAll('button')].find(b => String(b.textContent).includes('保存登录账号'))
    fireEvent.click(button)

    await waitFor(() => expect(dialog.alert).toHaveBeenCalled())
    expect(calls.some(c => c.options.method === 'PUT')).toBe(false)
    globalThis.fetch = originalFetch
  })
})
