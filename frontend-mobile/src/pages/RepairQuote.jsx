import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'

export default function RepairQuote() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const requestId = searchParams.get('request_id')
  const baseUrl = env.apiBaseUrl

  const [quotes, setQuotes] = useState([])
  const [request, setRequest] = useState(null)
  const [selectedQuote, setSelectedQuote] = useState(null)
  const [loading, setLoading] = useState(true)
  const [paying, setPaying] = useState(false)

  const fetchData = async () => {
    if (!requestId) return
    setLoading(true)
    try {
      const [qRes, rRes] = await Promise.all([
        apiFetch(`${baseUrl}/quotes/${requestId}`),
        apiFetch(`${baseUrl}/repair-requests/${requestId}`),
      ])
      const q = await qRes.json()
      const r = await rRes.json()
      if (q.code === 20000) setQuotes(q.data?.list || [])
      if (r.code === 20000) setRequest(r.data)
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchData() }, [requestId])

  // Determine merchant type: check if site is transit
  const isTransit = request?.site_id || false

  const handlePay = async () => {
    if (!selectedQuote) { alert('请选择报价'); return }
    setPaying(true)
    try {
      // Accept quote
      const acceptRes = await apiFetch(`${baseUrl}/quotes/${selectedQuote}/accept`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ repair_request_id: requestId }),
      })
      const accept = await acceptRes.json()
      if (accept.code !== 20000) { alert(accept.message); return }

      // Pay
      const payRes = await apiFetch(`${baseUrl}/repair-requests/${requestId}/pay`, { method: 'POST' })
      const pay = await payRes.json()
      if (pay.code === 20000) {
        alert('支付成功，请填写物流信息')
        navigate(`/create-repair?request_id=${requestId}`)
      } else {
        alert(pay.message || '支付失败')
      }
    } catch {}
    setPaying(false)
  }

  if (loading) return <View><Text>加载中...</Text></View>
  if (!request) return <View><Text>报修单不存在</Text></View>

  return (
    <View className="h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100 flex items-center">
        <Text className="text-lg font-bold flex-1">报价选择</Text>
      </View>
      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-3">
          <Text className="text-sm font-bold text-black">维修报价 ({quotes.length})</Text>
          {quotes.length === 0 ? (
            <Text className="text-xs text-zinc-400">暂无报价</Text>
          ) : quotes.map(q => (
            <View key={q.id}
              className={`p-3 border rounded-xl ${selectedQuote === q.id ? 'border-black bg-zinc-50' : 'border-zinc-200'}`}
              onClick={() => setSelectedQuote(q.id)}>
              <View className="flex justify-between">
                <Text className="text-sm font-bold text-black">¥{q.quote_amount}</Text>
                <Text className="text-xs text-zinc-400">{q.timeframe || ''}</Text>
              </View>
              {q.comment && <Text className="text-xs text-zinc-500 mt-1">{q.comment}</Text>}
            </View>
          ))}
        </View>

        {selectedQuote && (
          <Button onClick={handlePay} disabled={paying}
            className="w-full mt-4 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
            {paying ? '处理中...' : '选择此报价并支付'}
          </Button>
        )}

        {/* Shipping info panel */}
        {(request.status === 'pending_payment' || request.status === 'pending_ship') && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-2">发往地址</Text>
            {isTransit ? (
              <Text className="text-xs text-zinc-500">中转网点地址（受控商户）</Text>
            ) : (
              <Text className="text-xs text-zinc-500">网点地址（全权商户）</Text>
            )}
          </View>
        )}
      </ScrollView>
    </View>
  )
}
