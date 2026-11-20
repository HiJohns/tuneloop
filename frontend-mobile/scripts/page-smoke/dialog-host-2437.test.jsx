// #2437：页面级 DialogHost 的队列归属、超时降级与 weapp 开关。
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { pickHostRoute, shouldFallbackOnTimeout } from '../../src/utils/dialogHostQueue.mjs'
import { showDialog, __resetDialogQueueForTest } from '../../src/components/AppDialog'
import { APP_DIALOG_ON_WEAPP, shouldUseAppDialog } from '../../src/utils/dialogFallback.mjs'

describe('pickHostRoute (#2437)', () => {
  it('顶层 route 有宿主时由顶层消费', () => {
    expect(pickHostRoute(['a', 'b'], 'b')).toBe('b')
  })
  it('顶层无宿主时不消费（交给超时降级）', () => {
    expect(pickHostRoute(['a', 'b'], 'c')).toBe('')
  })
  it('无宿主返回空', () => {
    expect(pickHostRoute([], 'a')).toBe('')
  })
})

describe('timeout + switch (#2437)', () => {
  beforeEach(() => { __resetDialogQueueForTest(); vi.useFakeTimers() })
  afterEach(() => { vi.useRealTimers() })

  it('宿主未消费时超时调用原生回退并 resolve', async () => {
    const p = showDialog({ type: 'alert', message: 'x', timeoutMs: 10, onTimeout: () => 'native' })
    expect(shouldFallbackOnTimeout({ consumed: false })).toBe(true)
    vi.advanceTimersByTime(20)
    await expect(p).resolves.toBe('native')
  })

  it('weapp 开关默认开启；宿主不可用仍走原生', () => {
    expect(APP_DIALOG_ON_WEAPP).toBe(true)
    expect(shouldUseAppDialog({ isMiniProgram: true, hostMounted: true })).toBe(true)
    expect(shouldUseAppDialog({ isMiniProgram: true, hostMounted: false })).toBe(false)
  })
})
