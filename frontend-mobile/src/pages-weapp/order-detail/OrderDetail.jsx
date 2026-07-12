import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Button, Image } from '@tarojs/components'
import { apiFetch } from '../../services/api'
import { env } from '../../platform'

const STATUS = {
  reserved: { color: '#f59e0b', label: '未支付' },
  paid: { color: '#3b82f6', label: '待发货' },
  pending_shipment: { color: '#3b82f6', label: '待发货' },
  in_transit: { color: '#06b6d4', label: '运输中' },
  shipped: { color: '#3b82f6', label: '已发货' },
  in_lease: { color: '#22c55e', label: '租赁中' },
  returning: { color: '#eab308', label: '归还中' },
  returned: { color: '#a1a1aa', label: '已归还' },
  completed: { color: '#a1a1aa', label: '已完成' },
  cancelled: { color: '#ef4444', label: '已取消' },
  expired: { color: '#ef4444', label: '超期' },
}

const LIFECYCLE = [
  { key: 'created', label: '下单', icon: '📝' },
  { key: 'paid', label: '付款', icon: '💰' },
  { key: 'shipped', label: '发货', icon: '📦' },
  { key: 'in_lease', label: '租赁', icon: '🎵' },
  { key: 'returned', label: '归还', icon: '↩️' },
  { key: 'completed', label: '完成', icon: '✅' },
]

const baseUrl = env.apiBaseUrl

export default function OrderDetail() {
  const params = Taro.getCurrentInstance().router?.params || {}
  const id = params.id
  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!id) return
    const load = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${id}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrder(result.data)
          if (result.data.instrument_id) {
            const iResp = await apiFetch(`${baseUrl}/public/instruments/${result.data.instrument_id}`)
            const iResult = await iResp.json()
            if (iResult.code === 20000) setInstrument(iResult.data)
          }
        }
      } catch {}
      setLoading(false)
    }
    load()
  }, [id])

  if (loading) return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
  if (!order) return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}><Text style={{ color: '#a1a1aa' }}>订单不存在</Text></View>

  const status = STATUS[order.status] || { color: '#a1a1aa', label: order.status }

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', paddingBottom: 100 }}>
      {/* Header */}
      <View style={{ background: 'linear-gradient(to bottom, #FDF4E7, #fff)', padding: '16px 16px 12px', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text style={{ fontSize: 20 }} onClick={() => Taro.navigateBack()}>❮</Text>
        <Text style={{ fontSize: 18, fontWeight: '700', flex: 1 }}>订单详情</Text>
        <Text style={{ fontSize: 14, fontWeight: '700', color: status.color }}>{status.label}</Text>
      </View>

      <ScrollView style={{ width: '100%' }}>
        {/* Instrument card */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <View style={{ display: 'flex', gap: 12 }}>
            {instrument?.cover_image && (
              <Image src={instrument.cover_image} style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#f4f4f5' }} mode="aspectFill" />
            )}
            <View style={{ flex: 1 }}>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#000' }}>{instrument?.category_name || '乐器'}</Text>
              <Text style={{ fontSize: 12, color: '#71717a', marginTop: 4 }}>SN: {instrument?.sn || '-'}</Text>
              <Text style={{ fontSize: 12, color: '#71717a' }}>{instrument?.level_name || ''}</Text>
            </View>
          </View>
        </View>

        {/* Lease info */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>租赁信息</Text>
          <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6 }}>
            <Text style={{ fontSize: 13, color: '#71717a' }}>租期</Text>
            <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>{order.start_date || '?'} ~ {order.end_date || '?'}</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6 }}>
            <Text style={{ fontSize: 13, color: '#71717a' }}>日租金</Text>
            <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>¥{order.base_daily_rate || instrument?.base_daily_rate || 0}</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6 }}>
            <Text style={{ fontSize: 13, color: '#71717a' }}>押金</Text>
            <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>¥{order.deposit || instrument?.deposit || 0}</Text>
          </View>
        </View>

        {/* Delivery address */}
        {order.delivery_address && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>收货地址</Text>
            <Text style={{ fontSize: 13, color: '#000' }}>
              {order.delivery_address.recipient_name} {order.delivery_address.phone}
            </Text>
            <Text style={{ fontSize: 12, color: '#71717a', marginTop: 4 }}>
              {order.delivery_address.province}{order.delivery_address.city}{order.delivery_address.district} {order.delivery_address.detail}
            </Text>
          </View>
        )}

        {/* Lifecycle */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>订单进度</Text>
          <View style={{ display: 'flex', flexDirection: 'row', justifyContent: 'space-between', paddingHorizontal: 4 }}>
            {LIFECYCLE.map((step, i) => {
              const done = LIFECYCLE.findIndex(s => s.key === order.status) >= i
              return (
                <View key={step.key} style={{ alignItems: 'center', width: 40 }}>
                  <Text style={{ fontSize: 18, opacity: done ? 1 : 0.3 }}>{step.icon}</Text>
                  <Text style={{ fontSize: 10, color: done ? '#000' : '#a1a1aa', marginTop: 4, textAlign: 'center' }}>{step.label}</Text>
                </View>
              )
            })}
          </View>
        </View>

        {/* Timeline logs */}
        {order.logs?.length > 0 && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>订单日志</Text>
            {order.logs.slice(0, 10).map((log, i) => (
              <View key={i} style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4, borderBottom: i < 9 ? '1px solid #f4f4f5' : 'none' }}>
                <Text style={{ fontSize: 12, color: '#71717a' }}>{log.event || log.action || '-'}</Text>
                <Text style={{ fontSize: 12, color: '#a1a1aa' }}>{log.created_at ? log.created_at.slice(0, 16) : ''}</Text>
              </View>
            ))}
          </View>
        )}
      </ScrollView>
    </View>
  )
}
