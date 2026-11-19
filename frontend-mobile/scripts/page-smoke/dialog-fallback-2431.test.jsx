// #2431 回归：weapp 端 dialog 必须降级原生，即使 DialogHost「已挂载」。
// 旧实现 isDialogHostMounted() === true 就入队 AppDialog；小程序 App 实例
// 没有视图层 → 弹窗不可见且 Promise 永不 resolve（购物车删除/地址删除/
// 取消订单全部无响应）。
import { describe, it, expect } from 'vitest'
import {
  APP_DIALOG_ON_WEAPP,
  shouldUseAppDialog,
  toNativeModalOptions,
  fromNativeModalResult,
} from '../../src/utils/dialogFallback.mjs'

describe('shouldUseAppDialog (#2431)', () => {
  it('小程序端即便宿主已挂载也默认走原生（负例：旧逻辑会入队 AppDialog）', () => {
    expect(APP_DIALOG_ON_WEAPP).toBe(false)
    expect(shouldUseAppDialog({ isMiniProgram: true, hostMounted: true })).toBe(false)
    expect(shouldUseAppDialog({ isMiniProgram: true, hostMounted: false })).toBe(false)
  })

  it('小程序端显式开启且宿主挂载时才用 AppDialog（方案 B 落地后的开关）', () => {
    expect(shouldUseAppDialog({ isMiniProgram: true, hostMounted: true, enabled: true })).toBe(true)
    expect(shouldUseAppDialog({ isMiniProgram: true, hostMounted: false, enabled: true })).toBe(false)
  })

  it('H5 始终使用 AppDialog（宿主可见）', () => {
    expect(shouldUseAppDialog({ isMiniProgram: false, hostMounted: false })).toBe(true)
  })
})

describe('toNativeModalOptions (#2431)', () => {
  it('confirm 保留自定义按钮文案（重试视频 / 仅提交照片）', () => {
    const p = toNativeModalOptions({
      type: 'confirm',
      title: '视频上传未完成',
      message: '照片已提交成功…',
      confirmText: '重试视频',
      cancelText: '仅提交照片',
    })
    expect(p.showCancel).toBe(true)
    expect(p.confirmText).toBe('重试视频')
    expect(p.cancelText).toBe('仅提交照片')
    expect(p.content).toBe('照片已提交成功…')
  })

  it('alert 无取消按钮', () => {
    const p = toNativeModalOptions({ type: 'alert', title: '提示', message: '已取消' })
    expect(p.showCancel).toBe(false)
    expect(p.confirmText).toBe('确定')
    expect(p.cancelText).toBeUndefined()
  })

  it('toast 走 showToast（返回 null）', () => {
    expect(toNativeModalOptions({ type: 'toast', message: '排序失败' })).toBeNull()
  })

  it('editable 携带 placeholder 与初始值', () => {
    const p = toNativeModalOptions({
      type: 'editable',
      title: '请输入',
      placeholder: '备注',
      defaultValue: 'abc',
    })
    expect(p.editable).toBe(true)
    expect(p.placeholderText).toBe('备注')
    expect(p.content).toBe('abc')
    expect(p.showCancel).toBe(true)
  })
})

describe('fromNativeModalResult (#2431)', () => {
  it('confirm → boolean', () => {
    expect(fromNativeModalResult({ confirm: true }, { type: 'confirm' })).toBe(true)
    expect(fromNativeModalResult({ confirm: false }, { type: 'confirm' })).toBe(false)
    expect(fromNativeModalResult({}, { type: 'confirm' })).toBe(false)
  })

  it('editable → string | null', () => {
    expect(fromNativeModalResult({ confirm: true, content: '备注' }, { type: 'editable' })).toBe('备注')
    expect(fromNativeModalResult({ confirm: true }, { type: 'editable' })).toBe('')
    expect(fromNativeModalResult({ confirm: false, content: 'x' }, { type: 'editable' })).toBeNull()
  })

  it('alert → undefined', () => {
    expect(fromNativeModalResult({ confirm: true }, { type: 'alert' })).toBeUndefined()
  })
})
