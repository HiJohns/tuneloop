import { useState, useEffect, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button, Input, Image } from '@tarojs/components'
import { addressesApi } from '../services/api'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { Camera } from 'lucide-react'

export default function CreateRepairRequest() {
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl

  const [form, setForm] = useState({
    sn: '', instrument_type: '', brand: '', model: '',
    description: '', photos: [], video: null,
    site_id: '', merchant_id: '',
    merchant_type: '', transit_site_id: '',
  })
  const [merchants, setMerchants] = useState([])
  const [hasControlled, setHasControlled] = useState(false)
  const [cooperativeMode, setCooperativeMode] = useState(false)
  const [sites, setSites] = useState([])
  const [transitSites, setTransitSites] = useState([])
  const [showMerchantPicker, setShowMerchantPicker] = useState(false)
  const [showSitePicker, setShowSitePicker] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const debounceTimer = useRef(null)

  const handleSnChange = (val) => {
    setForm(p => ({ ...p, sn: val }))
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(async () => {
      if (!val.trim()) return
      try {
        const resp = await apiFetch(`${baseUrl}/public/instruments/lookup?sn=${val}`)
        const r = await resp.json()
        if (r.code === 20000 && r.data?.instrument) {
          setForm(p => ({ ...p, sn: val, instrument_type: r.data.instrument.instrument_type || p.instrument_type, brand: r.data.instrument.brand || p.brand, model: r.data.instrument.model || p.model }))
        }
      } catch {}
    }, 500)
  }

  useEffect(() => {
    apiFetch(`${baseUrl}/public/merchants`).then(r => r.json()).then(r => {
      if (r.code === 20000) { setMerchants(r.data?.merchants || []); setHasControlled(r.data?.has_controlled || false) }
    }).catch(() => {})
  }, [])

  const handleMerchantSelect = (m) => {
    setForm(p => ({ ...p, merchant_id: m.id, site_id: '', merchant_type: '' }))
    setCooperativeMode(false)
    setShowMerchantPicker(false)
    if (m.id === '__cooperative__') {
      setCooperativeMode(true)
      setForm(p => ({ ...p, merchant_type: 'controlled' }))
      apiFetch(`${baseUrl}/public/sites?type=transit`).then(r => r.json()).then(r => { if (r.code === 20000) setTransitSites(r.data?.list || []) }).catch(() => {})
    } else {
      apiFetch(`${baseUrl}/public/sites?merchant_id=${m.id}`).then(r => r.json()).then(r => { if (r.code === 20000) setSites(r.data?.list || []) }).catch(() => {})
    }
  }

  const uploadFile = async (file) => {
    const fd = new FormData()
    fd.append('file', file)
    const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', body: fd })
    const r = await resp.json()
    if (r.code === 20000) return r.data.file_key
    throw new Error(r.message || 'upload failed')
  }

  const isFormValid = form.sn && form.instrument_type && form.brand && form.model &&
    form.description && form.photos.length > 0 && form.site_id

  const handleSubmit = async () => {
    if (!isFormValid) { alert('请填写所有必填项'); return }
    setSubmitting(true)
    try {
      const photoKeys = []
      for (const f of form.photos) {
        const key = await uploadFile(f)
        photoKeys.push(key)
      }
      let videoKey = ''
      if (form.video) {
        videoKey = await uploadFile(form.video)
      }
      const resp = await apiFetch(`${baseUrl}/repair-requests`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          sn: form.sn,
          instrument_type: form.instrument_type,
          brand: form.brand,
          model: form.model,
          description: form.description,
          photos: photoKeys,
          video_url: videoKey,
          site_id: form.site_id,
          merchant_type: form.merchant_type || undefined,
          transit_site_id: form.transit_site_id || undefined,
        }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        alert('报修单已提交，等待评估')
        navigate(-1)
      } else {
        alert(r.message || '提交失败')
      }
    } catch (err) {
      alert('提交失败: ' + (err.message || ''))
    }
    setSubmitting(false)
  }

  return (
    <View className="h-screen bg-gray-50 flex flex-col">
      <View className="bg-gradient-to-b from-blue-50 to-white px-4 py-3">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center">创建报修单</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-3">
          <View>
            <Text className="block text-sm font-medium text-gray-700 mb-1">识别码 *</Text>
            <input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
              value={form.sn} onChange={e => handleSnChange(e.target.value)} placeholder="输入识别码" />
          </View>
          <View className="grid grid-cols-2 gap-2">
            <View>
              <Text className="block text-sm font-medium text-gray-700 mb-1">类型 *</Text>
              <input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                value={form.instrument_type} onChange={e => setForm(p => ({ ...p, instrument_type: e.target.value }))} placeholder="乐器类型" />
            </View>
            <View>
              <Text className="block text-sm font-medium text-gray-700 mb-1">品牌 *</Text>
              <input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
                value={form.brand} onChange={e => setForm(p => ({ ...p, brand: e.target.value }))} placeholder="品牌" />
            </View>
          </View>
          <View>
            <Text className="block text-sm font-medium text-gray-700 mb-1">型号 *</Text>
            <input className="w-full border border-gray-300 rounded-lg px-4 py-3 text-sm focus:outline-none focus:ring-2 focus:ring-blue-400"
              value={form.model} onChange={e => setForm(p => ({ ...p, model: e.target.value }))} placeholder="型号" />
          </View>
          <View>
            <Text className="block text-sm font-medium text-gray-700 mb-1">描述 *</Text>
            <textarea className="w-full border border-gray-300 rounded-lg p-3 text-sm" rows={3}
              value={form.description} onChange={e => setForm(p => ({ ...p, description: e.target.value }))} placeholder="描述故障情况" />
          </View>
          <View>
            <Text className="block text-sm font-medium text-gray-700 mb-1">照片 *（{form.photos.length} 张）</Text>
            <View className="grid grid-cols-3 gap-2 mb-2">
              {form.photos.map((file, i) => (
                <View key={i} className="relative aspect-square rounded-lg overflow-hidden border">
                  <Image src={URL.createObjectURL(file)} className="w-full h-full object-cover" mode="aspectFill" />
                  <View className="absolute top-1 right-1 bg-black/50 rounded-full w-5 h-5 flex items-center justify-center"
                    onClick={() => setForm(p => ({ ...p, photos: p.photos.filter((_, j) => j !== i) }))}>
                    <Text className="text-white text-xs">✕</Text>
                  </View>
                </View>
              ))}
              {form.photos.length < 10 && (
                <label className="aspect-square border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center text-gray-400 active:opacity-60">
                  <Camera size={24} />
                  <Text className="text-xs mt-1">拍摄</Text>
                  <input type="file" accept="image/*" capture="environment" multiple className="hidden"
                    onChange={e => setForm(p => ({ ...p, photos: [...p.photos, ...Array.from(e.target.files || [])].slice(0, 10) }))} />
                </label>
              )}
            </View>
            {form.photos.length === 0 && (
              <Text className="text-xs text-red-500">请先拍照存档（至少 1 张）</Text>
            )}
          </View>
          <View>
            <Text className="block text-sm font-medium text-gray-700 mb-1">视频（可选，估价用）</Text>
            <label className="flex items-center gap-2 py-2 bg-gray-100 rounded-lg px-3 active:opacity-60">
              <Camera size={20} className="text-gray-500" />
              <Text className="text-xs text-gray-600">{form.video ? '已选择视频' : '上传视频'}</Text>
              <input type="file" accept="video/*" className="hidden"
                onChange={e => { const f = e.target.files?.[0]; if (f) setForm(p => ({ ...p, video: f })) }} />
            </label>
          </View>
          <View>
            <Text className="block text-sm font-medium text-gray-700 mb-1">选择商户</Text>
            <Button onClick={() => setShowMerchantPicker(true)}
              className="w-full py-2 bg-gray-100 rounded-lg text-xs text-left px-3 text-gray-600">
              {form.merchant_id ? merchants.find(m => m.id === form.merchant_id)?.name || '已选' : '点击选择商户 *'}
            </Button>
          </View>
          {form.merchant_id && (
            <View>
              <Text className="block text-sm font-medium text-gray-700 mb-1">选择网点</Text>
              <Button onClick={() => setShowSitePicker(true)}
                className="w-full py-2 bg-gray-100 rounded-lg text-xs text-left px-3 text-gray-600">
                {form.site_id ? sites.find(s => s.id === form.site_id)?.name || '已选' : '点击选择网点 *'}
              </Button>
            </View>
          )}
          <Button onClick={handleSubmit} disabled={!isFormValid || submitting}
            className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center mt-2">
            {submitting ? '提交中...' : '提交评估'}
          </Button>
        </View>
      </ScrollView>

      {/* Merchant picker modal */}
      {showMerchantPicker && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-end" onClick={() => setShowMerchantPicker(false)}>
          <View className="bg-white rounded-t-2xl w-full max-h-80 p-4" onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-black mb-3">选择商户</Text>
            {merchants.map(m => (
              <View key={m.id} className="py-3 border-b border-gray-50 active:opacity-60"
                onClick={() => handleMerchantSelect(m)}>
                <Text className="text-sm text-black">{m.name}</Text>
              </View>
            ))}
            {hasControlled && (
              <View className="py-3 border-b border-gray-50 active:opacity-60"
                onClick={() => handleMerchantSelect({ id: '__cooperative__', name: '合作商家' })}>
                <Text className="text-sm text-blue-600 font-bold">合作商家</Text>
              </View>
            )}
          </View>
        </View>
      )}

      {/* Site picker modal (full merchant) */}
      {showSitePicker && !cooperativeMode && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-end" onClick={() => setShowSitePicker(false)}>
          <View className="bg-white rounded-t-2xl w-full max-h-80 p-4" onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-black mb-3">选择网点</Text>
            {sites.map(s => (
              <View key={s.id} className="py-3 border-b border-gray-50 active:opacity-60"
                onClick={() => { setForm(p => ({ ...p, site_id: s.id, merchant_type: 'full' })); setShowSitePicker(false) }}>
                <Text className="text-sm text-black">{s.name}</Text>
              </View>
            ))}
          </View>
        </View>
      )}

      {/* Transit site picker (cooperative/controlled mode) */}
      {showSitePicker && cooperativeMode && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-end" onClick={() => setShowSitePicker(false)}>
          <View className="bg-white rounded-t-2xl w-full max-h-80 p-4" onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-black mb-3">选择中转网点</Text>
            {transitSites.map(s => (
              <View key={s.id} className="py-3 border-b border-gray-50 active:opacity-60"
                onClick={() => { setForm(p => ({ ...p, site_id: s.id, transit_site_id: s.id, merchant_type: 'controlled' })); setShowSitePicker(false) }}>
                <Text className="text-sm text-black">{s.name}</Text>
              </View>
            ))}
          </View>
        </View>
      )}
    </View>
  )
}
