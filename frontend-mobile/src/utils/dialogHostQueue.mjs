// #2437：页面级 DialogHost 的队列归属/超时纯函数。
export const DIALOG_HOST_TIMEOUT_MS = 1500

// 多页面宿主同时挂载时，仅当前顶层页面消费队列。
export function pickHostRoute(hostRoutes = [], topRoute = '') {
  if (!hostRoutes.length) return ''
  if (topRoute) return hostRoutes.includes(topRoute) ? topRoute : ''
  return hostRoutes[0]
}

export function shouldFallbackOnTimeout({ consumed = false } = {}) {
  return !consumed
}
