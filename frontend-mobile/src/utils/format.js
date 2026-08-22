export function formatDisplayDate(dateStr) {
  if (!dateStr) return '-'
  const clean = dateStr.slice(0, 10)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(clean)) return dateStr
  // #1759: full timestamps carry a timezone (Z) after the timestamptz
  // migration — parse them locally so the shown date matches Beijing
  // (raw slice(0,10) would take the UTC date, off by one around midnight).
  if (dateStr.length > 10) {
    const d = new Date(dateStr)
    if (!isNaN(d.getTime())) {
      const mm = String(d.getMonth() + 1).padStart(2, '0')
      const dd = String(d.getDate()).padStart(2, '0')
      const local = `${d.getFullYear()}-${mm}-${dd}`
      if (local.startsWith(`${new Date().getFullYear()}-`)) return local.slice(5)
      return local
    }
  }
  if (clean.startsWith(`${new Date().getFullYear()}-`)) return clean.slice(5)
  return clean
}

// formatLogTime renders "MM-DD HH:mm" for order timeline entries (#1701):
// the current year is omitted; a leading zero pads hour/minute.
export function formatLogTime(dateStr) {
  if (!dateStr) return '-'
  const d = new Date(dateStr)
  if (isNaN(d.getTime())) return formatDisplayDate(dateStr)
  const mm = String(d.getMonth() + 1).padStart(2, '0')
  const dd = String(d.getDate()).padStart(2, '0')
  const hh = String(d.getHours()).padStart(2, '0')
  const mi = String(d.getMinutes()).padStart(2, '0')
  const datePart = d.getFullYear() === new Date().getFullYear() ? `${mm}-${dd}` : `${d.getFullYear()}-${mm}-${dd}`
  return `${datePart} ${hh}:${mi}`
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
