import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { ArrowLeft, CheckCircle, Camera, AlertTriangle, Image as ImageIcon } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { apiFetch } from '../services/api'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'

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
      } catch (err) { console.error('Failed to load data:', err) }
      setLoading(false)
    }
    fetchData()
  }, [orderId, instrumentId])

  useEffect(() => {
    if (!orderId) return
    apiFetch(`${baseUrl}/orders/${orderId}/outbound-photos`)
      .then(r => r.json())
      .then(res => { if (res.code === 20000) setOutboundPhotos(res.data.outbound_photos || []) })
      .catch(() => {})
  }, [orderId])

  useEffect(() => {
    if (!instrument?.category_id) return
    apiFetch(`${baseUrl}/instrument-photo-specs/${instrument.category_id}`)
      .then(r => r.json())
      .then(res => { if (res.code === 20000) setPhotoSpecs(res.data?.photo_requirements || []) })
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
      const photoUrls = []
      for (const file of photoFiles) {
        const uploadResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const uploadResult = await uploadResp.json()
        if (uploadResult.code === 20000 && uploadResult.data?.url) { photoUrls.push(uploadResult.data.url) }
      }
      const condition = hasDamage ? 'damaged' : 'good'
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: instrument?.sn,
          scan_time: new Date().toISOString(),
          condition,
          notes: hasDamage ? damageReason.trim() : '验收通过',
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('接收确认成功')
        navigate('/staff/orders')
      } else { dialog.alert('接收失败: ' + (result.message || '')) }
    } catch (err) { dialog.alert('操作失败: ' + err.message) }
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
        <Text className="text-lg font-black text-black">接收确认</Text>
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

      {/* Photo Upload */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <Camera size={18} />拍照留档
        </Text>
        <Text className="text-xs text-zinc-400 mb-3">请拍摄乐器当前状态照片作为接收留档</Text>

        {photoSpecs.length > 0 && (
          <View className="mb-4 p-3 bg-blue-50 rounded-lg">
            <Text className="text-sm font-bold text-blue-800 mb-1">拍照要求</Text>
            <Text className="text-xs text-blue-700 space-y-0.5">
              {photoSpecs.map((spec, idx) => (
                <Text key={idx} className="block">• {spec.position}: {spec.description}</Text>
              ))}
            </Text>
          </View>
        )}

        {outboundPhotos.length > 0 && (
          <View className="mb-4">
            <Text className="text-xs font-bold text-zinc-500 mb-2 flex items-center gap-1">
              <ImageIcon size={14} />出库照片（供对比）
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
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <AlertTriangle size={18} />定损
        </Text>
        <View className="flex gap-3 mb-3">
          <Button onClick={() => { setHasDamage(false); setDamageReason(''); setDamageAmount('') }}
            className={`flex-1 py-2 rounded-lg font-bold text-sm ${!hasDamage ? 'bg-green-100 text-green-700 border-2 border-green-500' : 'bg-zinc-100 text-zinc-500'}`}>
            无损坏
          </Button>
          <Button onClick={() => setHasDamage(true)}
            className={`flex-1 py-2 rounded-lg font-bold text-sm ${hasDamage ? 'bg-red-100 text-red-700 border-2 border-red-500' : 'bg-zinc-100 text-zinc-500'}`}>
            有损坏
          </Button>
        </View>
        {hasDamage && (
          <View className="space-y-3">
            <View>
              <Text className="text-xs font-bold text-zinc-500 mb-1">定损理由</Text>
              <input type="text" value={damageReason} onChange={e => setDamageReason(e.target.value)}
                placeholder="请描述损坏情况" className="w-full border rounded-lg px-3 py-2 text-sm" />
            </View>
            <View>
              <Text className="text-xs font-bold text-zinc-500 mb-1">定损金额</Text>
              <input type="number" value={damageAmount} onChange={e => setDamageAmount(e.target.value)}
                placeholder="请输入定损金额" className="w-full border rounded-lg px-3 py-2 text-sm" />
            </View>
          </View>
        )}
      </View>
      </ScrollView>

      {/* Submit Button */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <Button onClick={handleConfirmReceive} disabled={submitting}
          className="w-full py-3 bg-green-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
          <CheckCircle size={20} />{submitting ? '提交中...' : '确认接收'}
        </Button>
      </View>
    </View>
  )
}
