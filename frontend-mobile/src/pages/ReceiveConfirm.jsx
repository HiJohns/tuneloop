import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { ArrowLeft, CheckCircle, Camera } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'

export default function ReceiveConfirm() {
  const { orderId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const instrumentId = searchParams.get('instrument')
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [photoFiles, setPhotoFiles] = useState([])

  useEffect(() => {
    const fetchData = async () => {
      try {
        const token = storage.getItem('token') || session.getItem('token')
        const headers = { 'Authorization': `Bearer ${token}` }
        const [orderResp, instResp] = await Promise.all([
          fetch(`${baseUrl}/orders/${orderId}`, { headers }),
          fetch(`${baseUrl}/public/instruments/${instrumentId}`, { headers }),
        ])
        const orderResult = await orderResp.json()
        const instResult = await instResp.json()
        if (orderResult.code === 20000) setOrder(orderResult.data)
        if (instResult.code === 20000) setInstrument(instResult.data)
      } catch (err) {
        console.error('Failed to load data:', err)
      }
      setLoading(false)
    }
    fetchData()
  }, [orderId, instrumentId])

  const handleConfirmReceive = async () => {
    setSubmitting(true)
    try {
      const token = storage.getItem('token') || session.getItem('token')
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
      const resp = await fetch(`${baseUrl}/warehouse/orders/${orderId}/delivery`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({ delivered_at: new Date().toISOString(), photos: photoUrls }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('确认收货成功')
        navigate('/my-leases', { replace: true })
      } else {
        dialog.alert('确认收货失败: ' + (result.message || ''))
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

  const startDate = order?.start_date ? (() => {
    const d = new Date(order.start_date)
    return `${d.getMonth() + 1}-${d.getDate()}`
  })() : ''

  const endDate = order?.end_date ? (() => {
    const d = new Date(order.end_date)
    return `${d.getMonth() + 1}-${d.getDate()}`
  })() : ''

  const leaseTerm = order?.lease_term || 0
  const rentalDays = (order?.start_date && order?.end_date)
    ? Math.max(1, Math.round((new Date(order.end_date) - new Date(order.start_date)) / 86400000))
    : leaseTerm * 30

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
        <Text className="text-lg font-black text-black">确认收货</Text>
      </View>

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

      {/* Order Info summary */}
      {order && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3">订单摘要</Text>
          <View className="space-y-2">
            <View className="flex justify-between text-sm">
              <Text className="text-zinc-500 font-medium">月租金</Text>
              <Text className="text-black font-black">¥{order.monthly_rent || 0}</Text>
            </View>
            <View className="flex justify-between text-sm">
              <Text className="text-zinc-500 font-medium">押金</Text>
              <Text className="text-black font-black">¥{order.deposit || 0}</Text>
            </View>
            <View className="flex justify-between text-sm">
              <Text className="text-zinc-500 font-medium">租赁人</Text>
              <Text className="text-black font-black">{order.user_name || '-'}</Text>
            </View>
          </View>
        </View>
      )}

      {/* Photo Upload */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <Camera size={18} />
          拍照留档
        </Text>
        <Text className="text-xs text-zinc-400 mb-3">请拍摄乐器当前状态照片作为签收留档</Text>
        <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
      </View>

      {/* Confirm Button */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <Button
          onClick={handleConfirmReceive}
          disabled={submitting || photoFiles.length === 0}
          className="w-full py-3 bg-green-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '确认收货'}
        </Button>
      </View>
    </View>
  )
}
