// #2351: 租期天数的编辑文本与生效数值分离（H5/WeApp 共用）。
// 输入期允许空串；days 始终是合法整数，供租金/归还日/提交参数使用。
// 失焦时再由 commit 把文本归一化（Checkout 1..730，Renewal minDays..∞）。

export function clampRentDays(value, { min = 1, max = Infinity, fallback = min } = {}) {
  const n = parseInt(String(value ?? ''), 10)
  if (!Number.isFinite(n)) return fallback
  return Math.max(min, Math.min(max, n))
}

export function rentDaysInputState(raw, options) {
  return { text: String(raw ?? ''), days: clampRentDays(raw, options) }
}

export function rentDaysCommitState(raw, options) {
  const days = clampRentDays(raw, options)
  return { text: String(days), days }
}

export function rentDaysStepState(currentDays, delta, options) {
  const days = clampRentDays(Number(currentDays) + Number(delta), options)
  return { text: String(days), days }
}
