// #2426：微信绑定展示/轮询的纯函数（双端可测，不依赖 Taro/浏览器）。
export function maskOpenid(openid) {
  if (typeof openid !== 'string' || openid.length === 0) return ''
  if (openid.length <= 12) return '****'
  return `${openid.slice(0, 6)}****${openid.slice(-4)}`
}

export function classifyBindPollStatus(status) {
  switch (status) {
    case 'bound':
    case 'expired':
    case 'needs_merge':
      return status
    default:
      return 'wait'
  }
}
