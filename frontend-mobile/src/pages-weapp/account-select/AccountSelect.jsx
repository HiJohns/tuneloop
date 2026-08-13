import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input } from '@tarojs/components'
import { request, wxLogin as wxLoginCode, storage, session, env, eventBus, getInputValue } from '../../platform'

const TAB_PAGES = ['/pages-weapp/home/index', '/pages-weapp/my-leases/index', '/pages-weapp/profile/index']

function navigatePostAuth(url) {
  if (TAB_PAGES.includes(url)) {
    Taro.switchTab({ url })
  } else {
    Taro.redirectTo({ url })
  }
}

// AccountSelect — 微信多账户列表页（#1639）
// 展示 openid 关联的所有账户（顾客/员工），点击账户登录；员工场景提供
// 「用户名密码登录」展开面板 + 「注册为会员」入口。
export default function AccountSelect() {
  const [accounts, setAccounts] = useState([])
  const [loading, setLoading] = useState(true)
  const [showPwd, setShowPwd] = useState(false)
  const [identifier, setIdentifier] = useState('')
  const [password, setPassword] = useState('')
  const [loggingIn, setLoggingIn] = useState('')
  const [pwdLoggingIn, setPwdLoggingIn] = useState(false)

  const hasCustomer = accounts.some(a => a.is_customer)
  // 用户名密码登录入口仅员工场景显示（#1639 审计 Bug 3）
  const hasStaff = accounts.some(a => !a.is_customer)

  useEffect(() => {
    const load = async () => {
      try {
        const code = await wxLoginCode()
        if (!code) { Taro.showToast({ title: '登录状态失效，请重试', icon: 'none' }); setLoading(false); return }
        const res = await request(`${env.apiBaseUrl}/auth/wx-accounts?code=${encodeURIComponent(code)}`)
        const result = await res.json()
        if (result.code === 20000 && result.data) {
          setAccounts(result.data.accounts || [])
          // Keep the exchange_token for wx-login-select on account click
          // (WeChat code is single-use, consumed by wx-accounts above)
          session.setItem('wx_login_token', result.data.exchange_token || '')
        } else {
          Taro.showToast({ title: result.message || '获取账户失败', icon: 'none' })
        }
      } catch {
        Taro.showToast({ title: '网络错误，请重试', icon: 'none' })
      }
      setLoading(false)
    }
    load()
  }, [])

  const handleAccountLogin = async (acc) => {
    setLoggingIn(acc.user_id)
    try {
      const exchangeToken = session.getItem('wx_login_token') || ''
      const res = await request(`${env.apiBaseUrl}/auth/wx-login-select`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ exchange_token: exchangeToken, user_id: acc.user_id }),
      })
      const result = await res.json()
      if (result.code === 20000 && result.data?.access_token) {
        storage.setItem('token', result.data.access_token)
        if (result.data.expires_in) {
          storage.setItem('token_expiry', (new Date().getTime() + result.data.expires_in * 1000).toString())
        }
        if (result.data.refresh_token) storage.setItem('refresh_token', result.data.refresh_token)
        session.removeItem('wx_login_token')
        eventBus.emit('loginSuccess')
        const postAuth = session.getItem('post_auth_redirect')
        if (postAuth) {
          session.removeItem('post_auth_redirect')
          navigatePostAuth(postAuth)
        } else {
          Taro.switchTab({ url: '/pages-weapp/profile/index' })
        }
        return
      }
      Taro.showToast({ title: result.message || '登录失败，请重试', icon: 'none' })
    } catch {
      Taro.showToast({ title: '网络错误，请重试', icon: 'none' })
    }
    setLoggingIn('')
  }

  const handlePwdLogin = async () => {
    if (!identifier.trim() || !password) {
      Taro.showToast({ title: '请输入账号和密码', icon: 'none' })
      return
    }
    setPwdLoggingIn(true)
    try {
      const res = await request(`${env.apiBaseUrl}/auth/login`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ identifier: identifier.trim(), password }),
      })
      const result = await res.json()
      if (result.code === 20000 && result.data?.access_token) {
        storage.setItem('token', result.data.access_token)
        if (result.data.expires_in) {
          storage.setItem('token_expiry', (new Date().getTime() + result.data.expires_in * 1000).toString())
        }
        session.removeItem('wx_login_token')
        eventBus.emit('loginSuccess')
        const postAuth = session.getItem('post_auth_redirect')
        if (postAuth) {
          session.removeItem('post_auth_redirect')
          navigatePostAuth(postAuth)
        } else {
          Taro.switchTab({ url: '/pages-weapp/profile/index' })
        }
        return
      }
      Taro.showToast({ title: result.message || '登录失败，请重试', icon: 'none' })
    } catch {
      Taro.showToast({ title: '网络错误，请重试', icon: 'none' })
    }
    setPwdLoggingIn(false)
  }

  const goRegister = () => {
    Taro.redirectTo({ url: '/pages-weapp/profile-complete/index' })
  }

  if (loading) {
    return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
  }

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#fafafa', padding: 24, boxSizing: 'border-box' }}>
      <Text style={{ fontSize: 20, fontWeight: '900', color: '#000', display: 'block', marginBottom: 4 }}>选择登录账户</Text>
      <Text style={{ fontSize: 13, color: '#a1a1aa', display: 'block', marginBottom: 20 }}>该微信关联了 {accounts.length} 个账户</Text>

      {accounts.map(acc => (
        <View key={acc.user_id} onClick={() => !loggingIn && handleAccountLogin(acc)}
          style={{ backgroundColor: '#fff', borderRadius: 12, padding: '14px 16px', marginBottom: 12, display: 'flex', alignItems: 'center', justifyContent: 'space-between', boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}>
          <View style={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
            <View style={{ width: 36, height: 36, borderRadius: 999, backgroundColor: acc.is_customer ? '#FDEBD0' : '#E3EAF3', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 12, flexShrink: 0 }}>
              <Text style={{ fontSize: 16 }}>{acc.is_customer ? '👤' : '💼'}</Text>
            </View>
            <View style={{ minWidth: 0 }}>
              <Text style={{ fontSize: 15, fontWeight: '700', color: '#18181b', display: 'block' }} numberOfLines={1}>{acc.nickname || acc.name || '未命名账户'}</Text>
              {!acc.is_customer && (
                <Text style={{ fontSize: 12, color: '#a1a1aa', display: 'block', marginTop: 2 }}>
                  {[acc.merchant_name, acc.site_name].filter(Boolean).join('-') || '员工账户'}
                </Text>
              )}
            </View>
          </View>
          <View style={{ backgroundColor: '#915F38', borderRadius: 999, padding: '6px 14px', flexShrink: 0 }}>
            <Text style={{ color: '#fff', fontSize: 13, fontWeight: '700' }}>{loggingIn === acc.user_id ? '登录中...' : '登录'}</Text>
          </View>
        </View>
      ))}

      {!hasCustomer && (
        <View onClick={goRegister}
          style={{ backgroundColor: '#fff', borderRadius: 12, padding: '14px 16px', marginBottom: 12, display: 'flex', alignItems: 'center', justifyContent: 'space-between', boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}>
          <View>
            <Text style={{ fontSize: 15, fontWeight: '700', color: '#18181b', display: 'block' }}>注册为会员</Text>
            <Text style={{ fontSize: 12, color: '#a1a1aa', display: 'block', marginTop: 2 }}>使用微信注册新会员账户</Text>
          </View>
          <Text style={{ color: '#915F38', fontSize: 16 }}>›</Text>
        </View>
      )}

      {hasStaff && (
        <View onClick={() => setShowPwd(!showPwd)}
          style={{ padding: '10px 4px', display: 'flex', justifyContent: 'center' }}>
          <Text style={{ color: '#915F38', fontSize: 14, fontWeight: '600' }}>{showPwd ? '收起' : '用户名密码登录'}</Text>
        </View>
      )}

      {showPwd && (
        <View style={{ backgroundColor: '#fff', borderRadius: 12, padding: 16, marginTop: 4, boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}>
          <Input value={identifier} onInput={e => setIdentifier(getInputValue(e))} placeholder="用户名 / 手机号 / 邮箱"
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, marginBottom: 12, boxSizing: 'border-box' }} />
          <Input value={password} onInput={e => setPassword(getInputValue(e))} password placeholder="密码"
            style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, marginBottom: 12, boxSizing: 'border-box' }} />
          <View onClick={handlePwdLogin}
            style={{ width: '100%', height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>{pwdLoggingIn ? '登录中...' : '登录'}</Text>
          </View>
        </View>
      )}

      <View style={{ marginTop: 24, display: 'flex', justifyContent: 'center' }} onClick={() => Taro.navigateBack()}>
        <Text style={{ color: '#a1a1aa', fontSize: 13 }}>返回</Text>
      </View>
    </View>
  )
}
