import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { ArrowLeft, CheckCircle, Camera } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

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

      // 1. Upload photos first
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

      // 2. Confirm delivery
      const resp = await fetch(`${baseUrl}/warehouse/orders/${orderId}/delivery`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          delivered_at: new Date().toISOString(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('确认收货成功')
        navigate('/profile')
      } else {
        dialog.alert('确认收货失败: ' + (result.message || ''))
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
        <Text className="text-lg font-bold">确认收货</Text>
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
            {instrument?.tenant_name && <View className="flex justify-between"><Text className="text-gray-500">商户</Text><Text>{instrument.tenant_name}</Text></View>}
            <View className="flex justify-between"><Text className="text-gray-500">所属网点</Text><Text>{instrument?.site_name || '-'}</Text></View>
            {instrument?.properties && Object.keys(instrument.properties).length > 0 && (
              <View className="flex justify-between"><Text className="text-gray-500">动态属性</Text><Text>{JSON.stringify(instrument.properties)}</Text></View>
            )}
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

        {/* Photo Upload */}
        <View className="bg-white rounded-xl p-4">
          <Text className="font-medium mb-3 flex items-center gap-2">
            <Camera size={18} />
            拍照留档
          </Text>
          <Text className="text-sm text-gray-500 mb-3">请拍摄乐器当前状态照片作为签收留档</Text>
          <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
        </View>

        {/* Confirm Button */}
        <Button
          onClick={handleConfirmReceive}
          disabled={submitting || photoFiles.length === 0}
          className="w-full py-3 bg-green-500 text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '确认收货'}
        </Button>
      </View>
    </View>
  )
}
