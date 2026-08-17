import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button, Image } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { apiFetch , resolveErrorMessage } from '../services/api'
import { ArrowLeft, Camera } from 'lucide-react'
import { calculateDays } from '../utils/daycalc'
import { dialog, env, storage, session, uploadFile, toWeappRoute } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'

export default function ReceiveConfirm() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  // Cross-end navigation (issue-1673): weapp has no react-router short paths;
  // central toWeappRoute maps H5 paths → /pages-weapp/... page urls.
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }
  // 跨端统一 query 约定（#1674）：跳转一律 ?order_id= / ?instrument=
  const orderId = searchParams.get('order_id') || ''
  const instrumentId = searchParams.get('instrument') || ''
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [photoFiles, setPhotoFiles] = useState([])

  useEffect(() => {
    const fetchData = async () => {
      try {
        // apiFetch is cross-end (weapp has no global fetch) and injects auth
        const [orderResp, instResp] = await Promise.all([
          apiFetch(`${baseUrl}/orders/${orderId}`),
          apiFetch(`${baseUrl}/public/instruments/${instrumentId}`),
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
    if (orderId) fetchData()
  }, [orderId, instrumentId, baseUrl])

  // 拍照（复用发货页模式：weapp Taro.chooseImage / H5 file input）
  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setPhotoFiles(prev => [...prev, ...files].slice(0, 5))
  }

  const handlePhotoCaptureWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: 5 - photoFiles.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setPhotoFiles(prev => [...prev, ...(res.tempFilePaths || [])].slice(0, 5))
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const removePhoto = (idx) => {
    setPhotoFiles(prev => prev.filter((_, i) => i !== idx))
  }

  const handleConfirmReceive = async () => {
    if (!orderId) { dialog.alert('订单不存在'); return }
    setSubmitting(true)
    try {
      const token = storage.getItem('token') || session.getItem('token')
      const photoUrls = []
      for (const file of photoFiles) {
        const upResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const upResult = env.isMiniProgram ? JSON.parse(upResp.data || '{}') : await upResp.json()
        if (upResult.code === 20000 && upResult.data?.url) {
          photoUrls.push(upResult.data.url)
        }
      }
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/delivery`, {
        method: 'PUT',
        body: JSON.stringify({ delivered_at: new Date().toISOString(), photos: photoUrls }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('确认收货成功')
        if (env.isMiniProgram) {
          Taro.navigateBack()
        } else {
          navigate('/my-leases', { replace: true })
        }
      } else {
        dialog.alert('确认收货失败: ' + (resolveErrorMessage(result, '')))
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
    ? calculateDays(new Date(order.start_date), new Date(order.end_date))
    : leaseTerm * 30

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      {!env.isMiniProgram && (
        <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
          <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
          <Text className="text-lg font-black text-black">确认收货</Text>
        </View>
      )}

      <View className="mx-4">{instrument && <InstrumentInfo instrument={instrument} onClick={() => nav(`/instrument/${instrument.id}`)} />}</View>

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

      {/* Fee Info */}
      {order && (() => {
        const pb = order.pricing_breakdown || {}
        const subtotal = pb.total_amount || 0
        const deposit = order.deposit || 0
        const shipping = pb.shipping_fee || order.shipping_fee || 0
        const total = subtotal + deposit + shipping
        return (
          <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
            <Text className="text-base font-black text-black mb-3">费用信息</Text>
            <View className="space-y-2">
              <View className="flex justify-between text-sm">
                <Text className="text-zinc-500 font-medium">租金小计</Text>
                <Text className="text-black font-black">¥{subtotal.toFixed(2)}</Text>
              </View>
              <View className="flex justify-between text-sm">
                <Text className="text-zinc-500 font-medium">押金</Text>
                <Text className="text-black font-black">¥{deposit.toFixed(2)}</Text>
              </View>
              {shipping > 0 && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">物流费</Text>
                  <Text className="text-black font-black">¥{shipping.toFixed(2)}</Text>
                </View>
              )}
              <View className="border-t border-zinc-100 pt-2 mt-1">
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-700 font-bold">合计</Text>
                  <Text className="text-green-600 font-black text-base">¥{total.toFixed(2)}</Text>
                </View>
              </View>
            </View>
          </View>
        )
      })()}

      {/* Photo Upload */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <Camera size={18} />
          拍照留档
        </Text>
        <Text className="text-xs text-zinc-400 mb-3">请拍摄乐器当前状态照片作为签收留档</Text>
        <View className="flex flex-wrap gap-2">
          {photoFiles.map((file, i) => (
            <View key={i} className="relative">
              <Image src={env.isMiniProgram ? file : URL.createObjectURL(file)} className="w-20 h-20 object-cover rounded-lg" mode="aspectFill" />
              <View onClick={() => removePhoto(i)}
                className="absolute -top-2 -right-2 bg-red-500 text-white rounded-full w-5 h-5 flex items-center justify-center">
                <Text className="text-white text-xs">✕</Text>
              </View>
            </View>
          ))}
          {photoFiles.length < 5 && (
            env.isMiniProgram ? (
              <View onClick={handlePhotoCaptureWeapp}
                className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center text-gray-400">
                <Camera size={20} />
                <Text className="text-xs mt-1">拍摄</Text>
              </View>
            ) : (
              <label className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center cursor-pointer text-gray-400">
                <Camera size={20} />
                <Text className="text-xs mt-1">拍摄</Text>
                <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
              </label>
            )
          )}
        </View>
      </View>

      {/* Confirm Button */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <Button
          onClick={handleConfirmReceive}
          disabled={submitting || photoFiles.length === 0}
          style={{ width: '100%', margin: 0, backgroundColor: '#16a34a', color: '#fff', fontWeight: '800', fontSize: 16, height: 48, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', letterSpacing: '0.05em', opacity: submitting || photoFiles.length === 0 ? 0.5 : 1 }}
        >
          {submitting ? '处理中...' : '确认收货'}
        </Button>
      </View>
    </View>
  )
}
