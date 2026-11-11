// #2325: 会话过期时间推导（纯函数，无平台依赖，可直接 node --test 覆盖）。
//
// 背景（#2324 缺陷 A/B）：刷新与微信登录写入 token 时没有同步写 token_expiry，
// getToken() 立即判定过期 → 请求不带 Authorization → 反复 401 → 最终 degradeToGuest 登出。
//
// 规则优先级：
//   1) 响应 expires_in（秒）> 0  → now + expires_in * 1000
//   2) 否则读 JWT exp（秒）      → exp * 1000
//   3) 都取不到                  → now + 3600 * 1000（与 Register.jsx 既有兜底一致）

const FALLBACK_TTL_SECONDS = 3600

function decodeExpSeconds(accessToken) {
  if (!accessToken || typeof accessToken !== 'string' || !accessToken.includes('.')) return 0
  try {
    const payload = accessToken.split('.')[1].replace(/-/g, '+').replace(/_/g, '/')
    const claims = JSON.parse(atob(payload))
    const exp = Number(claims && claims.exp)
    return Number.isFinite(exp) && exp > 0 ? exp : 0
  } catch {
    return 0
  }
}

export function computeExpiryMs(expiresIn, accessToken, now = Date.now()) {
  const seconds = Number(expiresIn)
  if (Number.isFinite(seconds) && seconds > 0) return now + seconds * 1000

  const exp = decodeExpSeconds(accessToken)
  if (exp > 0) return exp * 1000

  return now + FALLBACK_TTL_SECONDS * 1000
}
