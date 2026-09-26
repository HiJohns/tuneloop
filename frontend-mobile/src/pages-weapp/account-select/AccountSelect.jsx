import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input } from '@tarojs/components'
import { request, wxLogin as wxLoginCode, storage, session, env, eventBus, getInputValue } from '../../platform'
import { resolveErrorMessage } from '../../services/api'

const ROLE_LABELS = { site_member: '员工', site_admin: '网点管理员', repair_technician: '维修师傅', merchant_admin: '商户管理员', worker: '员工', OWNER: '负责人', ADMIN: '管理员' }

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

  // #2027 S1 (B1): flatten to loginable contexts (org + customer). Legacy IAM
  // responses without contexts fall back to one context per account.
  const contextItems = []
  accounts.forEach(a => {
    if (Array.isArray(a.contexts) && a.contexts.length > 0) {
      a.contexts.forEach((ct, i) => contextItems.push({
        key: `${a.user_id}-${ct.type}-${ct.org_id || i}`,
        functional_roles: ct.functional_roles || [],
        user_id: a.user_id,
        context: ct.type === 'customer' ? 'customer' : ct.org_id,
        type: ct.type,
        label: ct.label || (ct.type === 'customer' ? '顾客' : ct.org_name),
        account: a,
      }))
    } else {
      contextItems.push({
        key: `${a.user_id}-legacy`,
        user_id: a.user_id,
        context: a.is_customer ? 'customer' : (a.org_id || ''),
        type: a.is_customer ? 'customer' : 'org',
        label: a.is_customer ? '顾客' : ([a.merchant_name, a.site_name].filter(Boolean).join('-') || a.nickname || a.name || '员工账户'),
        account: a,
      })
    }
  })
  // 用户名密码登录入口仅员工场景显示（#1639 审计 Bug 3）
  // #2077：判定改为「存在非顾客上下文」——B1 后自服务注册用户一律带 customer 角色，
  // 旧口径 `!is_customer` 会让「顾客+员工」双身份账号误隐藏入口。
  // legacy 分支（无 contexts）的 type 已被映射为 'org'，同样被覆盖。
  const hasStaff = contextItems.some(it => it.type !== 'customer') || accounts.some(a => !a.is_customer)
  const greetingName = accounts[0]?.name || accounts[0]?.nickname || ''

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
          Taro.showToast({ title: resolveErrorMessage(result, '获取账户失败'), icon: 'none' })
        }
      } catch {
        Taro.showToast({ title: '网络错误，请重试', icon: 'none' })
      }
      setLoading(false)
    }
    load()
  }, [])

  const handleContextLogin = async (item) => {
    setLoggingIn(item.key)
    try {
      const exchangeToken = session.getItem('wx_login_token') || ''
      const body = { exchange_token: exchangeToken, context: item.context }
      // Legacy multi-account IAM resolves by user_id; harmless when context is set.
      if (item.user_id) body.user_id = item.user_id
      const res = await request(`${env.apiBaseUrl}/auth/wx-login-select`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
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
      Taro.showToast({ title: resolveErrorMessage(result, '登录失败，请重试'), icon: 'none' })
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
      Taro.showToast({ title: resolveErrorMessage(result, '登录失败，请重试'), icon: 'none' })
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
      <Text style={{ fontSize: 20, fontWeight: '900', color: '#000', display: 'block', marginBottom: 4 }}>{greetingName ? `欢迎 ${greetingName}` : '选择登录身份'}</Text>
      <Text style={{ fontSize: 13, color: '#a1a1aa', display: 'block', marginBottom: 20 }}>该微信关联了 {contextItems.length} 个身份</Text>

      {contextItems.map(item => (
        <View key={item.key} onClick={() => !loggingIn && handleContextLogin(item)}
          style={{ backgroundColor: '#fff', borderRadius: 12, padding: '14px 16px', marginBottom: 12, display: 'flex', alignItems: 'center', justifyContent: 'space-between', boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}>
          <View style={{ display: 'flex', alignItems: 'center', minWidth: 0 }}>
            <View style={{ width: 36, height: 36, borderRadius: 999, backgroundColor: item.type === 'customer' ? '#FDEBD0' : '#E3EAF3', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 12, flexShrink: 0 }}>
              <Text style={{ fontSize: 16 }}>{item.type === 'customer' ? '👤' : '💼'}</Text>
            </View>
            <View style={{ minWidth: 0 }}>
              <Text style={{ fontSize: 15, fontWeight: '700', color: '#18181b', display: 'block' }} numberOfLines={1}>{item.label}</Text>
              <Text style={{ fontSize: 12, color: '#a1a1aa', display: 'block', marginTop: 2 }}>
                {item.type === 'customer'
                  ? '顾客身份'
                  : ((item.functional_roles || []).map(r => ROLE_LABELS[r] || r).join('·') || '员工身份')}
              </Text>
            </View>
          </View>
          <View style={{ backgroundColor: '#915F38', borderRadius: 999, padding: '6px 14px', flexShrink: 0 }}>
            <Text style={{ color: '#fff', fontSize: 13, fontWeight: '700' }}>{loggingIn === item.key ? '登录中...' : '登录'}</Text>
          </View>
        </View>
      ))}

      {!hasCustomer && (
        <View onClick={goRegister}
          style={{ backgroundColor: '#fff', borderRadius: 12, padding: '14px 16px', marginBottom: 12, display: 'flex', alignItems: 'center', justifyContent: 'space-between', boxShadow: '0 1px 2px rgba(0,0,0,0.04)' }}>
          <View>
            <Text style={{ fontSize: 15, fontWeight: '700', color: '#18181b', display: 'block' }}>注册为顾客</Text>
            <Text style={{ fontSize: 12, color: '#a1a1aa', display: 'block', marginTop: 2 }}>使用微信注册顾客身份</Text>
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
