import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button, Input } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function RepairQuote() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get('request_id')
  const baseUrl = env.apiBaseUrl

  const [request, setRequest] = useState(null)
  const [acceptedQuote, setAcceptedQuote] = useState(null)
  const [loading, setLoading] = useState(true)
  const [paying, setPaying] = useState(false)

  const fetchData = async () => {
    if (!requestId) return
    setLoading(true)
    try {
      const [rRes, qRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-requests/${requestId}`),
        apiFetch(`${baseUrl}/repair-requests/${requestId}/quotes`),
      ])
      const r = await rRes.json()
      const q = await qRes.json()
      if (r.code === 20000) setRequest(r.data)
      if (q.code === 20000) {
        const all = q.data?.list || []
        const accepted = all.find(qq => qq.status === 'accepted')
        setAcceptedQuote(accepted || null)
      }
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchData() }, [requestId])

  const handlePay = async () => {
    const payAmount = (acceptedQuote?.material_fee || 0) + (acceptedQuote?.service_fee || 0) + (acceptedQuote?.logistics_fee || 0) + (request?.merchant_type === 'controlled' ? (request?.transit_service_fee || 0) + (request?.transit_logistics_fee || 0) : 0)
    setPaying(true)
    try {
      const payRes = await apiFetch(`${baseUrl}/repair-requests/${requestId}/pay`, { method: 'POST' })
      const pay = await payRes.json()
      if (pay.code === 20000) {
        navigate('/repair-payment-complete', { state: { amount: payAmount, requestId } })
      } else {
        alert(pay.message || '支付失败')
      }
    } catch { alert('支付失败') }
    setPaying(false)
  }

  if (loading) return <View className="h-screen flex items-center justify-center"><Text className="text-zinc-400">加载中...</Text></View>
  if (!request) return <View className="h-screen flex items-center justify-center"><Text className="text-zinc-400">报修单不存在</Text></View>

  const status = request.status
  const isControlled = request.merchant_type === 'controlled'

  const materialFee = acceptedQuote?.material_fee || 0
  const serviceFee = acceptedQuote?.service_fee || 0
  const logisticsFee = acceptedQuote?.logistics_fee || 0
  const transitServiceFee = request?.transit_service_fee || 0
  const transitLogisticsFee = request?.transit_logistics_fee || 0
  const total = materialFee + serviceFee + logisticsFee + (isControlled ? transitServiceFee + transitLogisticsFee : 0)

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100 flex items-center gap-2">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1">维修报价</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        {/* Fee breakdown */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black mb-3">费用明细</Text>
          {acceptedQuote ? (
            <View className="space-y-2">
              <View className="flex justify-between">
                <Text className="text-xs text-zinc-500">材料费</Text>
                <Text className="text-xs text-zinc-700">¥{materialFee}</Text>
              </View>
              <View className="flex justify-between">
                <Text className="text-xs text-zinc-500">服务费</Text>
                <Text className="text-xs text-zinc-700">¥{serviceFee}</Text>
              </View>
              <View className="flex justify-between">
                <Text className="text-xs text-zinc-500">物流费 (C段)</Text>
                <Text className="text-xs text-zinc-700">¥{logisticsFee}</Text>
              </View>
              {isControlled && (
                <>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-500">中转服务费</Text>
                    <Text className="text-xs text-zinc-700">¥{transitServiceFee}</Text>
                  </View>
                  <View className="flex justify-between">
                    <Text className="text-xs text-zinc-500">中转物流费 (B+D段)</Text>
                    <Text className="text-xs text-zinc-700">¥{transitLogisticsFee}</Text>
                  </View>
                </>
              )}
              <View className="border-t border-zinc-200 pt-2 mt-2">
                <View className="flex justify-between">
                  <Text className="text-sm font-bold text-black">合计</Text>
                  <Text className="text-sm font-bold text-red-600">¥{total}</Text>
                </View>
              </View>
              {acceptedQuote.duration && (
                <View className="flex justify-between">
                  <Text className="text-xs text-zinc-500">工期</Text>
                  <Text className="text-xs text-zinc-700">{acceptedQuote.duration}</Text>
                </View>
              )}
              {acceptedQuote.comment && (
                <Text className="text-xs text-zinc-500 mt-1">备注：{acceptedQuote.comment}</Text>
              )}
            </View>
          ) : (
            <Text className="text-xs text-zinc-400">暂无报价信息</Text>
          )}
        </View>

        {/* PENDING PAYMENT: Pay button */}
        {status === 'pending_payment' && acceptedQuote && (
          <Button onClick={handlePay} disabled={paying}
            className="w-full mt-4 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
            {paying ? '处理中...' : '确认支付'}
          </Button>
        )}
      </ScrollView>
    </View>
  )
}
