import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { message } from 'antd'
import { api, apiFetch, getToken, redirectToLogin, addressesApi, resendEmailConfirmation } from '../services/api'
import { User, MapPin, Bell, ChevronRight, LogOut, Edit3, Key, Package, History, Clock, FileText, ClipboardList, Plus, Trash2, CheckCircle, Send, AlertCircle } from 'lucide-react'
import AddressForm from '../components/AddressForm'

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
      const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
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
    <div className="fixed inset-0 bg-black/50 z-50 flex items-end justify-center">
      <div className="bg-white rounded-t-2xl w-full max-w-[480px] p-6">
        <h3 className="text-lg font-bold mb-4">Edit Profile</h3>
        <div className="space-y-4">
          <div>
            <label className="text-sm text-gray-500">Name</label>
            <input
              type="text"
              value={form.name}
              onChange={e => setForm({ ...form, name: e.target.value })}
              className="w-full border rounded-lg px-3 py-2 mt-1"
              placeholder="Enter your name"
            />
          </div>
          <div>
            <label className="text-sm text-gray-500">Phone</label>
            <input
              type="text"
              value={form.phone}
              onChange={e => setForm({ ...form, phone: e.target.value })}
              className="w-full border rounded-lg px-3 py-2 mt-1"
              placeholder="Enter phone number"
            />
          </div>
          <div>
            <label className="text-sm text-gray-500">Email</label>
            <input
              type="email"
              value={form.email}
              onChange={e => setForm({ ...form, email: e.target.value })}
              className="w-full border rounded-lg px-3 py-2 mt-1"
              placeholder="Enter email"
            />
          </div>
          {message && (
            <div className="text-sm text-brand-primary bg-blue-50 p-2 rounded">{message}</div>
          )}
        </div>
        <div className="flex gap-3 mt-6">
          <button onClick={onClose} className="flex-1 py-2 border rounded-lg text-gray-600">
            Cancel
          </button>
          <button
            onClick={handleSave}
            disabled={saving}
            className="flex-1 py-2 bg-brand-primary text-white rounded-lg disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save'}
          </button>
        </div>
      </div>
    </div>
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

  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) {
          setUser(result.data)
        }
      } catch (err) {
        console.error('Failed to fetch user:', err)
      }
      setLoading(false)
    }
    fetchUser()
  }, [])

  const loadAddresses = async () => {
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
    localStorage.removeItem('token')
    localStorage.removeItem('token_expiry')
    localStorage.removeItem('user_info')
    localStorage.removeItem('user_sys_perm')
    localStorage.removeItem('user_cus_perm')
    localStorage.removeItem('user_cus_perm_ext')
    sessionStorage.removeItem('token')
    document.cookie = 'token=; expires=Thu, 01 Jan 1970 00:00:00 UTC; path=/;'
    navigate('/')
  }

  const handlePasswordSetup = () => {
    const iamUrl = window.APP_CONFIG?.wx?.iamExternalUrl ||
                   import.meta.env.VITE_BEACONIAM_EXTERNAL_URL || ''
    if (iamUrl) {
      window.open(`${iamUrl}/auth/setup-password`, '_blank')
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
      <div className="min-h-screen bg-brand-bg flex items-center justify-center">
        <div className="text-gray-500">Loading...</div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-6">
        <div className="flex items-center gap-4">
          <div className="w-16 h-16 bg-white/20 rounded-full flex items-center justify-center">
            <User size={32} />
          </div>
          <div className="flex-1">
            <h1 className="text-lg font-bold">{user?.name || 'User'}</h1>
            <p className="text-sm opacity-90">{user?.phone || user?.email || ''}</p>
          </div>
          <button onClick={handleLogout} className="text-white/80 hover:text-white">
            <LogOut size={20} />
          </button>
        </div>
      </div>

      <div className="p-4 space-y-3">
        {/* Personal Info - localized */}
        <div className="bg-white rounded-xl p-4">
          <div className="flex justify-between items-center mb-3">
            <h3 className="font-medium">个人信息</h3>
            <button onClick={() => setShowEdit(true)} className="text-brand-primary">
              <Edit3 size={18} />
            </button>
          </div>
          <div className="space-y-2 text-sm text-gray-600">
            <div className="flex justify-between"><span>姓名</span><span>{user?.name || '-'}</span></div>
            <div className="flex justify-between"><span>电话</span><span>{user?.phone || '-'}</span></div>
            <div className="flex justify-between items-start">
              <span>邮箱</span>
              <div className="text-right">
                <span>{user?.email || '-'}</span>
                {user?.email && (() => {
                  const sent = user.email_sent_at ? new Date(user.email_sent_at) : null
                  const confirmed = user.email_confirmed_at ? new Date(user.email_confirmed_at) : null
                  const isConfirmed = confirmed && sent && confirmed > sent
                  const expired = sent && (Date.now() - sent.getTime() > 24 * 60 * 60 * 1000)

                  if (isConfirmed) {
                    return <p className="text-xs text-green-600 flex items-center gap-1 mt-0.5 justify-end"><CheckCircle size={12} /> 已确认</p>
                  } else if (sent && !expired) {
                    return <p className="text-xs text-orange-500 mt-0.5">确认邮件已发送，请检查邮箱及垃圾箱</p>
                  } else if (sent && expired) {
                    return (
                      <button
                        onClick={async () => {
                          try {
                            await resendEmailConfirmation()
                            message.success('确认邮件已重新发送')
                          } catch { message.error('重发失败，请稍后重试') }
                        }}
                        className="text-xs text-red-500 underline mt-0.5"
                      >
                        确认邮件已失效，点击重发
                      </button>
                    )
                  }
                  return null
                })()}
              </div>
            </div>
          </div>
        </div>

        {/* 设置密码 */}
        <div className="bg-white rounded-xl p-4">
          <button
            onClick={handlePasswordSetup}
            className="flex items-center gap-3 w-full py-2"
          >
            <Key size={18} className="text-gray-400" />
            <span className="flex-1 text-left text-sm">设置密码</span>
            <ChevronRight size={16} className="text-gray-300" />
          </button>
        </div>

        {/* 收货地址 — 仅对顾客显示 */}
        {businessRole !== 'site_admin' && businessRole !== 'site_member' && (
          <div className="bg-white rounded-xl p-4">
            <div className="flex justify-between items-center mb-3">
              <h3 className="font-medium flex items-center gap-2">
                <MapPin size={18} className="text-brand-primary" />
                收货地址
              </h3>
              <button onClick={() => { setEditingAddress(null); setShowAddressForm(true) }} className="text-brand-primary">
                <Plus size={18} />
              </button>
            </div>
            {addresses.length === 0 ? (
              <p className="text-sm text-red-500">请设置默认收货地址</p>
            ) : (
              <div className="space-y-2">
                {addresses.filter(a => a.is_default).slice(0, 1).map(addr => (
                  <div key={addr.id} className="text-sm text-gray-600">
                    <p className="font-medium">{addr.recipient_name} · {addr.phone}</p>
                    <p className="text-xs text-gray-400">{addr.province}{addr.city}{addr.district}{addr.detail}{addr.postal_code ? ` ${addr.postal_code}` : ''}</p>
                    <div className="flex items-center gap-2 mt-1">
                      <span className="text-xs bg-green-100 text-green-700 px-1.5 py-0.5 rounded">默认</span>
                      <button onClick={() => { setEditingAddress(addr); setShowAddressForm(true) }} className="text-xs text-brand-primary">编辑</button>
                      <button onClick={async () => { if (confirm('确认删除？')) { await addressesApi.delete(addr.id); loadAddresses() } }} className="text-xs text-red-500">删除</button>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </div>
        )}

        {/* Current Rentals */}
        {businessRole !== 'site_admin' && businessRole !== 'site_member' && activeLeases.length > 0 && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-3 flex items-center gap-2">
              <Package size={18} className="text-brand-primary" />
              当前租赁
            </h3>
            <div className="space-y-3">
              {activeLeases.map(order => (
                <div
                  key={order.id}
                  className="border rounded-lg p-3 cursor-pointer"
                  onClick={() => navigate(`/order/${order.id}`)}
                >
                  <div className="flex justify-between items-start mb-2">
                    <span className="font-medium text-sm">Order #{order.id?.slice(0, 8)}</span>
                    <span className={`text-xs px-2 py-1 rounded-full ${statusColor[order.status] || 'bg-gray-100'}`}>
                      {statusLabel[order.status] || order.status}
                    </span>
                  </div>
                  <div className="text-xs text-gray-500 space-y-1">
                    {order.start_date && <p>开始: {order.start_date}</p>}
                    {order.end_date && <p>结束: {order.end_date}</p>}
                    {isOverdue(order) && (
                      <p className="text-red-500 font-medium">
                        超期 {overdueDays(order)} 天 · 超期费 ¥{((order.monthly_rent || 0) / 30 * overdueDays(order)).toFixed(0)}
                      </p>
                    )}
                    {(order.status === 'in_transit' || order.status === 'shipped') && order.tracking_number && (
                      <p>物流: {order.courier_company || ''} {order.tracking_number}</p>
                    )}
                    {order.status === 'returning' && order.tracking_number && (
                      <p>归还物流: {order.courier_company || ''} {order.tracking_number}</p>
                    )}
                    <p>月租: ¥{order.monthly_rent} · 押金: ¥{order.deposit}</p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Rental History */}
        {businessRole !== 'site_admin' && businessRole !== 'site_member' && leaseHistory.length > 0 && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-3 flex items-center gap-2">
              <History size={18} className="text-gray-400" />
              租赁历史
            </h3>
            <div className="space-y-2">
              {leaseHistory
                .sort((a, b) => new Date(b.end_date || b.updated_at) - new Date(a.end_date || a.updated_at))
                .map(order => (
                <div
                  key={order.id}
                  className="border rounded-lg p-3 cursor-pointer"
                  onClick={() => navigate(`/order/${order.id}`)}
                >
                  <div className="flex justify-between items-start mb-1">
                    <span className="text-sm font-medium">Order #{order.id?.slice(0, 8)}</span>
                    <span className="text-xs text-gray-500">{statusLabel[order.status]}</span>
                  </div>
                  <div className="text-xs text-gray-500 space-y-1">
                    {order.start_date && <p>开始: {order.start_date}</p>}
                    {order.end_date && <p>归还: {order.end_date}</p>}
                    <p>月租: ¥{order.monthly_rent} · 押金: ¥{order.deposit}</p>
                    {(order.status === 'returned' || order.status === 'completed') && (
                      <p className={order.deposit_refunded ? 'text-green-600' : 'text-yellow-600'}>
                        {order.deposit_refunded ? '押金已退还' : '押金退还中'}
                      </p>
                    )}
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* My Account (customer) or Staff Functions */}
        {(businessRole === 'site_admin' || businessRole === 'site_member') && (
          <div className="space-y-3">
            <div className="bg-white rounded-xl p-4">
              <h3 className="font-medium mb-3">员工功能</h3>
              <div className="grid grid-cols-2 gap-4">
                {(() => {
                  const mapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')
                  const cusPerm = parseInt(localStorage.getItem('user_cus_perm') || '0')
                  const has = (code) => { const b = mapping[code]; return b !== undefined && (cusPerm & (1 << b)) !== 0 }
                  return (
                    <>
                      {has('instrument:read') && (
                        <button onClick={() => navigate('/staff/instruments')} className="flex flex-col items-center p-2">
                          <MapPin size={24} className="text-brand-primary" />
                          <span className="text-xs mt-1 text-gray-600">乐器管理</span>
                        </button>
                      )}
                      {has('order:read') && (
                        <button onClick={() => navigate('/staff/orders')} className="flex flex-col items-center p-2">
                          <ClipboardList size={24} className="text-brand-primary" />
                          <span className="text-xs mt-1 text-gray-600">订单管理</span>
                        </button>
                      )}
                    </>
                  )
                })()}
              </div>
            </div>
          </div>
        )}
        {!(businessRole === 'site_admin' || businessRole === 'site_member') && (
          <div className="space-y-3">
            <div className="bg-white rounded-xl p-4">
              <h3 className="font-medium mb-3">My Account</h3>
              <button
                onClick={() => navigate('/my-contracts')}
                className="flex items-center gap-3 w-full py-2"
              >
                <FileText size={18} className="text-gray-400" />
                <span className="flex-1 text-left text-sm">我的合同</span>
                <ChevronRight size={16} className="text-gray-300" />
              </button>
              <button
                onClick={() => navigate('/messages')}
                className="flex items-center gap-3 w-full py-2"
              >
                <Bell size={18} className="text-gray-400" />
                <span className="flex-1 text-left text-sm">Messages</span>
                <ChevronRight size={16} className="text-gray-300" />
              </button>
            </div>
          </div>
        )}
      </div>

      <EditProfileModal
        visible={showEdit}
        user={user}
        onClose={() => setShowEdit(false)}
        onSave={(updated) => setUser({ ...user, ...updated })}
      />

      <div className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <div className="flex justify-around py-3 max-w-[480px] mx-auto">
          <div
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/')}
          >
            <span className="text-xl">🏠</span>
            <span className="text-xs mt-1">Home</span>
          </div>
          <div
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <span className="text-xl">🔧</span>
            <span className="text-xs mt-1">Service</span>
          </div>
          {getToken() ? (
            <div
              className="flex flex-col items-center text-brand-primary cursor-pointer"
              onClick={() => navigate('/profile')}
            >
              <span className="text-xl">👤</span>
              <span className="text-xs mt-1">Me</span>
            </div>
          ) : (
            <div
              className="flex flex-col items-center text-brand-primary cursor-pointer"
              onClick={() => redirectToLogin()}
            >
              <span className="text-xl">👤</span>
              <span className="text-xs mt-1">登录</span>
            </div>
          )}
        </div>
      </div>
      {showAddressForm && (
        <AddressForm
          address={editingAddress}
          onClose={() => { setShowAddressForm(false); setEditingAddress(null) }}
          onSaved={() => loadAddresses()}
        />
      )}
    </div>
  )
}
