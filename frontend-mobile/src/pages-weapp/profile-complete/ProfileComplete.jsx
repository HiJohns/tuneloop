import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input } from '@tarojs/components'
import { storage, env, request, eventBus } from '../../platform'

export default function ProfileComplete() {
  const [name, setName] = useState('')
  const [phone, setPhone] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    const params = Taro.getCurrentInstance().router?.params || {}
    if (params.phone) setPhone(params.phone)
  }, [])

  const handleRegister = async () => {
    if (!name.trim()) { Taro.showToast({ title: '请输入姓名', icon: 'none' }); return }
    if (!phone.trim()) { Taro.showToast({ title: '请输入手机号', icon: 'none' }); return }
    if (!password.trim() || password.length < 6) { Taro.showToast({ title: '密码至少6位', icon: 'none' }); return }
    setSaving(true)
    try {
      const res = await request(`${env.apiBaseUrl}/auth/register`, {
        method: 'POST',
        body: JSON.stringify({ name: name.trim(), phone: phone.trim(), email: email.trim(), password: password.trim() }),
      })
      const result = await res.json()
      if (result.code === 20000 && result.data?.access_token) {
        storage.setItem('token', result.data.access_token)
        storage.setItem('token_expiry', (Date.now() + (result.data.expires_in || 3600) * 1000).toString())
        eventBus.emit('loginSuccess')
        Taro.reLaunch({ url: '/pages-weapp/profile/index' })
      } else {
        Taro.showToast({ title: result.message || '注册失败, 请重试', icon: 'none', duration: 3000 })
      }
    } catch (err) {
      Taro.showToast({ title: '网络错误, 请重试', icon: 'none' })
    }
    setSaving(false)
  }

  return (
    <View style={{ height: '100vh', backgroundColor: '#fafafa', display: 'flex', flexDirection: 'column', alignItems: 'center', padding: 32 }}>
      <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', marginBottom: 8, marginTop: 32 }}>注册账号</Text>
      <Text style={{ fontSize: 14, color: '#a1a1aa', marginBottom: 32 }}>填写信息即可开始租赁</Text>

      <Input placeholder="姓名" value={name} onInput={e => setName(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="手机号" value={phone} onInput={e => setPhone(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="邮箱（选填）" value={email} onInput={e => setEmail(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12 }} />
      <Input placeholder="密码（至少6位）" password value={password} onInput={e => setPassword(e.detail.value)}
        style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 24 }} />

      <View onClick={handleRegister}
        style={{ width: '100%', height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 12 }}>
        <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>{saving ? '注册中...' : '注册'}</Text>
      </View>
      <Text style={{ fontSize: 14, color: '#a1a1aa' }} onClick={() => Taro.navigateBack()}>返回</Text>
    </View>
  )
}
