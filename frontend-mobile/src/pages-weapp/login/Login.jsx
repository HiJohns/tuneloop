import { useState } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import { wxLogin, storage, env, request, eventBus } from '../../platform'

async function handleWxLogin() {
  Taro.showLoading({ title: '正在登录...' })
  try {
    const code = await wxLogin()
    if (!code) { Taro.hideLoading(); return }
    const res = await request(`${env.apiBaseUrl}/auth/wx-login`, {
      method: 'POST',
      body: JSON.stringify({ code }),
    })
    const result = await res.json()
    Taro.hideLoading()
    if (result.code === 20000 && result.data?.token) {
      storage.setItem('token', result.data.token)
      Taro.navigateTo({ url: '/pages-weapp/profile-complete/index' })
    }
  } catch {}
}

async function handleIAMLogin(identifier, password) {
  Taro.showLoading({ title: '正在登录...' })
  try {
    const res = await request(`${env.apiBaseUrl}/auth/login`, {
      method: 'POST',
      body: JSON.stringify({ identifier, password }),
    })
    const result = await res.json()
    Taro.hideLoading()
    if (result.code === 20000 && result.data?.access_token) {
      storage.setItem('token', result.data.access_token)
      storage.setItem('token_expiry', (Date.now() + (result.data.expires_in || 3600) * 1000).toString())
      eventBus.emit('loginSuccess')
      Taro.reLaunch({ url: '/pages-weapp/profile/index' })
    } else {
      Taro.showToast({ title: result.message || '登录失败 [L1]', icon: 'none' })
    }
  } catch (err) {
    Taro.hideLoading()
    Taro.showToast({ title: '网络错误 ' + (err.message || ''), icon: 'none' })
  }
}

function handleGuestBrowse() {
  Taro.navigateBack()
}

export default function Login() {
  const [identifier, setIdentifier] = useState('')
  const [password, setPassword] = useState('')
  const [tapCount, setTapCount] = useState(0)
  const [showDevMode, setShowDevMode] = useState(false)

  const handleVersionTap = () => {
    const next = tapCount + 1
    setTapCount(next)
    if (next >= 5) {
      setShowDevMode(true)
      setTapCount(0)
    }
  }

  return (
    <View style={{ height: '100vh', width: '100vw', backgroundColor: '#fafafa' }}>
      <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100%', width: '100%', paddingLeft: 32, paddingRight: 32, boxSizing: 'border-box' }}>
      <Text style={{ fontSize: 28, fontWeight: '900', color: '#000', marginBottom: 48 }}>登录</Text>

      {/* Channel 1: WeChat one-click */}
      <View style={{ width: '100%', paddingLeft: 0, paddingRight: 0 }}>
        <Button onClick={handleWxLogin}
          style={{ margin: 0, width: '100%', backgroundColor: '#07c160', color: '#fff', borderRadius: 24, fontSize: 16, fontWeight: '700', marginBottom: 16, paddingTop: 12, paddingBottom: 12, border: 'none', boxSizing: 'border-box' }}>
          <Text style={{ color: '#fff', fontSize: 16, fontWeight: '700' }}>📱 微信用户一键登录</Text>
        </Button>
      </View>

      {/* Divider */}
      <View style={{ display: 'flex', alignItems: 'center', width: '100%', marginBottom: 16 }}>
        <View style={{ flex: 1, height: 1, backgroundColor: '#e4e4e7' }} />
        <Text style={{ paddingLeft: 12, paddingRight: 12, fontSize: 14, color: '#a1a1aa' }}>其他方式</Text>
        <View style={{ flex: 1, height: 1, backgroundColor: '#e4e4e7' }} />
      </View>

      {/* Channel 2: IAM account login */}
      <View style={{ width: '100%', marginBottom: 12 }}>
        <Input placeholder="邮箱/手机号" value={identifier} onInput={e => setIdentifier(e.detail.value)}
          style={{ margin: 0, width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 12, boxSizing: 'border-box' }} />
        <Input placeholder="密码" password value={password} onInput={e => setPassword(e.detail.value)}
          style={{ margin: 0, width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 12, padding: '0 16px', fontSize: 14, marginBottom: 16, boxSizing: 'border-box' }} />
        <Button onClick={() => handleIAMLogin(identifier, password)}
          style={{ margin: 0, width: '100%', height: 44, backgroundColor: '#915F38', color: '#fff', borderRadius: 22, fontSize: 14, fontWeight: '700', lineHeight: '44px', boxSizing: 'border-box' }}>
          登录
        </Button>
      </View>

      {/* Channel 3: Guest browse */}
      <View style={{ marginTop: 24 }}>
        <Text style={{ fontSize: 14, color: '#71717a' }} onClick={handleGuestBrowse}>👀 随便看看</Text>
      </View>

      {/* Developer mode toggle */}
      <Text style={{ fontSize: 10, color: '#d4d4d8', marginTop: 32 }} onClick={handleVersionTap}>版本 v1.0.0</Text>
      {showDevMode && (
        <View style={{ marginTop: 12, width: '100%' }}>
          <Button onClick={() => { storage.removeItem('token'); storage.removeItem('token_expiry'); Taro.navigateBack() }}
            style={{ width: '100%', height: 36, backgroundColor: '#fee2e2', color: '#b91c1c', borderRadius: 8, fontSize: 12, display: 'flex', alignItems: 'center', justifyContent: 'center', lineHeight: '36px' }}>
            清除 Token
          </Button>
        </View>
      )}
    </View>
    </View>
  )
}
