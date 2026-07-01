import { useState, useEffect, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button, Input } from '@tarojs/components'
import { addressesApi } from '../services/api'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function CreateRepairRequest() {
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl

  const [form, setForm] = useState({
    sn: '', instrument_type: '', brand: '', model: '',
    description: '', photos: [],
    tracking_company: '', tracking_number: '',
    site_id: '', merchant_id: '',
  })
  const [merchants, setMerchants] = useState([])
  const [sites, setSites] = useState([])
  const [showMerchantPicker, setShowMerchantPicker] = useState(false)
  const [showSitePicker, setShowSitePicker] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const debounceTimer = useRef(null)

  // SN debounce lookup (300-500ms)
  const handleSnChange = (val) => {
    setForm(p => ({ ...p, sn: val }))
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(async () => {
      if (!val.trim()) return
      try {
        const resp = await apiFetch(`${baseUrl}/user-instruments/lookup?sn=${val}`)
        const r = await resp.json()
        if (r.code === 20000 && r.data?.instrument) {
          setForm(p => ({
            ...p, sn: val,
            instrument_type: r.data.instrument.instrument_type || p.instrument_type,
            brand: r.data.instrument.brand || p.brand,
            model: r.data.instrument.model || p.model,
          }))
        }
      } catch {}
    }, 500)
  }

  // Load merchants
  useEffect(() => {
    apiFetch(`${baseUrl}/merchants`).then(r => r.json()).then(r => {
      if (r.code === 20000) setMerchants(r.data?.list || [])
    }).catch(() => {})
  }, [])

  const handleMerchantSelect = (m) => {
    setForm(p => ({ ...p, merchant_id: m.id, site_id: '' }))
    setShowMerchantPicker(false)
    // Load sites for this merchant
    apiFetch(`${baseUrl}/sites?merchant_id=${m.id}`).then(r => r.json()).then(r => {
      if (r.code === 20000) setSites(r.data?.list || [])
    }).catch(() => {})
  }

  const isFormValid = form.sn && form.instrument_type && form.brand && form.model &&
    form.description && form.photos.length > 0 && form.site_id

  const handleSubmit = async () => {
    if (!isFormValid) { alert('请填写所有必填项'); return }
    setSubmitting(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(form),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        alert('报修单已提交')
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
    <View className="h-screen bg-zinc-50 flex flex-col">
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center">创建报修单</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-3">
          <View>
            <Text className="block text-xs font-medium text-zinc-500 mb-1">识别码 *</Text>
            <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
              value={form.sn} onChange={e => handleSnChange(e.target.value)} placeholder="输入识别码" />
          </View>
          <View className="grid grid-cols-2 gap-2">
            <View>
              <Text className="block text-xs font-medium text-zinc-500 mb-1">类型 *</Text>
              <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
                value={form.instrument_type} onChange={e => setForm(p => ({ ...p, instrument_type: e.target.value }))} placeholder="乐器类型" />
            </View>
            <View>
              <Text className="block text-xs font-medium text-zinc-500 mb-1">品牌 *</Text>
              <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
                value={form.brand} onChange={e => setForm(p => ({ ...p, brand: e.target.value }))} placeholder="品牌" />
            </View>
          </View>
          <View>
            <Text className="block text-xs font-medium text-zinc-500 mb-1">型号 *</Text>
            <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
              value={form.model} onChange={e => setForm(p => ({ ...p, model: e.target.value }))} placeholder="型号" />
          </View>
          <View>
            <Text className="block text-xs font-medium text-zinc-500 mb-1">描述 *</Text>
            <textarea className="w-full border border-zinc-300 rounded-lg p-3 text-sm" rows={3}
              value={form.description} onChange={e => setForm(p => ({ ...p, description: e.target.value }))} placeholder="描述故障情况" />
          </View>
          <View>
            <Text className="block text-xs font-medium text-zinc-500 mb-1">照片 *</Text>
            <Button onClick={() => setForm(p => ({ ...p, photos: [...p.photos, `photo_${Date.now()}.jpg`] }))}
              className="py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">+ 拍照（{form.photos.length} 张）</Button>
          </View>
          <View>
            <Text className="block text-xs font-medium text-zinc-500 mb-1">选择商户</Text>
            <Button onClick={() => setShowMerchantPicker(true)}
              className="w-full py-2 bg-zinc-100 rounded-lg text-xs text-left px-3 text-zinc-600">
              {form.merchant_id ? merchants.find(m => m.id === form.merchant_id)?.name || '已选' : '点击选择商户 *'}
            </Button>
          </View>
          {form.merchant_id && (
            <View>
              <Text className="block text-xs font-medium text-zinc-500 mb-1">选择网点</Text>
              <Button onClick={() => setShowSitePicker(true)}
                className="w-full py-2 bg-zinc-100 rounded-lg text-xs text-left px-3 text-zinc-600">
                {form.site_id ? sites.find(s => s.id === form.site_id)?.name || '已选' : '点击选择网点 *'}
              </Button>
            </View>
          )}
          <View className="border-t border-zinc-100 pt-3">
            <Text className="text-xs font-medium text-zinc-500 mb-1">物流信息（可选）</Text>
            <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm mb-2"
              value={form.tracking_company} onChange={e => setForm(p => ({ ...p, tracking_company: e.target.value }))} placeholder="物流公司" />
            <input className="w-full border border-zinc-300 rounded-lg px-3 py-2 text-sm"
              value={form.tracking_number} onChange={e => setForm(p => ({ ...p, tracking_number: e.target.value }))} placeholder="物流单号" />
          </View>
          <Button onClick={handleSubmit} disabled={!isFormValid || submitting}
            className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center mt-2">
            {submitting ? '提交中...' : '提交报修单'}
          </Button>
        </View>
      </ScrollView>

      {/* Merchant picker modal */}
      {showMerchantPicker && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-end" onClick={() => setShowMerchantPicker(false)}>
          <View className="bg-white rounded-t-2xl w-full max-h-80 p-4" onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-black mb-3">选择商户</Text>
            {merchants.map(m => (
              <View key={m.id} className="py-3 border-b border-zinc-50 active:opacity-60"
                onClick={() => handleMerchantSelect(m)}>
                <Text className="text-sm text-black">{m.name}</Text>
              </View>
            ))}
          </View>
        </View>
      )}

      {/* Site picker modal */}
      {showSitePicker && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-end" onClick={() => setShowSitePicker(false)}>
          <View className="bg-white rounded-t-2xl w-full max-h-80 p-4" onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-black mb-3">选择网点</Text>
            {sites.map(s => (
              <View key={s.id} className="py-3 border-b border-zinc-50 active:opacity-60"
                onClick={() => { setForm(p => ({ ...p, site_id: s.id })); setShowSitePicker(false) }}>
                <Text className="text-sm text-black">{s.name}</Text>
              </View>
            ))}
          </View>
        </View>
      )}
    </View>
  )
}
