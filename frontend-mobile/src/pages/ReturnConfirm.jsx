import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { ArrowLeft, CheckCircle, Camera, Truck } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { getToken, redirectToLogin } from '../services/api'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

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

      // Upload photos
      const photoUrls = []
      for (const file of photoFiles) {
        const fd = new FormData()
        fd.append('file', file)
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
        dialog.alert('已提交归还申请')
        navigate('/profile')
      } else {
        dialog.alert('归还失败: ' + (result.message || ''))
      }
    } catch (err) {
      dialog.alert('操作失败: ' + err.message)
    }
    setSubmitting(false)
  }

  if (loading) {
    return <View className="min-h-screen bg-brand-bg flex items-center justify-center">
      <Text className="text-gray-500">加载中...</Text>
    </View>
  }

  const images = (() => {
    if (!instrument?.images) return []
    if (Array.isArray(instrument.images)) return instrument.images
    if (typeof instrument.images === 'string') {
      try { return JSON.parse(instrument.images) } catch { return [] }
    }
    return []
  })()

  return (
    <View className="min-h-screen bg-brand-bg pb-24">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">归还乐器</Text>
      </View>

      <View className="p-4 space-y-4">
        {/* Instrument Info */}
        <View className="bg-white rounded-xl p-4 cursor-pointer" onClick={() => navigate(`/instrument/${instrument.id}`)}>
          <Text className="font-medium mb-3">乐器信息</Text>
          <Image
            src={images[0] || PLACEHOLDER_IMAGE}
            alt={instrument?.sn}
            className="w-full h-40 object-contain bg-gray-100 rounded-lg mb-3"
          />
          <View className="space-y-2 text-sm">
            <View className="flex justify-between"><Text className="text-gray-500">识别码</Text><Text>{instrument?.sn || '-'}</Text></View>
            <View className="flex justify-between"><Text className="text-gray-500">类别</Text><Text>{instrument?.category_name || '-'}</Text></View>
            <View className="flex justify-between"><Text className="text-gray-500">商户</Text><Text>{instrument?.tenant_name || '-'}</Text></View>
            <View className="flex justify-between"><Text className="text-gray-500">所属网点</Text><Text>{instrument?.site_name || '-'}</Text></View>
          </View>
        </View>

        {/* Order Info */}
        {order && (
          <View className="bg-white rounded-xl p-4">
            <Text className="font-medium mb-3">租赁信息</Text>
            <View className="space-y-2 text-sm">
              <View className="flex justify-between"><Text className="text-gray-500">租期</Text><Text>{formatDisplayDate(order.start_date)} 至 {formatDisplayDate(order.end_date)}</Text></View>
              <View className="flex justify-between"><Text className="text-gray-500">月租金</Text><Text>¥{order.monthly_rent || 0}</Text></View>
              <View className="flex justify-between"><Text className="text-gray-500">押金</Text><Text>¥{order.deposit || 0}</Text></View>
              <View className="flex justify-between"><Text className="text-gray-500">租赁人</Text><Text>{order.user_name || '-'}</Text></View>
            </View>
          </View>
        )}

        {/* Logistics Info */}
        <View className="bg-white rounded-xl p-4">
          <Text className="font-medium mb-3 flex items-center gap-2">
            <Truck size={18} />
            物流信息
          </Text>
          <Text className="text-sm text-gray-500 mb-3">请填写返程物流信息，用于归还乐器</Text>
          <View className="space-y-3">
            <View>
              <label className="block text-sm text-gray-600 mb-1">承运公司</label>
              <input
                type="text"
                value={courierCompany}
                onChange={e => setCourierCompany(e.target.value)}
                placeholder="如：顺丰速运"
                className="w-full border rounded-lg px-3 py-2 text-sm"
              />
            </View>
            <View>
              <label className="block text-sm text-gray-600 mb-1">快递单号</label>
              <input
                type="text"
                value={trackingNumber}
                onChange={e => setTrackingNumber(e.target.value)}
                placeholder="请输入快递单号"
                className="w-full border rounded-lg px-3 py-2 text-sm"
              />
            </View>
          </View>
        </View>

        {/* Photo Upload */}
        <View className="bg-white rounded-xl p-4">
          <Text className="font-medium mb-3 flex items-center gap-2">
            <Camera size={18} />
            拍照留档
          </Text>
          <Text className="text-sm text-gray-500 mb-3">请拍摄乐器当前状态照片作为归还留档</Text>
          <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
        </View>

        {/* Submit Button */}
        <Button
          onClick={handleSubmitReturn}
          disabled={submitting || !courierCompany.trim() || !trackingNumber.trim()}
          className="w-full py-3 bg-orange-500 text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '提交归还'}
        </Button>
      </View>
    </View>
  )
}
