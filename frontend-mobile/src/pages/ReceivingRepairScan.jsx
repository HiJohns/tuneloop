import { useState, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Button, Image } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function ReceivingRepairScan() {
  const navigate = useNavigate()
  const [sn, setSn] = useState('')
  const [request, setRequest] = useState(null)
  const [searching, setSearching] = useState(false)
  const [actionLoading, setActionLoading] = useState(false)
  const [mode, setMode] = useState(null) // null | 'receive' | 'transit_in'
  const [unpackPhotos, setUnpackPhotos] = useState([])
  const baseUrl = env.apiBaseUrl
  const photoInputRef = useRef(null)

  const handleSearch = async () => {
    if (!sn.trim()) return
    setSearching(true)
    setRequest(null)
    setMode(null)
    try {
      // Search repair requests in shipping / transit_in status
      const resp = await apiFetch(`${baseUrl}/repair-requests?status=shipping,transit_in`)
      const data = await resp.json()
      if (data.code === 20000) {
        const matches = (data.data?.list || []).filter(r => r.instrument_sn === sn.trim())
        if (matches.length > 0) {
          setRequest(matches[0])
          setSearching(false)
          return
        }
      }
      alert('未找到匹配的待收货报修单')
    } catch {}
    setSearching(false)
  }

  const handleReceive = async () => {
    if (!request) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${request.id}/receive`, { method: 'POST' })
      const r = await resp.json()
      if (r.code === 20000) {
        alert('收货成功，报修单进入维修状态')
        navigate(`/repair-request?request_id=${request.id}`)
      } else {
        alert(r.message || '操作失败')
      }
    } catch { alert('操作失败') }
    setActionLoading(false)
  }

  const handleTransitRelay = async () => {
    if (!request) return
    if (mode === 'transit_in' && unpackPhotos.length === 0) {
      alert('请至少拍摄一张拆箱照片')
      return
    }
    setActionLoading(true)
    try {
      const body = {
        direction: mode === 'transit_in' ? 'in' : 'out',
        unpack_photos: unpackPhotos,
      }
      const resp = await apiFetch(`${baseUrl}/repair-requests/${request.id}/transit-relay`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        alert('中转处理成功')
        navigate(`/repair-request?request_id=${request.id}`)
      } else {
        alert(r.message || '操作失败')
      }
    } catch { alert('操作失败') }
    setActionLoading(false)
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

  const isControlled = request?.merchant_type === 'controlled'
  const status = request?.status

  return (
    <View className="flex flex-col h-screen bg-zinc-50 p-4">
      <View className="flex items-center mb-4">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1">收货识别</Text>
      </View>

      <View className="bg-white rounded-2xl shadow-sm p-4 mb-4">
        <Text className="text-sm font-bold text-black mb-2">输入/扫描乐器识别码</Text>
        <View className="flex gap-2">
          <input className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
            value={sn} onChange={e => setSn(e.target.value)} placeholder="扫描或输入识别码" />
          <Button onClick={handleSearch} disabled={searching}
            className="px-4 py-2 bg-black text-white rounded-lg text-sm font-bold">
            查询
          </Button>
        </View>
      </View>

      {request && (
        <View className="bg-white rounded-2xl shadow-sm p-4">
          <Text className="text-sm font-bold text-green-700 mb-2">匹配到报修单</Text>
          <View className="space-y-1 mb-3">
            <Text className="text-xs text-zinc-500">乐器：{request.instrument_type} {request.brand}</Text>
            <Text className="text-xs text-zinc-500">描述：{request.description}</Text>
            <Text className="text-xs text-zinc-500">当前状态：{status} {isControlled ? '(受控)' : '(全权)'}</Text>
          </View>

          {!mode && (
            <View className="space-y-2">
              {status === 'shipping' && (
                <>
                  <Button onClick={() => setMode('receive')}
                    className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
                    收货（全权网点）
                  </Button>
                  {isControlled && (
                    <Button onClick={() => setMode('transit_in')}
                      className="w-full py-3 bg-blue-600 text-white rounded-xl font-bold text-sm text-center">
                      中转处理（中转网点）
                    </Button>
                  )}
                </>
              )}
              {status === 'transit_in' && (
                <Button onClick={() => setMode('receive')}
                  className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
                  收货（受控网点）
                </Button>
              )}
            </View>
          )}

          {mode === 'receive' && (
            <Button onClick={handleReceive} disabled={actionLoading}
              className="w-full py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
              确认收货
            </Button>
          )}

          {mode === 'transit_in' && (
            <View>
              <Text className="text-xs text-zinc-500 mb-2">请拍摄拆箱照片：</Text>
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
              <Button onClick={handleTransitRelay} disabled={actionLoading || unpackPhotos.length === 0}
                className="w-full py-3 bg-blue-600 text-white rounded-xl font-bold text-sm text-center">
                提交中转处理
              </Button>
            </View>
          )}
        </View>
      )}
    </View>
  )
}
