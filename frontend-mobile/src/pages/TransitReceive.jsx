// 中转收货页（#1931/#1934 Sub4）—— 会话详情+拍照留痕+提交
// PUT /forwarding/sessions/:id/receive { photo_keys }
import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Image, Text, View, Button } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, uploadFile as uploadFileApi } from '../platform'

const MAX_PHOTOS = 6

export default function TransitReceive() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const sessionId = params.get('session') || ''
  const baseUrl = env.apiBaseUrl

  const [photos, setPhotos] = useState([])
  const [submitting, setSubmitting] = useState(false)

  const addPhotoWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: MAX_PHOTOS - photos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setPhotos(p => [...p, ...(res.tempFilePaths || [])].slice(0, MAX_PHOTOS))
    } catch (e) { console.error('choose image failed', e) }
  }

  const uploadFile = async (file) => {
    const authHeaders = { Authorization: 'Bearer ' + (getToken() || '') }
    if (env.isMiniProgram) {
      const resp = await uploadFileApi(`${baseUrl}/upload`, file, { headers: authHeaders })
      const r = JSON.parse(resp.data)
      if (r.code === 20000) return r.data.file_key
      throw new Error(r.message || 'upload failed')
    }
    const fd = new FormData()
    fd.append('file', file)
    const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', headers: authHeaders, body: fd })
    const r = await resp.json()
    if (r.code === 20000) return r.data.file_key
    throw new Error(resolveErrorMessage(r, 'upload failed'))
  }

  const handleSubmit = async () => {
    if (photos.length === 0) { dialog.alert('请先拍照留痕'); return }
    setSubmitting(true)
    try {
      const keys = []
      for (const f of photos) keys.push(await uploadFile(f))
      const resp = await apiFetch(`${baseUrl}/forwarding/sessions/${sessionId}/receive`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ photo_keys: keys }),
      })
      const r = await resp.json()
      if (r.code === 20000) {
        dialog.alert('收货已确认，等待转发')
        navigate('/transit-workbench')
      } else {
        dialog.alert(resolveErrorMessage(r, '提交失败'))
      }
    } catch (e) {
      dialog.alert('提交失败: ' + (e.message || ''))
    }
    setSubmitting(false)
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      {!env.isMiniProgram && (
      <View style={{ padding: '16px 16px 8px', backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
        <Text style={{ fontSize: 20, fontWeight: 900, color: '#000' }}>中转收货</Text>
      </View>
      )}
      <View style={{ padding: 16, display: 'flex', flexDirection: 'column' }}>
        <Text style={{ fontSize: 13, color: '#6b7280', marginBottom: 12 }}>开具物流时拍照存档，收货后订单进入「等待转发」状态。</Text>

        <View style={{ display: 'flex', flexWrap: 'wrap', gap: 8, marginBottom: 16 }}>
          {photos.map((p, i) => (
            <Image key={i} src={p} style={{ width: 96, height: 96, borderRadius: 8, objectFit: 'cover' }} />
          ))}
          {photos.length < MAX_PHOTOS && (
            env.isMiniProgram ? (
              <View onClick={addPhotoWeapp} style={{ width: 96, height: 96, borderRadius: 8, border: '1px dashed #d4d4d8', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fff' }}>
                <Text style={{ fontSize: 20, color: '#d4d4d8' }}>+</Text>
              </View>
            ) : (
              <label style={{ width: 96, height: 96, borderRadius: 8, border: '1px dashed #d4d4d8', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fff', cursor: 'pointer' }}>
                <Text style={{ fontSize: 20, color: '#d4d4d8' }}>+</Text>
                <input type="file" accept="image/*" capture="camera" style={{ display: 'none' }} onChange={e => { const f = e.target.files?.[0]; if (f) setPhotos(p => [...p, f].slice(0, MAX_PHOTOS)) }} />
              </label>
            )
          )}
        </View>

        <Button
          onClick={handleSubmit}
          disabled={submitting}
          style={{ width: '100%', margin: 0, height: 48, display: 'flex', alignItems: 'center', justifyContent: 'center', borderRadius: 999, backgroundColor: photos.length > 0 ? '#B98E5F' : '#d4d4d8', color: '#fff', fontWeight: 800, fontSize: 16, letterSpacing: '0.05em', border: 'none' }}
        >
          {submitting ? '处理中...' : `提交收货（${photos.length} 张）`}
        </Button>
      </View>
    </View>
  )
}
