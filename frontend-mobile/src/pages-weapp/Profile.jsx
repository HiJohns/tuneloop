import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken, notificationApi } from '../services/api'
import { env, storage } from '../platform'
import { parseJWT } from '../platform/init'
import BottomNav from '../components-weapp/BottomNav'

function Badge({ count }) {
  return (
    <View style={{ position: 'absolute', top: -4, right: -8, backgroundColor: '#FF2A55', color: '#fff', fontSize: 10, fontWeight: '900', width: 16, height: 16, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', border: '1px solid #fff' }}>
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
  const nav = (url) => { Taro.navigateTo({ url }) }
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const [showEdit, setShowEdit] = useState(false)
  const [unreadCount, setUnreadCount] = useState(0)
  const [orderCounts, setOrderCounts] = useState({ reserved: 0, in_lease: 0, returning: 0, completed: 0 })

  const baseUrl = env.apiBaseUrl

  useEffect(() => {
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
  const isGuest = claims.role === 'GUEST' || (!token && user === null)

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
    nav('/pages-weapp/home/index')
  }

  if (loading) {
    return <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
  }

  return (
    <View style={{ height: '100vh', width: '100vw', backgroundColor: '#fafafa', overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      <ScrollView style={{ width: '100%', flex: '1 1 0%' }} scrollY showScrollbar={false}>

        {/* 1. 头部渐变身份区 */}
        <View style={{ width: '100%', background: 'linear-gradient(to bottom, #FDF4E7, #fff)', paddingLeft: 24, paddingRight: 24, paddingTop: 32, paddingBottom: 16, display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', position: 'relative' }}>
          <View style={{ display: 'flex', alignItems: 'center' }}>
            <View style={{ width: 80, height: 80, borderRadius: 999, overflow: 'hidden', border: '2px solid #fff', boxShadow: '0 1px 2px rgba(0,0,0,0.05)', flexShrink: 0, backgroundColor: '#e4e4e7', display: 'flex', alignItems: 'center', justifyContent: 'center' }} onClick={() => !isGuest && setShowEdit(true)}>
              {!isGuest && user?.avatar ? (
                <Image src={user.avatar} style={{ width: '100%', height: '100%' }} mode="aspectFill" />
              ) : (
                <Text style={{ fontSize: 30 }}>👤</Text>
              )}
            </View>
            <View style={{ marginLeft: 16 }}>
            {isGuest ? (
              <View style={{ backgroundColor: '#915F38', padding: '10px 24px', borderRadius: 999 }} onClick={() => nav('/pages-weapp/login/index')}>
                <Text style={{ color: '#fff', fontWeight: '700', fontSize: 14 }}>👉 登录查看资产</Text>
              </View>
            ) : (
              <>
              <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', letterSpacing: '0.025em' }}>{displayName}</Text>
              {user?.membership_level_id && (
                <Text style={{ fontSize: 12, color: '#b45309', marginTop: 2 }}>
                  {['', '初级会员', '中级会员', '高级会员'][user.membership_level_id] || `Level ${user.membership_level_id}`}
                </Text>
              )}
              {!isStaff && (
                <Text style={{ fontSize: 14, color: '#71717a', marginTop: 6 }}>{user?.phone || '未绑定手机'}</Text>
              )}
              </>
            )}
          </View>
          </View>
          {!isGuest && (
          <View
            style={{ backgroundColor: 'rgba(255,255,255,0.8)', border: '1px solid #f4f4f5', color: '#92400e', fontSize: 12, fontWeight: '700', padding: '0 16px', height: 32, borderRadius: 999, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
            onClick={handleLogout}
          >
            退出登录
          </View>
          )}
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
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/my-leases/index?status=reserved')}>
                <View style={{ fontSize: 24, marginBottom: 4, position: 'relative' }}>
                  📥
                  {orderCounts.reserved > 0 && <Badge count={orderCounts.reserved} />}
                </View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>待付款</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/my-leases/index?status=in_lease')}>
                <View style={{ fontSize: 24, marginBottom: 4, position: 'relative' }}>
                  💬
                  {orderCounts.in_lease > 0 && <Badge count={orderCounts.in_lease} />}
                </View>
                <Text style={{ fontSize: 12, fontWeight: '700', color: '#3f3f46' }}>服务中</Text>
              </View>
              <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', paddingTop: 4, paddingBottom: 4, borderRadius: 12 }} onClick={() => nav('/pages-weapp/my-leases/index?status=completed')}>
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
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>🎁</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>收藏</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }} onClick={() => nav('/pages-weapp/membership/index')}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>👑</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>会员中心</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>⚙️</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>设置</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14, borderBottom: '1px solid #f4f4f5' }}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>💼</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>商务合作</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 14, paddingBottom: 14 }}>
            <View style={{ display: 'flex', alignItems: 'center' }}>
              <Text style={{ fontSize: 18, marginRight: 8 }}>📞</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#27272a' }}>联系我们</Text>
            </View>
            <Text style={{ fontSize: 14, color: '#d4d4d8' }}>❯</Text>
          </View>
        </View>

      </ScrollView>

      {/* 5. 底部固定导航栏 */}
      <BottomNav
        active="profile"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => nav('/pages-weapp/home/index') },
          { key: 'rent', icon: '🪕', label: '租赁', onClick: () => nav('/pages-weapp/my-leases/index') },
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => nav('/pages-weapp/my-repairs/index') },
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
