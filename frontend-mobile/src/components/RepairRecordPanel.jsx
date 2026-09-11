import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, Image, Textarea } from '@tarojs/components'
import { apiFetch, resolveErrorMessage, getToken } from '../services/api'
import { dialog, env, getInputValue, uploadFile, previewImage } from '../platform'
import { formatBeijingDateTimeShort } from '../utils/format'
import { parsePhotos, photoSrc } from '../utils/media'

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

export default function RepairRecordPanel({ instrumentId, records, onRecordAdded, baseUrl: customUrl, hideForm }) {
  const [comment, setComment] = useState('')
  const [photoFiles, setPhotoFiles] = useState([])
  const [videoFile, setVideoFile] = useState(null)
  const [submitting, setSubmitting] = useState(false)
  const baseUrl = customUrl || env.apiBaseUrl

  const apiPath = `${baseUrl}/repair-requests/${instrumentId}/records`

  // #1878: cross-platform capture (H5 input capture / weapp Taro.chooseImage)
  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setPhotoFiles(prev => [...prev, ...files].slice(0, 9))
  }

  const handlePhotoCaptureWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: 9 - photoFiles.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setPhotoFiles(prev => [...prev, ...(res.tempFilePaths || [])].slice(0, 9))
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const handleVideoCapture = (e) => {
    const f = e.target.files?.[0]
    if (f) setVideoFile(f)
  }

  const handleVideoCaptureWeapp = async () => {
    try {
      const res = await Taro.chooseMedia({ count: 1, mediaType: ['video'], sourceType: ['camera', 'album'] })
      const f = res.tempFiles?.[0]?.tempFilePath
      if (f) setVideoFile(f)
    } catch (err) {
      console.error('Failed to choose video:', err)
    }
  }

  const handleSubmitRecord = async () => {
    if (!comment && photoFiles.length === 0 && !videoFile) { dialog.alert('请输入评论、拍照或选择视频'); return }
    setSubmitting(true)
    try {
      const token = getToken()
      const uploadOne = async (file) => {
        const resp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const parsed = env.isMiniProgram ? JSON.parse(resp.data || '{}') : await resp.json()
        if (parsed.code === 20000 && parsed.data?.file_key) return parsed.data.file_key
        throw new Error(resolveErrorMessage(parsed, 'upload failed'))
      }
      const photoKeys = []
      for (const f of photoFiles) {
        photoKeys.push(await uploadOne(f))
      }
      let videoKey = ''
      if (videoFile) {
        videoKey = await uploadOne(videoFile)
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
        dialog.alert(resolveErrorMessage(r, '提交失败'))
      }
    } catch (err) { dialog.alert('提交失败: ' + (err?.message || '')) }
    setSubmitting(false)
  }

  const renderPhotos = (photosStr) => {
    const parsed = parsePhotos(photosStr)
    if (!parsed.length) return null
    return (
      <View className="flex flex-wrap gap-1 mt-1">
        {parsed.map((p, i) => (
          <Image key={i} src={photoSrc(p)} className="w-12 h-12 rounded object-cover" mode="aspectFill"
            onClick={() => previewImage({ urls: parsed.map(photoSrc), current: photoSrc(p) })} />
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
          <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {records.map(r => (
              <View key={r.id} className="border-b border-zinc-100 pb-3 mb-1">
                <View className="flex justify-between items-center">
                  <Text className="text-sm font-bold text-black">{RECORD_TYPE_LABELS[r.record_type] || r.comment || r.record_type}</Text>
                  <Text className="text-xs text-zinc-400">{formatBeijingDateTimeShort(r.created_at)}</Text>
                </View>
                <View className="mt-0.5"><Text className="text-xs text-zinc-400">{r.worker_name || '系统'}</Text></View>
                {r.comment && r.record_type !== 'progress' && (
                  <View className="mt-1"><Text className="text-xs text-zinc-600">{r.comment}</Text></View>
                )}
                {r.record_type === 'progress' && r.comment && (
                  <View className="mt-1"><Text className="text-sm text-black">{r.comment}</Text></View>
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
        <Textarea className="w-full border border-zinc-300 rounded-lg p-3 text-sm"
          value={comment} onInput={e => setComment(getInputValue(e))} placeholder="输入评论..." />
        <View className="flex flex-wrap gap-2 mt-2">
          {env.isMiniProgram ? (
            <Button onClick={handlePhotoCaptureWeapp}
              className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">+ 照片（{photoFiles.length}）</Button>
          ) : (
            <label className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600 text-center cursor-pointer">
              <Text className="text-xs font-bold text-zinc-600">+ 照片（{photoFiles.length}）</Text>
              <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
            </label>
          )}
          {env.isMiniProgram ? (
            <Button onClick={handleVideoCaptureWeapp}
              className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">{videoFile ? '✓ 已选视频' : '+ 视频'}</Button>
          ) : (
            <label className="flex-1 py-2 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600 text-center cursor-pointer">
              <Text className="text-xs font-bold text-zinc-600">{videoFile ? '✓ 已选视频' : '+ 视频'}</Text>
              <input type="file" accept="video/*" className="hidden" onChange={handleVideoCapture} />
            </label>
          )}
          <Button onClick={handleSubmitRecord} disabled={submitting}
            className="flex-1 py-2 bg-black text-white rounded-lg text-xs font-bold">{submitting ? '处理中...' : '提交记录'}</Button>
        </View>
        {photoFiles.length > 0 && (
          <View className="flex flex-wrap gap-1 mt-2">
            {photoFiles.map((f, i) => {
              const src = env.isMiniProgram ? f : URL.createObjectURL(f)
              return (
                <View key={i} className="relative w-16 h-16">
                  <Image src={src} className="w-16 h-16 rounded object-cover" mode="aspectFill"
                    onClick={() => previewImage({ urls: [src], current: src })} />
                  <Button onClick={() => setPhotoFiles(prev => prev.filter((_, j) => j !== i))}
                    className="absolute top-0 right-0 rounded-full w-5 h-5 flex items-center justify-center"
                    style={{ padding: 0, margin: 0, minWidth: 0, backgroundColor: 'rgba(0,0,0,0.6)' }}>
                    <Text className="text-white text-xs">✕</Text>
                  </Button>
                </View>
              )
            })}
          </View>
        )}
      </View>
      )}
    </View>
  )
}
