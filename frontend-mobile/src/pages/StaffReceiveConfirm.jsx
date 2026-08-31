import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Image, Button, ScrollView, Input } from '@tarojs/components'
import { ArrowLeft, CheckCircle, Camera, AlertTriangle, Image as ImageIcon } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { apiFetch , resolveErrorMessage } from '../services/api'
import { dialog, env, storage, session, uploadFile, getInputValue } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'

export default function StaffReceiveConfirm() {
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const orderId = searchParams.get('order_id') || ''
  const instrumentId = searchParams.get('instrument') || ''
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [photoFiles, setPhotoFiles] = useState([])
  const [outboundPhotos, setOutboundPhotos] = useState([])
  const [photoSpecs, setPhotoSpecs] = useState([])
  const [overdueFee, setOverdueFee] = useState('')
  const [additionalShippingFee, setAdditionalShippingFee] = useState('')
  const [damageAmount, setDamageAmount] = useState('')
  const [damageReason, setDamageReason] = useState('')

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
    setSubmitting(true)
    const token = storage.getItem('token') || session.getItem('token')
    try {
      const photoUrls = []
      for (const file of photoFiles) {
        const uploadResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const uploadResult = env.isMiniProgram ? JSON.parse(uploadResp.data || '{}') : await uploadResp.json()
        if (uploadResult.code === 20000 && uploadResult.data?.url) { photoUrls.push(uploadResult.data.url) }
      }
      const dmgAmt = parseFloat(damageAmount) || 0
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: instrument?.sn,
          scan_time: new Date().toISOString(),
          notes: damageReason.trim() || '验收通过',
          photos: photoUrls,
          damage_amount: dmgAmt,
          overdue_fee: parseFloat(overdueFee) || 0,
          additional_shipping_fee: parseFloat(additionalShippingFee) || 0,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        const sa = result.data?.shortfall_amount || 0
        if (sa > 0) {
          dialog.alert(`已发起结算，待顾客补缴 ¥${(sa / 100).toFixed(2)}，补缴完成后订单自动完成`)
        } else {
          dialog.alert('接收确认成功')
        }
        if (env.isMiniProgram) {
          Taro.navigateBack()
        } else if (dmgAmt <= 0) {
          navigate(`/return-settlement?order_id=${orderId}`)
        } else {
          navigate('/staff/orders')
        }
      } else { dialog.alert('接收失败: ' + (resolveErrorMessage(result, ''))) }
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
      {!env.isMiniProgram && (
        <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
          <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
          <Text className="text-lg font-black text-black">接收确认</Text>
        </View>
      )}

      <ScrollView>
      <View className="mx-4">{instrument && <InstrumentInfo instrument={instrument} onClick={() => env.isMiniProgram ? Taro.navigateTo({ url: `/pages-weapp/detail/index?id=${instrument.id}` }) : navigate(`/instrument?id=${instrument.id}`)} />}</View>

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

      {/* 追缴费用区块 (#1801) */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <AlertTriangle size={18} />追缴费用
        </Text>
        <View className="space-y-3">
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">逾期未缴租金（元）</Text>
            <Input type="number" value={overdueFee} onInput={e => setOverdueFee(getInputValue(e))}
              placeholder="0.00" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">追加物流费（元）</Text>
            <Input type="number" value={additionalShippingFee} onInput={e => setAdditionalShippingFee(getInputValue(e))}
              placeholder="0.00" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">损坏维修赔偿（元）</Text>
            <Input type="number" value={damageAmount} onInput={e => setDamageAmount(getInputValue(e))}
              placeholder="0.00（填0表示无损坏）" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
          <View>
            <Text className="text-xs font-bold text-zinc-500 mb-1">备注说明</Text>
            <Input value={damageReason} onInput={e => setDamageReason(getInputValue(e))}
              placeholder="选填" className="w-full border rounded-lg px-3 py-2 text-sm" />
          </View>
        </View>
      </View>
      </ScrollView>

      {/* Submit Button */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <Button onClick={handleConfirmReceive} disabled={submitting || photoFiles.length === 0}
          className="w-full py-3 bg-green-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
          <CheckCircle size={20} />{submitting ? '处理中...' : (photoFiles.length === 0 ? '请先拍照存档' : '确认接收')}
        </Button>
      </View>
    </View>
  )
}
