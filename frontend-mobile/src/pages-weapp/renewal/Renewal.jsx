import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Input, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../../services/api'
import { env } from '../../platform'

export default function Renewal() {
  const params = Taro.getCurrentInstance()?.router?.params || {}
  const orderId = params.id
  const baseUrl = env.apiBaseUrl

  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [days, setDays] = useState(30)
  const [calcResult, setCalcResult] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [customDays, setCustomDays] = useState('')

  useEffect(() => {
    if (!orderId) return
    const load = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${orderId}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrder(result.data)
          if (result.data.instrument_id) {
            const iresp = await apiFetch(`${baseUrl}/public/instruments/${result.data.instrument_id}`)
            const iresult = await iresp.json()
            if (iresult.code === 20000) setInstrument(iresult.data)
          }
        }
      } catch (err) {
        console.error('Failed to load order:', err)
      } finally {
        setLoading(false)
      }
    }
    load()
  }, [orderId])

  useEffect(() => {
    if (!order || !days || days <= 0) return
    const calc = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${orderId}/renewal/calculate`, {
          method: 'POST',
          body: JSON.stringify({ additional_days: days }),
        })
        const result = await resp.json()
        if (result.code === 20000) setCalcResult(result.data)
      } catch (err) {
        console.error('Failed to calculate:', err)
      }
    }
    calc()
  }, [days, order, orderId])

  const handleSubmit = async () => {
    if (submitting || !calcResult) return
    setSubmitting(true)
    try {
      const token = getToken()
      const resp = await fetch(`${baseUrl}/orders/${orderId}/renewal/confirm`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ additional_days: days }),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data?.success) {
        Taro.redirectTo({
          url: `/pages-weapp/payment/index?type=renewal&id=${orderId}&amount=${calcResult.total_amount}`,
        })
      } else {
        Taro.showModal({ title: '续期失败', content: result.data?.message || '请重试', showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '网络错误', content: err.message, showCancel: false })
    } finally {
      setSubmitting(false)
    }
  }

  const btnStyle = (bg) => ({
    width: '100%',
    padding: '12px 0',
    borderRadius: 16,
    fontWeight: '900',
    textAlign: 'center',
    color: '#fff',
    backgroundColor: bg,
  })

  const pickDay = (d) => {
    setDays(d)
    setCustomDays('')
  }

  const dayOptions = [7, 15, 30, 60, 90, 180, 365]

  if (loading) return <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}><Text>加载中...</Text></View>
  if (!order) return <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', height: '100vh' }}><Text>订单不存在</Text></View>

  const overdueDays = calcResult?.overdue_days || 0

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#f9fafb' }}>
      <View style={{ backgroundColor: '#fff', padding: '12px 16px', display: 'flex', alignItems: 'center', gap: 8, borderBottom: '1px solid #e5e7eb' }}>
        <View onClick={() => Taro.navigateBack()}>
          <Text style={{ fontSize: 18 }}>{'<'}</Text>
        </View>
        <Text style={{ fontSize: 16, fontWeight: '700' }}>续期</Text>
      </View>

      <ScrollView style={{ padding: 16 }}>

        {instrument && (
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', marginBottom: 8 }}>{instrument.category_name || '乐器'}</Text>
            <Text style={{ fontSize: 12, color: '#71717a' }}>SN: {instrument.sn || '-'}</Text>
            <Text style={{ fontSize: 12, color: '#71717a', marginTop: 2 }}>当前到期: {order.end_date || '-'}</Text>
            {overdueDays > 0 && <Text style={{ fontSize: 12, color: '#ef4444', marginTop: 4 }}>已逾期 {overdueDays} 天</Text>}
          </View>
        )}

        <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12 }}>
          <Text style={{ fontSize: 14, fontWeight: '700', marginBottom: 8 }}>续期天数</Text>
          <View style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 8 }}>
            {dayOptions.map(d => (
              <View key={d} onClick={() => pickDay(d)}
                style={{
                  padding: '8px 16px', borderRadius: 20, border: '1px solid #d1d5db',
                  backgroundColor: days === d && !customDays ? '#2563eb' : '#fff',
                }}>
                <Text style={{ fontSize: 13, color: days === d && !customDays ? '#fff' : '#374151' }}>{d}天</Text>
              </View>
            ))}
          </View>
          <View style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
            <Text style={{ fontSize: 12, color: '#71717a' }}>自定义:</Text>
            <Input
              type="number"
              placeholder="天数"
              value={customDays}
              onInput={(e) => { setCustomDays(e.detail.value); setDays(parseInt(e.detail.value) || 1) }}
              style={{ width: 60, padding: '4px 8px', border: '1px solid #d1d5db', borderRadius: 8, textAlign: 'center', fontSize: 12 }}
            />
            <Text style={{ fontSize: 12, color: '#71717a' }}>天</Text>
          </View>
        </View>

        {calcResult && (
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', marginBottom: 8 }}>费用明细</Text>
            {calcResult.tier_breakdown?.map((t, i) => (
              <View key={i} style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4 }}>
                <Text style={{ fontSize: 12, color: '#71717a' }}>第{t.tier}阶 {t.days}天</Text>
                <Text style={{ fontSize: 12, fontWeight: '500' }}>¥{t.subtotal?.toFixed(2)}</Text>
              </View>
            ))}
            <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4, borderTop: '1px solid #f3f4f6', marginTop: 4 }}>
              <Text style={{ fontSize: 12, color: '#71717a' }}>续期费</Text>
              <Text style={{ fontSize: 12, fontWeight: '500' }}>¥{calcResult.renewal_cost?.toFixed(2)}</Text>
            </View>
            {calcResult.overdue_balance > 0 && (
              <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 4 }}>
                <Text style={{ fontSize: 12, color: '#ef4444' }}>逾期费</Text>
                <Text style={{ fontSize: 12, fontWeight: '500', color: '#ef4444' }}>¥{calcResult.overdue_balance?.toFixed(2)}</Text>
              </View>
            )}
            <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 8, borderTop: '1px solid #e5e7eb', marginTop: 4 }}>
              <Text style={{ fontSize: 14, fontWeight: '700' }}>合计</Text>
              <Text style={{ fontSize: 14, fontWeight: '700' }}>¥{calcResult.total_amount?.toFixed(2)}</Text>
            </View>
            <Text style={{ fontSize: 11, color: '#9ca3af', marginTop: 8 }}>新到期日: {calcResult.new_end_date}</Text>
            {calcResult.renewal_cost <= 0 && (
              <Text style={{ fontSize: 12, color: '#ef4444', marginTop: 8 }}>当前订单定价数据不完整，请联系管理员</Text>
            )}
          </View>
        )}

        <View onClick={submitting ? undefined : handleSubmit}
          style={submitting ? { ...btnStyle('#9ca3af') } : btnStyle('#000')}>
          <Text>{submitting ? '处理中...' : `确认续期 ¥${calcResult?.total_amount?.toFixed(2) || '0.00'}`}</Text>
        </View>
      </ScrollView>
    </View>
  )
}
