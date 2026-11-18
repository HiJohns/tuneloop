// #2411 回归：评测页「返回继续支付」在 weapp 不得因 return_path 未解码而误报
// 「该功能请在微信小程序中完成」。
//
// 现场：支付页用 encodeURIComponent('/payment?type=rent&id=…') 传 return_path，
// Taro router.params 不保证解码 → toWeappRoute 匹配失败 → 弹错误提示、无法返回。
import { describe, it, expect } from 'vitest'
import { decodeReturnPath } from '../../src/utils/creditFlow.mjs'
import { toWeappRoute } from '../../src/platform/navigation'

describe('credit return path (#2411)', () => {
  it('decodes an encoded H5 payment path and maps it to the weapp page', () => {
    const raw = encodeURIComponent('/payment?type=rent&id=o-2410')
    expect(raw).not.toBe('/payment?type=rent&id=o-2410')
    expect(decodeReturnPath(raw)).toBe('/payment?type=rent&id=o-2410')

    const route = toWeappRoute(decodeReturnPath(raw))
    expect(route).not.toBeNull()
    expect(route.url.startsWith('/pages-weapp/payment/index')).toBe(true)
    expect(route.url).toContain('type=rent')
    expect(route.url).toContain('id=o-2410')
  })

  it('keeps plain H5 paths and native /pages paths untouched', () => {
    expect(decodeReturnPath('/payment?type=rent&id=x')).toBe('/payment?type=rent&id=x')
    expect(decodeReturnPath('/pages-weapp/payment/index?type=rent&id=x'))
      .toBe('/pages-weapp/payment/index?type=rent&id=x')
  })

  it('never throws on malformed encoding', () => {
    expect(decodeReturnPath('/payment?x=%E0%A4%A')).toBe('/payment?x=%E0%A4%A')
  })

  it('returns empty string for missing input', () => {
    expect(decodeReturnPath('')).toBe('')
    expect(decodeReturnPath(undefined)).toBe('')
    expect(decodeReturnPath(null)).toBe('')
  })
})
