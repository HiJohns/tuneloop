import { useState } from 'react'
import { View, Text, Button } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'

export default function RepairRecordPanel({ instrumentId, records, onRecordAdded, baseUrl: customUrl }) {
  const [comment, setComment] = useState('')
  const [photos, setPhotos] = useState([])
  const [submitting, setSubmitting] = useState(false)
  const baseUrl = customUrl || env.apiBaseUrl

  const apiPath = `${baseUrl}/repair/${instrumentId}/records`

  const handleSubmitRecord = async () => {
    if (!comment && photos.length === 0) { alert('请输入评论或拍照'); return }
    setSubmitting(true)
    try {
      const resp = await apiFetch(apiPath, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ comment, photos }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        setComment('')
        setPhotos([])
        if (onRecordAdded) onRecordAdded()
      }
    } catch {}
    setSubmitting(false)
  }

  return (
    <View>
      <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
        <Text className="text-sm font-bold text-black mb-2">维修记录（{records.length}）</Text>
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

      <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
        <Text className="text-sm font-bold text-black mb-2">添加记录</Text>
        <textarea className="w-full border border-zinc-300 rounded-lg p-3 text-sm" rows={3}
          value={comment} onChange={e => setComment(e.target.value)} placeholder="输入评论..." />
        <View className="flex gap-2 mt-2">
          <Button onClick={() => setPhotos(p => [...p, `photo_${Date.now()}.jpg`])}
            className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">+ 拍照（{photos.length}）</Button>
          <Button onClick={handleSubmitRecord} disabled={submitting}
            className="flex-1 py-2 bg-black text-white rounded-lg text-xs font-bold">提交记录</Button>
        </View>
      </View>
    </View>
  )
}
