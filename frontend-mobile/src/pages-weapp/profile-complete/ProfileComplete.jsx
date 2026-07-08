import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button } from '@tarojs/components'
import { storage, env, request } from '../../platform'

export default function ProfileComplete() {
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [saving, setSaving] = useState(false)

  const handleSave = async () => {
    setSaving(true)
    try {
      const token = storage.getItem('token')
      await request(`${env.apiBaseUrl}/users/me`, {
        method: 'PUT',
        headers: { 'Authorization': `Bearer ${token}`, 'Content-Type': 'application/json' },
        body: JSON.stringify({ name, email }),
      })
      Taro.navigateBack()
    } catch (err) {
      console.error('[ProfileComplete] save failed:', err)
    }
    setSaving(false)
  }

  return (
    <View style={{ height: '100vh', backgroundColor: '#fafafa', display: 'flex', flexDirection: 'column', alignItems: 'center', padding: 32 }}>
      <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', marginBottom: 8, marginTop: 32 }}>完善个人资料</Text>
      <Text style={{ fontSize: 14, color: '#a1a1aa', marginBottom: 32 }}>租赁需要真实姓名</Text>

      <Input placeholder="姓名" value={name} onInput={e => setName(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="邮箱" value={email} onInput={e => setEmail(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 24 }} />

      <Button onClick={handleSave} disabled={saving}
        style={{ width: '100%', height: 44, backgroundColor: '#915F38', color: '#fff', borderRadius: 22, fontSize: 14, fontWeight: '700', display: 'flex', alignItems: 'center', justifyContent: 'center', lineHeight: '44px', marginBottom: 12 }}>
        {saving ? '保存中...' : '保存'}
      </Button>
      <Text style={{ fontSize: 14, color: '#a1a1aa' }} onClick={() => Taro.navigateBack()}>跳过</Text>
    </View>
  )
}
