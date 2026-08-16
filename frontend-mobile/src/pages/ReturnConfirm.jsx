import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button, ScrollView, Input, Image } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { ArrowLeft, CheckCircle, Camera, Truck } from 'lucide-react'
import { getToken, redirectToLogin } from '../services/api'
import { dialog, env, uploadFile, getInputValue } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import OrderTimeline from '../components/OrderTimeline'

export default function ReturnConfirm() {
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
        if (orderResult.code === 20000) setOrder(orderResult.data)
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
        const upResult = env.isMiniProgram ? JSON.parse(upResp.data || '{}') : await upResp.json()
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

  if (loading) {
    return <View className="min-h-screen bg-[#FDFBF7] flex items-center justify-center">
      <Text className="text-zinc-400 font-medium">加载中...</Text>
    </View>
  }

  return (
    <View className="min-h-screen pb-24" style={{ backgroundColor: '#FDFBF7' }}>
      {!env.isMiniProgram && (
        <View className="px-4 pt-4 pb-3 flex items-center gap-2" style={{ background: 'linear-gradient(to bottom, #FDF4E7, #fff)' }}>
          <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
          <Text className="text-lg font-black text-black">归还乐器</Text>
        </View>
      )}

      <ScrollView>
      <View className="mx-4">{instrument && <InstrumentInfo instrument={instrument} onClick={() => navigate(`/instrument/${instrument.id}`)} />}</View>

      {order && (
        <>
          <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4 space-y-3">
            <View><Text className="text-base font-black text-black">订单信息</Text></View>
            <View className="space-y-2">
              <View className="flex items-center">
                <Text className="text-sm text-zinc-400 w-16">订单号</Text>
                <Text className="text-sm font-bold text-black">{(order.id || '').slice(0, 8)}</Text>
              </View>
              {order.start_date && (
                <View className="flex items-center">
                  <Text className="text-sm text-zinc-400 w-16">起始日</Text>
                  <Text className="text-sm font-bold text-black">{formatDisplayDate(order.start_date)}</Text>
                </View>
              )}
              {order.end_date && (
                <View className="flex items-center">
                  <Text className="text-sm text-zinc-400 w-16">到期日</Text>
                  <Text className="text-sm font-bold text-black">{formatDisplayDate(order.end_date)}</Text>
                </View>
              )}
            </View>
          </View>
          <OrderTimeline orderId={order.id} status={order.status} />
        </>
      )}

      {/* Return Address — preferred: transit_info (controlled), fallback: instrument.site_* */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 block">发回地址</Text>
        {(() => {
          const ti = order?.transit_info
          if (ti) {
            return (
              <View className="text-sm text-zinc-600">
                {ti.contact ? <Text className="block font-medium">{ti.contact}</Text> : null}
                {ti.phone ? <Text className="block">{ti.phone}</Text> : null}
                {ti.address ? <Text className="block">{ti.address}</Text> : null}
              </View>
            )
          }
          if (instrument?.site_name) {
            return (
              <View className="text-sm text-zinc-600">
                <Text className="block">{instrument.site_name}</Text>
                <Text className="block">{instrument.site_phone || ''}</Text>
                <Text className="block">{instrument.site_address || ''}</Text>
              </View>
            )
          }
          return null
        })()}
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
            <Input value={courierCompany} onInput={e => setCourierCompany(getInputValue(e))}
              placeholder="如：顺丰速运" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">快递单号</Text>
            <Input value={trackingNumber} onInput={e => setTrackingNumber(getInputValue(e))}
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
      </ScrollView>

      {/* Submit Button */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <Button onClick={handleSubmitReturn}
          disabled={submitting || !courierCompany.trim() || !trackingNumber.trim()}
          className="w-full py-3 bg-orange-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
          {submitting ? '提交中...' : '提交归还'}
        </Button>
      </View>
    </View>
  )
}
