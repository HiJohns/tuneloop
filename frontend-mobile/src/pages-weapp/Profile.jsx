import { useState, useEffect } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, Image, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken, notificationApi, resolveLogin , resolveErrorMessage } from '../services/api'
import { env, storage, session, eventBus, wxLogin } from '../platform'
import { parseJWT } from '../platform/init'
import BottomNav from '../components-weapp/BottomNav'
import ErrorBoundary from '../components-weapp/ErrorBoundary'

function Badge({ count }) {
  return (
    <View style={{ position: 'absolute', top: -4, right: -8, backgroundColor: '#FF2A55', color: '#fff', fontSize: 10, fontWeight: '900', width: 16, height: 16, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', border: '1px solid #fff' }}>
      {count > 9 ? '9+' : count}
    </View>
  )
}

function EditProfileModal({ visible, user, onClose, onSave, baseUrl }) {
  const [form, setForm] = useState({ name: '', phone: '', email: '' })
  const [saving, setSaving] = useState(false)
  const [msg, setMsg] = useState('')

  useEffect(() => {
    if (user) setForm({ name: user.name || '', phone: user.phone || '', email: user.email || '' })
  }, [user, visible])

  if (!visible) return null

  const handleSave = async () => {
    setSaving(true)
    setMsg('')
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`, {
        method: 'PUT',
        body: JSON.stringify({ name: form.name, phone: form.phone, email: form.email }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        if (result.data?.email_confirmation === 'pending') {
          setMsg('邮箱修改已提交，请查收确认邮件')
        } else {
          setMsg('资料已更新')
          onSave(form)
        }
      } else {
        setMsg(resolveErrorMessage(result, '更新失败'))
      }
    } catch (err) {
      setMsg('网络错误: ' + err.message)
    }
    setSaving(false)
  }

  return (
    <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.5)', zIndex: 50, display: 'flex', alignItems: 'flex-end', justifyContent: 'center' }}>
      <View style={{ backgroundColor: '#fff', borderTopLeftRadius: 16, borderTopRightRadius: 16, width: '100%', maxWidth: 480, padding: 24 }}>
        <Text style={{ fontSize: 18, fontWeight: '700', marginBottom: 16 }}>编辑资料</Text>
        <View>
          <View style={{ marginBottom: 16 }}>
            <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 4 }}>姓名</Text>
            <Input value={form.name} onInput={e => setForm({ ...form, name: e.detail.value })} style={{ width: '100%', border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14 }} />
          </View>
          <View style={{ marginBottom: 16 }}>
            <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 4 }}>手机</Text>
            <Input value={form.phone} onInput={e => setForm({ ...form, phone: e.detail.value })} style={{ width: '100%', border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14 }} />
          </View>
          <View style={{ marginBottom: 16 }}>
            <Text style={{ fontSize: 14, color: '#6b7280', marginBottom: 4 }}>邮箱</Text>
            <Input value={form.email} onInput={e => setForm({ ...form, email: e.detail.value })} style={{ width: '100%', border: '1px solid #d4d4d8', borderRadius: 8, padding: '8px 12px', fontSize: 14 }} />
          </View>
        </View>
        {msg && <Text style={{ fontSize: 14, textAlign: 'center', marginTop: 12, color: '#d97706' }}>{msg}</Text>}
        <View style={{ display: 'flex', marginTop: 16 }}>
          <View style={{ flex: '1 1 0%', textAlign: 'center', paddingTop: 8, paddingBottom: 8, border: '1px solid #d4d4d8', borderRadius: 8, color: '#6b7280', marginRight: 12 }} onClick={onClose}>取消</View>
          <View style={{ flex: '1 1 0%', textAlign: 'center', paddingTop: 8, paddingBottom: 8, backgroundColor: '#92400e', color: '#fff', borderRadius: 8 }} onClick={handleSave}>{saving ? '保存中...' : '保存'}</View>
        </View>
      </View>
    </View>
  )
}

export default function Profile() {
  const nav = (url) => {
    if (Taro.getCurrentPages().length >= 9) {
      Taro.reLaunch({ url })
    } else {
      Taro.navigateTo({ url })
    }
  }
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  // #1694: login button debounce — rapid taps must not open multiple
  // register/login flows (several stacked modal windows).
  const [loginBusy, setLoginBusy] = useState(false)
  const [showEdit, setShowEdit] = useState(false)
  const [unreadCount, setUnreadCount] = useState(0)
  const [orderCounts, setOrderCounts] = useState({ reserved: 0, in_lease: 0, returning: 0, completed: 0 })

  const baseUrl = env.apiBaseUrl
  const fixImg = (url) => url && !url.startsWith('http') && !url.startsWith('data:') ? baseUrl.replace(/\/api$/, '') + url : url

  // Refresh user data every time the page becomes visible (e.g. returning
  // from the profile edit page) — #1588. The eventBus subscription stays
  // mounted once via useEffect.
  useDidShow(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) setUser(result.data)
        else setUser(null) // not logged in — clear stale UI (#1620)
      } catch (err) {
        console.error('Failed to fetch user:', err)
        setUser(null) // token missing/invalid — clear stale UI (#1620)
      }
      setLoading(false)
    }
    fetchUser()
  })

  useEffect(() => {
    const refreshUser = async () => {
      setLoading(true)
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) setUser(result.data)
      } catch (err) {
        console.error('Failed to fetch user:', err)
      }
      setLoading(false)
    }
    eventBus.on('loginSuccess', refreshUser)
    return () => eventBus.off('loginSuccess', refreshUser)
  }, [baseUrl])

  useEffect(() => {
    const fetchUnread = async () => {
      try {
        const resp = await notificationApi.unreadCount()
        setUnreadCount(resp?.data?.count ?? 0)
      } catch {}
    }
    fetchUnread()
    const interval = setInterval(fetchUnread, 30000)
    return () => clearInterval(interval)
  }, [])

  const displayName = user?.nickname || user?.name || user?.username || '路人'
  const token = getToken()
  const claims = token ? parseJWT(token) : {}
  // isStaff 统一判定（#1639）：role === 'STAFF' 或 oid/tid 非空（员工有组织绑定）
  const isStaff = claims.role === 'STAFF' || !!(claims.oid || claims.tid)
  const isGuest = claims.role === 'GUEST' || (!token && user === null)
  const hasGuestToken = claims.role === 'GUEST'
  // Two-phase registration (#1663): a pending registration session means the
  // guest already filled the form once — surface "继续完成注册" instead of a
  // fresh "注册为会员" entry.
  const [pendingSession, setPendingSession] = useState(null)
  // P-05: the guest button label reflects the current WeChat account state —
  // an already-registered user (≥1 bound account, just not logged in) sees
  // "登录" instead of the misleading "注册为会员". Until the probe query
  // finishes the label defaults to "登录" (never flashes a wrong label).
  const [wechatHasAccount, setWechatHasAccount] = useState(false)
  const [wechatQueryDone, setWechatQueryDone] = useState(false)

  useEffect(() => {
    let cancelled = false
    const checkPendingSession = async () => {
      if (!isGuest) return
      const sid = session.getItem('pending_registration_session')
      if (!sid) { setPendingSession(null); return }
      try {
        const resp = await apiFetch(`${baseUrl}/auth/registration-sessions/me?session_id=${sid}`)
        const result = await resp.json()
        if (!cancelled) {
          if (result.code === 20000 && result.data?.status === 'pending') {
            setPendingSession(sid)
          } else {
            session.removeItem('pending_registration_session')
            setPendingSession(null)
          }
        }
      } catch {
        if (!cancelled) setPendingSession(null)
      }
    }
    checkPendingSession()
    return () => { cancelled = true }
  }, [isGuest, baseUrl])

  // P-05: silently query wx-accounts on page entry (guest only) to decide
  // the button label. The code consumed here is fine — the actual login
  // (resolveLogin) re-runs wx.login for a fresh code on click.
  useEffect(() => {
    let cancelled = false
    const checkWechatAccounts = async () => {
      if (!isGuest) return
      try {
        const code = await wxLogin()
        if (!code || cancelled) return
        const resp = await apiFetch(`${baseUrl}/auth/wx-accounts?code=${encodeURIComponent(code)}`)
        const result = await resp.json()
        if (!cancelled && result.code === 20000) {
          setWechatHasAccount((result.data?.accounts || []).length > 0)
          setWechatQueryDone(true)
        }
      } catch { /* keep default (登录) */ }
    }
    checkWechatAccounts()
    return () => { cancelled = true }
  }, [isGuest, baseUrl])

  useEffect(() => {
    const fetchCounts = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/user/orders/counts`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrderCounts(result.data || {})
        }
      } catch {}
    }
    if (!isStaff) fetchCounts()
  }, [baseUrl, isStaff])

  const handleLogout = () => {
    storage.removeItem('token')
    storage.removeItem('token_expiry')
    storage.removeItem('refresh_token')
    // Clear UI state immediately so the page does not keep showing the
    // previous account after logout (#1620).
    setUser(null)
    setLoading(false)
    // reLaunch forces a fresh page stack — switchTab would keep the
    // cached tab state alive (stale login UI on tab return).
    Taro.reLaunch({ url: '/pages-weapp/home/index' })
  }

  const handleSwitchAccount = () => {
    Taro.navigateTo({ url: '/pages-weapp/account-select/index' })
  }

  // 来源 A 分流（#1639）：未登录点击「我的」→ wx.login → wx-accounts → 0/1/N
  const handleGuestLogin = async () => {
    if (loginBusy) return
    setLoginBusy(true)
    try {
      const ok = await resolveLogin('profile')
      if (ok) {
        Taro.reLaunch({ url: '/pages-weapp/profile/index' })
      }
    } finally {
      setTimeout(() => setLoginBusy(false), 800)
    }
  }

  const goMyLeasesStatus = (status) => {
    try {
      Taro.setStorageSync('tab_params', { status })
    } catch {}
    Taro.switchTab({ url: '/pages-weapp/my-leases/index' })
  }

  if (loading) {
    return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#FDFBF7' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
  }

  return (
    <ErrorBoundary>
    <View style={{ height: '100vh', width: '100vw', backgroundColor: '#FDFBF7', overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <ScrollView style={{ width: '100%', flex: '1 1 0%' }} scrollY showScrollbar={false}>

        {/* 1. 头部渐变身份区 */}
        <View style={{ width: '100%', background: 'linear-gradient(to bottom, #FDF4E7, #fff)', paddingLeft: 24, paddingRight: 24, paddingTop: 32, paddingBottom: 16, display: 'flex', alignItems: 'flex-start', boxSizing: 'border-box' }}>
          <View style={{ display: 'flex', alignItems: 'center' }}>
            <View style={{ width: 80, height: 80, borderRadius: 999, overflow: 'hidden', border: '2px solid #fff', boxShadow: '0 1px 2px rgba(0,0,0,0.05)', flexShrink: 0, backgroundColor: '#e4e4e7', display: 'flex', alignItems: 'center', justifyContent: 'center' }} onClick={() => !isGuest && setShowEdit(true)}>
              {!isGuest && user?.avatar ? (
                <Image src={fixImg(user.avatar)} style={{ width: '100%', height: '100%' }} mode="aspectFill" />
              ) : (
                <Text style={{ fontSize: 30 }}>👤</Text>
              )}
            </View>
            <View style={{ marginLeft: 16 }}>
            {isGuest ? (
              <View style={{ backgroundColor: '#915F38', padding: '10px 24px', borderRadius: 999 }} onClick={handleGuestLogin}>
                <Text style={{ color: '#fff', fontWeight: '700', fontSize: 14 }}>{hasGuestToken ? '👋 轻触绑定手机' : (pendingSession ? '✏️ 继续完成注册' : (wechatQueryDone ? (wechatHasAccount ? '👉 登录' : '👉 注册为会员') : '👉 登录'))}</Text>
              </View>
            ) : (
              <>
              <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center' }}>
                <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', letterSpacing: '0.025em' }}>{displayName}</Text>
                {isStaff && (user?.tenant_name || user?.site_name) && (
                  <Text style={{ fontSize: 11, color: '#71717a', marginLeft: 8, backgroundColor: '#f4f4f5', padding: '2px 8px', borderRadius: 999, overflow: 'hidden', maxWidth: 180 }}>
                    {[user?.tenant_name, user?.site_name].filter(Boolean).join(' · ')}
                  </Text>
                )}
              </View>
              {user?.membership_level_id && (
                <Text style={{ fontSize: 12, color: '#b45309', marginTop: 2 }}>
                  {['', '初级会员', '中级会员', '高级会员'][user?.membership_level_id] || `Level ${user?.membership_level_id}`}
                </Text>
              )}
              {!isStaff && (
                <Text style={{ fontSize: 14, color: '#71717a', marginTop: 6 }}>{user?.phone || '未绑定手机'}</Text>
              )}
              <View style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', marginTop: 8 }}>
                <View
                  style={{ backgroundColor: 'rgba(255,255,255,0.8)', border: '1px solid #f4f4f5', color: '#92400e', fontSize: 12, fontWeight: '700', padding: '0 16px', height: 32, borderRadius: 999, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', display: 'flex', alignItems: 'center', justifyContent: 'center', alignSelf: 'flex-start' }}
                  onClick={handleLogout}
                >
                  退出登录
                </View>
                {isStaff && (
                  <View
                    style={{ marginLeft: 8, backgroundColor: 'rgba(255,255,255,0.8)', border: '1px solid #f4f4f5', color: '#92400e', fontSize: 12, fontWeight: '700', padding: '0 16px', height: 32, borderRadius: 999, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', display: 'flex', alignItems: 'center', justifyContent: 'center', alignSelf: 'flex-start' }}
                    onClick={handleSwitchAccount}
                  >
                    切换账户
                  </View>
                )}
              </View>
              </>
            )}
          </View>
          </View>
        </View>

        {/* 2. 金刚过滤区 */}
        <View style={{ marginLeft: 16, marginRight: 16, backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginTop: 12, padding: 16, display: 'flex', justifyContent: 'space-around' }}>
          {isGuest ? (
            <>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12, opacity: 0.5 }}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>🔒</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>待付款</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12, opacity: 0.5 }}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>🔒</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>服务中</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12, opacity: 0.5 }}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>🔒</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>已完成</Text>
              </View>
            </>
          ) : isStaff ? (
            <>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/staff-orders/index')}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>📋</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>订单管理</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/staff-instruments/index')}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>🎸</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>乐器管理</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/receiving-interface/index')}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>📥</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>接收</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/shipping-interface/index')}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>📤</View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>发货</Text>
              </View>
            </>
          ) : (
            <>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => goMyLeasesStatus('reserved')}>
                <View style={{ fontSize: 24, marginBottom: 4, position: 'relative' }}>
                  📥
                  {orderCounts.reserved > 0 && <Badge count={orderCounts.reserved} />}
                </View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>待付款</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => goMyLeasesStatus('in_lease')}>
                <View style={{ fontSize: 24, marginBottom: 4, position: 'relative' }}>
                  💬
                  {orderCounts.in_lease > 0 && <Badge count={orderCounts.in_lease} />}
                </View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>服务中</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => goMyLeasesStatus('completed')}>
                <View style={{ fontSize: 24, marginBottom: 4 }}>
                  ✖️
                </View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>已完成</Text>
              </View>
            </>
          )}
        </View>

        {/* 4. 下方通用抽屉式列表 */}
        <View style={{ marginLeft: 16, marginRight: 16, backgroundColor: '#fff', borderRadius: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginTop: 12, padding: 16 }}>
          {!isGuest && (
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }} onClick={() => nav('/pages-weapp/messages/index')}>
              <View style={{ display: 'flex', alignItems: 'center' }}>
                <Text style={{ fontSize: 18, marginRight: 8 }}>✉️</Text>
                <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>系统信息</Text>
              </View>
              <View style={{ display: 'flex', alignItems: 'center' }}>
                {unreadCount > 0 && <Text style={{ fontSize: 12, color: '#FF2A55', fontWeight: '700', marginRight: 4 }}>{unreadCount}条未读</Text>}
                <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
              </View>
            </View>
          )}
          {!isStaff && !isGuest && (
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }} onClick={() => nav('/pages-weapp/membership/index')}>
              <View style={{ display: 'flex', alignItems: 'center' }}>
                <Text style={{ fontSize: 18, marginRight: 8 }}>👑</Text>
                <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>会员中心</Text>
              </View>
              <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
            </View>
          )}
          {!isGuest && (
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }} onClick={() => nav('/pages-weapp/setting/index')}>
              <View style={{ display: 'flex', alignItems: 'center' }}>
                <Text style={{ fontSize: 18, marginRight: 8 }}>⚙️</Text>
                <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>设置</Text>
              </View>
              <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
            </View>
          )}
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }} onClick={() => nav('/pages-weapp/content/index?key=cooperation')}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>💼</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>商务合作</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14 }} onClick={() => nav('/pages-weapp/content/index?key=contact_us')}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>📞</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>联系我们</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
        </View>

        {/* #1692: frontend package version for build attribution */}
        {env.version && (
          <Text style={{ display: 'block', textAlign: 'center', fontSize: 12, color: '#d4d4d8', marginTop: 24, marginBottom: 12 }}>
            v{env.version}
          </Text>
        )}

      </ScrollView>

      {/* 5. 底部固定导航栏 */}
      <BottomNav
        active="profile"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => Taro.switchTab({ url: '/pages-weapp/home/index' }) },
          { key: 'rent', icon: '🪕', label: '租赁', onClick: () => Taro.switchTab({ url: '/pages-weapp/my-leases/index' }) },
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => nav('/pages-weapp/my-repairs/index') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => {} },
        ]}
        badges={{ profile: isStaff ? 0 : unreadCount }}
      />

      <EditProfileModal
        visible={showEdit}
        user={user}
        baseUrl={baseUrl}
        onClose={() => setShowEdit(false)}
        onSave={(updated) => { setUser({ ...user, ...updated }); setShowEdit(false) }}
      />
    </View>
    </ErrorBoundary>
  )
}
