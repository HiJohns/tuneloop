// #2352 F1：本人未支付订单（status=reserved）的提示文案。
// 抽成纯函数便于两端共用与单测（H5 `pages/Detail.jsx` / weapp `pages-weapp/Detail.jsx`）。
//
// 背景：提交订单即创建订单并锁库存（评测必须绑定 order_id），未支付 10 分钟后由
// ReservedOrderScheduler 自动取消并释放乐器。详情页若只显示「已预约」，用户会误以为
// 被他人占用，因此本人订单要显示「待支付 + 截止时间 + 继续支付」。

export const PENDING_HINT_FALLBACK = '请在支付截止前完成信用评测与支付，超时订单将自动取消'

function formatHHMM(date) {
  const hh = String(date.getHours()).padStart(2, '0')
  const mm = String(date.getMinutes()).padStart(2, '0')
  return `${hh}:${mm}`
}

/**
 * @param {string|null|undefined} deadlineIso 订单支付截止时间（RFC3339）
 * @param {number} [now] 当前时间戳（毫秒），便于单测注入
 * @returns {string} 直接展示的中文提示
 */
export function pendingOrderHint(deadlineIso, now = Date.now()) {
  if (!deadlineIso) return PENDING_HINT_FALLBACK
  const t = new Date(deadlineIso)
  const ms = t.getTime()
  if (!Number.isFinite(ms)) return PENDING_HINT_FALLBACK
  const left = ms - now
  if (left <= 0) return '已超过支付截止时间，订单可能已被自动取消'
  const minutes = Math.max(1, Math.ceil(left / 60000))
  return `请在 ${formatHHMM(t)}（约 ${minutes} 分钟）前完成信用评测与支付，超时订单将自动取消`
}
