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
