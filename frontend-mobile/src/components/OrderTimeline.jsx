import { useState, useEffect } from 'react'
import { View, Text } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'

const LIFECYCLE_ORDER = [
  'created', 'paid', 'pending_shipment', 'shipped', 'in_transit',
  'delivered', 'in_lease', 'renewed', 'returning', 'returned',
  'damage_assessed', 'return_inspected', 'assessed', 'maintenance',
  'repaired', 'completed', 'settlement_confirmed', 'cancelled', 'expired',
]

const EVENT_LABELS = {
  created: '下单',
  paid: '已付款',
  pending_shipment: '待发货',
  shipped: '已发货',
  in_transit: '运输中',
  delivered: '已收货',
  in_lease: '租赁开始',
  renewed: '续期',
  returning: '申请归还',
  returned: '已归还',
  damage_assessed: '定损完成',
  return_inspected: '验货完成',
  assessed: '定损',
  maintenance: '维修中',
  repaired: '完成维修',
  completed: '已完成',
  cancelled: '已取消',
  expired: '已超期',
  settlement_confirmed: '结算确认',
}

export default function OrderTimeline({ orderId, status }) {
  const [logs, setLogs] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!orderId) return
    const fetchLogs = async () => {
      setLoading(true)
      try {
        const resp = await apiFetch(`${env.apiBaseUrl}/orders/${orderId}/logs?page=1&pageSize=50`)
        const res = await resp.json()
        if (res.code === 20000 && res.data) {
          setLogs(res.data.logs || [])
        }
      } catch {}
      setLoading(false)
    }
    fetchLogs()
  }, [orderId])

  if (loading) {
    return (
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-sm text-zinc-400">加载中...</Text>
      </View>
    )
  }

  if (logs.length === 0) return null

  return (
    <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
      <Text className="text-base font-black text-black mb-4">订单动态</Text>
      <View className="space-y-0">
        {logs.map((log, idx) => {
          const statusIdx = LIFECYCLE_ORDER.indexOf(status)
          const eventIdx = LIFECYCLE_ORDER.indexOf(log.event)
          const isFuture = eventIdx >= 0 && statusIdx >= 0 && eventIdx > statusIdx
          const isCurrent = log.event === status
          const dotClass = isCurrent
            ? 'bg-black ring-2 ring-black ring-offset-2'
            : isFuture
              ? 'border-2 border-zinc-300 bg-transparent'
              : 'bg-zinc-300'
          return (
            <View key={idx} className="flex gap-3">
              <View className="flex flex-col items-center">
                <View className={`w-3 h-3 rounded-full mt-1.5 ${dotClass}`} />
                {idx < logs.length - 1 && <View className="w-0.5 flex-1 bg-zinc-200 mt-0.5" />}
              </View>
              <View className="flex-1 pb-4">
                <Text className={`text-sm font-black ${isCurrent ? 'text-black' : isFuture ? 'text-zinc-300' : 'text-zinc-500'}`}>
                  {EVENT_LABELS[log.event] || log.event}
                </Text>
                <Text className="text-xs text-zinc-400 mt-0.5">
                  {formatDisplayDate(log.time || log.created_at)}
                  {log.operator && <Text className="ml-2">by {log.operator}</Text>}
                </Text>
              </View>
            </View>
          )
        })}
      </View>
    </View>
  )
}
