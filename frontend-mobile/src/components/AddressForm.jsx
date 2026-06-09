import { useState } from 'react'
import { addressesApi } from '../services/api'
import { X } from 'lucide-react'

export default function AddressForm({ address, onClose, onSaved }) {
  const [form, setForm] = useState({
    recipient_name: address?.recipient_name || '',
    phone: address?.phone || '',
    province: address?.province || '',
    city: address?.city || '',
    district: address?.district || '',
    detail: address?.detail || '',
    is_default: address?.is_default || false,
  })
  const [saving, setSaving] = useState(false)

  const handleSubmit = async () => {
    if (!form.recipient_name) { alert('请填写收货人'); return }
    if (!form.phone) { alert('请填写手机号'); return }
    setSaving(true)
    try {
      let resp
      if (address?.id) {
        resp = await addressesApi.update(address.id, form)
      } else {
        resp = await addressesApi.create(form)
      }
      if (resp.code === 20000 || resp.code === 20100) {
        if (onSaved) onSaved()
        onClose()
      } else {
        alert(resp.message || '保存失败')
      }
    } catch (err) {
      alert('保存失败: ' + (err.message || '网络错误'))
    }
    setSaving(false)
  }

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  return (
    <div className="fixed inset-0 bg-black/50 z-50 flex items-end sm:items-center justify-center" onClick={onClose}>
      <div className="bg-white rounded-t-2xl sm:rounded-2xl w-full max-w-md p-5 max-h-[90vh] overflow-y-auto" onClick={e => e.stopPropagation()}>
        <div className="flex items-center justify-between mb-4">
          <h3 className="font-medium text-lg">{address ? '编辑地址' : '新建地址'}</h3>
          <button onClick={onClose} className="p-1"><X size={20} /></button>
        </div>

        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-2">
            <div>
              <label className={labelClass}>收货人</label>
              <input className={inputClass} value={form.recipient_name} onChange={e => setForm(prev => ({ ...prev, recipient_name: e.target.value }))} placeholder="姓名" />
            </div>
            <div>
              <label className={labelClass}>电话</label>
              <input className={inputClass} value={form.phone} onChange={e => setForm(prev => ({ ...prev, phone: e.target.value }))} placeholder="手机号" />
            </div>
          </div>
          <div className="grid grid-cols-3 gap-2">
            <input className={inputClass} value={form.province} onChange={e => setForm(prev => ({ ...prev, province: e.target.value }))} placeholder="省" />
            <input className={inputClass} value={form.city} onChange={e => setForm(prev => ({ ...prev, city: e.target.value }))} placeholder="市" />
            <input className={inputClass} value={form.district} onChange={e => setForm(prev => ({ ...prev, district: e.target.value }))} placeholder="区" />
          </div>
          <input className={inputClass} value={form.detail} onChange={e => setForm(prev => ({ ...prev, detail: e.target.value }))} placeholder="详细地址" />
          <label className="flex items-center gap-2 text-sm text-gray-500 cursor-pointer">
            <input type="checkbox" checked={form.is_default} onChange={e => setForm(prev => ({ ...prev, is_default: e.target.checked }))} />
            设为默认地址
          </label>
          <button
            onClick={handleSubmit}
            disabled={saving}
            className="w-full py-3 bg-brand-primary text-white rounded-lg font-medium disabled:opacity-50"
          >
            {saving ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}
