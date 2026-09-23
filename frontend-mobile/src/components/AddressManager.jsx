import { useState, useEffect } from 'react'
import { View, Text, Button, Input, Picker } from '@tarojs/components'
import { addressesApi, resolveErrorMessage } from '../services/api'
import { env, dialog, getInputValue } from '../platform'
import regions from '../data/regions.json'

const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm'
const provinceNames = regions.map(r => r.name)

// AddressManager — 收货地址管理（#2022：从会员中心迁至个人资料）
// 自包含：列表 / 新增 / 编辑 / 删除 / 设默认，走 addressesApi。
export default function AddressManager({ user }) {
  const [addresses, setAddresses] = useState([])
  const [showForm, setShowForm] = useState(false)
  const [editingId, setEditingId] = useState(null)
  const [form, setForm] = useState({})
  const [saving, setSaving] = useState(false)

  const fetchAddresses = async () => {
    try {
      const res = await addressesApi.list()
      if (Array.isArray(res)) setAddresses(res)
      else if (res?.code === 20000) setAddresses(res.data?.list || [])
    } catch {}
  }

  useEffect(() => { fetchAddresses() }, [])

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
    if (!form.recipient_name) { dialog.alert('请填写收货人'); return }
    if (!form.phone) { dialog.alert('请填写手机号'); return }
    if (form.postal_code && !/^\d{6}$/.test(form.postal_code)) { dialog.alert('邮编格式不正确，请输入6位数字'); return }
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
        dialog.alert(resolveErrorMessage(resp, '保存失败'))
      }
    } catch (err) {
      dialog.alert('保存失败: ' + (err.message || '网络错误'))
    }
    setSaving(false)
  }

  const handleDelete = async (id) => {
    // #2022: weapp 无全局 confirm → 统一走平台 dialog.confirm（跨端）
    const ok = await dialog.confirm('确认删除此地址？')
    if (!ok) return
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
    <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-4">
      <View className="flex items-center justify-between mb-3">
        <Text className="text-sm font-bold text-zinc-800">收货地址</Text>
        {!showForm && (
          <Button onClick={openNewForm} className="text-xs text-white bg-black px-3 py-1 rounded-full font-bold">
            + 新地址
          </Button>
        )}
      </View>

      {!showForm && addresses.length > 0 && (
        <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
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

      {showForm && (
        <View style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
          <View className="grid grid-cols-2 gap-2">
            <View>
              <Text className="block text-xs font-medium text-zinc-500 mb-1">收货人</Text>
              <Input className={inputClass} value={form.recipient_name} onInput={e => setForm(p => ({ ...p, recipient_name: getInputValue(e) }))} placeholder="姓名" />
            </View>
            <View>
              <Text className="block text-xs font-medium text-zinc-500 mb-1">电话</Text>
              <Input className={inputClass} type="number" value={form.phone} onInput={e => setForm(p => ({ ...p, phone: getInputValue(e) }))} placeholder="手机号" />
            </View>
          </View>
          <View className={`grid gap-2 ${(() => { const prov = regions.find(r => r.name === form.province); if (!prov) return 'grid-cols-2'; const city = prov.children.find(c => c.name === form.city); return (city && city.children && city.children.length > 0) ? 'grid-cols-3' : 'grid-cols-2' })()}`}>
            {env.isMiniProgram ? (
              <Picker mode="selector" range={provinceNames} value={form.province ? Math.max(provinceNames.indexOf(form.province), 0) : 0}
                onChange={e => { const v = provinceNames[e.detail.value]; setForm(p => ({ ...p, province: v, city: '', district: '' })) }}>
                <View className={`${inputClass} ${form.province ? '' : 'text-gray-400'}`}>{form.province || '省'}</View>
              </Picker>
            ) : (
              <select className={inputClass} value={form.province} onChange={e => setForm(p => ({ ...p, province: e.target.value, city: '', district: '' }))}>
                <option value="">省</option>
                {regions.map((r, i) => <option key={i} value={r.name}>{r.name}</option>)}
              </select>
            )}
            {(() => {
              const prov = regions.find(r => r.name === form.province)
              const cityNames = prov ? prov.children.map(c => c.name) : []
              if (cityNames.length === 0) {
                return <View className={`${inputClass} text-gray-400`}>市</View>
              }
              if (!env.isMiniProgram) {
                return (
                  <select className={inputClass} value={form.city} onChange={e => setForm(p => ({ ...p, city: e.target.value, district: '' }))}>
                    <option value="">市</option>
                    {cityNames.map((c, i) => <option key={i} value={c}>{c}</option>)}
                  </select>
                )
              }
              return (
                <Picker mode="selector" range={cityNames} value={form.city ? Math.max(cityNames.indexOf(form.city), 0) : 0}
                  onChange={e => { const v = cityNames[e.detail.value]; setForm(p => ({ ...p, city: v, district: '' })) }}>
                  <View className={`${inputClass} ${form.city ? '' : 'text-gray-400'}`}>{form.city || '市'}</View>
                </Picker>
              )
            })()}
            {(() => {
              const prov = regions.find(r => r.name === form.province)
              if (!prov) return null
              const city = prov.children.find(c => c.name === form.city)
              const districts = city ? city.children || [] : []
              if (districts.length === 0) return null
              const districtNames = districts.map(d => d.name)
              if (!env.isMiniProgram) {
                return (
                  <select className={inputClass} value={form.district} onChange={e => setForm(p => ({ ...p, district: e.target.value }))}>
                    <option value="">区</option>
                    {districts.map((d, i) => <option key={i} value={d.name}>{d.name}</option>)}
                  </select>
                )
              }
              return (
                <Picker mode="selector" range={districtNames} value={form.district ? Math.max(districtNames.indexOf(form.district), 0) : 0}
                  onChange={e => { const v = districtNames[e.detail.value]; setForm(p => ({ ...p, district: v })) }}>
                  <View className={`${inputClass} ${form.district ? '' : 'text-gray-400'}`}>{form.district || '区'}</View>
                </Picker>
              )
            })()}
          </View>
          <Input className={inputClass} value={form.detail} onInput={e => setForm(p => ({ ...p, detail: getInputValue(e) }))} placeholder="详细地址" />
          <Input className={inputClass} type="number" value={form.postal_code} onInput={e => setForm(p => ({ ...p, postal_code: getInputValue(e) }))} placeholder="邮编" />
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

      {!showForm && addresses.length === 0 && (
        <Text className="text-xs text-zinc-400 text-center py-4">暂无地址，点击上方"+ 新地址"添加</Text>
      )}
    </View>
  )
}
