// 中转收货页（#1931/#1934 Sub4）—— 会话详情+拍照留痕+提交
// PUT /forwarding/sessions/:id/receive { photo_keys } → received
// PUT /forwarding/sessions/:id/ready {} → ready「等待转发」（audit #1937 Bug 3：
//   last-mile 强制要求 ready，收货成功后必须补调 ready，否则发货页不可达）
import { useState } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { Image, Text, View, Button } from '@tarojs/components'
import Taro from '@tarojs/taro'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, uploadFile as uploadFileApi, toWeappRoute } from '../platform'

const MAX_PHOTOS = 6

export default function TransitReceive() {
  const navigate = useNavigate()
  const [params] = useSearchParams()
  const sessionId = params.get('session') || ''
  const baseUrl = env.apiBaseUrl

  const [photos, setPhotos] = useState([])
  const [submitting, setSubmitting] = useState(false)

  // Cross-end navigation (issue-1673): weapp must use /pages-weapp/... urls
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }

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

  const markReady = async () => {
    const resp = await apiFetch(`${baseUrl}/forwarding/sessions/${sessionId}/ready`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({}),
    })
    return resp.json()
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
      if (r.code === 20000 || r.code === 40002) {
        // 40002 = 会话已收货（received）——补调 ready 继续（重试路径）
        if (r.code === 40002) {
          const readyResp = await markReady()
          if (readyResp.code !== 20000) {
            dialog.alert(resolveErrorMessage(readyResp, '确认转发失败'))
            setSubmitting(false)
            return
          }
          dialog.alert('收货已确认，等待转发')
          nav('/transit-workbench')
          setSubmitting(false)
          return
        }
        // 收货成功 → 显式 ready（等待转发），否则 last-mile 永远不可达
        const readyResp = await markReady()
        if (readyResp.code !== 20000) {
          dialog.alert('收货已确认，但确认转发失败: ' + resolveErrorMessage(readyResp, '') + '（可重试提交）')
          setSubmitting(false)
          return
        }
        dialog.alert('收货已确认，等待转发')
        nav('/transit-workbench')
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
