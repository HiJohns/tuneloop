import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Textarea } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, Camera, Scan, CheckCircle, AlertTriangle, User, MapPin, Package } from 'lucide-react'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'
import InstrumentInfo from '../components/InstrumentInfo'

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
  const [notes, setNotes] = useState('')
  const [instruments, setInstruments] = useState([])
  const [scanning, setScanning] = useState(false)
  const [preloadedData, setPreloadedData] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const [photos, setPhotos] = useState([])
  const [photoSpecs, setPhotoSpecs] = useState([])
  const baseUrl = env.apiBaseUrl

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setPhotos(prev => [...prev, ...files].slice(0, 10))
  }

  const removePhoto = (idx) => setPhotos(prev => prev.filter((_, i) => i !== idx))

  const onSubmitReturn = async () => {
    if (!condition) { dialog.alert('请先评估乐器状况'); return }
    setSubmitting(true)
    const token = storage.getItem('token') || session.getItem('token')
    try {
      const photoUrls = []
      for (const file of photos) {
        const upResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const upResult = await upResp.json()
        if (upResult.code === 20000 && upResult.data?.url) photoUrls.push(upResult.data.url)
      }
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderData.order_id}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: currentItem?.sn,
          scan_time: new Date().toISOString(),
          condition,
          notes: notes || '验收通过',
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) { dialog.alert('接收确认成功'); navigate('/staff/orders') }
      else dialog.alert('提交失败: ' + (result.message || ''))
    } catch (e) { dialog.alert('操作失败') }
    setSubmitting(false)
  }

  const handleScan = async () => {
    try {
      const { scanQRCode } = await import('../platform')
      const code = await scanQRCode()
      if (code) { setSnInput(code); handleQuery(code) }
    } catch { dialog.alert('扫码失败，请手动输入') }
  }

  const handleQuery = async (sn) => {
    const code = sn || snInput
    if (!code.trim()) return
    setCurrentSN(code.trim())
    try {
      const resp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(code.trim())}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data) {
        if (result.data.order_status !== 'returning') { dialog.alert('该订单状态不是"归还中"，无法接收'); return }
        setOrderData(result.data)
        await loadInstrument(result.data.instrument_id)
      } else dialog.alert('未找到乐器')
    } catch (e) { dialog.alert('查询失败') }
  }

  const loadInstrument = async (id) => {
    try {
      const resp = await apiFetch(`${baseUrl}/public/instruments/${id}`)
      const result = await resp.json()
      if (result.code === 20000) setCurrentItem(result.data)
    } catch {}
  }

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
        <Text className="text-lg font-black text-black">接收</Text>
      </View>

      <ScrollView className="flex-1">
      {/* Scan/Input panel */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
          <Scan size={18} />扫描乐器识别码
        </Text>
        <View className="flex gap-2">
          <input type="text" value={snInput} onChange={e => setSnInput(e.target.value)}
            placeholder="输入乐器 SN"
            className="flex-1 border rounded-lg px-3 py-2 text-sm" />
          <Button onClick={() => handleQuery()} className="px-4 py-2 bg-black text-white rounded-lg text-sm font-black">查询</Button>
          <Button onClick={handleScan} className="px-4 py-2 bg-black text-white rounded-lg text-sm font-black"><Scan size={18} /></Button>
        </View>
      </View>

      {currentItem && <InstrumentInfo instrument={currentItem} onClick={() => currentItem?.id && navigate(`/instrument/${currentItem.id}`)} />}

      {orderData && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3">订单信息</Text>
          <View className="space-y-2">
            <View className="flex justify-between text-sm">
              <Text className="text-zinc-500 font-medium">租期起点</Text>
              <Text className="text-black font-black">{orderData.start_date ? formatDisplayDate(orderData.start_date) : '-'}</Text>
            </View>
            <View className="flex justify-between text-sm">
              <Text className="text-zinc-500 font-medium">预计到期</Text>
              <Text className="text-black font-black">{orderData.end_date ? formatDisplayDate(orderData.end_date) : '-'}</Text>
            </View>
            <View className="flex justify-between text-sm">
              <Text className="text-zinc-500 font-medium">状态</Text>
              <Text className="text-orange-600 font-black">归还中</Text>
            </View>
          </View>
        </View>
      )}

      {/* Damage Assessment */}
      {currentItem && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
            <AlertTriangle size={18} />乐器状况
          </Text>
          <View className="flex gap-3 mb-3">
            <Button onClick={() => setCondition('good')}
              className={`flex-1 py-2 rounded-lg font-bold text-sm ${condition === 'good' ? 'bg-green-100 text-green-700 border-2 border-green-500' : 'bg-zinc-100 text-zinc-500'}`}>
              无损坏
            </Button>
            <Button onClick={() => setCondition('damaged')}
              className={`flex-1 py-2 rounded-lg font-bold text-sm ${condition === 'damaged' ? 'bg-red-100 text-red-700 border-2 border-red-500' : 'bg-zinc-100 text-zinc-500'}`}>
              有损坏
            </Button>
          </View>
          <Textarea value={notes} onChange={e => setNotes(e.target.value)}
            placeholder="备注（可选）" className="w-full border rounded-lg px-3 py-2 text-sm" rows={2} />
        </View>
      )}

      {/* Photo Capture */}
      {currentItem && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3 flex items-center gap-2">
            <Camera size={18} />拍照留档
          </Text>
          <View className="grid grid-cols-3 gap-2 mb-3">
            {photos.map((file, i) => (
              <View key={i} className="relative aspect-square rounded-lg overflow-hidden border">
                <Image src={URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                <Button onClick={() => removePhoto(i)}
                  className="absolute top-1 right-1 bg-black/50 rounded-full w-5 h-5 flex items-center justify-center">
                  <Text className="text-white text-xs">✕</Text>
                </Button>
              </View>
            ))}
            {photos.length < 10 && (
              <label className="aspect-square border-2 border-dashed border-zinc-300 rounded-lg flex flex-col items-center justify-center cursor-pointer text-zinc-400 hover:text-brand-primary">
                <Camera size={24} /><Text className="text-xs mt-1">拍摄</Text>
                <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
              </label>
            )}
          </View>
          <Text className="text-xs text-zinc-400">已拍摄 {photos.length} 张，最多 10 张</Text>
        </View>
      )}
      </ScrollView>

      {currentItem && (
        <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
          <Button onClick={onSubmitReturn} disabled={submitting || !condition}
            className="w-full py-3 bg-green-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
            <CheckCircle size={20} />{submitting ? '提交中...' : '确认接收'}
          </Button>
        </View>
      )}
    </View>
  )
}
