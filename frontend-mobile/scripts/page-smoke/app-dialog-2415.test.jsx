import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, fireEvent, screen, waitFor, cleanup, act } from '@testing-library/react'
import fs from 'node:fs'
import path from 'node:path'

import { DialogHost, showDialog, isDialogHostMounted, __resetDialogQueueForTest } from '../../src/components/AppDialog'

vi.mock('../../src/platform/init', () => ({
  initializeApp: vi.fn(),
  setInitDeps: vi.fn(),
  storeToken: vi.fn(),
  parseJWT: vi.fn(() => ({})),
  cachePermissions: vi.fn(),
  getWXConfig: vi.fn(),
  showLoginReason: vi.fn(),
}))

beforeEach(() => {
  __resetDialogQueueForTest()
})

afterEach(() => {
  cleanup()
})

// R1
describe('AppDialog alert (#2415)', () => {
  it('TestAppDialogAlert_2415 renders a white card + title and resolves on confirm', async () => {
    render(<DialogHost />)
    let p
    await act(async () => {
      p = showDialog({ type: 'alert', title: '提示', message: '操作完成' })
    })
    expect(screen.getByText('提示')).toBeTruthy()
    expect(screen.getByText('操作完成')).toBeTruthy()
    const host = document.querySelector('[data-dialog-host]')
    expect(host).toBeTruthy()
    expect(host.getAttribute('style') || '').toContain('background-color')
    await act(async () => {
      fireEvent.click(screen.getByText('确定'))
    })
    await expect(p).resolves.toBeUndefined()
  })
})

// R2
describe('AppDialog confirm (#2415)', () => {
  it('TestAppDialogConfirm_2415 resolves false on cancel and true on confirm', async () => {
    render(<DialogHost />)
    let p
    await act(async () => {
      p = showDialog({ type: 'confirm', title: '确认', message: '继续？' })
    })
    await act(async () => {
      fireEvent.click(screen.getByText('取消'))
    })
    await expect(p).resolves.toBe(false)

    let q
    await act(async () => {
      q = showDialog({ type: 'confirm', title: '确认', message: '继续？' })
    })
    await act(async () => {
      fireEvent.click(screen.getByText('确定'))
    })
    await expect(q).resolves.toBe(true)
  })
})

// R3
describe('AppDialog scroll (#2415)', () => {
  it('TestAppDialogScroll_2415 renders a scroll container and the title for long messages', async () => {
    render(<DialogHost />)
    const long = '第一行\n'.repeat(40)
    let p
    await act(async () => {
      p = showDialog({ type: 'alert', title: '长文案', message: long })
    })
    expect(screen.getByText('长文案')).toBeTruthy()
    expect(document.querySelector('[data-dialog-scroll]')).toBeTruthy()
    await act(async () => {
      fireEvent.click(screen.getByText('确定'))
    })
    await p
  })
})

// R5
describe('AppDialog queue (#2415)', () => {
  it('TestAppDialogQueue_2415 queues a second call instead of overwriting the first', async () => {
    render(<DialogHost />)
    let p1, p2
    await act(async () => {
      p1 = showDialog({ type: 'alert', title: '提示', message: '第一条' })
      p2 = showDialog({ type: 'alert', title: '提示', message: '第二条' })
    })
    expect(screen.getByText('第一条')).toBeTruthy()
    expect(screen.queryByText('第二条')).toBeNull()

    await act(async () => {
      fireEvent.click(screen.getByText('确定'))
    })
    await p1
    await waitFor(() => expect(screen.getByText('第二条')).toBeTruthy())
    await act(async () => {
      fireEvent.click(screen.getByText('确定'))
    })
    await p2
  })
})

// R6
describe('AppDialog editable (#2415)', () => {
  it('TestAppDialogEditable_2415 returns the typed value or null on cancel', async () => {
    render(<DialogHost />)
    let p
    await act(async () => {
      p = showDialog({ type: 'editable', title: '申诉', message: '请输入原因', placeholder: '原因' })
    })
    const input = document.querySelector('input')
    expect(input).toBeTruthy()
    fireEvent.input(input, { target: { value: '尺寸不符' } })
    await act(async () => {
      fireEvent.click(screen.getByText('确定'))
    })
    await expect(p).resolves.toBe('尺寸不符')

    let q
    await act(async () => {
      q = showDialog({ type: 'editable', title: '申诉', message: '请输入原因' })
    })
    await act(async () => {
      fireEvent.click(screen.getByText('取消'))
    })
    await expect(q).resolves.toBeNull()
  })
})

// R7
describe('AppDialog toast (#2415)', () => {
  it('TestAppDialogToast_2415 renders a self-drawn toast and auto-dismisses', async () => {
    render(<DialogHost />)
    let p
    await act(async () => {
      p = showDialog({ type: 'toast', message: '已保存', duration: 30 })
    })
    expect(document.querySelector('[data-dialog-toast]')).toBeTruthy()
    expect(screen.getByText('已保存')).toBeTruthy()
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 80))
    })
    await expect(p).resolves.toBeUndefined()
  })
})

// R8
describe('DialogHost mount (#2415/#2437)', () => {
  it('TestDialogHostMounted_2415 mounts via PageShell; H5 keeps App-H5 host', async () => {
    const PageShell = (await import('../../src/components/PageShell')).default
    render(<PageShell><div>page-content</div></PageShell>)
    expect(screen.getByText('page-content')).toBeTruthy()
    expect(isDialogHostMounted()).toBe(true)

    const h5Source = fs.readFileSync(path.resolve(process.cwd(), 'src/App-H5.jsx'), 'utf8')
    expect(h5Source).toContain("import DialogHost from './components/DialogHost'")
    expect(h5Source).toContain('<DialogHost />')
  })
})
