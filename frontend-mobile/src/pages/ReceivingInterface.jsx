import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, Camera, Scan, CheckCircle, AlertTriangle, Upload, User, MapPin, Package } from 'lucide-react'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent('<svg width="200" height="200" xmlns="http://www.w3.org/2000/svg"><rect fill="#f3f4f6" width="200" height="200"/><text x="100" y="100" text-anchor="middle" dominant-baseline="middle" fill="#9ca3af" font-size="14">暂无图片</text></svg>')

export default function ReceivingInterface() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const preloadedOrderId = searchParams.get('order_id')
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

        const instResp = await apiFetch(`${baseUrl}/instruments/${orderResult.data.instrument_id}`)
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
      } catch (err) {
        console.error('Failed to load order:', err)
      }
    }
    loadOrder()
  }, [preloadedOrderId])

  useEffect(() => {
    if (orderID) {
      apiFetch(`${baseUrl}/orders/${orderID}/outbound-photos`)
        .then(r => r.json())
        .then(res => {
          if (res.code === 20000) {
            setOutboundPhotos(res.data.outbound_photos || [])
          }
        })
        .catch(() => {})
    }
  }, [orderID])

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setCapturedPhotos(prev => [...prev, ...files].slice(0, 10))
  }

  const checkInstrument = async (sn) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/check?sn=${encodeURIComponent(sn)}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data?.exists) {
        const inst = result.data.info
        setCurrentItem(inst)
        setCurrentSN(sn)
        setSnInput('')
        if (inst.category_id) {
          const specResp = await apiFetch(`${baseUrl}/instrument-photo-specs/${inst.category_id}`)
          const specResult = await specResp.json()
          if (specResult.code === 20000) {
            setPhotoSpecs(specResult.data?.photo_requirements || [])
          }
        }
        const orderResp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(sn)}`)
        const orderResult = await orderResp.json()
        setOrderID(orderResult.code === 20000 ? orderResult.data?.order_id : null)
      } else {
        dialog.alert('未找到该乐器')
      }
    } catch (err) {
      console.error('Failed to check instrument:', err)
    }
  }

  const handleSubmit = async () => {
    if (!currentItem) return
    if (!orderID) {
      dialog.alert('未找到该乐器的活跃订单')
      return
    }
    setSubmitting(true)
    const token = storage.getItem('token') || session.getItem('token')

    try {
      // Upload photos
      const photoUrls = []
      for (const file of capturedPhotos) {
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

      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderID}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: currentSN,
          scan_time: new Date().toISOString(),
          condition: condition,
          notes: condition === 'damaged' ? damageDesc : '',
          photos: photoUrls,
        }),
      })
      const result = await resp.json()

      if (result.code === 20000 && condition === 'damaged') {
        const damageResp = await apiFetch(`${baseUrl}/warehouse/orders/${orderID}/damage`, {
          method: 'PUT',
          body: JSON.stringify({
            damage_description: damageDesc,
            damage_amount: parseFloat(damageAmount) || 0,
          }),
        })
        const damageResult = await damageResp.json()
        if (damageResult.code === 20000) {
          navigate('/staff/orders')
          return
        } else {
          dialog.alert('定损评估失败: ' + damageResult.message)
          setSubmitting(false)
          return
        }
      } else if (result.code === 20000) {
        navigate('/staff/orders')
        return
      } else {
        dialog.alert('失败: ' + result.message)
      }

      setCurrentItem(null)
      setCurrentSN('')
      setCondition('')
      setDamageDesc('')
      setDamageAmount('')
      setOrderID(null)
      setOutboundPhotos([])
      setCapturedPhotos([])
    } catch (err) {
      dialog.alert('错误: ' + err.message)
    }
    setSubmitting(false)
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">收货确认</Text>
      </View>

      <View className="p-4 space-y-4">
        {/* Customer & Instrument Info (preloaded from order) */}
        {orderData && (
          <>
            <View className="bg-white rounded-xl p-4">
              <Text className="font-medium text-gray-900 mb-3 flex items-center gap-2">
                <User size={16} className="text-brand-primary" />
                租赁人信息
              </Text>
              <Text className="text-sm font-medium">{orderData.user_name || '-'}</Text>
              {orderData.delivery_address && (
                <View className="flex items-start gap-2 mt-2 text-sm text-gray-600">
                  <MapPin size={14} className="mt-0.5 flex-shrink-0" />
                  <Text>{formatDeliveryAddress(orderData.delivery_address)}</Text>
                </View>
              )}
            </View>

            {currentItem && (
              <View className="bg-white rounded-xl p-4 cursor-pointer" onClick={() => navigate(`/instrument/${currentItem.id}`)}>
                <Text className="font-medium text-gray-900 mb-3 flex items-center gap-2">
                  <Package size={16} className="text-brand-primary" />
                  乐器信息
                </Text>
                <View className="flex gap-3">
                  {(() => {
                    try {
                      const imgs = JSON.parse(currentItem.images || '[]')
                      if (imgs[0]) return <Image src={imgs[0]} alt="" className="w-16 h-16 object-cover rounded bg-gray-100" />
                    } catch {}
                    return <Image src={PLACEHOLDER_IMAGE} alt="" className="w-16 h-16 object-cover rounded bg-gray-100" />
                  })()}
                  <View>
                    <Text className="text-sm font-mono font-medium">SN: {currentItem.sn || '-'}</Text>
                    <Text className="text-xs text-gray-500">{currentItem.category_name}{currentItem.level_name ? ` · ${currentItem.level_name}` : ''}</Text>
                    {currentItem.tenant_name && <Text className="text-xs text-gray-400 mt-0.5">{currentItem.tenant_name}</Text>}
                    {currentItem.site_name && <Text className="text-xs text-gray-400">网点: {currentItem.site_name}</Text>}
                  </View>
                </View>
              </View>
            )}

            <View className="bg-white rounded-xl p-4">
              <Text className="font-medium text-gray-900 mb-2">租赁信息</Text>
              <View className="text-sm space-y-1 text-gray-600">
                <Text>租期: {formatDisplayDate(orderData.start_date)} 至 {formatDisplayDate(orderData.end_date)}</Text>
                {orderData.deposit > 0 && <Text>押金: ¥{orderData.deposit}</Text>}
              </View>
            </View>
          </>
        )}

        {/* QR Scan / SN Entry — only when not preloaded */}
        {!preloadedOrderId && (
        <View className="bg-white rounded-xl p-4">
          <Text className="font-medium mb-3">扫码或输入识别码</Text>
          <View className="flex gap-2">
            <input
              type="text"
              value={snInput}
              onChange={e => setSnInput(e.target.value)}
              placeholder="输入识别码或扫码"
              className="flex-1 border rounded-lg px-3 py-2"
              onKeyDown={e => e.key === 'Enter' && snInput && checkInstrument(snInput)}
            />
            <Button
              onClick={() => dialog.alert('扫码功能暂不可用')}
              className="px-4 py-2 border rounded-lg"
            >
              <Scan size={18} />
            </Button>
          </View>
        </View>
        )}

        {currentItem && (
          <View className="bg-white rounded-xl p-4">
            <View className="flex justify-between items-start mb-3 cursor-pointer" onClick={() => navigate(`/instrument/${currentItem.id}`)}>
              <View>
                <Text className="font-medium">{currentItem.name}</Text>
                <Text className="text-sm text-gray-500">{currentItem.brand} {currentItem.model}</Text>
                {currentItem.tenant_name && <Text className="text-xs text-gray-400 mt-0.5">{currentItem.tenant_name}</Text>}
                {currentItem.site_name && <Text className="text-xs text-gray-400">网点: {currentItem.site_name}</Text>}
              </View>
              <Text className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded-full">
                {currentItem.stock_status}
              </Text>
            </View>

            {photoSpecs.length > 0 && (
              <View className="mb-4">
                <Text className="text-sm font-medium flex items-center gap-1 mb-1">
                  <Camera size={14} className="text-brand-primary" />
                  拍照要求
                </Text>
                <ul className="text-xs text-gray-500 space-y-0.5">
                  {photoSpecs.map((spec, idx) => (
                    <li key={idx}>• {spec.position}: {spec.description}</li>
                  ))}
                </ul>
              </View>
            )}

            {outboundPhotos.length > 0 && (
              <View className="mb-4">
                <Text className="text-sm font-medium text-gray-700 mb-2">出库照片（供对比）</Text>
                <View className="grid grid-cols-2 gap-2">
                  {outboundPhotos.map((p, i) => (
                    <Image key={i} src={p.url} alt="outbound" className="w-full rounded border object-cover h-24" />
                  ))}
                </View>
              </View>
            )}

            <View className="mb-4">
              <Text className="text-sm font-medium text-gray-700 mb-2">归还拍照</Text>
              <label className="flex items-center gap-2 px-4 py-3 border-2 border-dashed rounded-lg cursor-pointer text-gray-500 hover:text-brand-primary">
                <Upload size={18} />
                <Text className="text-sm">拍照上传</Text>
                <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
              </label>
              {capturedPhotos.length > 0 && (
                <View className="grid grid-cols-3 gap-2 mt-2">
                  {capturedPhotos.map((file, i) => (
                    <View key={i} className="relative">
                      <Image src={URL.createObjectURL(file)} alt="captured" className="w-full rounded border object-cover h-20" />
                    </View>
                  ))}
                </View>
              )}
            </View>

            <View className="space-y-3">
              <View className="flex gap-2">
                <Button
                  onClick={() => setCondition('good')}
                  className={`flex-1 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-1 ${
                    condition === 'good' ? 'bg-green-500 text-white' : 'border text-gray-600'
                  }`}
                >
                  <CheckCircle size={16} /> 无损坏
                </Button>
                <Button
                  onClick={() => setCondition('damaged')}
                  className={`flex-1 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-1 ${
                    condition === 'damaged' ? 'bg-red-500 text-white' : 'border text-gray-600'
                  }`}
                >
                  <AlertTriangle size={16} /> 有损坏
                </Button>
              </View>

              {condition === 'damaged' && (
                <View className="space-y-2">
                  <textarea
                    value={damageDesc}
                    onChange={e => setDamageDesc(e.target.value)}
                    placeholder="请描述损坏情况"
                    className="w-full border rounded-lg px-3 py-2 text-sm"
                    rows={3}
                  />
                  <input
                    type="number"
                    value={damageAmount}
                    onChange={e => setDamageAmount(e.target.value)}
                    placeholder="定损金额"
                    className="w-full border rounded-lg px-3 py-2 text-sm"
                  />
                </View>
              )}

              {condition && (
                <Button
                  onClick={handleSubmit}
                  disabled={submitting}
                  className="w-full py-3 bg-brand-primary text-white rounded-lg disabled:opacity-50 font-medium"
                >
                  {submitting ? '提交中...' : '提交'}
                </Button>
              )}
            </View>
          </View>
        )}
      </View>
    </View>
  )
}
