import { useState, useEffect } from 'react'
import { useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Image } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { env } from '../platform'
import { ArrowLeft } from 'lucide-react'


function formatDate(raw) {
  if (!raw) return '-'
  const d = new Date(raw.slice(0, 10))
  if (isNaN(d.getTime())) return raw.slice(0, 10)
  return `${d.getFullYear()}年${d.getMonth() + 1}月${d.getDate()}日`
}

export default function Renewal() {
  // #1674: weapp jumps use query (?order_id=); H5 legacy links may use
  // /renewal/:orderId path param — dual-source read keeps both working.
  const { orderId: pathOrderId } = useParams()
  const [searchParams] = useSearchParams()
  const orderId = searchParams.get('order_id') || searchParams.get('orderId') || pathOrderId || ''
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl
  // #1733: weapp <Image> 相对路径 src 被当作包内路径 → 补全域名前缀
  const IMG_BASE = env.apiBaseUrl.replace(/\/api$/, '')
  const fixImg = (url) => !url || url.startsWith('http') || url.startsWith('data:') ? url : IMG_BASE + url

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
  }, [days, order, orderId])

  const handleSubmit = async () => {
    if (submitting || !calcResult) return
    setSubmitting(true)
    try {
      // #1722: 裸 fetch 在 weapp 不存在（ReferenceError "fetch is not defined"）→
      // 改用 apiFetch（platformRequest 跨端适配 + 自动带 Authorization）。
      const resp = await apiFetch(`${baseUrl}/orders/${orderId}/renewal/confirm`, {
        method: 'POST',
        body: JSON.stringify({ additional_days: days, open_id: '' }),
      })
      const result = await resp.json()
      if (result.code === 20000 && result.data?.success) {
        navigate(`/payment?type=renewal&id=${orderId}&amount=${calcResult.total_amount}`, { replace: true })
      } else {
        alert(resolveErrorMessage(result.data, '创建续期失败'))
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
  const minDays = calcResult?.min_additional_days || 0

  const dayOptions = [7, 15, 30, 60, 90, 180, 365]

  return (
    <View className="min-h-screen" style={{ backgroundColor: '#FDFBF7' }}>
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
        <Text className="text-lg font-black text-black">续期</Text>
      </View>

      <ScrollView className="w-full flex-1">
        <View className="px-4 box-border">
        {instrument && (
          <View className="bg-white rounded-2xl p-4 shadow-sm mb-3" style={{ cursor: 'pointer' }} onClick={() => navigate(`/instrument/${instrument.id}`)}>
            {(instrument.cover_image || instrument.images?.[0]) && (
              <Image src={fixImg(instrument.cover_image || instrument.images?.[0])} className="w-full h-40 object-cover rounded-lg bg-zinc-100 mb-3" mode="aspectFill" />
            )}
            <View className="text-base font-bold mb-2"><Text>{instrument.category_name || '乐器'}</Text></View>
            <View className="flex flex-row text-sm mb-1"><Text className="text-gray-500 w-24">SN</Text><Text className="text-black font-medium">{instrument.sn || '-'}</Text></View>
            <View className="flex flex-row text-sm mb-1"><Text className="text-gray-500 w-24">下单日</Text><Text className="text-black font-medium">{formatDate(order?.created_at)}</Text></View>
            <View className="flex flex-row text-sm mb-1"><Text className="text-gray-500 w-24">原预期归还</Text><Text className="text-black font-medium">{formatDate(endDate)}</Text></View>
            {overdueDays > 0 && (
              <View className="flex flex-row text-sm mb-1"><Text className="text-gray-500 w-24">超期</Text><Text className="font-medium text-red-500">{overdueDays} 天（续期需覆盖）</Text></View>
            )}
          </View>
        )}

        <View className="bg-white rounded-2xl p-4 shadow-sm mb-3">
          <Text className="text-base font-bold mb-3">续期设置</Text>
          {minDays > 1 && (
            <View className="text-xs text-red-500 mb-2"><Text>已超期，最少续期 {minDays} 天</Text></View>
          )}
          <View className="flex flex-wrap gap-2 mb-3">
            {dayOptions.map(d => {
              const disabled = d < minDays
              return (
                <View key={d}
                  onClick={() => !disabled && setDays(d)}
                  className={`px-4 py-2 rounded-full border cursor-pointer ${days === d ? 'bg-blue-600 text-white border-blue-600' : disabled ? 'bg-gray-100 text-gray-400 border-gray-200 opacity-60' : 'bg-white text-gray-700 border-gray-300'}`}>
                  <Text>{d}天</Text>
                </View>
              )
            })}
          </View>
          <View className="flex items-center gap-2">
            <Text className="text-sm text-gray-500">自定义:</Text>
            <input
              type="number"
              min={minDays || 1}
              value={days}
              onChange={e => setDays(Math.max(minDays || 1, parseInt(e.target.value) || (minDays || 1)))}
              className="w-20 px-2 py-1 border border-gray-300 rounded text-center text-sm"
            />
            <Text className="text-sm text-gray-500">天</Text>
          </View>
          {days > 0 && calcResult?.new_end_date && (
            <View className="mt-2 text-sm text-gray-500"><Text>预期归还日: </Text><Text className="text-black font-medium">{formatDate(calcResult.new_end_date)}</Text></View>
          )}
        </View>

        {calcResult && (
          <View className="bg-white rounded-2xl p-4 shadow-sm mb-3">
            <Text className="text-base font-bold mb-3">费用明细</Text>
            {calcResult.tier_breakdown?.map((t, i) => {
              const tierRate = (t.rate * (t.discount || 1)).toFixed(2)
              return (
              <View key={i} className="flex justify-between py-1 text-sm">
                <Text className="text-gray-500">第{t.tier}阶 {t.days}天 · ¥{((tierRate || 0) / 100).toFixed(2)}/天</Text>
                <Text className="font-medium">¥{(Number(t.subtotal || 0) / 100).toFixed(2)}</Text>
              </View>
            )})}
            <View className="flex justify-between py-1 text-sm border-t border-gray-100 mt-1">
              <Text className="text-gray-500">续期费</Text>
              <Text className="font-medium">¥{(Number(calcResult.renewal_cost || 0) / 100).toFixed(2)}</Text>
            </View>
            <View className="flex justify-between py-2 text-base font-bold border-t border-gray-200 mt-1">
              <Text>合计</Text>
              <Text>¥{(Number(calcResult.total_amount || 0) / 100).toFixed(2)}</Text>
            </View>
            <View className="mt-2 text-sm text-gray-400">
              <Text>新到期日: {formatDate(calcResult.new_end_date)}</Text>
            </View>
          </View>
        )}

        <View
          onClick={submitting ? undefined : handleSubmit}
          className={`w-full py-3 rounded-2xl font-black text-center text-white ${submitting ? 'bg-gray-400' : 'bg-black'}`}>
          <Text>{submitting ? '处理中...' : `确认续期 ¥${((calcResult?.total_amount || 0) / 100).toFixed(2)}`}</Text>
        </View>
        </View>
      </ScrollView>
    </View>
  )
}
