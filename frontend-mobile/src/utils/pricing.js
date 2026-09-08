// #1842: 首页卡片价格展示——阶梯最低日均价。
// /public/instruments（首页列表 API）返回 pricing JSON 字符串：
// {"tiers":[{days_max, daily_rate},...], "deposit":...}，其中 daily_rate 单位为【元】
// （实证：首档 9 元 == daily_rate_cents 900 分）。本函数取最低档日均价，返回【元】；
// pricing 缺失/解析失败返回 null，调用方回退 daily_rate_cents 基准日租显示。
export function getMinTierDailyRateYuan(instrument) {
  const raw = instrument && (instrument.pricing || '')
  if (!raw) return null
  let parsed
  try { parsed = typeof raw === 'string' ? JSON.parse(raw) : raw } catch (e) { return null }
  const tiers = parsed && Array.isArray(parsed.tiers) ? parsed.tiers : null
  if (!tiers || tiers.length === 0) return null
  const rates = tiers
    .map(t => Number(t && t.daily_rate))
    .filter(n => Number.isFinite(n) && n > 0)
  if (rates.length === 0) return null
  return Math.min(...rates)
}
