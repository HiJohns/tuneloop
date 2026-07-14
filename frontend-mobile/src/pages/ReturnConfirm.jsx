import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button, ScrollView } from '@tarojs/components'
import { ArrowLeft, CheckCircle, Camera, Truck } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { getToken, redirectToLogin } from '../services/api'
import { dialog, env, uploadFile } from '../platform'
import { formatDisplayDate, formatDeliveryAddress } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'

export default function ReturnConfirm() {
  const { orderId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const instrumentId = searchParams.get('instrument')
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [order, setOrder] = useState(null)
  const [site, setSite] = useState(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [photoFiles, setPhotoFiles] = useState([])
  const [courierCompany, setCourierCompany] = useState('')
  const [trackingNumber, setTrackingNumber] = useState('')

  useEffect(() => {
    const fetchData = async () => {
      try {
        const token = getToken()
        const headers = { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) }
        const [orderResp, instResp] = await Promise.all([
          fetch(`${baseUrl}/orders/${orderId}`, { headers }),
          fetch(`${baseUrl}/public/instruments/${instrumentId}`, { headers }),
        ])
        const orderResult = await orderResp.json()
        const instResult = await instResp.json()
        if (orderResult.code === 20000) {
          setOrder(orderResult.data)
          // Fetch site info for return address
          if (orderResult.data.org_id) {
            try {
              const siteResp = await fetch(`${baseUrl}/common/sites/${orderResult.data.org_id}`, { headers })
              const siteResult = await siteResp.json()
              if (siteResult.code === 20000) setSite(siteResult.data)
            } catch {}
          }
        }
        if (instResult.code === 20000) setInstrument(instResult.data)
      } catch (err) {
        console.error('Failed to load data:', err)
      }
      setLoading(false)
    }
    fetchData()
  }, [orderId, instrumentId])

  const handleSubmitReturn = async () => {
    if (!courierCompany.trim() || !trackingNumber.trim()) {
      dialog.alert('请填写物流信息')
      return
    }
    setSubmitting(true)
    try {
      const token = getToken()
      if (!token) { redirectToLogin(); return }
      const photoUrls = []
      for (const file of photoFiles) {
        const upResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const upResult = await upResp.json()
        if (upResult.code === 20000 && upResult.data?.url) {
          photoUrls.push(upResult.data.url)
        }
      }
      const resp = await fetch(`${baseUrl}/orders/${orderId}/return`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          courier_company: courierCompany.trim(),
          tracking_number: trackingNumber.trim(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        navigate(`/return-settlement/${orderId}`, { replace: true })
      } else {
        dialog.alert('归还失败: ' + (result.message || ''))
      }
    } catch (err) {
      dialog.alert('操作失败: ' + err.message)
    }
    setSubmitting(false)
  }

  if (loading) {
    return <View className="min-h-screen bg-[#FDFBF7] flex items-center justify-center">
      <Text className="text-zinc-400 font-medium">加载中...</Text>
    </View>
  }

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
        <Text className="text-lg font-black text-black">归还乐器</Text>
      </View>

      <ScrollView>
      <View className="mx-4">{instrument && <InstrumentInfo instrument={instrument} onClick={() => navigate(`/instrument/${instrument.id}`)} />}</View>

      {order && (
        <LeaseInfo
          status={order.status}
          startDate={order.start_date}
          endDate={order.end_date}
          deliveredAt={order.delivered_at}
          dailyRate={order.pricing_breakdown?.final_daily_rent || order.pricing_breakdown?.base_daily_rent || 0}
          rentDays={order.pricing_breakdown?.rent_days || 0}
          createdAt={order.created_at}
        />
      )}

      {/* Return Address — use site address, not customer delivery_address */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">发回地址</Text>
        {site ? (
          <View className="text-sm text-zinc-600 space-y-1">
            <Text className="block">{site.name}</Text>
            <Text className="block">{site.phone || ''}</Text>
            <Text className="block">{site.address || ''}</Text>
          </View>
        ) : (
          <Text className="text-sm text-zinc-600">{order.delivery_address ? formatDeliveryAddress(order.delivery_address) : '-'}</Text>
        )}
        <Text className="text-xs text-red-500 mt-2 block">* 请确保在物流留言中填写中转单号（若有）</Text>
      </View>

      {/* Logistics Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <Truck size={18} />物流信息
        </Text>
        <Text className="text-xs text-zinc-400 mb-3">请填写返程物流信息，用于归还乐器</Text>
        <View className="space-y-3">
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">承运公司</Text>
            <input type="text" value={courierCompany} onChange={e => setCourierCompany(e.target.value)}
              placeholder="如：顺丰速运" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">快递单号</Text>
            <input type="text" value={trackingNumber} onChange={e => setTrackingNumber(e.target.value)}
              placeholder="请输入快递单号" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
        </View>
      </View>

      {/* Photo Upload */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <Camera size={18} />拍照留档
        </Text>
        <Text className="text-xs text-zinc-400 mb-3">请拍摄乐器当前状态照片作为归还留档</Text>
        <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
      </View>
      </ScrollView>

      {/* Submit Button */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <Button onClick={handleSubmitReturn}
          disabled={submitting || !courierCompany.trim() || !trackingNumber.trim()}
          className="w-full py-3 bg-orange-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
          <CheckCircle size={20} />{submitting ? '提交中...' : '提交归还'}
        </Button>
      </View>
    </View>
  )
}
