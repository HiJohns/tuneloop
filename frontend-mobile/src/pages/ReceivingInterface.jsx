import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Image, Button, ScrollView, Input } from '@tarojs/components'
import { apiFetch , resolveErrorMessage } from '../services/api'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, Camera, Scan, AlertTriangle, User, MapPin } from 'lucide-react'
import { dialog, env, storage, session, uploadFile, navigation, getInputValue } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import StaffIdPhotoViewer from '../components/StaffIdPhotoViewer'

export default function ReceivingInterface() {
  const navigate = useNavigate()
  const preloadedOrderId = navigation.getQueryParams().order_id || ''
  const [snInput, setSnInput] = useState('')
  const [currentItem, setCurrentItem] = useState(null)
  const [currentSN, setCurrentSN] = useState('')
  const [orderData, setOrderData] = useState(null)
  const [condition, setCondition] = useState('')
  const [damageDesc, setDamageDesc] = useState('')
  const [damageAmount, setDamageAmount] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [photoSpecs, setPhotoSpecs] = useState([])
  const [orderID, setOrderID] = useState(null)
  const [outboundPhotos, setOutboundPhotos] = useState([])
  const [capturedPhotos, setCapturedPhotos] = useState([])

  const baseUrl = env.apiBaseUrl

  // Auto-load order when order_id is provided via URL
  useEffect(() => {
    if (!preloadedOrderId) return
    const loadOrder = async () => {
      try {
        const orderResp = await apiFetch(`${baseUrl}/orders/${preloadedOrderId}`)
        const orderResult = await orderResp.json()
        if (orderResult.code !== 20000) return
        setOrderData(orderResult.data)
        setOrderID(preloadedOrderId)

        const instResp = await apiFetch(`${baseUrl}/public/instruments/${orderResult.data.instrument_id}`)
        const instResult = await instResp.json()
        if (instResult.code !== 20000) return
        const inst = instResult.data
        setCurrentItem(inst)
        setCurrentSN(inst.sn || '')
        if (inst.category_id) {
          const specResp = await apiFetch(`${baseUrl}/instrument-photo-specs/${inst.category_id}`)
          const specResult = await specResp.json()
          if (specResult.code === 20000) setPhotoSpecs(specResult.data?.photo_requirements || [])
        }
      } catch (err) { console.error('Failed to load order:', err) }
    }
    loadOrder()
  }, [preloadedOrderId])

  useEffect(() => {
    if (orderID) {
      apiFetch(`${baseUrl}/orders/${orderID}/outbound-photos`)
        .then(r => r.json())
        .then(res => { if (res.code === 20000) setOutboundPhotos(res.data.outbound_photos || []) })
        .catch(() => {})
    }
  }, [orderID])

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setCapturedPhotos(prev => [...prev, ...files].slice(0, 10))
  }

  const handlePhotoCaptureWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: 10 - capturedPhotos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setCapturedPhotos(prev => [...prev, ...(res.tempFilePaths || [])].slice(0, 10))
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const checkInstrument = async (sn) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/check?sn=${encodeURIComponent(sn)}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data?.exists) {
        const inst = result.data.info
        // Load full instrument data (check API returns minimal fields)
        const fullResp = await apiFetch(`${baseUrl}/public/instruments/${inst.id}`)
        const fullResult = await fullResp.json()
        if (fullResult.code === 20000) {
          setCurrentItem(fullResult.data)
        } else {
          setCurrentItem(inst)
        }
        setCurrentSN(sn)
        setSnInput('')
        if (inst.category_id) {
          const specResp = await apiFetch(`${baseUrl}/instrument-photo-specs/${inst.category_id}`)
          const specResult = await specResp.json()
          if (specResult.code === 20000) setPhotoSpecs(specResult.data?.photo_requirements || [])
        }
        const orderResp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(sn)}`)
        const orderResult = await orderResp.json()
        setOrderID(orderResult.code === 20000 ? orderResult.data?.order_id : null)
      } else { dialog.alert('未找到该乐器') }
    } catch (err) { console.error('Failed to check instrument:', err) }
  }

  const handleSubmit = async () => {
    if (!currentItem) return
    if (!orderID) { dialog.alert('未找到该乐器的活跃订单'); return }
    setSubmitting(true)
    const token = storage.getItem('token') || session.getItem('token')
    try {
      const photoUrls = []
      for (const file of capturedPhotos) {
        const uploadResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const uploadResult = env.isMiniProgram ? JSON.parse(uploadResp.data || '{}') : await uploadResp.json()
        if (uploadResult.code === 20000 && uploadResult.data?.url) photoUrls.push(uploadResult.data.url)
      }
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderID}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: currentSN,
          scan_time: new Date().toISOString(),
          condition,
          notes: condition === 'damaged' ? damageDesc : '',
          photos: photoUrls,
          // L-04: 一次提交带 damage_amount（后端 damaged 时创建定损报告并
          // 通知顾客决策），不再单独调 /damage 二次定损
          damage_amount: condition === 'damaged' ? (parseFloat(damageAmount) || 0) : 0,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        if (condition === 'damaged') {
          dialog.alert('已提交定损，已通知顾客确认')
        } else {
          dialog.alert('验收通过，结算退款已自动发起')
        }
        env.isMiniProgram ? Taro.navigateBack() : navigate('/staff/orders')
        return
      }
      dialog.alert('失败: ' + resolveErrorMessage(result))
      setCurrentItem(null); setCurrentSN(''); setCondition(''); setDamageDesc(''); setDamageAmount(''); setOrderID(null); setOutboundPhotos([]); setCapturedPhotos([])
    } catch (err) { dialog.alert('错误: ' + err.message) }
    setSubmitting(false)
  }

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      {!env.isMiniProgram && (
        <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
          <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
          <Text className="text-lg font-black text-black">收货确认</Text>
        </View>
      )}

      <ScrollView className="flex-1">
      {/* Scan/Input panel — only show when no order_id preloaded */}
      {!preloadedOrderId && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3 flex items-center gap-2"><Scan size={18} />扫描乐器识别码</Text>
          <View className="flex gap-2">
            <Input value={snInput} onInput={e => setSnInput(getInputValue(e))} placeholder="输入乐器 SN"
              className="flex-1 border rounded-lg px-3 py-2 text-sm" />
            <Button onClick={() => checkInstrument(snInput)} className="px-4 py-2 bg-black text-white rounded-lg text-sm font-black">查询</Button>
          </View>
        </View>
      )}

      {/* Customer Info — when via order_id */}
      {orderData && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3 flex items-center gap-2"><User size={16} />归还人信息</Text>
          <Text className="text-sm font-black text-black">{orderData.user_name || '-'}</Text>
          {orderData.delivery_address && (
            <View className="flex items-start gap-2 mt-2 text-sm text-zinc-500">
              <MapPin size={14} className="mt-0.5 flex-shrink-0" />
              <Text>{formatDeliveryAddress(orderData.delivery_address)}</Text>
            </View>
          )}
          {orderData.user_id && <StaffIdPhotoViewer userId={orderData.user_id} showLabel={false} />}
        </View>
      )}

      {/* Instrument Info */}
      <View className="mx-4">{currentItem && <InstrumentInfo instrument={currentItem} />}</View>

      {/* Photo Specs + Outbound Photos */}
      {(photoSpecs.length > 0 || outboundPhotos.length > 0) && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          {(photoSpecs.length > 0) && (
            <View className="mb-4 p-3 bg-blue-50 rounded-lg">
              <Text className="text-sm font-bold text-blue-800 mb-1">拍照要求</Text>
              {photoSpecs.map((spec, idx) => (
                <Text key={idx} className="block text-xs text-blue-700">• {spec.position}: {spec.description}</Text>
              ))}
            </View>
          )}
          {outboundPhotos.length > 0 && (
            <View>
              <Text className="text-xs font-bold text-zinc-500 mb-2">出库照片（供对比）</Text>
              <View className="grid grid-cols-3 gap-2 mb-3">
                {outboundPhotos.map((p, i) => (
                  <Image key={i} src={p.url} alt="出库照" className="w-full rounded border object-cover h-20" />
                ))}
              </View>
            </View>
          )}
        </View>
      )}

      {/* Photo Capture */}
      {currentItem && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3 flex items-center gap-2"><Camera size={18} />归还拍照</Text>
          <View className="grid grid-cols-3 gap-2 mb-3">
            {capturedPhotos.map((file, i) => (
              <View key={i} className="relative aspect-square rounded-lg overflow-hidden border">
                <Image src={env.isMiniProgram ? file : URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                <Button onClick={() => setCapturedPhotos(prev => prev.filter((_, j) => j !== i))}
                  className="absolute top-1 right-1 bg-black/50 rounded-full w-5 h-5 flex items-center justify-center">
                  <Text className="text-white text-xs">✕</Text>
                </Button>
              </View>
            ))}
            {capturedPhotos.length < 10 && (
              env.isMiniProgram ? (
                <View className="aspect-square border-2 border-dashed border-zinc-300 rounded-lg flex flex-col items-center justify-center text-zinc-400" onClick={handlePhotoCaptureWeapp}>
                  <Camera size={24} /><Text className="text-xs mt-1">拍摄</Text>
                </View>
              ) : (
                <label className="aspect-square border-2 border-dashed border-zinc-300 rounded-lg flex flex-col items-center justify-center cursor-pointer text-zinc-400">
                  <Camera size={24} /><Text className="text-xs mt-1">拍摄</Text>
                  <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
                </label>
              )
            )}
          </View>
          <Text className="text-xs text-zinc-400">已拍摄 {capturedPhotos.length} 张，最多 10 张</Text>
        </View>
      )}

      {/* Damage Assessment */}
      {currentItem && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3 flex items-center gap-2"><AlertTriangle size={18} />定损评估</Text>
          <View className="flex gap-3 mb-3">
            <Button onClick={() => setCondition('good')}
              className={`flex-1 py-2 rounded-lg font-bold text-sm ${condition === 'good' ? 'bg-green-100 text-green-700 border-2 border-green-500' : 'bg-zinc-100 text-zinc-500'}`}>无损坏</Button>
            <Button onClick={() => { setCondition('damaged'); if (!damageAmount) setDamageAmount('0') }}
              className={`flex-1 py-2 rounded-lg font-bold text-sm ${condition === 'damaged' ? 'bg-red-100 text-red-700 border-2 border-red-500' : 'bg-zinc-100 text-zinc-500'}`}>有损坏</Button>
          </View>
          {condition === 'damaged' && (
            <View className="space-y-3">
              <View>
                <Text className="text-xs font-bold text-zinc-500 mb-1">损坏描述</Text>
                <Input value={damageDesc} onInput={e => setDamageDesc(getInputValue(e))} placeholder="描述损坏情况" className="w-full border rounded-lg px-3 py-2 text-sm" />
              </View>
              <View>
                <Text className="text-xs font-bold text-zinc-500 mb-1">定损金额</Text>
                <Input type="number" value={damageAmount} onInput={e => setDamageAmount(getInputValue(e))} placeholder="请输入金额" className="w-full border rounded-lg px-3 py-2 text-sm" />
               </View>
             </View>
           )}
         </View>
      )}
      </ScrollView>

      {/* Submit Button */}
      {currentItem && (
        <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
          <Button onClick={handleSubmit} disabled={submitting || !condition}
            style={{ width: '100%', margin: 0, backgroundColor: '#16a34a', color: '#fff', fontWeight: '800', fontSize: 16, height: 48, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', letterSpacing: '0.05em', opacity: submitting || !condition ? 0.5 : 1 }}>
            {submitting ? '处理中...' : '确认接收'}
          </Button>
        </View>
      )}
    </View>
  )
}
