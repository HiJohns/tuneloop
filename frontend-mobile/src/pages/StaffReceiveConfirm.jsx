import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { ArrowLeft, CheckCircle, Camera, AlertTriangle, Image as ImageIcon } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { apiFetch } from '../services/api'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

export default function StaffReceiveConfirm() {
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
  const [outboundPhotos, setOutboundPhotos] = useState([])
  const [photoSpecs, setPhotoSpecs] = useState([])
  const [hasDamage, setHasDamage] = useState(false)
  const [damageReason, setDamageReason] = useState('')
  const [damageAmount, setDamageAmount] = useState('')

  useEffect(() => {
    const fetchData = async () => {
      try {
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
    fetchData()
  }, [orderId, instrumentId])

  useEffect(() => {
    if (!orderId) return
    apiFetch(`${baseUrl}/orders/${orderId}/outbound-photos`)
      .then(r => r.json())
      .then(res => {
        if (res.code === 20000) setOutboundPhotos(res.data.outbound_photos || [])
      })
      .catch(() => {})
  }, [orderId])

  useEffect(() => {
    if (!instrument?.category_id) return
    apiFetch(`${baseUrl}/instrument-photo-specs/${instrument.category_id}`)
      .then(r => r.json())
      .then(res => {
        if (res.code === 20000) setPhotoSpecs(res.data?.photo_requirements || [])
      })
      .catch(() => {})
  }, [instrument?.category_id])

  const handleConfirmReceive = async () => {
    if (hasDamage && (!damageReason.trim() || !damageAmount.trim())) {
      dialog.alert('请填写定损理由和金额')
      return
    }
    setSubmitting(true)
    const token = storage.getItem('token') || session.getItem('token')

    try {
      // Upload photos first
      const photoUrls = []
      for (const file of photoFiles) {
        const fd = new FormData()
        fd.append('file', file)
        const uploadResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const uploadResult = await uploadResp.json()
        if (uploadResult.code === 20000 && uploadResult.data?.url) {
          photoUrls.push(uploadResult.data.url)
        }
      }

      const condition = hasDamage ? 'damaged' : 'good'

      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: instrument?.sn,
          scan_time: new Date().toISOString(),
          condition: condition,
          notes: hasDamage ? damageReason.trim() : '验收通过',
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('接收确认成功')
        navigate('/staff/orders')
      } else {
        dialog.alert('接收失败: ' + (result.message || ''))
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

  const parseImages = (images) => {
    if (!images) return []
    if (Array.isArray(images)) return images
    if (typeof images === 'string') {
      try { return JSON.parse(images) } catch { return [] }
    }
    return []
  }

  const images = parseImages(instrument?.images)

  return (
    <View className="min-h-screen bg-brand-bg pb-24">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">接收确认</Text>
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
          </View>
        </View>

        {/* Order Info */}
        {order && (
          <View className="bg-white rounded-xl p-4">
            <Text className="font-medium mb-3">租赁信息</Text>
            <View className="space-y-2 text-sm">
              <View className="flex justify-between"><Text className="text-gray-500">租期</Text><Text>{formatDisplayDate(order.start_date)} 至 {formatDisplayDate(order.end_date)}</Text></View>
              {order.deposit > 0 && <View className="flex justify-between"><Text className="text-gray-500">押金</Text><Text>¥{order.deposit}</Text></View>}
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
          <Text className="text-sm text-gray-500 mb-3">请拍摄乐器当前状态照片作为接收留档</Text>

          {photoSpecs.length > 0 && (
            <View className="mb-4 p-3 bg-blue-50 rounded-lg">
              <Text className="text-sm font-medium text-blue-800 mb-1">拍照要求</Text>
              <ul className="text-xs text-blue-700 space-y-0.5">
                {photoSpecs.map((spec, idx) => (
                  <li key={idx}>• {spec.position}: {spec.description}</li>
                ))}
              </ul>
            </View>
          )}

          {outboundPhotos.length > 0 && (
            <View className="mb-4">
              <Text className="text-sm font-medium text-gray-700 mb-2 flex items-center gap-1">
                <ImageIcon size={14} />
                出库照片（供对比）
              </Text>
              <View className="grid grid-cols-3 gap-2">
                {outboundPhotos.map((p, i) => (
                  <Image key={i} src={p.url} alt="出库照" className="w-full rounded border object-cover h-20" />
                ))}
              </View>
            </View>
          )}

          <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
        </View>

        {/* Damage Assessment */}
        <View className="bg-white rounded-xl p-4">
          <Text className="font-medium mb-3 flex items-center gap-2">
            <AlertTriangle size={18} />
            定损
          </Text>
          <View className="flex gap-3 mb-3">
            <Button
              onClick={() => { setHasDamage(false); setDamageReason(''); setDamageAmount('') }}
              className={`flex-1 py-2 rounded-lg font-medium text-sm ${!hasDamage ? 'bg-green-100 text-green-700 border-2 border-green-500' : 'bg-gray-100 text-gray-500'}`}
            >
              无损坏
            </Button>
            <Button
              onClick={() => setHasDamage(true)}
              className={`flex-1 py-2 rounded-lg font-medium text-sm ${hasDamage ? 'bg-red-100 text-red-700 border-2 border-red-500' : 'bg-gray-100 text-gray-500'}`}
            >
              有损坏
            </Button>
          </View>
          {hasDamage && (
            <View className="space-y-3">
              <View>
                <label className="block text-sm text-gray-600 mb-1">定损理由</label>
                <input
                  type="text"
                  value={damageReason}
                  onChange={e => setDamageReason(e.target.value)}
                  placeholder="请描述损坏情况"
                  className="w-full border rounded-lg px-3 py-2 text-sm"
                />
              </View>
              <View>
                <label className="block text-sm text-gray-600 mb-1">定损金额</label>
                <input
                  type="number"
                  value={damageAmount}
                  onChange={e => setDamageAmount(e.target.value)}
                  placeholder="请输入定损金额"
                  className="w-full border rounded-lg px-3 py-2 text-sm"
                />
              </View>
            </View>
          )}
        </View>

        {/* Submit Button */}
        <Button
          onClick={handleConfirmReceive}
          disabled={submitting}
          className="w-full py-3 bg-green-600 text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '确认接收'}
        </Button>
      </View>
    </View>
  )
}
