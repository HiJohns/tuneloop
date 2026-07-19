import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, ScrollView } from '@tarojs/components'
import { apiFetch, getToken, notificationApi } from '../services/api'
import { env, storage } from '../platform'
import { parseJWT, getAppConfig } from '../platform/init'
import BottomNav from '../components/BottomNav'

function Badge({ count }) {
  return (
    <View className="absolute -top-1 -right-2 bg-[#FF2A55] text-white text-[10px] font-black w-4 h-4 rounded-full flex items-center justify-center border border-white">
      {count > 9 ? '9+' : count}
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
        setMsg(result.message || '更新失败')
      }
    } catch (err) {
      setMsg('网络错误: ' + err.message)
    }
    setSaving(false)
  }

  return (
    <View className="fixed inset-0 bg-black/50 z-50 flex items-end justify-center">
      <View className="bg-white rounded-t-2xl w-full max-w-[480px] p-6">
        <Text className="text-lg font-bold mb-4">编辑资料</Text>
        <View className="space-y-4">
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
          <View className="flex-1 text-center py-2 border rounded-lg text-gray-500 active:bg-gray-50" onClick={onClose}>取消</View>
          <View className="flex-1 text-center py-2 bg-amber-800 text-white rounded-lg active:opacity-80" onClick={handleSave}>{saving ? '保存中...' : '保存'}</View>
        </View>
      </View>
    </View>
  )
}

export default function Profile() {
  const navigate = useNavigate()
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const [showEdit, setShowEdit] = useState(false)
  const [unreadCount, setUnreadCount] = useState(0)
  const [orderCounts, setOrderCounts] = useState({ reserved: 0, in_lease: 0, returning: 0, completed: 0 })
  const [appVersion, setAppVersion] = useState('')

  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const config = getAppConfig()
    if (config?.version && config.version !== 'dev') setAppVersion(config.version)
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) setUser(result.data)
      } catch (err) {
        console.error('Failed to fetch user:', err)
      }
      setLoading(false)
    }
    fetchUser()
  }, [])

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

  const displayName = user?.name || '路人'
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
      <ScrollView className="w-full flex-1 pb-20" scrollY showScrollbar={false}>

        {/* 1. 头部渐变身份区 */}
        <View className="w-full bg-gradient-to-b from-[#FDF4E7] to-white px-6 pt-8 pb-4 flex items-start justify-between relative">
          <View className="flex items-center gap-4">
            <View className="w-20 h-20 rounded-full overflow-hidden border-2 border-white shadow-sm flex-shrink-0 bg-zinc-200 flex items-center justify-center" onClick={() => setShowEdit(true)}>
              {user?.avatar ? (
                <Image src={user.avatar} className="w-full h-full" mode="aspectFill" />
              ) : (
                <Text className="text-3xl">👤</Text>
              )}
            </View>
            <View>
            <Text className="block text-2xl font-black text-black tracking-wide">{displayName}</Text>
            {user?.membership_level_id && (
              <Text className="text-xs text-amber-700 mt-0.5">
                {['', '初级会员', '中级会员', '高级会员'][user.membership_level_id] || `Level ${user.membership_level_id}`}
              </Text>
            )}

            {!isStaff && (
              <>
                <Text className="block text-sm text-zinc-500 mt-1.5">{user?.phone || '未绑定手机'}</Text>
              </>
            )}

          </View>
          </View>
          <View
            className="bg-white/80 backdrop-blur-sm border border-zinc-100 text-amber-800 text-xs font-bold px-4 h-8 rounded-full shadow-sm flex items-center justify-center active:opacity-70"
            onClick={handleLogout}
          >
            退出登录
          </View>
        </View>

        {/* 2. 金刚过滤区 — 员工 vs 顾客 */}
        <View className="mx-4 bg-white rounded-2xl shadow-sm mt-3 p-4 grid grid-cols-3 gap-2 text-center">
          {isStaff ? (
            <>
              <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl" onClick={() => navigate('/staff/instruments')}>
                <View className="text-2xl mb-1">🎸</View>
                <Text className="text-xs font-bold text-zinc-700">乐器管理</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl" onClick={() => navigate('/staff/receiving')}>
                <View className="text-2xl mb-1">📥</View>
                <Text className="text-xs font-bold text-zinc-700">接收</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl" onClick={() => navigate('/staff/shipping')}>
                <View className="text-2xl mb-1">📤</View>
                <Text className="text-xs font-bold text-zinc-700">发货</Text>
              </View>
            </>
          ) : (
            <>
              <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl" onClick={() => navigate('/my-leases?status=reserved')}>
                <View className="text-2xl mb-1 relative">
                  📥
                  {orderCounts.reserved > 0 && <Badge count={orderCounts.reserved} />}
                </View>
                <Text className="text-xs font-bold text-zinc-700">待付款</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl" onClick={() => navigate('/my-leases?status=in_lease')}>
                <View className="text-2xl mb-1 relative">
                  💬
                  {orderCounts.in_lease > 0 && <Badge count={orderCounts.in_lease} />}
                </View>
                <Text className="text-xs font-bold text-zinc-700">服务中</Text>
              </View>
              <View className="flex flex-col items-center justify-center py-1 active:bg-zinc-50 rounded-xl" onClick={() => navigate('/my-leases?status=completed')}>
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
          <View className="flex justify-between items-center py-3.5 active:opacity-60" onClick={() => navigate('/messages')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">✉️</Text>
              <Text className="text-base font-bold text-zinc-800">系统信息</Text>
            </View>
            <View className="flex items-center gap-1">
              {unreadCount > 0 && <Text className="text-xs text-[#FF2A55] font-bold">{unreadCount}条未读</Text>}
              <Text className="text-sm text-zinc-300">❯</Text>
            </View>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center gap-2">
              <Text className="text-lg">🎁</Text>
              <Text className="text-base font-bold text-zinc-800">收藏</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60" onClick={() => navigate('/membership')}>
            <View className="flex items-center gap-2">
              <Text className="text-lg">👑</Text>
              <Text className="text-base font-bold text-zinc-800">会员中心</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center gap-2">
              <Text className="text-lg">⚙️</Text>
              <Text className="text-base font-bold text-zinc-800">设置</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center gap-2">
              <Text className="text-lg">💼</Text>
              <Text className="text-base font-bold text-zinc-800">商务合作</Text>
            </View>
            <Text className="text-sm text-zinc-300">❯</Text>
          </View>
          <View className="flex justify-between items-center py-3.5 active:opacity-60">
            <View className="flex items-center gap-2">
              <Text className="text-lg">📞</Text>
              <Text className="text-base font-bold text-zinc-800">联系我们</Text>
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
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => token && navigate(isStaff ? '/my-repairs' : '/my-repairs') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => {} },
        ]}
        badges={{ profile: isStaff ? 0 : unreadCount }}
      />

      <EditProfileModal
        visible={showEdit}
        user={user}
        onClose={() => setShowEdit(false)}
        onSave={(updated) => { setUser({ ...user, ...updated }); setShowEdit(false) }}
      />
    </View>
  )
}
