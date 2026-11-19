// #2434：扫码 / 分享 / 通知直达的页面**没有上一页**（页面栈深度 = 1），
// Taro.navigateBack() 在这种情况下静默失败（fail 未处理）→ 按钮"点了没反应"。
// 统一出口：栈深 > 1 才 navigateBack，否则回到首页（tabBar 页必须 switchTab，
// redirectTo('/') 会 page not found，见 #2326）。

export const WEAPP_HOME_ROUTE = '/pages-weapp/home/index'

/**
 * 纯函数：根据页面栈深度决定返回动作。
 * @param {{ stackDepth?: number, fallbackRoute?: string }} params
 * @returns {{ type: 'navigateBack' } | { type: 'switchTab', url: string }}
 */
export function decideBackAction({ stackDepth, fallbackRoute = WEAPP_HOME_ROUTE } = {}) {
  const depth = Number.isFinite(stackDepth) ? stackDepth : 1
  if (depth > 1) return { type: 'navigateBack' }
  return { type: 'switchTab', url: fallbackRoute || WEAPP_HOME_ROUTE }
}
