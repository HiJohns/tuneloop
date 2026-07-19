import { useState, useEffect } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { View, Text, ScrollView } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'

export default function Renewal() {
  const { orderId } = useParams()
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl

  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [days, setDays] = useState(30)
  const [calcResult, setCalcResult] = useState(null)
  const [submitting, setSubmitting] = useState(false)

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
    if (!order || !days) return
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
  }, [days, orderId])

  const handleSubmit = async () => {
    if (submitting || !calcResult) return
    setSubmitting(true)
    try {
      const token = getToken()
      const resp = await fetch(`${baseUrl}/orders/${orderId}/renewal/confirm`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` },
        body: JSON.stringify({ additional_days: days, open_id: '' }),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data?.success) {
        navigate(`/payment?type=renewal&id=${orderId}&amount=${calcResult.total_amount}`, { replace: true })
      } else {
        alert(result.data?.message || '创建续期失败')
      }
    } catch (err) {
      alert('网络错误')
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) return <View className="flex items-center justify-center h-screen"><Text>加载中...</Text></View>
  if (!order) return <View className="flex items-center justify-center h-screen"><Text>订单不存在</Text></View>

  const endDate = order.end_date || '-'
  const overdueDays = calcResult?.overdue_days || 0

  const dayOptions = [7, 15, 30, 60, 90, 180, 365]

  return (
    <View className="min-h-screen bg-gray-50">
      <View className="bg-white px-4 py-3 flex items-center gap-3 border-b border-gray-200">
        <View onClick={() => navigate(-1)} className="cursor-pointer">
          <Text className="text-lg">{'<'}</Text>
        </View>
        <Text className="text-lg font-bold">续期</Text>
      </View>

      <ScrollView className="p-4">
        {instrument && (
          <View className="bg-white rounded-2xl p-4 shadow-sm mb-3">
            <Text className="text-base font-bold mb-2">{instrument.category_name || '乐器'}</Text>
            <Text className="text-sm text-gray-500">SN: {instrument.sn || '-'}</Text>
            <Text className="text-sm text-gray-500">当前到期: {endDate}</Text>
            {overdueDays > 0 && (
              <Text className="text-sm text-red-500 mt-1">已逾期 {overdueDays} 天</Text>
            )}
          </View>
        )}

        <View className="bg-white rounded-2xl p-4 shadow-sm mb-3">
          <Text className="text-base font-bold mb-3">续期天数</Text>
          <View className="flex flex-wrap gap-2 mb-3">
            {dayOptions.map(d => (
              <View key={d}
                onClick={() => setDays(d)}
                className={`px-4 py-2 rounded-full border cursor-pointer ${days === d ? 'bg-blue-600 text-white border-blue-600' : 'bg-white text-gray-700 border-gray-300'}`}>
                <Text>{d}天</Text>
              </View>
            ))}
          </View>
          <View className="flex items-center gap-2">
            <Text className="text-sm text-gray-500">自定义:</Text>
            <input
              type="number"
              min={1}
              value={days}
              onChange={e => setDays(Math.max(1, parseInt(e.target.value) || 1))}
              className="w-20 px-2 py-1 border border-gray-300 rounded text-center text-sm"
            />
            <Text className="text-sm text-gray-500">天</Text>
          </View>
        </View>

        {calcResult && (
          <View className="bg-white rounded-2xl p-4 shadow-sm mb-3">
            <Text className="text-base font-bold mb-3">费用明细</Text>
            {calcResult.tier_breakdown?.map((t, i) => (
              <View key={i} className="flex justify-between py-1 text-sm">
                <Text className="text-gray-500">第{t.tier}阶 {t.days}天</Text>
                <Text className="font-medium">¥{t.subtotal?.toFixed(2)}</Text>
              </View>
            ))}
            <View className="flex justify-between py-1 text-sm border-t border-gray-100 mt-1">
              <Text className="text-gray-500">续期费</Text>
              <Text className="font-medium">¥{calcResult.renewal_cost?.toFixed(2)}</Text>
            </View>
            {calcResult.overdue_balance > 0 && (
              <View className="flex justify-between py-1 text-sm">
                <Text className="text-red-500">逾期费（待结清）</Text>
                <Text className="font-medium text-red-500">¥{calcResult.overdue_balance?.toFixed(2)}</Text>
              </View>
            )}
            <View className="flex justify-between py-2 text-base font-bold border-t border-gray-200 mt-1">
              <Text>合计</Text>
              <Text>¥{calcResult.total_amount?.toFixed(2)}</Text>
            </View>
            <View className="mt-2 text-sm text-gray-400">
              <Text>新到期日: {calcResult.new_end_date}</Text>
            </View>
          </View>
        )}

        <View
          onClick={submitting ? undefined : handleSubmit}
          className={`w-full py-3 rounded-2xl font-black text-center text-white ${submitting ? 'bg-gray-400' : 'bg-black'}`}>
          <Text>{submitting ? '处理中...' : `确认续期 ¥${calcResult?.total_amount?.toFixed(2) || '0.00'}`}</Text>
        </View>
      </ScrollView>
    </View>
  )
}
