// ui.md §1.6 日期与时间显示规范（强制）：北京时间（UTC+8）+ 中文格式，
// 禁止 MM-DD / YYYY-MM-DD 等连字符格式出现在用户可见展示。
// 当年省略年份；跨年保留年份。「当年」判定同样基于北京时间。

function beijingParts(dateStr) {
  if (!dateStr) return null
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return null
  const bj = new Date(d.getTime() + 8 * 3600000)
  return {
    y: bj.getUTCFullYear(),
    m: bj.getUTCMonth() + 1,
    d: bj.getUTCDate(),
    hh: String(bj.getUTCHours()).padStart(2, '0'),
    mi: String(bj.getUTCMinutes()).padStart(2, '0'),
  }
}

function beijingNowYear() {
  return new Date(Date.now() + 8 * 3600000).getUTCFullYear()
}

// formatBeijingDate — 仅日期展示：当年 "M月D日"，跨年 "YYYY年M月D日"。
export function formatBeijingDate(dateStr) {
  if (!dateStr) return '-'
  const p = beijingParts(dateStr)
  if (!p) return dateStr
  const datePart = p.y === beijingNowYear() ? `${p.m}月${p.d}日` : `${p.y}年${p.m}月${p.d}日`
  return datePart
}

// formatBeijingDateTimeShort — 日期+时间展示：当年 "M月D日 HH:mm"，跨年 "YYYY年M月D日 HH:mm"。
export function formatBeijingDateTimeShort(dateStr) {
  if (!dateStr) return '-'
  const p = beijingParts(dateStr)
  if (!p) return dateStr
  const datePart = p.y === beijingNowYear() ? `${p.m}月${p.d}日` : `${p.y}年${p.m}月${p.d}日`
  return `${datePart} ${p.hh}:${p.mi}`
}

export function formatDeliveryAddress(raw) {
  if (!raw) return ''
  try {
    const obj = JSON.parse(raw)
    if (typeof obj === 'string') return obj
    if (typeof obj === 'object' && obj !== null) {
      if (obj.street) {
        return [obj.street, obj.phone ? `电话:${obj.phone}` : ''].filter(Boolean).join(' ')
      }
      const parts = [obj.province, obj.city, obj.district, obj.detail].filter(Boolean)
      const addr = parts.join('')
      const prefix = [obj.recipient_name, obj.phone].filter(Boolean).join(' ')
      return [prefix, addr].filter(Boolean).join(' ')
    }
    return raw
  } catch {
    return raw
  }
}

// #1756: payment method codes → user-facing labels. Unknown values fall
// back to the raw code so no information is hidden.
export const PAY_METHOD_LABEL = {
  jsapi: '微信支付',
  native: '扫码支付',
  waived: '优惠码免付',
  mock: '测试支付',
}

export function formatPayMethod(method) {
  if (!method) return '支付'
  return PAY_METHOD_LABEL[method] || method
}

// #1890: instrument repair workflow status → user-facing labels. The pending
// acceptance state (repair_completed) is labelled 待验收 so it cannot be
// confused with the damage-assessment status shown elsewhere.
const REPAIR_STATUS_LABEL = {
  repair_pending: '待维修',
  repair_in_progress: '维修中',
  repair_completed: '待验收',
}

export function repairStatusLabel(status) {
  if (!status) return '-'
  return REPAIR_STATUS_LABEL[status] || status
}
