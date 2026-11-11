// #2326: 会话失效后的「公开页判定」与「降级去重」——纯函数，无平台依赖，可直接 node --test。
//
// 背景（#2324 缺陷 C/D）：
//   1) publicRoutes 为 H5 短路径，小程序 getCurrentPath() 返回 /pages-weapp/xxx/index
//      → 小程序下 isPublicRoute() 恒为 false，公开页的 401 直接触发登出；
//   2) H5 带 query 时（/cart?x=1、/instrument?id=abc）同样失配；
//   3) 并发 401 会重复触发降级与跳转，需要窗口去重。

const PUBLIC_H5 = ['/', '/instrument', '/content', '/cart', '/success', '/callback', '/mall']

const PUBLIC_WEAPP = [
  '/pages-weapp/home/index',
  '/pages-weapp/detail/index',
  '/pages-weapp/content/index',
  '/pages-weapp/cart/index',
  '/pages-weapp/mall/index',
]

const DEGRADE_WINDOW_MS = 2000

/** 当前路径是否属于「公开页」（真值时 401 只返回空数据，不登出） */
export function isPublicPath(path, { isMiniProgram = false } = {}) {
  if (typeof path !== 'string' || !path) return false
  const clean = path.split('?')[0]
  const list = isMiniProgram ? PUBLIC_WEAPP : PUBLIC_H5
  return list.some((p) => clean === p || clean.startsWith(`${p}/`))
}

/** 降级窗口去重：同一窗口内的并发 401 只执行一次登出+跳转 */
export function shouldDegrade(now, lastAt, windowMs = DEGRADE_WINDOW_MS) {
  if (!lastAt) return true
  return now - lastAt >= windowMs
}
