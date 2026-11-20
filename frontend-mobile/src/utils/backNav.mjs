// #2434：扫码 / 分享 / 通知直达的页面**没有上一页**（页面栈深度 = 1），
// Taro.navigateBack() 在这种情况下静默失败（fail 未处理）→ 按钮"点了没反应"。
// 统一出口：栈深 > 1 才 navigateBack，否则回到首页（tabBar 页必须 switchTab，
// redirectTo('/') 会 page not found，见 #2326）。

export const WEAPP_HOME_ROUTE = '/pages-weapp/home/index'

/**
 * 纯函数：根据页面栈深度与前后 route 决定返回动作。
 *
 * 判定表（#2434 R4）：
 *   depth 0/1（扫码/分享/redirectTo 直达）→ switchTab 首页
 *   depth >1 且 previousRoute == currentRoute（同级页重复打开）→ switchTab 首页
 *   depth >1 且上一页不同 → navigateBack（正常返回语义）
 *
 * @param {{ stackDepth?: number, previousRoute?: string, currentRoute?: string, fallbackRoute?: string }} params
 * @returns {{ type: 'navigateBack' } | { type: 'switchTab', url: string }}
 */
export function decideBackAction({
  stackDepth,
  previousRoute = '',
  currentRoute = '',
  fallbackRoute = WEAPP_HOME_ROUTE,
} = {}) {
  const depth = Number.isFinite(stackDepth) ? stackDepth : 1
  const home = fallbackRoute || WEAPP_HOME_ROUTE
  if (depth > 1) {
    // 小程序允许同一 route 重复入栈；返回上一层只是回到同名页，等同无出口。
    if (previousRoute && currentRoute && previousRoute === currentRoute) {
      return { type: 'switchTab', url: home }
    }
    return { type: 'navigateBack' }
  }
  return { type: 'switchTab', url: home }
}
