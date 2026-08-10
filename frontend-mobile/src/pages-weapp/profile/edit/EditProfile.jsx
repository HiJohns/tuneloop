import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../../../services/api'
import { env, dialog, getInputValue } from '../../../platform'
import IdPhotoUploader from '../../../components/IdPhotoUploader'

export default function EditProfile() {
  const [name, setName] = useState('')
  const [nickname, setNickname] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [idPhotoFront, setIdPhotoFront] = useState('')
  const [idPhotoBack, setIdPhotoBack] = useState('')
  const [saving, setSaving] = useState(false)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          setName(result.data.name || '')
          // WeChat nickname is only the default value; the user may edit it
          // freely (#1588). phone/email prefill from current profile.
          setNickname(result.data.nickname || result.data.wx_nickname || result.data.name || '')
          setPhone(result.data.phone || '')
          setEmail(result.data.email || '')
          setIdPhotoFront(result.data.id_photo_front || '')
          setIdPhotoBack(result.data.id_photo_back || '')
        }
      } catch {}
    }
    fetchUser()
  }, [])

  const handleSave = async () => {
    setSaving(true)
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, nickname, phone, email }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        // H5 (Vite) has no Taro runtime — use platform dialog/navigation
        if (env.isMiniProgram) {
          Taro.showToast({ title: '保存成功', icon: 'success' })
          setTimeout(() => Taro.navigateBack(), 800)
        } else {
          dialog.toast('保存成功')
          setTimeout(() => window.history.back(), 800)
        }
      } else {
        dialog.toast(result.message || '保存失败')
      }
    } catch {
      dialog.toast('网络错误')
    }
    setSaving(false)
  }

  return (
    <View style={{ height: '100vh', backgroundColor: '#f4f4f5', display: 'flex', flexDirection: 'column' }}>
      <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 12, padding: 16 }}>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>姓名</Text>
          <Input value={name} onInput={e => setName(getInputValue(e))}
            placeholder={name ? '' : '请输入真实姓名'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14 }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>昵称</Text>
          <Input value={nickname} onInput={e => setNickname(getInputValue(e))}
            placeholder={nickname ? '' : '请输入昵称'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14 }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>手机号</Text>
          <Input value={phone} onInput={e => setPhone(getInputValue(e))}
            placeholder={phone ? '' : '请输入手机号'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14 }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 6 }}>邮箱</Text>
          <Input value={email} onInput={e => setEmail(getInputValue(e))}
            placeholder={email ? '' : '请输入邮箱'}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14 }} />
        </View>
        <View style={{ marginBottom: 20 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 10 }}>身份证照片</Text>
          <View style={{ display: 'flex', flexDirection: 'row' }}>
            <View style={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="front" initialUrl={idPhotoFront} onChange={setIdPhotoFront} />
            </View>
            <View style={{ flex: 1, display: 'flex', justifyContent: 'center' }}>
              <IdPhotoUploader side="back" initialUrl={idPhotoBack} onChange={setIdPhotoBack} />
            </View>
          </View>
        </View>
        <Button onClick={handleSave}
          style={{ width: '100%', height: 44, backgroundColor: '#915F38', color: '#fff', borderRadius: 22, fontSize: 16, fontWeight: '700', lineHeight: '44px', border: 'none', marginTop: 8 }}>
          {saving ? '保存中...' : '保存'}
        </Button>
      </View>
    </View>
  )
}
