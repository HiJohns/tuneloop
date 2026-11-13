// 负例夹具：忽略 expires_in 的错误实现（用于证明测试有鉴别力）
const FALLBACK_TTL_SECONDS = 3600

export function computeExpiryMs(_expiresIn, _accessToken, now = Date.now()) {
  return now + FALLBACK_TTL_SECONDS * 1000
}
