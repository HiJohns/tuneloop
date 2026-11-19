import { describe, it, expect } from 'vitest'
import { maskOpenid, classifyBindPollStatus } from '../../src/utils/wechatBind.mjs'

describe('wechat bind UI (#2426)', () => {
  it('TestMaskOpenid_2426 masks long, short and empty openids', () => {
    const full = 'opjhZ3fPYGxxxxxxxxxxxxxxxxxxx'
    expect(maskOpenid(full)).toBe('opjhZ3****xxxx')
    expect(maskOpenid(full)).not.toBe(full)
    expect(maskOpenid('abcd')).toBe('****')
    expect(maskOpenid('123456789012')).toBe('****')
    expect(maskOpenid('')).toBe('')
    expect(maskOpenid(null)).toBe('')
    expect(maskOpenid(undefined)).toBe('')
  })

  it('TestClassifyBindPollStatus_2426 maps four states', () => {
    expect(classifyBindPollStatus('bound')).toBe('bound')
    expect(classifyBindPollStatus('expired')).toBe('expired')
    expect(classifyBindPollStatus('needs_merge')).toBe('needs_merge')
    expect(classifyBindPollStatus('pending')).toBe('wait')
    expect(classifyBindPollStatus('')).toBe('wait')
    expect(classifyBindPollStatus(undefined)).toBe('wait')
  })
})
