// #2425：账号选择页的纯函数（token 过期/重试/预取新鲜度）。
export function isExchangeTokenExpired(code) {
  return Number(code) === 40110
}

export function isAccountPrefetchFresh(ts, now = Date.now(), ttlMs = 240000) {
  const t = Number(ts)
  return Number.isFinite(t) && t > 0 && now - t < ttlMs
}

export function decideAfterSelectFailure(code) {
  return isExchangeTokenExpired(code) ? 'refresh-and-retry' : 'clear-and-fail'
}
