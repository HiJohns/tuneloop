import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView, Button } from '@tarojs/components'
import { apiFetch, addressesApi } from '../services/api'
import { env } from '../platform'
import regions from '../data/regions.json'

const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm'

export default function MembershipCenter() {
  const navigate = useNavigate()
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  // Address state
  const [addresses, setAddresses] = useState([])
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState({ recipient_name: '', phone: '', province: '', city: '', district: '', detail: '', postal_code: '' })
  const [saving, setSaving] = useState(false)

  const fetchUser = async () => {
    try {
      const resp = await apiFetch(`${baseUrl}/users/me`)
      const result = await resp.json()
      if (result.code === 20000) {
        setUser(result.data)
        setForm(prev => ({ ...prev, recipient_name: result.data?.name || '', phone: result.data?.phone || '' }))
      }
    } catch {}
    setLoading(false)
  }

  const fetchAddresses = async () => {
    try {
      const res = await addressesApi.list()
      if (res.code === 20000) setAddresses(res.data?.list || [])
    } catch {}
  }

  useEffect(() => { fetchUser(); fetchAddresses() }, [])

  const openNewForm = () => {
    setEditingId(null)
    setForm({ recipient_name: user?.name || '', phone: user?.phone || '', province: '', city: '', district: '', detail: '', postal_code: '' })
    setShowForm(true)
  }

  const openEditForm = (addr) => {
    setEditingId(addr.id)
    setForm({ recipient_name: addr.recipient_name, phone: addr.phone, province: addr.province, city: addr.city, district: addr.district, detail: addr.detail, postal_code: addr.postal_code || '' })
    setShowForm(true)
  }

  const handleSave = async () => {
    if (!form.recipient_name) { alert('请填写收货人'); return }
    if (!form.phone) { alert('请填写手机号'); return }
    setSaving(true)
    try {
      let resp
      if (editingId) {
        resp = await addressesApi.update(editingId, form)
      } else {
        resp = await addressesApi.create(form)
      }
      if (resp.code === 20000 || resp.code === 20100) {
        await fetchAddresses()
        setShowForm(false)
        setEditingId(null)
      } else {
        alert(resp.message || '保存失败')
      }
    } catch (err) {
      alert('保存失败: ' + (err.message || '网络错误'))
    }
    setSaving(false)
  }

  const handleDelete = async (id) => {
    if (!confirm('确认删除此地址？')) return
    try {
      const res = await addressesApi.delete(id)
      if (res.code === 20000) fetchAddresses()
    } catch {}
  }

  const handleSetDefault = async (id) => {
    try {
      const res = await addressesApi.setDefault(id)
      if (res.code === 20000) fetchAddresses()
    } catch {}
  }

  return (
    <ScrollView className="h-screen w-screen bg-zinc-50">
      {/* Navigation bar */}
      <View className="flex items-center px-4 py-3 bg-white border-b border-zinc-100">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center mr-4">会员中心</Text>
      </View>

      {/* Membership level card */}
      <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-6">
        <View className="items-center">
          <Text className="text-2xl font-bold text-amber-700">
            {user?.membership_level_name || '普通会员'}
          </Text>
        </View>
      </View>

      {/* Stats cards */}
      <View className="mx-4 mt-4">
        <View className="bg-white rounded-2xl shadow-sm p-4">
          <View className="flex justify-between items-center py-3 border-b border-zinc-50">
            <Text className="text-sm text-zinc-500">消费总额</Text>
            <Text className="text-base font-bold text-zinc-800">
              ¥{user?.total_spending ? Number(user.total_spending).toLocaleString() : '0'}
            </Text>
          </View>
          <View className="flex justify-between items-center py-3 border-b border-zinc-50">
            <Text className="text-sm text-zinc-500">预付点数</Text>
            <Text className="text-base font-bold text-zinc-800">
              {user?.prepaid_points ? Number(user.prepaid_points).toLocaleString() : '0'} 点
            </Text>
          </View>
          <View className="flex justify-between items-center py-3">
            <Text className="text-sm text-zinc-500">赠点数</Text>
            <Text className="text-base font-bold text-zinc-800">
              {user?.promo_points ? Number(user.promo_points).toLocaleString() : '0'} 点
            </Text>
          </View>
        </View>
      </View>

      {/* Address management section */}
      <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-4">
        <View className="flex items-center justify-between mb-3">
          <Text className="text-sm font-bold text-zinc-800">收货地址</Text>
          {!showForm && (
            <Button onClick={openNewForm} className="text-xs text-white bg-black px-3 py-1 rounded-full font-bold">
              + 新地址
            </Button>
          )}
        </View>

        {/* Address list (when not showing form) */}
        {!showForm && addresses.length > 0 && (
          <View className="space-y-2">
            {addresses.map(addr => (
              <View key={addr.id} className="border border-zinc-100 rounded-xl p-3">
                <View className="flex items-center gap-2 mb-1">
                  <Text className="text-sm font-bold text-black">{addr.recipient_name}</Text>
                  <Text className="text-xs text-zinc-400">{addr.phone}</Text>
                  {addr.is_default && <Text className="text-xs text-white bg-red-500 px-1.5 py-0.5 rounded">默认</Text>}
                </View>
                <Text className="text-xs text-zinc-500">{addr.province} {addr.city} {addr.district} {addr.detail}</Text>
                <View className="flex gap-2 mt-2 pt-2 border-t border-zinc-50">
                  <Button onClick={() => openEditForm(addr)} className="flex-1 py-1.5 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">编辑</Button>
                  {!addr.is_default && (
                    <Button onClick={() => handleSetDefault(addr.id)} className="flex-1 py-1.5 bg-zinc-100 rounded-lg text-xs font-bold text-zinc-600">设默认</Button>
                  )}
                  <Button onClick={() => handleDelete(addr.id)} className="flex-1 py-1.5 bg-red-50 rounded-lg text-xs font-bold text-red-500">删除</Button>
                </View>
              </View>
            ))}
          </View>
        )}

        {/* Address form */}
        {showForm && (
          <View className="space-y-3">
            <View className="grid grid-cols-2 gap-2">
              <View>
                <Text className="block text-xs font-medium text-zinc-500 mb-1">收货人</Text>
                <input className={inputClass} value={form.recipient_name} onChange={e => setForm(p => ({ ...p, recipient_name: e.target.value }))} placeholder="姓名" />
              </View>
              <View>
                <Text className="block text-xs font-medium text-zinc-500 mb-1">电话</Text>
                <input className={inputClass} value={form.phone} onChange={e => setForm(p => ({ ...p, phone: e.target.value }))} placeholder="手机号" />
              </View>
            </View>
            <View className="grid grid-cols-3 gap-2">
              <select className={inputClass} value={form.province} onChange={e => setForm(p => ({ ...p, province: e.target.value, city: '', district: '' }))}>
                <option value="">省</option>
                {regions.map((r, i) => <option key={i} value={r.name}>{r.name}</option>)}
              </select>
              <select className={inputClass} value={form.city} onChange={e => setForm(p => ({ ...p, city: e.target.value, district: '' }))}>
                <option value="">市</option>
                {(() => {
                  const prov = regions.find(r => r.name === form.province)
                  return prov ? prov.children.map((c, i) => <option key={i} value={c.name}>{c.name}</option>) : null
                })()}
              </select>
              <select className={inputClass} value={form.district} onChange={e => setForm(p => ({ ...p, district: e.target.value }))}>
                <option value="">区</option>
                {(() => {
                  const prov = regions.find(r => r.name === form.province)
                  if (!prov) return null
                  const city = prov.children.find(c => c.name === form.city)
                  return city ? city.children.map((d, i) => <option key={i} value={d.name}>{d.name}</option>) : null
                })()}
              </select>
            </View>
            <input className={inputClass} value={form.detail} onChange={e => setForm(p => ({ ...p, detail: e.target.value }))} placeholder="详细地址" />
            <input className={inputClass} value={form.postal_code} onChange={e => setForm(p => ({ ...p, postal_code: e.target.value }))} placeholder="邮编" />
            <View className="flex gap-2">
              <Button onClick={handleSave} disabled={saving} className="flex-1 py-2.5 bg-black text-white rounded-xl font-bold text-sm">
                {saving ? '保存中...' : editingId ? '保存修改' : '新增地址'}
              </Button>
              <Button onClick={() => { setShowForm(false); setEditingId(null) }} className="px-4 py-2.5 bg-zinc-100 rounded-xl font-bold text-sm text-zinc-600">
                取消
              </Button>
            </View>
          </View>
        )}

        {/* Empty state */}
        {!showForm && addresses.length === 0 && (
          <Text className="text-xs text-zinc-400 text-center py-4">暂无地址，点击上方"+ 新地址"添加</Text>
        )}
      </View>

      {loading && (
        <View className="flex-1 items-center justify-center mt-20">
          <Text className="text-zinc-400">加载中...</Text>
        </View>
      )}
    </ScrollView>
  )
}
