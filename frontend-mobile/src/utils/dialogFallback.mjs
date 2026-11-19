// #2431：weapp 端 App 层 DialogHost 不可见（小程序 App 实例没有视图层），
// 组件虽在 JS 层挂载（isDialogHostMounted() 恒 true），但 View 不在任何页面
// 可视树 → showDialog 的 Promise 永不 resolve，所有 await dialog.confirm(...)
// 之后的逻辑永久挂起（购物车删除/地址删除/取消订单等全部无响应）。
//
// 因此 weapp 端默认关闭 AppDialog，统一降级到原生 Taro.showModal /
// Taro.showToast；自定义文案（确认/取消按钮）通过原生 confirmText/cancelText
// 保留。AppDialog 仍用于 H5（H5 有真实 DOM，宿主可见）。
//
// 后续方案 B（页面级挂载 DialogHost）落地后，把 APP_DIALOG_ON_WEAPP 置 true 即可。

export const APP_DIALOG_ON_WEAPP = false

// 是否使用自绘 AppDialog。
// - H5/浏览器：始终使用（宿主挂在 App-H5，可见）；
// - 小程序：仅在显式开启且宿主确实挂载时使用（默认关闭，见文件头 #2431）。
export function shouldUseAppDialog({ isMiniProgram = false, hostMounted = false, enabled = APP_DIALOG_ON_WEAPP } = {}) {
  if (!isMiniProgram) return true
  return enabled === true && hostMounted === true
}

// showDialog(options) → Taro.showModal 参数。
// toast 返回 null（走 Taro.showToast）。
export function toNativeModalOptions(options = {}) {
  const type = options.type || 'alert'
  if (type === 'toast') return null

  const editable = type === 'editable' || options.editable === true
  const confirmable = type === 'confirm' || editable

  const params = {
    title: options.title || '',
    content: editable && options.defaultValue ? options.defaultValue : (options.message || options.content || ''),
    showCancel: confirmable,
    confirmText: options.confirmText || '确定',
  }
  if (confirmable) {
    params.cancelText = options.cancelText || '取消'
  }
  if (editable) {
    params.editable = true
    params.placeholderText = options.placeholder || ''
  }
  return params
}

// Taro.showModal 回调结果 → showDialog 的 resolve 语义
// （confirm → boolean；editable → string | null；alert → undefined）。
export function fromNativeModalResult(result = {}, options = {}) {
  const type = options.type || 'alert'
  if (type === 'editable' || options.editable === true) {
    if (!result.confirm) return null
    return result.content === undefined || result.content === null ? '' : result.content
  }
  if (type === 'confirm') return !!result.confirm
  return undefined
}
