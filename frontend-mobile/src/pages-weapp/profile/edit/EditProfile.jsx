import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../../../services/api'
import { env } from '../../../platform'

export default function EditProfile() {
  const [nickname, setNickname] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [saving, setSaving] = useState(false)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          setNickname(result.data.nickname || '')
          setPhone(result.data.phone || '')
          setEmail(result.data.email || '')
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
        body: JSON.stringify({ nickname, phone, email }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '保存成功', icon: 'success' })
        Taro.navigateBack()
      } else {
        Taro.showToast({ title: result.message || '保存失败', icon: 'none' })
      }
    } catch {
      Taro.showToast({ title: '网络错误', icon: 'none' })
    }
    setSaving(false)
  }

  return (
    <View style={{ height: '100vh', backgroundColor: '#f4f4f5', display: 'flex', flexDirection: 'column' }}>
      <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 12, padding: 16 }}>
        <View style={{ marginBottom: 16 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 4 }}>微信昵称</Text>
          <Input type="nickname" value={nickname} onInput={e => setNickname(e.detail.value)}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, padding: '0 12px', fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <View style={{ marginBottom: 16 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 4 }}>手机号</Text>
          <Input value={phone} onInput={e => setPhone(e.detail.value)}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, padding: '0 12px', fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <View style={{ marginBottom: 16 }}>
          <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 4 }}>邮箱</Text>
          <Input value={email} onInput={e => setEmail(e.detail.value)}
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, padding: '0 12px', fontSize: 14, boxSizing: 'border-box' }} />
        </View>
        <Button onClick={handleSave}
          style={{ width: '100%', height: 44, backgroundColor: '#915F38', color: '#fff', borderRadius: 22, fontSize: 16, fontWeight: '700', lineHeight: '44px', border: 'none', marginTop: 8 }}>
          {saving ? '保存中...' : '保存'}
        </Button>
      </View>
    </View>
  )
}
