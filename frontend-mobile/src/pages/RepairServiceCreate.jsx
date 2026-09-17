import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Textarea, Image, Button } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute, uploadFile as uploadFileApi } from '../platform'

// #1955 阶段3a RS-01：创建维修服务单（描述 + 照片 ≤6，不填识别码）
// 提交后展示 6 位编码（repair_code），提示用户写在物流单信息栏。

const MAX_PHOTOS = 6

export default function RepairServiceCreate() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [description, setDescription] = useState('')
  const [photos, setPhotos] = useState([])
  const [submitting, setSubmitting] = useState(false)
  const [created, setCreated] = useState(null) // {id, repair_code}
  const baseUrl = env.apiBaseUrl

  const goBack = () => nav(-1)

  const addPhotosH5 = (e) => {
    const files = Array.from(e.target.files || [])
    setPhotos(p => [...p, ...files].slice(0, MAX_PHOTOS))
    e.target.value = ''
  }

  const addPhotosWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: MAX_PHOTOS - photos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      setPhotos(p => [...p, ...(res.tempFilePaths || [])].slice(0, MAX_PHOTOS))
    } catch (err) {
      console.error('choose image failed:', err)
    }
  }

  const uploadOne = async (file) => {
    // #1924：/api/upload 在严格鉴权组，必须带 token
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

  const submit = async () => {
    if (!description.trim()) { dialog.alert('请填写问题描述'); return }
    if (photos.length === 0) { dialog.alert('请至少上传一张照片'); return }
    setSubmitting(true)
    try {
      const keys = []
      for (const f of photos) keys.push(await uploadOne(f))
      const resp = await apiFetch(`${baseUrl}/user/repair-services`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ description: description.trim(), photos: keys }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        setCreated(result.data || {})
        setPhotos([])
        setDescription('')
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  const thumbSrc = (f) => (env.isMiniProgram ? f : URL.createObjectURL(f))

  if (created) {
    return (
      <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={goBack} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>创建成功</Text>
        </View>
        <View style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 14 }}>
          <View style={{ backgroundColor: '#FFFFFF', borderRadius: 12, padding: 20, display: 'flex', flexDirection: 'column', gap: 10, alignItems: 'center' }}>
            <Text style={{ fontSize: 13, color: '#71717A' }}>您的维修编码</Text>
            <Text style={{ fontSize: 40, fontWeight: 'bold', color: '#18181B', letterSpacing: 6 }}>{created.repair_code}</Text>
            <Text style={{ fontSize: 12, color: '#D97706', textAlign: 'center' }}>
              请将该编码写在物流单信息栏，作为唯一标记
            </Text>
          </View>
          <Button onClick={() => nav(`/repair-service-detail?order_id=${created.id}`)}
            style={{ width: '100%', margin: 0, height: 46, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 15, fontWeight: 'bold' }}>
            下一步：选择维修师
          </Button>
          <Button onClick={() => nav('/my-repairs')}
            style={{ width: '100%', margin: 0, height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#F4F4F5', color: '#3F3F46', borderRadius: 10, fontSize: 13, fontWeight: 'bold' }}>
            返回维修列表
          </Button>
        </View>
      </View>
    )
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={goBack} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>创建维修服务</Text>
      </View>

      <View style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 12 }}>
        <View style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>问题描述 *</Text>
          <Textarea style={{ width: '100%', boxSizing: 'border-box', minHeight: 100, backgroundColor: '#FFFFFF', border: '1px solid #E4E4E7', borderRadius: 10, padding: 10, fontSize: 13 }}
            value={description} maxlength={500} placeholder="描述乐器问题（无需填写识别码）"
            onInput={e => setDescription(getInputValue(e))} />
        </View>

        <View style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>照片（{photos.length}/{MAX_PHOTOS}）*</Text>
          <View style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
            {photos.map((f, i) => (
              <View key={i} style={{ width: 84, height: 84, borderRadius: 8, overflow: 'hidden', position: 'relative' }}>
                <Image src={thumbSrc(f)} mode="aspectFill" style={{ width: '100%', height: '100%' }} />
                <View onClick={() => setPhotos(p => p.filter((_, j) => j !== i))}
                  style={{ position: 'absolute', top: 2, right: 2, width: 18, height: 18, borderRadius: 9, backgroundColor: 'rgba(0,0,0,0.55)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                  <Text style={{ fontSize: 10, color: '#FFFFFF' }}>✕</Text>
                </View>
              </View>
            ))}
            {photos.length < MAX_PHOTOS && (env.isMiniProgram ? (
              <View onClick={addPhotosWeapp}
                style={{ width: 84, height: 84, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Text style={{ fontSize: 24, color: '#A1A1AA' }}>＋</Text>
              </View>
            ) : (
              <View style={{ width: 84, height: 84, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Text style={{ fontSize: 24, color: '#A1A1AA' }}>＋</Text>
                <input type="file" accept="image/*" capture="environment" multiple className="hidden"
                  style={{ position: 'absolute', inset: 0, opacity: 0 }} onChange={addPhotosH5} />
              </View>
            ))}
          </View>
        </View>

        <Button disabled={submitting} onClick={submit}
          style={{ width: '100%', margin: 0, height: 46, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: submitting ? '#A1A1AA' : '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 15, fontWeight: 'bold' }}>
          {submitting ? '处理中...' : '提交维修服务单'}
        </Button>
      </View>
    </View>
  )
}
