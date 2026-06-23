import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, Camera, User, MapPin, Package, Scan } from 'lucide-react'
import { dialog, env, storage, session, uploadFile, scanQRCode } from '../platform'
import InstrumentInfo from '../components/InstrumentInfo'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent('<svg width="200" height="200" xmlns="http://www.w3.org/2000/svg"><rect fill="#f3f4f6" width="200" height="200"/><text x="100" y="100" text-anchor="middle" dominant-baseline="middle" fill="#9ca3af" font-size="14">暂无图片</text></svg>')

export default function ShippingInterface() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [site, setSite] = useState(null)
  const [logistics, setLogistics] = useState({ company: '', trackingNumber: '' })
  const [photos, setPhotos] = useState([])
  const [submitting, setSubmitting] = useState(false)
  const [codeInput, setCodeInput] = useState('')
  const [lookupError, setLookupError] = useState('')
  const [lookupLoading, setLookupLoading] = useState(false)

  const baseUrl = env.apiBaseUrl

  const orderId = searchParams.get('order')

  useEffect(() => {
    if (orderId) {
      fetchOrder(orderId)
    }
  }, [orderId])

  const fetchOrder = async (orderId) => {
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${orderId}`)
      const result = await resp.json()
      if (result.code === 20000) {
        setOrder(result.data)
        const inst = await fetchInstrumentById(result.data.instrument_id)
        if (inst) {
          setInstrument(inst)
          if (inst.site_id) fetchSiteDetail(inst.site_id)
        }
      }
    } catch (err) {
      console.error('Failed to fetch order:', err)
    }
  }

  const fetchInstrumentById = async (id) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/${id}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data) return result.data
    } catch (err) {
      console.error('Failed to fetch instrument:', err)
    }
    return null
  }

  const fetchSiteDetail = async (siteId) => {
    try {
      const resp = await apiFetch(`${baseUrl}/common/sites/${siteId}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data) setSite(result.data)
    } catch (err) {
      console.error('Failed to fetch site:', err)
    }
  }

  const handleLookupByCode = async (code) => {
    if (!code.trim()) return
    setLookupError('')
    setLookupLoading(true)
    try {
      const sn = code.trim()
      const resp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(sn)}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data) {
        const orderStatus = result.data.order_status
        if (orderStatus === 'paid' || orderStatus === 'pending_shipment') {
          fetchOrder(result.data.order_id)
          setCodeInput('')
        } else {
          setLookupError('该订单当前不可发货（状态：' + (orderStatus || '未知') + '）')
        }
      } else {
        setLookupError(result.message || '未找到该乐器的待发货订单')
      }
    } catch (err) {
      setLookupError('查询失败: ' + err.message)
    }
    setLookupLoading(false)
  }

  const handleScan = async () => {
    try {
      const code = await scanQRCode()
      if (code) handleLookupByCode(code)
    } catch (err) {
      setLookupError('扫码失败: ' + err.message)
    }
  }

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setPhotos(prev => [...prev, ...files].slice(0, 10))
  }

  const removePhoto = (idx) => {
    setPhotos(prev => prev.filter((_, i) => i !== idx))
  }

  const canSubmit = !!(logistics.company.trim() && logistics.trackingNumber.trim() && photos.length >= 1 && orderId && !submitting)

  const handleSubmit = async () => {
    if (!canSubmit || !orderId) return

    setSubmitting(true)
    const token = storage.getItem('token') || session.getItem('token')

    try {
      const photoUrls = []
      for (const file of photos) {
        const resp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const result = await resp.json()
        if (result.code === 20000 && result.data?.url) {
          photoUrls.push(result.data.url)
        }
      }

      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/shipping`, {
        method: 'PUT',
        body: JSON.stringify({
          tracking_number: logistics.trackingNumber,
          company: logistics.company,
          shipped_at: new Date().toISOString(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('发货成功')
        navigate('/staff/orders')
      } else {
        dialog.alert('发货失败: ' + result.message)
      }
    } catch (err) {
      dialog.alert('发货失败: ' + err.message)
    }
    setSubmitting(false)
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-24">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-black text-black">发货</Text>
      </View>

      <ScrollView className="p-4 space-y-3">
        {!order && !orderId && (
          <View className="bg-white rounded-2xl shadow-sm p-4 space-y-4">
            <Text className="font-black text-black flex items-center gap-2">
              <Scan size={18} />
              扫描乐器识别码
            </Text>
            <View className="flex gap-2">
              <input
                type="text"
                value={codeInput}
                onChange={e => setCodeInput(e.target.value)}
                placeholder="输入乐器 SN 或扫码"
                className="flex-1 border rounded-lg px-3 py-2 text-sm"
              />
              <Button
                onClick={() => handleLookupByCode(codeInput)}
                disabled={lookupLoading || !codeInput.trim()}
                className="px-4 py-2 bg-black text-white rounded-lg text-sm font-black disabled:opacity-50"
              >
                <Text className="text-sm">{lookupLoading ? '查询中...' : '查询'}</Text>
              </Button>
              <Button
                onClick={handleScan}
                className="px-4 py-2 bg-black text-white rounded-lg text-sm font-black"
              >
                <Scan size={18} />
              </Button>
            </View>
            {lookupError && (
              <Text className="text-sm text-red-500 font-medium">{lookupError}</Text>
            )}
          </View>
        )}

        {order && instrument && (
          <>
            <InstrumentInfo instrument={instrument} />

            {/* Panel B: Site Info */}
            <View className="bg-white rounded-2xl shadow-sm p-4">
              <Text className="font-black text-black mb-3 flex items-center gap-2">
                <MapPin size={16} />
                网点信息
              </Text>
              <View className="space-y-2">
                {instrument.tenant_name && (
                  <View className="flex items-start gap-2">
                    <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">商户</Text>
                    <Text className="text-sm text-black font-medium">{instrument.tenant_name}</Text>
                  </View>
                )}
                {instrument.site_name && (
                  <View className="flex items-start gap-2">
                    <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">网点</Text>
                    <Text className="text-sm text-black font-medium">{instrument.site_name}</Text>
                  </View>
                )}
                {site?.address && (
                  <View className="flex items-start gap-2">
                    <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">地址</Text>
                    <Text className="text-sm text-black font-medium">{site.address}</Text>
                  </View>
                )}
                {site?.phone && (
                  <View className="flex items-start gap-2">
                    <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">电话</Text>
                    <Text className="text-sm text-black font-medium">{site.phone}</Text>
                  </View>
                )}
              </View>
            </View>

            {/* Panel C: Order Info */}
            <View className="bg-white rounded-2xl shadow-sm p-4">
              <Text className="font-black text-black mb-3">订单信息</Text>
              <View className="space-y-2">
                <View className="flex items-start gap-2">
                  <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">订单号</Text>
                  <Text className="text-sm text-black font-mono font-medium">{order.id}</Text>
                </View>
                <View className="flex items-start gap-2">
                  <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">创建时间</Text>
                  <Text className="text-sm text-black font-medium">{order.created_at ? new Date(order.created_at).toLocaleString() : '-'}</Text>
                </View>
                {order.user_name && (
                  <View className="flex items-start gap-2">
                    <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">创建人</Text>
                    <Text className="text-sm text-black font-medium">{order.user_name}</Text>
                  </View>
                )}
                {order.delivery_address && (
                  <View className="flex items-start gap-2">
                    <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">发货地址</Text>
                    <Text className="text-sm text-black font-medium">{formatDeliveryAddress(order.delivery_address)}</Text>
                  </View>
                )}
              </View>
            </View>

            {/* Customer Info (legacy section) */}
            {order.delivery_address && (
              <View className="bg-white rounded-2xl shadow-sm p-4">
                <Text className="font-black text-black mb-3 flex items-center gap-2">
                  <User size={16} />
                  收货人信息
                </Text>
                {order.user_name && (
                  <Text className="text-sm font-black text-black">{order.user_name}</Text>
                )}
                <View className="flex items-start gap-2 mt-1 text-sm text-zinc-500">
                  <MapPin size={14} className="mt-0.5 flex-shrink-0" />
                  <Text>{formatDeliveryAddress(order.delivery_address)}</Text>
                </View>
              </View>
            )}

            {/* Logistics Info */}
            <View className="bg-white rounded-2xl shadow-sm p-4 space-y-3">
              <Text className="font-black text-black flex items-center gap-2">
                <Text className="w-2 h-2 bg-red-500 rounded-full inline-block" />
                物流信息
                <Text className="text-xs text-zinc-400 font-normal">（必填）</Text>
              </Text>
              <input
                type="text"
                value={logistics.company}
                onChange={e => setLogistics({ ...logistics, company: e.target.value })}
                placeholder="承运公司"
                className="w-full border rounded-lg px-3 py-2 text-sm"
              />
              <input
                type="text"
                value={logistics.trackingNumber}
                onChange={e => setLogistics({ ...logistics, trackingNumber: e.target.value })}
                placeholder="快递单号"
                className="w-full border rounded-lg px-3 py-2 text-sm"
              />
            </View>

            {/* Photo Capture */}
            <View className="bg-white rounded-2xl shadow-sm p-4">
              <Text className="font-black text-black mb-3 flex items-center gap-2">
                <Camera size={16} />
                拍照留档
                <Text className="text-xs text-red-500 font-normal">（必填，至少 1 张）</Text>
              </Text>
              <View className="grid grid-cols-3 gap-2 mb-3">
                {photos.map((file, i) => (
                  <View key={i} className="relative aspect-square rounded-lg overflow-hidden border">
                    <Image src={URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                    <Button
                      onClick={() => removePhoto(i)}
                      className="absolute top-1 right-1 bg-black/50 rounded-full w-5 h-5 flex items-center justify-center"
                    >
                      <Text className="text-white text-xs">✕</Text>
                    </Button>
                  </View>
                ))}
                {photos.length < 10 && (
                  <label className="aspect-square border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center cursor-pointer text-gray-400 hover:text-brand-primary">
                    <Camera size={24} />
                    <Text className="text-xs mt-1">拍摄</Text>
                    <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
                  </label>
                )}
              </View>
              {photos.length === 0 && (
                <Text className="text-xs text-red-400">请先拍照存档（至少 1 张）</Text>
              )}
              {photos.length > 0 && (
                <Text className="text-xs text-gray-400">已拍摄 {photos.length} 张，最多 10 张</Text>
              )}
            </View>
          </>
        )}
      </ScrollView>

      {/* Submit Button */}
      {order && (
        <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
          <Button
            onClick={handleSubmit}
            disabled={!canSubmit}
            className="w-full py-3 bg-black text-white rounded-2xl font-black disabled:opacity-50 text-base"
          >
            {submitting ? '提交中...' : photos.length === 0 ? '请先拍照存档' : '提交'}
          </Button>
        </View>
      )}
    </View>
  )
}
