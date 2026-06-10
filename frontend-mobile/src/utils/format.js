export function formatDisplayDate(dateStr) {
  if (!dateStr) return '-'
  const clean = dateStr.slice(0, 10)
  if (!/^\d{4}-\d{2}-\d{2}$/.test(clean)) return dateStr
  if (clean.startsWith(`${new Date().getFullYear()}-`)) return clean.slice(5)
  return clean
}

export function formatDeliveryAddress(raw) {
  if (!raw) return ''
  try {
    const obj = JSON.parse(raw)
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
