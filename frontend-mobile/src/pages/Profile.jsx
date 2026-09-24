import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken, notificationApi , resolveErrorMessage } from '../services/api'
import { env, storage, toWeappRoute, getInputValue } from '../platform'
import { parseJWT, getAppConfig } from '../platform/init'
import BottomNav from '../components/BottomNav'

function Badge({ count }) {
  return (
    <View className="absolute -top-1 -right-2 text-white font-black w-4 h-4 rounded-full flex items-center justify-center border border-white" style={{ backgroundColor: '#FF2A55' }}>
      {count > 9 ? '9+' : count}
    </View>
  )
}

function PendingOrdersModal({ visible, onClose, navigate, user }) {
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  useEffect(() => {
    if (!visible) return
    setLoading(true)
    apiFetch(`${env.apiBaseUrl}/user/pending-orders`)
      .then(r => r.json())
      .then(res => { if (res.code === 20000) setList(res.data?.list || []) })
      .catch(() => {})
      .finally(() => setLoading(false))
  }, [visible])
  const remove = async (id) => {
    try {
      await apiFetch(`${env.apiBaseUrl}/user/pending-orders/${id}`, { method: 'DELETE' })
      setList(prev => prev.filter(x => x.id !== id))
    } catch {}
  }
  if (!visible) return null
  // #2041: 已实名 → 「继续提交」直接回跳结算页回填续提；未实名 → 引导去实名
  const idVerified = ['verified', 'pending_review'].includes(user?.id_verify_status)
  return (
    <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.45)' }}>
      <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 20, width: '88%', boxSizing: 'border-box', maxHeight: '70vh', overflow: 'auto' }}>
        <Text style={{ fontSize: 17, fontWeight: '900', color: '#18181b', display: 'block', marginBottom: 12 }}>待提交订单</Text>
        {loading ? (
          <Text style={{ fontSize: 13, color: '#a1a1aa' }}>加载中...</Text>
        ) : list.length === 0 ? (
          <Text style={{ fontSize: 13, color: '#a1a1aa' }}>暂无待提交订单</Text>
        ) : list.map(item => (
          <View key={item.id} style={{ border: '1px solid #f4f4f5', borderRadius: 10, padding: 10, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, color: '#18181b', display: 'block' }}>乐器：{item.payload?.instrument_id || '—'}</Text>
            <Text style={{ fontSize: 11, color: '#a1a1aa', display: 'block', marginTop: 2 }}>{item.created_at ? String(item.created_at).slice(0, 16).replace('T', ' ') : ''}</Text>
            <View style={{ display: 'flex', flexDirection: 'row', marginTop: 8, gap: 8 }}>
              <View onClick={() => {
                onClose()
                if (idVerified && item.payload?.instrument_id) {
                  navigate(`/checkout?id=${item.payload.instrument_id}&resume_pending=${item.id}`)
                } else {
                  navigate('/profile/edit')
                }
              }} style={{ flex: 1, height: 32, borderRadius: 16, backgroundColor: '#915F38', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Text style={{ color: '#fff', fontSize: 12, fontWeight: '700' }}>{idVerified ? '继续提交' : '完成实名后提交'}</Text>
              </View>
              <View onClick={() => remove(item.id)} style={{ height: 32, paddingLeft: 12, paddingRight: 12, borderRadius: 16, backgroundColor: '#f4f4f5', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                <Text style={{ color: '#71717a', fontSize: 12 }}>放弃</Text>
              </View>
            </View>
          </View>
        ))}
        <View onClick={onClose} style={{ width: '100%', height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', marginTop: 4 }}>
          <Text style={{ color: '#a1a1aa', fontSize: 14 }}>关闭</Text>
        </View>
      </View>
    </View>
  )
}

function JoinSiteModal({ visible, onClose }) {
  const [code, setCode] = useState('')
  const [joining, setJoining] = useState(false)
  if (!visible) return null
  const submit = async () => {
    if (!code.trim()) { Taro.showToast({ title: '请输入邀请码', icon: 'none' }); return }
    setJoining(true)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/user/accept-invite`, {
        method: 'POST',
        body: JSON.stringify({ code: code.trim() }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '加入成功', icon: 'success' })
        setCode('')
        onClose()
      } else {
        Taro.showToast({ title: resolveErrorMessage(result, '加入失败'), icon: 'none' })
      }
    } catch {
      Taro.showToast({ title: '网络错误，请重试', icon: 'none' })
    }
    setJoining(false)
  }
  return (
    <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: 'rgba(0,0,0,0.45)' }}>
      <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 20, width: '85%', boxSizing: 'border-box' }}>
        <Text style={{ fontSize: 17, fontWeight: '900', color: '#18181b', display: 'block', marginBottom: 8 }}>加入网点</Text>
        <Text style={{ fontSize: 12, color: '#71717a', display: 'block', marginBottom: 14 }}>请输入管理员提供的邀请码（员工身份将加到你的当前账户）</Text>
        <Input
          value={code}
          onInput={e => setCode(getInputValue(e))}
          placeholder="邀请码"
          style={{ width: '100%', height: 44, border: '1px solid #d4d4d8', borderRadius: 8, paddingLeft: 12, paddingRight: 12, fontSize: 14, marginBottom: 16, boxSizing: 'border-box' }}
        />
        <View onClick={joining ? undefined : submit} style={{ width: '100%', height: 44, backgroundColor: '#915F38', borderRadius: 22, display: 'flex', alignItems: 'center', justifyContent: 'center', marginBottom: 8 }}>
          <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>{joining ? '处理中...' : '确认加入'}</Text>
        </View>
        <View onClick={onClose} style={{ width: '100%', height: 40, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
          <Text style={{ color: '#a1a1aa', fontSize: 14 }}>取消</Text>
        </View>
      </View>
    </View>
  )
}

function EditProfileModal({ visible, user, onClose, onSave }) {
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
      const baseUrl = env.apiBaseUrl
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
    <View className="fixed inset-0 z-50 flex items-end justify-center" style={{ backgroundColor: 'rgba(0,0,0,0.5)' }}>
      <View className="bg-white rounded-t-2xl w-full p-6">
        <Text className="text-lg font-bold mb-4">编辑资料</Text>
        <View style={{ display: 'flex', flexDirection: 'column', gap: 16 }}>
          <View>
            <Text className="text-sm text-gray-500">姓名</Text>
            <input type="text" value={form.name} onChange={e => setForm({ ...form, name: e.target.value })} className="w-full border rounded-lg px-3 py-2 mt-1" />
          </View>
          <View>
            <Text className="text-sm text-gray-500">手机</Text>
            <input type="text" value={form.phone} onChange={e => setForm({ ...form, phone: e.target.value })} className="w-full border rounded-lg px-3 py-2 mt-1" />
          </View>
          <View>
            <Text className="text-sm text-gray-500">邮箱</Text>
            <input type="text" value={form.email} onChange={e => setForm({ ...form, email: e.target.value })} className="w-full border rounded-lg px-3 py-2 mt-1" />
          </View>
        </View>
        {msg && <Text className="text-sm text-center mt-3 text-amber-600">{msg}</Text>}
        <View className="flex gap-3 mt-4">
          <View className="flex-1 text-center py-2 border rounded-lg text-gray-500" onClick={onClose}>取消</View>
          <View className="flex-1 text-center py-2 bg-amber-800 text-white rounded-lg" onClick={handleSave}>{saving ? '保存中...' : '保存'}</View>
        </View>
      </View>
    </View>
  )
}

export default function Profile() {
  const navigate = useNavigate()
  // #1686: cross-end nav — H5 short path vs weapp full path (Cart.jsx pattern).
  const nav = (to) => {
    if (!env.isMiniProgram) { navigate(to); return }
    const [path, query] = to.split('?')
    const page = {
      '/messages': 'messages',
      '/membership': 'membership',
      '/setting': 'setting',
      '/profile/edit': 'profile/edit',
      '/content': 'content',
    }[path]
    if (!page) { navigate(to); return }
    const url = `/pages-weapp/${page}/index${query ? '?' + query : ''}`
    Taro.navigateTo({ url })
  }
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const [showEdit, setShowEdit] = useState(false)
  const [showJoin, setShowJoin] = useState(false)
  const [showPending, setShowPending] = useState(false)
  const [unreadCount, setUnreadCount] = useState(0)
  const [orderCounts, setOrderCounts] = useState({ reserved: 0, in_lease: 0, returning: 0, completed: 0 })
  const [appVersion, setAppVersion] = useState('')
  const [transitMember, setTransitMember] = useState(false) // #1937: 中转网点成员身份

  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    // #1692: frontend package version (1.0.<git short hash>, injected at
    // build) is authoritative for version attribution; backend config
    // version is the fallback.
    setAppVersion(env.version || '')
    const config = getAppConfig()
    if (!env.version && config?.version && config.version !== 'dev') setAppVersion(config.version)
    const fetchUser = async () => {
      // #1903: guest state has no token — skip the protected call (#1620 stale UI)
      if (!getToken()) { setUser(null); setLoading(false); return }
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) setUser(result.data)
      } catch (err) {
        console.error('Failed to fetch user:', err)
      }
      setLoading(false)
      // #2041: 实名认证通知直达待提交订单列表
      try {
        if (storage.getItem('open_pending_modal') === '1') {
          storage.removeItem('open_pending_modal')
          setShowPending(true)
        }
      } catch { /* 读取失败不影响页面 */ }
    }
    fetchUser()
  }, [])

  useEffect(() => {
    const fetchUnread = async () => {
      // #1903: guest state must not poll the backend
      if (!getToken()) { setUnreadCount(0); return }
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
  const isStaff = claims.role === 'STAFF'

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

  // #1937: 中转网点成员 → 个人中心展示「中转工作台」入口（/site-members/me 返回 site_type）
  useEffect(() => {
    const fetchMySites = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/site-members/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          const sites = result.data?.sites || []
          setTransitMember(sites.some(s => s.site_type === 'transit'))
        }
      } catch {}
    }
    if (isStaff) fetchMySites()
  }, [baseUrl, isStaff])

  const navTransitWorkbench = () => {
    if (!env.isMiniProgram) return navigate('/transit-workbench')
    const route = toWeappRoute('/transit-workbench')
    if (route) Taro.navigateTo({ url: route.url })
  }

  const handleLogout = () => {
    storage.removeItem('token')
    storage.removeItem('token_expiry')
    storage.removeItem('refresh_token')
    navigate('/')
  }

  if (loading) {
    return <View className="h-screen flex items-center justify-center bg-zinc-50"><Text className="text-zinc-400">加载中...</Text></View>
  }

  return (
    <View className="h-screen w-screen bg-zinc-50 overflow-hidden flex flex-col relative antialiased">
      <ScrollView className="w-full flex-1 min-h-0 pb-36" scrollY showScrollbar={false}>

        {/* 1. 头部渐变身份区 */}
        <View className="w-full px-6 pt-8 pb-4 flex items-start relative" style={{ backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
          <View className="flex items-center gap-4">
            <View className="w-20 h-20 rounded-full overflow-hidden border-2 border-white shadow-sm flex-shrink-0 bg-zinc-200 flex items-center justify-center" onClick={() => setShowEdit(true)}>
              {user?.avatar ? (
                <Image src={user.avatar} className="w-full h-full" mode="aspectFill" />
              ) : (
                <Text className="text-3xl">👤</Text>
              )}
            </View>
            <View className="flex-1 min-w-0">
              {/* 第1行: 姓名 + 电话 水平并排 (#1634) */}
              <View className="flex items-baseline gap-2 flex-wrap">
                <Text className="text-2xl font-black text-black tracking-wide">{displayName}</Text>
                {!isStaff && (
                  <Text className="text-sm text-zinc-500">{user?.phone || '未绑定手机'}</Text>
                )}
              </View>
              {user?.membership_level_id && (
                <Text className="text-xs text-amber-700 mt-0.5">
                  {user.membership_level_name || `Level ${user.membership_level_id}`}
                </Text>
              )}

              {/* 退出登录 — 与小程序一致，位于昵称下方（#1608） */}
              <View
                className="backdrop-blur-sm border border-zinc-100 text-amber-800 text-xs font-bold px-4 h-8 rounded-full shadow-sm flex items-center justify-center self-start mt-2"
                style={{ backgroundColor: 'rgba(255,255,255,0.8)' }}
                onClick={handleLogout}
              >
                退出登录
              </View>
            </View>
          </View>
        </View>

        {/* 2. 金刚过滤区 — 员工 vs 顾客 */}
        <View className={`mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 grid ${isStaff && transitMember ? 'grid-cols-4' : 'grid-cols-3'} gap-2 text-center`}>
          {isStaff ? (
            <>
              <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={() => navigate('/staff/instruments')}>
                <View className="text-2xl mb-1">🎸</View>
                <Text className="text-xs font-bold text-zinc-700">乐器管理</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={() => navigate('/staff/receiving')}>
                <View className="text-2xl mb-1">📥</View>
                <Text className="text-xs font-bold text-zinc-700">接收</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={() => navigate('/staff/shipping')}>
                <View className="text-2xl mb-1">📤</View>
                <Text className="text-xs font-bold text-zinc-700">发货</Text>
              </View>
              {transitMember && (
                <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={navTransitWorkbench}>
                  <View className="text-2xl mb-1">🚚</View>
                  <Text className="text-xs font-bold text-zinc-700">中转工作台</Text>
                </View>
              )}
            </>
          ) : (
            <>
              <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={() => navigate('/my-leases?status=reserved')}>
                <View className="text-2xl mb-1 relative">
                  📥
                  {orderCounts.reserved > 0 && <Badge count={orderCounts.reserved} />}
                </View>
                <Text className="text-xs font-bold text-zinc-700">待付款</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={() => navigate('/my-leases?status=in_lease')}>
                <View className="text-2xl mb-1 relative">
                  💬
                  {orderCounts.in_lease > 0 && <Badge count={orderCounts.in_lease} />}
                </View>
                <Text className="text-xs font-bold text-zinc-700">服务中</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 rounded-xl" onClick={() => navigate('/my-leases?status=completed')}>
                <View className="text-2xl mb-1">
                  ✖️
                </View>
                <Text className="text-xs font-bold text-zinc-700">已完成</Text>
              </View>
            </>
          )}
        </View>


        {/* 4. 下方通用抽屉式列表 */}
        <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 divide-y divide-zinc-100">
          {/* 1. 平台规则 */}
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/setting')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">📄</Text>
              <Text className="text-base font-bold text-zinc-800">平台规则</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          {/* 2. 会员中心（仅顾客） */}
          {!isStaff && (
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/membership')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">👑</Text>
              <Text className="text-base font-bold text-zinc-800">会员中心</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          )}
          {/* 3. 个人资料 */}
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/profile/edit')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">✏️</Text>
              <Text className="text-base font-bold text-zinc-800">个人资料</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          {/* 4. 系统通知 */}
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/messages')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">✉️</Text>
              <Text className="text-base font-bold text-zinc-800">系统通知</Text>
            </View>
            <View className="flex items-center gap-1">
              {unreadCount > 0 && <Text className="text-xs font-bold" style={{ color: '#FF2A55' }}>{unreadCount}条未读</Text>}
              <Text className="text-sm text-zinc-300">❯</Text>
            </View>
          </View>
          {/* 5. 申请发票（仅顾客） */}
          {!isStaff && (
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/content?key=invoice')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">🧾</Text>
              <Text className="text-base font-bold text-zinc-800">申请发票</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          )}
          {/* 5.5 加入网点（#2031 邀请制自助加入） */}
          <View className="flex justify-between items-center py-3.5" onClick={() => setShowJoin(true)}>
            <View className="flex items-center">
              <Text className="text-base font-bold text-zinc-800">加入网点</Text>
            </View>
            <Text className="text-zinc-300 text-lg">›</Text>
          </View>

          {/* 5.6 待提交订单（#2041 未实名拦截后缓存） */}
          <View className="flex justify-between items-center py-3.5" onClick={() => setShowPending(true)}>
            <View className="flex items-center">
              <Text className="text-base font-bold text-zinc-800">待提交订单</Text>
            </View>
            <Text className="text-zinc-300 text-lg">›</Text>
          </View>

          {/* 6. 商务合作 */}
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/content?key=cooperation')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">💼</Text>
              <Text className="text-base font-bold text-zinc-800">商务合作</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          {/* 7. 联系我们 */}
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/content?key=contact_us')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">📞</Text>
              <Text className="text-base font-bold text-zinc-800">联系我们</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          {/* 8. 关于 */}
          <View className="flex justify-between items-center py-3.5" onClick={() => nav('/about')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">ℹ️</Text>
              <Text className="text-base font-bold text-zinc-800">关于</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          {appVersion && (
            <Text className="block text-center text-xs text-zinc-300 mt-8 mb-4">v{appVersion}</Text>
          )}
        </View>

      </ScrollView>

      {/* 5. 底部固定导航栏 */}
      <BottomNav
        active="profile"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => navigate('/') },
          { key: 'rent', icon: '🪕', label: '租赁', onClick: () => token && navigate(isStaff ? '/staff/orders' : '/my-leases') },
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => token && navigate('/tech-list') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => {} },
        ]}
        badges={{ profile: isStaff ? 0 : unreadCount }}
      />

      <JoinSiteModal visible={showJoin} onClose={() => setShowJoin(false)} />
      <PendingOrdersModal visible={showPending} onClose={() => setShowPending(false)} navigate={nav} user={user} />

      <EditProfileModal
        visible={showEdit}
        user={user}
        onClose={() => setShowEdit(false)}
        onSave={(updated) => { setUser({ ...user, ...updated }); setShowEdit(false) }}
      />
    </View>
  )
}
