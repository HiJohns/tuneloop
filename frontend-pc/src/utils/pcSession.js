// #2456: PC 管理后台静置超时（idle timeout）前端判定与活动上报节流。
//
// 这里只放"要不要上报活动"和"这个响应是不是 idle 过期"两类纯判定 +
// 一个浏览器侧的事件订阅器，便于 `node --test` 直接覆盖（frontend-pc 没有
// 测试运行器，见 Accepted Plan D7）。
export const PC_ACTIVITY_THROTTLE_MS = 60 * 1000
export const PC_ACTIVITY_ENDPOINT = '/auth/pc-activity'
export const PC_IDLE_TIMEOUT_CODE = 40110
export const PC_IDLE_TIMEOUT_MESSAGE = '登录已超时，请重新登录'

// 用户交互事件白名单：只有显式交互才刷新 PC 会话活动时间。
// 后台轮询（通知中心 60s）与定时刷新（30s token 检查）故意不在其中，
// 否则 idle 永远不会触发（Accepted Plan D3）。
export const PC_ACTIVITY_EVENTS = ['pointerdown', 'keydown', 'wheel', 'touchstart']

/**
 * 判断本次交互是否已越过上报节流窗口。
 * @param {number} now 当前毫秒时间戳
 * @param {number} lastSentAt 上次上报的毫秒时间戳（0/空表示尚未上报）
 * @returns {boolean}
 */
export function shouldSendActivity(now, lastSentAt) {
  const current = Number(now) || 0
  const last = Number(lastSentAt) || 0
  if (last <= 0) return true
  return current - last >= PC_ACTIVITY_THROTTLE_MS
}

/**
 * 判断响应体是否是 PC 静置超时（后端 code 40110）。
 * @param {{ code?: number } | null | undefined} payload
 * @returns {boolean}
 */
export function isIdleTimeoutPayload(payload) {
  return Boolean(payload) && payload.code === PC_IDLE_TIMEOUT_CODE
}

/**
 * 订阅用户交互并按 60s 节流上报活动。
 * @param {() => (Promise<unknown> | unknown)} sendActivity 上报函数（通常为 api.reportPCActivity）
 * @returns {() => void} 取消订阅函数
 */
export function startActivityTracking(sendActivity) {
  if (typeof window === 'undefined' || typeof document === 'undefined') return () => {}
  if (typeof sendActivity !== 'function') return () => {}

  let lastSentAt = 0

  const handleActivity = () => {
    const now = Date.now()
    if (!shouldSendActivity(now, lastSentAt)) return
    lastSentAt = now
    try {
      const result = sendActivity()
      if (result && typeof result.catch === 'function') {
        // 活动上报是尽力而为：失败不应打断业务请求或弹错。
        result.catch(() => {})
      }
    } catch (e) {
      // 同上：上报失败静默降级，由后端 idle 阈值兜底。
    }
  }

  PC_ACTIVITY_EVENTS.forEach((eventName) => {
    document.addEventListener(eventName, handleActivity, { passive: true })
  })

  return () => {
    PC_ACTIVITY_EVENTS.forEach((eventName) => {
      document.removeEventListener(eventName, handleActivity)
    })
  }
}
