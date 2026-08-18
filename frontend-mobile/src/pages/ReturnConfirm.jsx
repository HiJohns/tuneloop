import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button, ScrollView, Input, Image } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { ArrowLeft, CheckCircle, Camera, Truck } from 'lucide-react'
import { getToken, redirectToLogin, apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, uploadFile, getInputValue, toWeappRoute } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'
import OrderTimeline from '../components/OrderTimeline'

export default function ReturnConfirm() {
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
  const [courierCompany, setCourierCompany] = useState('')
  const [trackingNumber, setTrackingNumber] = useState('')

  useEffect(() => {
    const fetchData = async () => {
      try {
        // apiFetch is cross-end (weapp has no global fetch) — it also
        // injects the Authorization header automatically.
        const orderResp = await apiFetch(`${baseUrl}/orders/${orderId}`)
        const orderResult = await orderResp.json()
        if (orderResult.code === 20000) {
          setOrder(orderResult.data)
          // instrument: URL 参数优先，否则从订单推导（weapp 跳转可能
          // 未带 / 带空 instrument，依赖订单自身 instrument_id 最稳）
          const instId = instrumentId || orderResult.data.instrument_id
          if (instId) {
            const instResp = await apiFetch(`${baseUrl}/public/instruments/${instId}`)
            const instResult = await instResp.json()
            if (instResult.code === 20000) setInstrument(instResult.data)
          }
        }
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
      const resp = await apiFetch(`${baseUrl}/orders/${orderId}/return`, {
        method: 'POST',
        body: JSON.stringify({
          courier_company: courierCompany.trim(),
          tracking_number: trackingNumber.trim(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        // L-02: 归还提交 → 结算页（收据明细 + 定损说明）。weapp 必须用
        // 完整页面路径（Taro shim 会把 /return-settlement/:id 转成不存在的
        // /pages/return-settlement/xxx/index → 静默失败）。
        if (env.isMiniProgram) {
          // #1702: redirectTo（替换归还物流页）而非 navigateTo（push）——
          // 结算页返回时不再回归还物流页
          Taro.redirectTo({ url: `/pages-weapp/return-settlement/index?order_id=${orderId}` })
        } else {
          navigate(`/return-settlement?order_id=${orderId}`, { replace: true })
        }
      } else {
        dialog.alert('归还失败: ' + (resolveErrorMessage(result, '')))
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
      <View className="mx-4">{instrument && <InstrumentInfo instrument={instrument} onClick={() => nav(`/instrument/${instrument.id}`)} />}</View>

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
          style={{ width: '100%', margin: 0, backgroundColor: '#f97316', color: '#fff', fontWeight: '800', fontSize: 16, height: 48, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', letterSpacing: '0.05em', opacity: submitting || !courierCompany.trim() || !trackingNumber.trim() ? 0.5 : 1 }}>
          {submitting ? '处理中...' : '提交归还'}
        </Button>
      </View>
    </View>
  )
}
