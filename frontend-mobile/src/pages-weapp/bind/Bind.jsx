import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { wxLogin, storage, env, request } from '../../platform'

export default function Bind() {
  const [status, setStatus] = useState('loading')

  useEffect(() => {
    const params = Taro.getCurrentInstance().router?.params || {}
    const token = params.token
    if (!token) {
      setStatus('error')
      return
    }
    doBind(token)
  }, [])

  const doBind = async (token) => {
    try {
      const code = await wxLogin()
      if (!code) { setStatus('error'); return }
      const res = await request(`${env.apiBaseUrl}/auth/wx-login`, {
        method: 'POST',
        body: JSON.stringify({ code }),
      })
      const result = await res.json()
      if (result.code === 20000 && result.data?.wx_openid) {
        const confRes = await request(`${env.apiBaseUrl}/wechat-bind/confirm`, {
          method: 'POST',
          body: JSON.stringify({ token, wx_openid: result.data.wx_openid }),
        })
        const confResult = await confRes.json()
        if (confResult.code === 20000) {
          setStatus('success')
          return
        }
      }
      setStatus('error')
    } catch {
      setStatus('error')
    }
  }

  return (
    <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa', padding: 32 }}>
      {status === 'loading' && <Text style={{ color: '#a1a1aa' }}>正在绑定...</Text>}
      {status === 'success' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>✅</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>绑定成功</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24 }}>PC 端将自动刷新</Text>
          <Button onClick={() => Taro.navigateBack()} style={{ backgroundColor: '#915F38', color: '#fff', borderRadius: 22, padding: '12px 32px', fontSize: 14, fontWeight: '700', border: 'none' }}>
            返回
          </Button>
        </View>
      )}
      {status === 'error' && (
        <View style={{ alignItems: 'center' }}>
          <Text style={{ fontSize: 48, marginBottom: 16 }}>❌</Text>
          <Text style={{ fontSize: 18, fontWeight: '700', color: '#000', marginBottom: 8 }}>绑定失败</Text>
          <Text style={{ fontSize: 14, color: '#71717a', marginBottom: 24 }}>请重试</Text>
          <Button onClick={() => Taro.navigateBack()} style={{ backgroundColor: '#915F38', color: '#fff', borderRadius: 22, padding: '12px 32px', fontSize: 14, fontWeight: '700', border: 'none' }}>
            返回
          </Button>
        </View>
      )}
    </View>
  )
}
