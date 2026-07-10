import { useState, useRef } from 'react'
import { View, Text, Button, Image } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'

const RECORD_TYPE_LABELS = {
  created: '报修单已创建',
  quote_submitted: '师傅提交报价',
  quote_accepted: '接受报价',
  paid: '支付完成',
  shipped: '已发货',
  received: '已收货',
  requoted: '师傅重新报价',
  requote_rejected: '拒绝重新报价',
  progress: '维修进展',
  completed: '维修完成',
  return_shipped: '已发还',
  receipt_confirmed: '确认收货',
  transit_processed: '中转处理',
  transit_relayed: '中转转发',
}

const uploadFile = async (file, baseUrl) => {
  const fd = new FormData()
  fd.append('file', file)
  const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', body: fd })
  const r = await resp.json()
  if (r.code === 20000) return r.data.file_key
  throw new Error(r.message || 'upload failed')
}

export default function RepairRecordPanel({ instrumentId, records, onRecordAdded, baseUrl: customUrl, hideForm }) {
  const [comment, setComment] = useState('')
  const [photoFiles, setPhotoFiles] = useState([])
  const [videoFile, setVideoFile] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const baseUrl = customUrl || env.apiBaseUrl
  const photoInputRef = useRef(null)
  const videoInputRef = useRef(null)

  const apiPath = `${baseUrl}/repair-requests/${instrumentId}/records`

  const handleSubmitRecord = async () => {
    if (!comment && photoFiles.length === 0 && !videoFile) { alert('请输入评论、拍照或选择视频'); return }
    setSubmitting(true)
    try {
      const photoKeys = []
      for (const f of photoFiles) {
        const key = await uploadFile(f, baseUrl)
        photoKeys.push(key)
      }
      let videoKey = ''
      if (videoFile) {
        videoKey = await uploadFile(videoFile, baseUrl)
      }
      const resp = await apiFetch(apiPath, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ comment, photos: photoKeys, video_url: videoKey || undefined }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        setComment('')
        setPhotoFiles([])
        setVideoFile(null)
        if (onRecordAdded) onRecordAdded()
      } else {
        alert(r.message || '提交失败')
      }
    } catch { alert('提交失败') }
    setSubmitting(false)
  }

  const renderPhotos = (photosStr) => {
    if (!photosStr || photosStr === '[]') return null
    let parsed
    try { parsed = JSON.parse(photosStr) } catch { return null }
    if (!parsed.length) return null
    return (
      <View className="flex flex-wrap gap-1 mt-1">
        {parsed.map((p, i) => (
          <Image key={i} src={`/uploads/media/${p}`} className="w-12 h-12 rounded object-cover" mode="aspectFill" />
        ))}
      </View>
    )
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
              <View key={r.id} className="border-b border-zinc-100 pb-3 mb-1">
                <View className="flex justify-between items-center">
                  <Text className="text-sm font-bold text-black">{RECORD_TYPE_LABELS[r.record_type] || r.comment || r.record_type}</Text>
                  <Text className="text-xs text-zinc-400">{formatDisplayDate(r.created_at)}</Text>
                </View>
                <Text className="text-xs text-zinc-400 mt-0.5">{r.worker_name || '系统'}</Text>
                {r.comment && r.record_type !== 'progress' && (
                  <Text className="text-xs text-zinc-600 mt-1">{r.comment}</Text>
                )}
                {r.record_type === 'progress' && r.comment && (
                  <Text className="text-sm text-black mt-1">{r.comment}</Text>
                )}
                {renderPhotos(r.photos)}
              </View>
            ))}
          </View>
        )}
      </View>

      {!hideForm && (
      <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
        <Text className="text-sm font-bold text-black mb-2">添加记录</Text>
        <textarea className="w-full border border-zinc-300 rounded-lg p-3 text-sm" rows={3}
          value={comment} onChange={e => setComment(e.target.value)} placeholder="输入评论..." />
        <View className="flex flex-wrap gap-2 mt-2">
          <Button onClick={() => photoInputRef.current?.click()}
            className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">+ 照片（{photoFiles.length}）</Button>
          <Button onClick={() => videoInputRef.current?.click()}
            className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">{videoFile ? '✓ 已选视频' : '+ 视频'}</Button>
          <Button onClick={handleSubmitRecord} disabled={submitting}
            className="flex-1 py-2 bg-black text-white rounded-lg text-xs font-bold">提交记录</Button>
        </View>
        <input type="file" accept="image/*" multiple className="hidden" ref={photoInputRef}
          onChange={e => { setPhotoFiles([...photoFiles, ...Array.from(e.target.files || [])]) }} />
        <input type="file" accept="video/*" className="hidden" ref={videoInputRef}
          onChange={e => { const f = e.target.files?.[0]; if (f) setVideoFile(f) }} />
      </View>
      )}
    </View>
  )
}
