import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, ScrollView, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'

const statusLabels = {
  repair_pending: '待维修', repair_in_progress: '维修中', repair_completed: '已修复',
}

export default function RepairWorkflow() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const instrumentId = searchParams.get('instrument_id')
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [records, setRecords] = useState([])
  const [comment, setComment] = useState('')
  const [photos, setPhotos] = useState([])
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)

  const token = getToken()
  const currentUserId = token ? JSON.parse(atob(token.split('.')[1]))?.sub || '' : ''

  const fetchData = async () => {
    if (!instrumentId) return
    setLoading(true)
    try {
      const [instRes, recRes] = await Promise.all([
        apiFetch(`${baseUrl}/instruments/${instrumentId}`),
        apiFetch(`${baseUrl}/repair/${instrumentId}/records`),
      ])
      const inst = await instRes.json()
      const rec = await recRes.json()
      if (inst.code === 20000) setInstrument(inst.data)
      if (rec.code === 20000) setRecords(rec.data?.records || [])
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchData() }, [instrumentId])

  const handleAction = async (action) => {
    if (!instrumentId) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/${action}`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        await fetchData()
      } else {
        alert(result.message || '操作失败')
      }
    } catch (err) {
      alert('操作失败: ' + (err.message || ''))
    }
    setActionLoading(false)
  }

  const handleTakeover = async () => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/takeover`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        await fetchData()
      } else {
        alert(result.message || '接手失败')
      }
    } catch (err) {
      alert('接手失败: ' + (err.message || ''))
    }
    setActionLoading(false)
  }

  if (!instrumentId) {
    return (
      <View className="h-screen bg-zinc-50 flex items-center justify-center p-4">
        <Text className="text-zinc-400">请扫描或选择乐器</Text>
      </View>
    )
  }

  if (loading) {
    return <View className="h-screen bg-zinc-50 flex items-center justify-center"><Text className="text-zinc-400">加载中...</Text></View>
  }

  if (!instrument) {
    return <View className="h-screen bg-zinc-50 flex items-center justify-center"><Text className="text-zinc-400">乐器不存在</Text></View>
  }

  const status = instrument.repair_status
  const workerId = instrument.repair_worker_id
  const isMyJob = workerId === currentUserId
  const isValid = ['repair_pending', 'repair_in_progress', 'repair_completed'].includes(status)

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100 flex items-center gap-2">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1">维修 - {instrument.sn || ''}</Text>
        <Text className={`text-xs px-2 py-1 rounded-full font-bold ${status === 'repair_completed' ? 'bg-green-100 text-green-700' : status === 'repair_in_progress' ? 'bg-yellow-100 text-yellow-700' : 'bg-blue-100 text-blue-700'}`}>
          {statusLabels[status] || status}
        </Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        {/* Instrument info */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black">乐器信息</Text>
          <View className="mt-2 space-y-1 text-sm">
            <Text className="text-zinc-500">编号: <Text className="text-black">{instrument.sn || '-'}</Text></Text>
            <Text className="text-zinc-500">类别: <Text className="text-black">{instrument.category_name || '-'}</Text></Text>
            {instrument.repair_worker_name && <Text className="text-zinc-500">负责人: <Text className="text-black">{instrument.repair_worker_name}</Text></Text>}
          </View>
        </View>

        {/* Repair records */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black mb-2">维修记录 ({records.length})</Text>
          {records.length === 0 ? (
            <Text className="text-xs text-zinc-400">暂无记录</Text>
          ) : (
            <View className="space-y-2">
              {records.map(r => (
                <View key={r.id} className="border-b border-zinc-50 pb-2">
                  <Text className="text-xs text-zinc-400">{formatDisplayDate(r.created_at)}</Text>
                  {r.comment && <Text className="text-sm text-black mt-1">{r.comment}</Text>}
                  {r.photos && r.photos !== '[]' && <Text className="text-xs text-blue-500 mt-1">[有照片]</Text>}
                </View>
              ))}
            </View>
          )}
        </View>

        {/* Status-specific actions */}
        {status === 'repair_pending' && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-600">此乐器等待维修</Text>
            <Button onClick={() => handleAction('start')} disabled={actionLoading}
              className="w-full mt-3 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '处理中...' : '开始维修'}
            </Button>
          </View>
        )}

        {status === 'repair_in_progress' && isMyJob && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm font-bold text-black mb-2">添加记录</Text>
            <textarea className="w-full border border-zinc-300 rounded-lg p-3 text-sm" rows={3}
              value={comment} onChange={e => setComment(e.target.value)} placeholder="输入评论..." />
            <View className="flex gap-2 mt-2">
              <Button onClick={() => { const p = [...photos, `photo_${Date.now()}.jpg`]; setPhotos(p) }}
                className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">+ 拍照</Button>
              <Button onClick={async () => {
                if (!comment && photos.length === 0) { alert('请输入评论或拍照'); return }
                try {
                  const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/records`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ comment, photos }),
                  })
                  const r = await resp.json()
                  if (r.code === 20000) { setComment(''); setPhotos([]); await fetchData() }
                } catch {}
              }} className="flex-1 py-2 bg-black text-white rounded-lg text-xs font-bold">提交记录</Button>
            </View>
            <Button onClick={() => handleAction('complete')} disabled={actionLoading}
              className="w-full mt-3 py-3 bg-green-600 text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '处理中...' : '维修完成'}
            </Button>
          </View>
        )}

        {status === 'repair_in_progress' && !isMyJob && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-600">此乐器由 {instrument.repair_worker_name || '其他师傅'} 负责处理中</Text>
            <Button onClick={handleTakeover} disabled={actionLoading}
              className="w-full mt-3 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
              {actionLoading ? '处理中...' : '接手'}
            </Button>
          </View>
        )}

        {status === 'repair_completed' && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-600">乐器已修复，等待验收</Text>
            <View className="flex gap-2 mt-3">
              <Button onClick={() => handleAction('accept')} disabled={actionLoading}
                className="flex-1 py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
                验收通过
              </Button>
              <Button onClick={async () => {
                const reason = prompt('请输入不通过原因')
                if (!reason) return
                try {
                  const resp = await apiFetch(`${baseUrl}/repair/${instrumentId}/reject`, {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({ comment: reason }),
                  })
                  const r = await resp.json()
                  if (r.code === 20000) { await fetchData() }
                  else { alert(r.message) }
                } catch {}
              }} className="flex-1 py-3 bg-red-500 text-white rounded-xl font-bold text-sm text-center">
                验收不通过
              </Button>
            </View>
          </View>
        )}

        {!isValid && (
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
            <Text className="text-sm text-zinc-400 text-center">乐器状态正常，不需要维修</Text>
          </View>
        )}
      </ScrollView>
    </View>
  )
}
