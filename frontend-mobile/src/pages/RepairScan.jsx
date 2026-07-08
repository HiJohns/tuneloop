import { useState, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button, Image, Input } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function RepairScan() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const baseUrl = env.apiBaseUrl

  // Transit-out relay mode
  const [orderNumber, setOrderNumber] = useState('')
  const [relayRequest, setRelayRequest] = useState(null)
  const [unpackPhotos, setUnpackPhotos] = useState([])
  const [searching, setSearching] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const photoInputRef = useRef(null)

  const handleSearchOrder = async () => {
    if (!orderNumber.trim()) return
    setSearching(true)
    setRelayRequest(null)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests?status=transit_out`)
      const data = await resp.json()
      if (data.code === 20000) {
        const matches = (data.data?.list || []).filter(r =>
          r.transit_order_number === orderNumber.trim()
        )
        if (matches.length > 0) {
          setRelayRequest(matches[0])
        } else {
          alert('未找到匹配的转出单')
        }
      }
    } catch {}
    setSearching(false)
  }

  const handlePhotoUpload = async (e) => {
    const file = e.target?.files?.[0] || e.detail?.value?.[0]
    if (!file) return
    const fd = new FormData()
    fd.append('file', file)
    try {
      const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', body: fd })
      const r = await resp.json()
      if (r.code === 20000) {
        setUnpackPhotos(p => [...p, r.data.file_key])
      }
    } catch {}
  }

  const handleSubmitRelay = async () => {
    if (!relayRequest) return
    if (unpackPhotos.length === 0) { alert('请至少拍摄一张拆箱照片'); return }
    setSubmitting(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${relayRequest.id}/transit-relay`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          direction: 'out',
          transit_order_number: orderNumber.trim(),
          unpack_photos: unpackPhotos,
        }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        alert('转出中转处理成功')
        navigate(`/repair-request?request_id=${relayRequest.id}`)
      } else {
        alert(r.message || '操作失败')
      }
    } catch { alert('操作失败') }
    setSubmitting(false)
  }

  return (
    <View className="flex flex-col h-screen bg-zinc-50 p-4">
      <View className="flex items-center mb-4">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1">转出中转处理</Text>
      </View>

      <View className="bg-white rounded-2xl shadow-sm p-4 mb-4">
        <Text className="text-sm font-bold text-black mb-2">输入转出单号</Text>
        <View className="flex gap-2">
          <input className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
            value={orderNumber} onChange={e => setOrderNumber(e.target.value)} placeholder="扫描或输入转出单号" />
          <Button onClick={handleSearchOrder} disabled={searching}
            className="px-4 py-2 bg-black text-white rounded-lg text-sm font-bold">
            查询
          </Button>
        </View>
      </View>

      {relayRequest && (
        <View className="bg-white rounded-2xl shadow-sm p-4">
          <Text className="text-sm font-bold text-green-700 mb-2">已匹配报修单</Text>
          <View className="space-y-1 mb-3">
            <Text className="text-xs text-zinc-500">乐器：{relayRequest.instrument_type} {relayRequest.brand}</Text>
            <Text className="text-xs text-zinc-500">描述：{relayRequest.description}</Text>
            <Text className="text-xs text-zinc-500">目标地址：{relayRequest.site_name || '-'}</Text>
          </View>

          <Text className="text-xs text-zinc-500 mb-2">拆箱拍照（转出）：</Text>
          <View className="flex flex-wrap gap-2 mb-3">
            {unpackPhotos.map((p, i) => (
              <Image key={i} src={`/uploads/media/${p}`} className="w-16 h-16 rounded object-cover" mode="aspectFill" />
            ))}
            <View className="w-16 h-16 border-2 border-dashed border-zinc-300 rounded flex items-center justify-center"
              onClick={() => photoInputRef.current?.click()}>
              <Text className="text-2xl text-zinc-300">+</Text>
            </View>
          </View>
          <input ref={photoInputRef} type="file" accept="image/*" className="hidden" onChange={handlePhotoUpload} />

          <Button onClick={handleSubmitRelay} disabled={submitting || unpackPhotos.length === 0}
            className="w-full py-3 bg-blue-600 text-white rounded-xl font-bold text-sm text-center">
            提交转出处理
          </Button>
        </View>
      )}
    </View>
  )
}
