import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { message } from 'antd'
import { api, apiFetch, getToken, redirectToLogin, addressesApi, notificationApi, resendEmailConfirmation } from '../services/api'
import { User, MapPin, Bell, ChevronRight, LogOut, Edit3, Key, Package, History, Clock, FileText, ClipboardList, Plus, Trash2, CheckCircle, Send, AlertCircle } from 'lucide-react'
import AddressForm from '../components/AddressForm'
import { dialog, env, storage, session, cookie, openLink } from '../platform'
import { formatDisplayDate } from '../utils/format'

function EditProfileModal({ visible, user, onClose, onSave }) {
  const [form, setForm] = useState({ name: '', phone: '', email: '' })
  const [saving, setSaving] = useState(false)
  const [message, setMessage] = useState('')

  useEffect(() => {
    if (user) {
      setForm({ name: user.name || '', phone: user.phone || '', email: user.email || '' })
    }
  }, [user, visible])

  if (!visible) return null

  const handleSave = async () => {
    setSaving(true)
    setMessage('')
    try {
      const token = getToken()
      const baseUrl = env.apiBaseUrl
      const resp = await apiFetch(`${baseUrl}/users/me`, {
        method: 'PUT',
        body: JSON.stringify({
          name: form.name,
          phone: form.phone,
          email: form.email,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        if (result.data?.email_confirmation === 'pending') {
          setMessage('Email change submitted. Check your inbox for confirmation.')
        } else {
          setMessage('Profile updated successfully.')
          onSave(form)
        }
      } else {
        setMessage(result.message || 'Update failed')
      }
    } catch (err) {
      setMessage('Network error: ' + err.message)
    }
    setSaving(false)
  }

  return (
    <View className="fixed inset-0 bg-black/50 z-50 flex items-end justify-center">
      <View className="bg-white rounded-t-2xl w-full max-w-[480px] p-6">
        <Text className="text-lg font-bold mb-4">Edit Profile</Text>
        <View className="space-y-4">
          <View>
            <label className="text-sm text-gray-500">Name</label>
            <input
              type="text"
              value={form.name}
              onChange={e => setForm({ ...form, name: e.target.value })}
              className="w-full border rounded-lg px-3 py-2 mt-1"
              placeholder="Enter your name"
            />
          </View>
          <View>
            <label className="text-sm text-gray-500">Phone</label>
            <input
              type="text"
              value={form.phone}
              onChange={e => setForm({ ...form, phone: e.target.value })}
              className="w-full border rounded-lg px-3 py-2 mt-1"
              placeholder="Enter phone number"
            />
          </View>
          <View>
            <label className="text-sm text-gray-500">Email</label>
            <input
              type="email"
              value={form.email}
              onChange={e => setForm({ ...form, email: e.target.value })}
              className="w-full border rounded-lg px-3 py-2 mt-1"
              placeholder="Enter email"
            />
          </View>
          {message && (
            <View className="text-sm text-brand-primary bg-blue-50 p-2 rounded">{message}</View>
          )}
        </View>
        <View className="flex gap-3 mt-6">
          <Button onClick={onClose} className="flex-1 py-2 border rounded-lg text-gray-600">
            Cancel
          </Button>
          <Button
            onClick={handleSave}
            disabled={saving}
            className="flex-1 py-2 bg-brand-primary text-white rounded-lg disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save'}
          </Button>
        </View>
      </View>
    </View>
  )
}

function parseImages(images) {
  if (!images) return []
  if (Array.isArray(images)) return images
  if (typeof images === 'string') {
    try { return JSON.parse(images) } catch { return [] }
  }
  return []
}

export default function Profile() {
  const navigate = useNavigate()
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const [showEdit, setShowEdit] = useState(false)
  const [activeLeases, setActiveLeases] = useState([])
  const [leaseHistory, setLeaseHistory] = useState([])
  const [addresses, setAddresses] = useState([])
  const [showAddressForm, setShowAddressForm] = useState(false)
  const [editingAddress, setEditingAddress] = useState(null)
  const [unreadCount, setUnreadCount] = useState(0)

  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchUser = async () => {
      console.warn('[Profile] fetchUser start, token=' + (getToken() ? getToken().substring(0, 20) + '...' : 'NULL'))
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        console.warn('[Profile] /users/me response status=' + resp.status)
        const result = await resp.json()
        console.warn('[Profile] /users/me result code=' + result.code)
        if (result.code === 20000) {
          setUser(result.data)
        }
      } catch (err) {
        console.error('[Profile] fetchUser error:', err)
      }
      setLoading(false)
    }
    fetchUser()
  }, [])

  const loadAddresses = async () => {
    if (!getToken()) return
    try {
      const resp = await addressesApi.list()
      if (Array.isArray(resp)) {
        setAddresses(resp)
      } else if (resp.code === 20000) {
        setAddresses(resp.data?.list || [])
      }
    } catch (err) {
      console.error('Failed to fetch addresses:', err)
    }
  }

  useEffect(() => {
    loadAddresses()
  }, [])

  useEffect(() => {
    fetchOrders()
  }, [])

  useEffect(() => {
    const fetchUnread = async () => {
      try {
        const resp = await notificationApi.unreadCount()
        const count = resp?.data?.count ?? 0
        setUnreadCount(count)
      } catch {}
    }
    fetchUnread()
    const interval = setInterval(fetchUnread, 30000)
    return () => clearInterval(interval)
  }, [])

  const fetchOrders = async () => {
    try {
      const resp = await apiFetch(`${baseUrl}/orders`)
      const result = await resp.json()
      if (result.code === 20000) {
        const allOrders = result.data?.list || []
        const active = allOrders.filter(o => ['reserved', 'paid', 'pending_shipment', 'in_transit', 'shipped', 'in_lease', 'returning', 'expired'].includes(o.status))
        const history = allOrders.filter(o => ['returned', 'completed', 'cancelled', 'transferred'].includes(o.status))
        setActiveLeases(active)
        setLeaseHistory(history)
      }
    } catch (err) {
      console.error('Failed to fetch orders:', err)
    }
  }

  const handleLogout = () => {
    storage.removeItem('token')
    storage.removeItem('token_expiry')
    storage.removeItem('user_info')
    storage.removeItem('user_sys_perm')
    storage.removeItem('user_cus_perm')
    storage.removeItem('user_cus_perm_ext')
    session.removeItem('token')
    cookie.remove('token')
    navigate('/')
  }

  const handlePasswordSetup = () => {
    const appConfig = storage.getJSON('app_config', {})
    const iamUrl = appConfig?.wx?.iamExternalUrl || env.iamExternalUrl
    if (iamUrl) {
      openLink(`${iamUrl}/auth/setup-password`)
    }
  }

  const businessRole = user?.business_role || ''

  const statusLabel = {
    reserved: '已预约',
    paid: '待发货',
    pending_shipment: '待发货',
    in_transit: '运输中',
    shipped: '已发货',
    in_lease: '租赁中',
    returning: '归还中',
    returned: '已归还',
    completed: '已完成',
    cancelled: '已取消',
    expired: '超期',
    deposit_refunding: '押金退还中',
    damage_appealing: '定损申诉中',
  }

  const statusColor = {
    reserved: 'bg-blue-100 text-blue-700',
    paid: 'bg-orange-100 text-orange-700',
    pending_shipment: 'bg-orange-100 text-orange-700',
    in_transit: 'bg-cyan-100 text-cyan-700',
    shipped: 'bg-green-100 text-green-700',
    in_lease: 'bg-indigo-100 text-indigo-700',
    returning: 'bg-yellow-100 text-yellow-700',
    returned: 'bg-gray-100 text-gray-600',
    completed: 'bg-gray-100 text-gray-600',
    cancelled: 'bg-red-100 text-red-700',
    expired: 'bg-red-100 text-red-700',
    deposit_refunding: 'bg-blue-100 text-blue-700',
    damage_appealing: 'bg-orange-100 text-orange-700',
  }

  const isOverdue = (order) => {
    if ((order.status !== 'in_lease' && order.status !== 'expired') || !order.end_date) return false
    return new Date(order.end_date) < new Date()
  }

  const overdueDays = (order) => {
    if (!isOverdue(order) || !order.end_date) return 0
    const diff = new Date() - new Date(order.end_date)
    return Math.ceil(diff / (1000 * 60 * 60 * 24))
  }

  if (loading) {
    return (
      <View className="min-h-screen bg-brand-bg flex items-center justify-center">
        <View className="text-gray-500">Loading...</View>
      </View>
    )
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-6">
        <View className="flex items-center gap-4">
          <View className="w-16 h-16 bg-white/20 rounded-full flex items-center justify-center">
            <User size={32} />
          </View>
          <View className="flex-1">
            <Text className="text-lg font-bold">{user?.name || 'User'}</Text>
            <Text className="text-sm opacity-90">{user?.phone || user?.email || ''}</Text>
          </View>
          <Button onClick={handleLogout} className="text-white/80 hover:text-white">
            <LogOut size={20} />
          </Button>
        </View>
      </View>

      <View className="p-4 space-y-3">
        {/* Personal Info - localized */}
        <View className="bg-white rounded-xl p-4">
          <View className="flex justify-between items-center mb-3">
            <Text className="font-medium">个人信息</Text>
            <Button onClick={() => setShowEdit(true)} className="text-brand-primary">
              <Edit3 size={18} />
            </Button>
          </View>
          <View className="space-y-2 text-sm text-gray-600">
            <View className="flex justify-between"><Text>姓名</Text><Text>{user?.name || '-'}</Text></View>
            <View className="flex justify-between"><Text>电话</Text><Text>{user?.phone || '-'}</Text></View>
            <View className="flex justify-between items-start">
              <Text>邮箱</Text>
              <View className="text-right">
                <Text>{user?.email || '-'}</Text>
                {user?.email && (() => {
                  const sent = user.email_sent_at ? new Date(user.email_sent_at) : null
                  const confirmed = user.email_confirmed_at ? new Date(user.email_confirmed_at) : null
                  const isConfirmed = confirmed && sent && confirmed > sent
                  const expired = sent && (Date.now() - sent.getTime() > 24 * 60 * 60 * 1000)

                  if (isConfirmed) {
                    return <Text className="text-xs text-green-600 flex items-center gap-1 mt-0.5 justify-end"><CheckCircle size={12} /> 已确认</Text>
                  } else if (sent && !expired) {
                    return <Text className="text-xs text-orange-500 mt-0.5">确认邮件已发送，请检查邮箱及垃圾箱</Text>
                  } else if (sent && expired) {
                    return (
                      <Button
                        onClick={async () => {
                          try {
                            await resendEmailConfirmation()
                            message.success('确认邮件已重新发送')
                          } catch { message.error('重发失败，请稍后重试') }
                        }}
                        className="text-xs text-red-500 underline mt-0.5"
                      >
                        确认邮件已失效，点击重发
                      </Button>
                    )
                  }
                  return null
                })()}
              </View>
            </View>
          </View>
        </View>

        {/* 设置密码 */}
        <View className="bg-white rounded-xl p-4">
          <Button
            onClick={handlePasswordSetup}
            className="flex items-center gap-3 w-full py-2"
          >
            <Key size={18} className="text-gray-400" />
            <Text className="flex-1 text-left text-sm">设置密码</Text>
            <ChevronRight size={16} className="text-gray-300" />
          </Button>
        </View>

        {/* 收货地址 — 仅对顾客显示 */}
        {businessRole !== 'site_admin' && businessRole !== 'site_member' && (
          <View className="bg-white rounded-xl p-4">
            <View className="flex justify-between items-center mb-3">
              <Text className="font-medium flex items-center gap-2">
                <MapPin size={18} className="text-brand-primary" />
                收货地址
              </Text>
              <Button onClick={() => { setEditingAddress(null); setShowAddressForm(true) }} className="text-brand-primary">
                <Plus size={18} />
              </Button>
            </View>
            {addresses.length === 0 ? (
              <Text className="text-sm text-red-500">请设置默认收货地址</Text>
            ) : (
              <View className="space-y-2">
                {addresses.filter(a => a.is_default).slice(0, 1).map(addr => (
                  <View key={addr.id} className="text-sm text-gray-600">
                    <Text className="font-medium">{addr.recipient_name} · {addr.phone}</Text>
                    <Text className="text-xs text-gray-400">{addr.province}{addr.city}{addr.district}{addr.detail}{addr.postal_code ? ` ${addr.postal_code}` : ''}</Text>
                    <View className="flex items-center gap-2 mt-1">
                      <Text className="text-xs bg-green-100 text-green-700 px-1.5 py-0.5 rounded">默认</Text>
                      <Button onClick={() => { setEditingAddress(addr); setShowAddressForm(true) }} className="text-xs text-brand-primary">编辑</Button>
                      <Button onClick={async () => { if (dialog.confirm('确认删除？')) { await addressesApi.delete(addr.id); loadAddresses() } }} className="text-xs text-red-500">删除</Button>
                    </View>
                  </View>
                ))}
              </View>
            )}
          </View>
        )}

        {/* Current Rentals */}
        {businessRole !== 'site_admin' && businessRole !== 'site_member' && activeLeases.length > 0 && (
          <View className="bg-white rounded-xl p-4">
            <Text className="font-medium mb-3 flex items-center gap-2">
              <Package size={18} className="text-brand-primary" />
              当前租赁
            </Text>
            <View className="space-y-3">
              {activeLeases.map(order => (
                <View
                  key={order.id}
                  className="border rounded-lg p-3 cursor-pointer"
                  onClick={() => navigate(`/order/${order.id}`)}
                >
                  <View className="flex justify-between items-start mb-2">
                    <Text className="font-medium text-sm">Order #{order.id?.slice(0, 8)}</Text>
                    <Text className={`text-xs px-2 py-1 rounded-full ${statusColor[order.status] || 'bg-gray-100'}`}>
                      {statusLabel[order.status] || order.status}
                    </Text>
                  </View>
                  <View className="text-xs text-gray-500 space-y-1">
                    {order.start_date && <Text>开始: {formatDisplayDate(order.start_date)}</Text>}
                    {order.end_date && <Text>结束: {formatDisplayDate(order.end_date)}</Text>}
                    {isOverdue(order) && (
                      <Text className="text-red-500 font-medium">
                        超期 {overdueDays(order)} 天 · 超期费 ¥{((order.monthly_rent || 0) / 30 * overdueDays(order)).toFixed(0)}
                      </Text>
                    )}
                    {(order.status === 'in_transit' || order.status === 'shipped') && order.tracking_number && (
                      <Text>物流: {order.courier_company || ''} {order.tracking_number}</Text>
                    )}
                    {order.status === 'returning' && order.tracking_number && (
                      <Text>归还物流: {order.courier_company || ''} {order.tracking_number}</Text>
                    )}
                    <Text>月租: ¥{order.monthly_rent} · 押金: ¥{order.deposit}</Text>
                  </View>
                </View>
              ))}
            </View>
          </View>
        )}

        {/* Rental History */}
        {businessRole !== 'site_admin' && businessRole !== 'site_member' && (
          <View className="bg-white rounded-xl p-4 cursor-pointer" onClick={() => navigate('/lease-history')}>
            <View className="flex items-center gap-3">
              <History size={18} className="text-gray-400" />
              <Text className="flex-1 text-sm">租赁历史</Text>
              <ChevronRight size={16} className="text-gray-300" />
            </View>
          </View>
        )}

        {/* My Account (customer) or Staff Functions */}
        {(businessRole === 'site_admin' || businessRole === 'site_member') && (
          <View className="space-y-3">
            <View className="bg-white rounded-xl p-4">
              <Text className="font-medium mb-3">员工功能</Text>
              <View className="grid grid-cols-2 gap-4">
                {(() => {
                  const mapping = storage.getJSON('permission_mapping', {})
                  const cusPerm = parseInt(storage.getItem('user_cus_perm') || '0')
                  const has = (code) => { const b = mapping[code]; return b !== undefined && (cusPerm & (1 << b)) !== 0 }
                  return (
                    <>
                      {has('instrument:read') && (
                        <Button onClick={() => navigate('/staff/instruments')} className="flex flex-col items-center p-2">
                          <MapPin size={24} className="text-brand-primary" />
                          <Text className="text-xs mt-1 text-gray-600">乐器管理</Text>
                        </Button>
                      )}
                      {has('order:read') && (
                        <Button onClick={() => navigate('/staff/orders')} className="flex flex-col items-center p-2">
                          <ClipboardList size={24} className="text-brand-primary" />
                          <Text className="text-xs mt-1 text-gray-600">订单管理</Text>
                        </Button>
                      )}
                    </>
                  )
                })()}
              </View>
            </View>
          </View>
        )}
        {!(businessRole === 'site_admin' || businessRole === 'site_member') && (
          <View className="space-y-3">
            <View className="bg-white rounded-xl p-4">
              <Text className="font-medium mb-3">My Account</Text>
              <Button
                onClick={() => navigate('/my-contracts')}
                className="flex items-center gap-3 w-full py-2"
              >
                <FileText size={18} className="text-gray-400" />
                <Text className="flex-1 text-left text-sm">我的合同</Text>
                <ChevronRight size={16} className="text-gray-300" />
              </Button>
              <Button
                onClick={() => navigate('/messages')}
                className="flex items-center gap-3 w-full py-2 relative"
              >
                <View className="relative">
                  <Bell size={18} className="text-gray-400" />
                  {unreadCount > 0 && (
                    <Text className="absolute -top-1 -right-1 w-4 h-4 bg-red-500 text-white text-[10px] rounded-full flex items-center justify-center">
                      {unreadCount > 9 ? '9+' : unreadCount}
                    </Text>
                  )}
                </View>
                <Text className="flex-1 text-left text-sm">消息</Text>
                {unreadCount > 0 && (
                  <Text className="text-xs text-brand-primary">{unreadCount} 条未读</Text>
                )}
                <ChevronRight size={16} className="text-gray-300" />
              </Button>
            </View>
          </View>
        )}
      </View>

      <EditProfileModal
        visible={showEdit}
        user={user}
        onClose={() => setShowEdit(false)}
        onSave={(updated) => setUser({ ...user, ...updated })}
      />

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <View className="flex justify-around py-3 max-w-[480px] mx-auto">
          <View
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/')}
          >
            <Text className="text-xl">🏠</Text>
            <Text className="text-xs mt-1">Home</Text>
          </View>
          <View
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <Text className="text-xl">🔧</Text>
            <Text className="text-xs mt-1">Service</Text>
          </View>
          {getToken() ? (
            <View
              className="flex flex-col items-center text-brand-primary cursor-pointer"
              onClick={() => navigate('/profile')}
            >
              <Text className="text-xl">👤</Text>
              <Text className="text-xs mt-1">Me</Text>
            </View>
          ) : (
            <View
              className="flex flex-col items-center text-brand-primary cursor-pointer"
              onClick={() => redirectToLogin()}
            >
              <Text className="text-xl">👤</Text>
              <Text className="text-xs mt-1">登录</Text>
            </View>
          )}
        </View>
      </View>
      {showAddressForm && (
        <AddressForm
          address={editingAddress}
          onClose={() => { setShowAddressForm(false); setEditingAddress(null) }}
          onSaved={() => loadAddresses()}
        />
      )}
    </View>
  )
}
