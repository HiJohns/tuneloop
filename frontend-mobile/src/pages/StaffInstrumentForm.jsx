import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { ArrowLeft, Upload, X } from 'lucide-react'
import { apiFetch } from '../services/api'
import { dialog, env, storage, uploadFile } from '../platform'

const BASE_URL = env.apiBaseUrl

export default function StaffInstrumentForm() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [categories, setCategories] = useState([])
  const [sites, setSites] = useState([])
  const [levels, setLevels] = useState([])
  const [properties, setProperties] = useState([])
  const [files, setFiles] = useState([])
  const [snChecking, setSnChecking] = useState(false)

  const [form, setForm] = useState({
    sn: '',
    category_id: '',
    site_id: '',
    level_id: '',
    description: '',
    base_daily_rate: '',
    shipping_fee: '',
    deposit: '',
    overdue_daily_fee: '',
  })

  const [propValues, setPropValues] = useState({})

  useEffect(() => {
    const loadData = async () => {
      try {
        const [catRes, siteRes, levelRes, propRes, userRes] = await Promise.all([
          apiFetch(`${BASE_URL}/categories`),
          apiFetch(`${BASE_URL}/common/sites`),
          apiFetch(`${BASE_URL}/instruments/levels`),
          apiFetch(`${BASE_URL}/properties`),
          apiFetch(`${BASE_URL}/users/me`),
        ])
        const catData = await catRes.json()
        const siteData = await siteRes.json()
        const levelData = await levelRes.json()
        const propData = await propRes.json()
        setCategories(catData?.data?.list || [])
        setSites(siteData?.data?.list || [])
        setLevels(Array.isArray(levelData?.data) ? levelData.data : [])
        setProperties(Array.isArray(propData?.data) ? propData.data : [])
        const userData = await userRes.json()
        const userSiteId = userData?.data?.site_id
        if (userSiteId) {
          setForm(prev => ({ ...prev, site_id: userSiteId }))
        }
      } catch (err) {
        console.error('Failed to load form data:', err)
      }
    }
    loadData()
  }, [])

  const handleChange = (field, value) => {
    setForm(prev => ({ ...prev, [field]: value }))
  }

  const [snTimer, setSnTimer] = useState(null)
  const [snExists, setSnExists] = useState(false)

  const handleSnChange = useCallback((value) => {
    setForm(prev => ({ ...prev, sn: value }))
    if (snTimer) clearTimeout(snTimer)
    if (!value.trim()) { setSnExists(false); return }
    const timer = setTimeout(async () => {
      setSnChecking(true)
      try {
        const resp = await apiFetch(`${BASE_URL}/instruments/check?sn=${encodeURIComponent(value.trim())}`)
        const result = await resp.json()
        setSnExists(result.code === 20000 && result.data?.exists)
      } catch { setSnExists(false) }
      setSnChecking(false)
    }, 800)
    setSnTimer(timer)
  }, [snTimer])

  const handleUpload = (e) => {
    const newFiles = Array.from(e.target.files || [])
    setFiles(prev => [...prev, ...newFiles].slice(0, 5))
  }

  const removeFile = (index) => {
    setFiles(prev => prev.filter((_, i) => i !== index))
  }

  const handleSubmit = async () => {
    if (!form.sn) { dialog.alert('请输入识别码'); return }
    if (!form.category_id) { dialog.alert('请选择分类'); return }

    setLoading(true)
    try {
      let images = []

      if (files.length > 0) {
        const uploaded = await Promise.all(files.map(async (file) => {
          const resp = await uploadFile(`${BASE_URL}/upload`, file, {
            headers: { Authorization: storage.getItem('token') ? `Bearer ${storage.getItem('token')}` : '' },
          })
          const result = await resp.json()
          return result?.data?.url || ''
        }))
        images = uploaded.filter(Boolean)
      }

      const pricing = {}
      if (form.base_daily_rate) pricing.daily_rent = parseFloat(form.base_daily_rate)
      if (form.deposit) pricing.deposit = parseFloat(form.deposit)
      if (form.shipping_fee) pricing.shipping_fee = parseFloat(form.shipping_fee)
      if (form.overdue_daily_fee) pricing.overdue_daily_fee = parseFloat(form.overdue_daily_fee)

      const body = {
        sn: form.sn,
        category_id: form.category_id,
        site_id: form.site_id || undefined,
        level_id: form.level_id || undefined,
        description: form.description || undefined,
        base_daily_rate: form.base_daily_rate ? parseFloat(form.base_daily_rate) : undefined,
        images,
        pricing: Object.keys(pricing).length > 0 ? pricing : undefined,
        properties: Object.keys(propValues).length > 0 ? propValues : undefined,
      }

      const resp = await apiFetch(`${BASE_URL}/instruments`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      })
      const result = await resp.json()
      if (result.code === 20000 || result.code === 20100) {
        navigate('/staff/instruments')
      } else {
        dialog.alert(result.message || '创建失败')
      }
    } catch (err) {
      dialog.alert('提交失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  return (
    <div className="min-h-screen bg-gray-50 pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">新建乐器</h1>
      </div>

      <div className="p-4 space-y-4">
        <div className="bg-white rounded-xl p-4 space-y-4">
          <h2 className="text-sm font-semibold text-gray-600">基本信息</h2>

          <div>
            <label className={labelClass}>识别码 *</label>
            <div className="relative">
              <input className={inputClass} value={form.sn} onChange={e => handleSnChange(e.target.value)} placeholder="请输入识别码" />
              {snChecking && <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-400">检查中...</span>}
              {!snChecking && snExists && <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-red-500">已存在</span>}
              {!snChecking && form.sn && !snExists && <span className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-green-500">可用</span>}
            </div>
          </div>

          <div>
            <label className={labelClass}>分类 *</label>
            <select className={inputClass} value={form.category_id} onChange={e => handleChange('category_id', e.target.value)}>
              <option value="">请选择分类</option>
              {categories.map(cat => (
                <option key={cat.id} value={cat.id}>{cat.name}</option>
              ))}
            </select>
          </div>

          <div>
            <label className={labelClass}>所属网点</label>
            <select className={inputClass} value={form.site_id} onChange={e => handleChange('site_id', e.target.value)}>
              <option value="">请选择网点</option>
              {sites.map(site => (
                <option key={site.id} value={site.id}>{site.name}</option>
              ))}
            </select>
          </div>

          <div>
            <label className={labelClass}>乐器分级</label>
            <select className={inputClass} value={form.level_id} onChange={e => handleChange('level_id', e.target.value)}>
              <option value="">请选择分级</option>
              {levels.map(lv => (
                <option key={lv.id} value={lv.id}>{lv.caption || lv.name}</option>
              ))}
            </select>
          </div>

          <div>
            <label className={labelClass}>描述</label>
            <textarea className={inputClass} rows={3} value={form.description} onChange={e => handleChange('description', e.target.value)} placeholder="可选描述" />
          </div>
        </div>

        <div className="bg-white rounded-xl p-4 space-y-4">
          <h2 className="text-sm font-semibold text-gray-600">租金设置</h2>

          <div className="grid grid-cols-2 gap-3">
            <div>
              <label className={labelClass}>第一阶梯日均价(¥)</label>
              <input className={inputClass} type="number" min="0" step="1" value={form.base_daily_rate} onChange={e => handleChange('base_daily_rate', e.target.value)} placeholder="0" />
            </div>
            <div>
              <label className={labelClass}>物流费(¥)</label>
              <input className={inputClass} type="number" min="0" step="1" value={form.shipping_fee} onChange={e => handleChange('shipping_fee', e.target.value)} placeholder="0" />
            </div>
            <div>
              <label className={labelClass}>押金(¥)</label>
              <input className={inputClass} type="number" min="0" step="1" value={form.deposit} onChange={e => handleChange('deposit', e.target.value)} placeholder="0" />
            </div>
            <div>
              <label className={labelClass}>逾期日费(¥/天)</label>
              <input className={inputClass} type="number" min="0" step="1" value={form.overdue_daily_fee} onChange={e => handleChange('overdue_daily_fee', e.target.value)} placeholder="0" />
            </div>
          </div>
        </div>

        {properties.length > 0 && (
          <div className="bg-white rounded-xl p-4 space-y-4">
            <h2 className="text-sm font-semibold text-gray-600">乐器属性</h2>
            {properties.map(prop => (
              <div key={prop.id}>
                <label className={labelClass}>{prop.caption || prop.name}</label>
                {prop.property_type === 'select' ? (
                  <select className={inputClass} value={propValues[prop.name] || ''} onChange={e => setPropValues(prev => ({ ...prev, [prop.name]: e.target.value }))}>
                    <option value="">请选择{prop.caption || prop.name}</option>
                    {(prop.options || []).filter(o => o.status !== 'obsolete').map(opt => (
                      <option key={opt.id} value={opt.value}>{opt.display_value || opt.value}</option>
                    ))}
                  </select>
                ) : (
                  <input className={inputClass} value={propValues[prop.name] || ''} onChange={e => setPropValues(prev => ({ ...prev, [prop.name]: e.target.value }))} placeholder={`请输入${prop.caption || prop.name}`} />
                )}
              </div>
            ))}
          </div>
        )}

        <div className="bg-white rounded-xl p-4 space-y-4">
          <h2 className="text-sm font-semibold text-gray-600">图片上传</h2>
          <div className="flex gap-2 flex-wrap">
            {files.map((file, i) => (
              <div key={i} className="relative w-20 h-20 rounded-lg overflow-hidden border">
                <img src={URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                <button onClick={() => removeFile(i)} className="absolute top-0.5 right-0.5 bg-black/50 rounded-full p-0.5">
                  <X size={12} className="text-white" />
                </button>
              </div>
            ))}
            {files.length < 5 && (
              <label className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex items-center justify-center cursor-pointer">
                <Upload size={20} className="text-gray-400" />
                <input type="file" accept="image/*" className="hidden" onChange={handleUpload} />
              </label>
            )}
          </div>
        </div>
      </div>

      <div className="fixed bottom-0 left-0 right-0 p-4 bg-white border-t">
        <button
          onClick={handleSubmit}
          disabled={loading || snExists}
          className="w-full py-3 bg-brand-primary text-white rounded-xl font-medium disabled:opacity-50"
        >
          {loading ? '提交中...' : '创建乐器'}
        </button>
      </div>
    </div>
  )
}
