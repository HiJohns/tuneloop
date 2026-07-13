import { View, Text } from '@tarojs/components'

function Row({ icon, label, value }) {
  return (
    <View style={{ display: 'flex', alignItems: 'center', paddingVertical: 5 }}>
      <Text style={{ fontSize: 16, width: 24, textAlign: 'center', marginRight: 4 }}>{icon}</Text>
      <Text style={{ fontSize: 13, color: '#71717a', width: 72, flexShrink: 0 }}>{label}</Text>
      <Text style={{ fontSize: 13, fontWeight: '700', color: '#000', flex: 1, textAlign: 'right' }}>{value}</Text>
    </View>
  )
}

export default function LeaseInfo({ status, startDate, endDate, dailyRate, rentDays, actualDays, createdAt }) {
  const notStarted = ['reserved', 'paid', 'pending_shipment', 'shipped', 'in_transit'].includes(status)
  const inLease = status === 'in_lease'
  const returning = status === 'returning'
  const ended = ['returned', 'completed'].includes(status)

  return (
    <View style={{ backgroundColor: '#fff', marginHorizontal: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
      <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 8 }}>订单信息</Text>
      {notStarted && (
        <>
          <Row icon="📅" label="创建日期" value={createdAt || '-'} />
          {rentDays > 0 ? <Row icon="📆" label="预计天数" value={`${rentDays} 天`} /> : null}
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
      {inLease && (
        <>
          <Row icon="📅" label="起始日期" value={startDate || '-'} />
          {rentDays > 0 ? <Row icon="📆" label="预计天数" value={`${rentDays} 天`} /> : null}
          {actualDays > 0 ? <Row icon="📊" label="租赁天数" value={`${actualDays} 天`} /> : null}
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
      {returning && (
        <>
          <Row icon="📅" label="起始日期" value={startDate || '-'} />
          <Row icon="📅" label="结束日期" value={endDate || '-'} />
          {actualDays > 0 ? <Row icon="📊" label="租赁天数" value={`${actualDays} 天`} /> : null}
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
      {ended && (
        <>
          <Row icon="📅" label="起始日期" value={startDate || '-'} />
          <Row icon="📅" label="结束日期" value={endDate || '-'} />
          {actualDays > 0 ? <Row icon="📊" label="租赁天数" value={`${actualDays} 天`} /> : null}
          <Row icon="💰" label="日租金" value={`¥${Number(dailyRate || 0).toFixed(2)}`} />
        </>
      )}
    </View>
  )
}
