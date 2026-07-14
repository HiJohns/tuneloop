import { View, Text } from '@tarojs/components'

function Row({ icon, label, value, valueColor }) {
  return (
    <View style={{ display: 'flex', alignItems: 'center', paddingVertical: 5 }}>
      <Text style={{ fontSize: 16, width: 24, textAlign: 'center', marginRight: 4 }}>{icon}</Text>
      <Text style={{ fontSize: 13, color: '#71717a', width: 72, flexShrink: 0 }}>{label}</Text>
      <Text style={{ fontSize: 13, fontWeight: '700', color: valueColor || '#000', flex: 1, textAlign: 'right' }}>{value}</Text>
    </View>
  )
}

function fmt(raw) {
  if (!raw) return '-'
  const s = raw.length >= 10 ? raw.slice(0, 10) : raw
  if (!/^\d{4}-\d{2}-\d{2}$/.test(s)) return raw.slice(0, 10)
  const now = new Date()
  if (s.startsWith(`${now.getFullYear()}-`)) return s.slice(5)
  return s
}

function today() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`
}

function parseDate(raw) {
  if (!raw) return null
  const s = raw.slice(0, 10)
  const d = new Date(s)
  return isNaN(d.getTime()) ? null : d
}

export default function LeaseInfo({ status, startDate, endDate, deliveredAt, dailyRate, rentDays, createdAt }) {
  const effStart = deliveredAt || startDate
  const effEnd = endDate

  const ended = ['returned', 'completed', 'returning'].includes(status)
  const inLease = status === 'in_lease'
  const notStarted = ['reserved', 'paid', 'pending_shipment', 'shipped', 'in_transit'].includes(status)

  const endDt = parseDate(effEnd)
  const startDt = parseDate(effStart)
  const nowDt = parseDate(today())

  const isOverdue = endDt && nowDt ? nowDt > endDt : false
  const overdueDays = endDt && nowDt && isOverdue ? Math.round((nowDt - endDt) / 86400000) : 0

  const leaseDays = startDt && endDt
    ? Math.max(1, Math.round((endDt - startDt) / 86400000))
    : 0

  const currentLeaseDays = startDt && nowDt
    ? Math.max(1, Math.round((nowDt - startDt) / 86400000) + 1)
    : 0

  return (
    <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
      <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>订单信息</Text>
      {notStarted && (
        <>
          <Row icon="📅" label="创建日期" value={fmt(createdAt)} />
          {rentDays > 0 ? <Row icon="⏳" label="预计天数" value={`${rentDays} 天`} /> : null}
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
      {inLease && (
        <>
          <Row icon="📅" label="起始日期" value={fmt(effStart)} />
          {rentDays > 0 ? <Row icon="⏳" label="预期天数" value={`${rentDays} 天`} /> : null}
          {effEnd ? (
            <Row
              icon="🎯"
              label="预期归还"
              value={isOverdue ? `${fmt(effEnd)}（已过期 ${overdueDays} 天）` : fmt(effEnd)}
              valueColor={isOverdue ? '#ef4444' : undefined}
            />
          ) : null}
          <Row icon="📊" label="租赁天数" value={`${currentLeaseDays} 天`} />
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
      {ended && (
        <>
          <Row icon="📅" label="起始日期" value={fmt(effStart)} />
          <Row icon="🏁" label="结束日期" value={fmt(effEnd)} />
          {leaseDays > 0 ? <Row icon="📊" label="租赁天数" value={`${leaseDays} 天`} /> : null}
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
    </View>
  )
}
