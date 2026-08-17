import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { ArrowLeft, Upload, X } from 'lucide-react'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, storage, uploadFile, getInputValue } from '../platform'

const BASE_URL = env.apiBaseUrl

export default function StaffInstrumentForm() {
  const navigate = useNavigate()
  const [loading, setLoading] = useState(false)
  const [categories, setCategories] = useState([])
  const [sites, setSites] = useState([])
  const [levels, setLevels] = useState([])
  const [properties, setProperties] = useState([])
  const [files, setFiles] = useState([])
  const [posterFile, setPosterFile] = useState(null)
  const [snChecking, setSnChecking] = useState(false)

  const [form, setForm] = useState({
    name: '',
    sn: '',
    category_id: '',
    site_id: '',
    level_id: '',
    description: '',
    base_daily_rate: '',
    shipping_fee: '',
    deposit: '',
    overdue_daily_fee: '',
    poster: '',
  })

  const [propValues, setPropValues] = useState({})
  const [picker, setPicker] = useState(null)

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

  const handleUploadWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: 5 - files.length, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      setFiles(prev => [...prev, ...(res.tempFilePaths || [])].slice(0, 5))
    } catch (err) {
      console.error('Failed to choose image:', err)
    }
  }

  const removeFile = (index) => {
    setFiles(prev => prev.filter((_, i) => i !== index))
  }

  const handlePosterUpload = async (e) => {
    const file = e.target.files?.[0]
    if (!file) return
    const resp = await uploadFile(`${BASE_URL}/upload`, file, {
      headers: { Authorization: storage.getItem('token') ? `Bearer ${storage.getItem('token')}` : '' },
    })
    const result = await resp.json()
    if (result?.data?.url) {
      setForm(prev => ({ ...prev, poster: result.data.url }))
    }
    setPosterFile(file)
  }

  const handlePosterUploadWeapp = async () => {
    try {
      const res = await Taro.chooseImage({ count: 1, sizeType: ['compressed'], sourceType: ['album', 'camera'] })
      const filePath = res.tempFilePaths?.[0]
      if (!filePath) return
      const resp = await uploadFile(`${BASE_URL}/upload`, filePath, {
        headers: { Authorization: storage.getItem('token') ? `Bearer ${storage.getItem('token')}` : '' },
      })
      const result = await resp.json()
      if (result?.data?.url) {
        setForm(prev => ({ ...prev, poster: result.data.url }))
      }
      setPosterFile(filePath)
    } catch (err) {
      console.error('Failed to choose poster:', err)
    }
  }

  const handleSubmit = async () => {
    if (!form.sn) { dialog.alert('请输入识别码'); return }
    if (!form.category_id) { dialog.alert('请选择分类'); return }

    setLoading(true)
    try {
      let images = []
      let fileKeys = []

      if (files.length > 0) {
        const uploaded = await Promise.all(files.map(async (file) => {
          const resp = await uploadFile(`${BASE_URL}/upload`, file, {
            headers: { Authorization: storage.getItem('token') ? `Bearer ${storage.getItem('token')}` : '' },
          })
          const result = await resp.json()
          const url = result?.data?.url || ''
          const key = result?.data?.file_key || ''
          if (key) fileKeys.push(key)
          return url
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
        name: form.name || '',
        category_id: form.category_id,
        site_id: form.site_id || undefined,
        level_id: form.level_id || undefined,
        description: form.description || undefined,
        base_daily_rate: form.base_daily_rate ? parseFloat(form.base_daily_rate) : undefined,
        images,
        poster: form.poster || undefined,
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
        // Bind uploaded media to generate display thumbnails (align with PC)
        const instrumentId = result.data?.id
        if (instrumentId && fileKeys.length > 0) {
          try {
            await apiFetch(`${BASE_URL}/instruments/${instrumentId}/media`, {
              method: 'POST',
              body: JSON.stringify({
                batch_type: 'shipping',
                is_display: true,
                files: fileKeys.map((key, i) => ({
                  file_key: key,
                  file_type: 'image',
                  sort_order: i,
                })),
              }),
            })
          } catch (e) {
            console.warn('[StaffInstrumentForm] Failed to bind media:', e)
          }
        }
        if (env.isMiniProgram) {
          Taro.navigateBack()
        } else {
          navigate('/staff/instruments')
        }
      } else {
        dialog.alert(resolveErrorMessage(result, '创建失败'))
      }
    } catch (err) {
      dialog.alert('提交失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const inputClass = 'w-full border border-gray-300 rounded-lg px-3 py-2 text-sm focus:outline-none focus:border-brand-primary'
  const labelClass = 'block text-sm font-medium text-gray-700 mb-1'

  const openPicker = (type, propName = '') => {
    let title = ''
    let options = []
    if (type === 'category') {
      title = '选择分类'
      options = categories.map(c => ({ id: c.id, label: c.name }))
    } else if (type === 'site') {
      title = '选择网点'
      options = sites.map(s => ({ id: s.id, label: s.name }))
    } else if (type === 'level') {
      title = '选择分级'
      options = levels.map(l => ({ id: l.id, label: l.caption || l.name }))
    } else if (type === 'property') {
      const prop = properties.find(p => p.name === propName)
      title = `选择${prop?.caption || propName}`
      options = (prop?.options || []).filter(o => o.status !== 'obsolete').map(o => ({ id: o.value, label: o.display_value || o.value }))
    }
    setPicker({ type, propName, title, options })
  }

  const selectOption = (id) => {
    const p = picker
    if (!p) return
    if (p.type === 'category') handleChange('category_id', id)
    else if (p.type === 'site') handleChange('site_id', id)
    else if (p.type === 'level') handleChange('level_id', id)
    else if (p.type === 'property') setPropValues(prev => ({ ...prev, [p.propName]: id }))
    setPicker(null)
  }

  const pickerLabel = (type, value) => {
    if (type === 'category') return categories.find(c => c.id === value)?.name || '请选择分类'
    if (type === 'site') return sites.find(s => s.id === value)?.name || '请选择网点'
    if (type === 'level') return levels.find(l => l.id === value)?.caption || levels.find(l => l.id === value)?.name || '请选择分级'
    return '请选择'
  }

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      {!env.isMiniProgram && (
        <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <Button onClick={() => navigate(-1)}>
            <ArrowLeft size={20} />
          </Button>
          <Text className="text-lg font-bold">新建乐器</Text>
        </View>
      )}

      <View className="p-4 space-y-4">
        <View className="bg-white rounded-xl p-4 space-y-4">
          <Text className="text-sm font-semibold text-gray-600">基本信息</Text>

          <View>
            <Text className={labelClass}>识别码 *</Text>
            <View className="relative">
              <Input className={inputClass} value={form.sn} onInput={e => handleSnChange(getInputValue(e))} placeholder="请输入识别码" />
              {snChecking && <Text className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-gray-400">检查中...</Text>}
              {!snChecking && snExists && <Text className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-red-500">已存在</Text>}
              {!snChecking && form.sn && !snExists && <Text className="absolute right-3 top-1/2 -translate-y-1/2 text-xs text-green-500">可用</Text>}
            </View>
          </View>

          <View>
            <Text className={labelClass}>乐器名</Text>
            <Input className={inputClass} value={form.name} onInput={e => setForm(prev => ({ ...prev, name: getInputValue(e) }))} placeholder="请输入乐器名称" />
          </View>

          <View>
            <Text className={labelClass}>分类 *</Text>
            <Button onClick={() => openPicker('category')} className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm text-left bg-white" style={{ margin: 0 }}>
              {form.category_id ? pickerLabel('category', form.category_id) : '请选择分类'}
            </Button>
          </View>

          <View>
            <Text className={labelClass}>所属网点</Text>
            <Button onClick={() => openPicker('site')} className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm text-left bg-white" style={{ margin: 0 }}>
              {form.site_id ? pickerLabel('site', form.site_id) : '请选择网点'}
            </Button>
          </View>

          <View>
            <Text className={labelClass}>乐器分级</Text>
            <Button onClick={() => openPicker('level')} className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm text-left bg-white" style={{ margin: 0 }}>
              {form.level_id ? pickerLabel('level', form.level_id) : '请选择分级'}
            </Button>
          </View>

          <View>
            <Text className={labelClass}>描述</Text>
            <Textarea className={`${inputClass} min-h-[72px]`} value={form.description} onInput={e => handleChange('description', getInputValue(e))} placeholder="可选描述" />
          </View>
        </View>

        <View className="bg-white rounded-xl p-4 space-y-4">
          <Text className="text-sm font-semibold text-gray-600">租金设置</Text>

          <View className="grid grid-cols-2 gap-3">
            <View>
              <Text className={labelClass}>第一阶梯日均价(¥)</Text>
              <Input className={inputClass} type="number" value={form.base_daily_rate} onInput={e => handleChange('base_daily_rate', getInputValue(e))} placeholder="0" />
            </View>
            <View>
              <Text className={labelClass}>物流费(¥)</Text>
              <Input className={inputClass} type="number" value={form.shipping_fee} onInput={e => handleChange('shipping_fee', getInputValue(e))} placeholder="0" />
            </View>
            <View>
              <Text className={labelClass}>押金(¥)</Text>
              <Input className={inputClass} type="number" value={form.deposit} onInput={e => handleChange('deposit', getInputValue(e))}
                placeholder={form.base_daily_rate ? `建议: ¥${(parseFloat(form.base_daily_rate) * 7).toFixed(0)}` : '输入押金金额'} />
            </View>
            <View>
              <Text className={labelClass}>逾期日费(¥/天)</Text>
              <Input className={inputClass} type="number" value={form.overdue_daily_fee} onInput={e => handleChange('overdue_daily_fee', getInputValue(e))} placeholder="0" />
            </View>
          </View>
        </View>

        {properties.length > 0 && (
          <View className="bg-white rounded-xl p-4 space-y-4">
            <Text className="text-sm font-semibold text-gray-600">乐器属性</Text>
            {properties.map(prop => (
              <View key={prop.id}>
                <Text className={labelClass}>{prop.caption || prop.name}</Text>
                {prop.property_type === 'select' ? (
                  <Button onClick={() => openPicker('property', prop.name)} className="w-full border border-gray-300 rounded-lg px-3 py-2 text-sm text-left bg-white" style={{ margin: 0 }}>
                    {propValues[prop.name] ? propValues[prop.name] : `请选择${prop.caption || prop.name}`}
                  </Button>
                ) : (
                  <Input className={inputClass} value={propValues[prop.name] || ''} onInput={e => setPropValues(prev => ({ ...prev, [prop.name]: getInputValue(e) }))} placeholder={`请输入${prop.caption || prop.name}`} />
                )}
              </View>
            ))}
          </View>
        )}

        <View className="bg-white rounded-xl p-4 space-y-4">
          <Text className="text-sm font-semibold text-gray-600">图片上传</Text>
          <View className="flex gap-2 flex-wrap">
            {files.map((file, i) => (
              <View key={i} className="relative w-20 h-20 rounded-lg overflow-hidden border">
                <Image src={env.isMiniProgram ? file : URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                <Button onClick={() => removeFile(i)} className="absolute top-0.5 right-0.5 bg-black/50 rounded-full p-0.5">
                  <X size={12} className="text-white" />
                </Button>
              </View>
            ))}
            {files.length < 5 && (
              env.isMiniProgram ? (
                <View className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex items-center justify-center" onClick={handleUploadWeapp}>
                  <Upload size={20} className="text-gray-400" />
                </View>
              ) : (
                <label className="w-20 h-20 border-2 border-dashed border-gray-300 rounded-lg flex items-center justify-center cursor-pointer">
                  <Upload size={20} className="text-gray-400" />
                  <input type="file" accept="image/*" className="hidden" onChange={handleUpload} />
                </label>
              )
            )}
          </View>
        </View>

        <View className="bg-white rounded-xl p-4 space-y-4">
          <Text className="text-sm font-semibold text-gray-600">海报上传</Text>
          <Text className="text-xs text-gray-400">建议宽度不超过 750px，适配手机阅读</Text>
          {posterFile ? (
            <View className="relative w-full">
              <Image src={env.isMiniProgram ? posterFile : URL.createObjectURL(posterFile)} className="w-full rounded-lg" mode="widthFix" />
              <Button onClick={() => { setPosterFile(null); setForm(prev => ({ ...prev, poster: '' })) }} className="absolute top-1 right-1 bg-black/50 rounded-full p-1">
                <X size={14} className="text-white" />
              </Button>
            </View>
          ) : (
            env.isMiniProgram ? (
              <View className="w-full h-32 border-2 border-dashed border-gray-300 rounded-lg flex items-center justify-center flex-col space-y-2" onClick={handlePosterUploadWeapp}>
                <Upload size={24} className="text-gray-400" />
                <Text className="text-sm text-gray-400">上传海报图片</Text>
              </View>
            ) : (
              <label className="w-full h-32 border-2 border-dashed border-gray-300 rounded-lg flex items-center justify-center cursor-pointer flex-col space-y-2">
                <Upload size={24} className="text-gray-400" />
                <Text className="text-sm text-gray-400">上传海报图片</Text>
                <input type="file" accept="image/*" className="hidden" onChange={handlePosterUpload} />
              </label>
            )
          )}
        </View>
      </View>

      {/* Cross-end picker modal (issue-1676): categories/sites/levels/property options */}
      {picker && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-end" onClick={() => setPicker(null)}>
          <View className="bg-white rounded-t-2xl w-full max-h-80 p-4" onClick={e => e.stopPropagation()}>
            <Text className="text-sm font-bold text-black mb-3">{picker.title}</Text>
            {picker.options.length === 0 ? (
              <View className="py-3 border-b border-gray-50">
                <Text className="text-sm text-gray-400">暂无选项</Text>
              </View>
            ) : (
              picker.options.map(opt => (
                <View key={opt.id} className="py-3 border-b border-gray-50 active:opacity-60" onClick={() => selectOption(opt.id)}>
                  <Text className="text-sm text-black">{opt.label}</Text>
                </View>
              ))
            )}
          </View>
        </View>
      )}

      <View className="fixed bottom-0 left-0 right-0 p-4 bg-white border-t">
        <Button
          onClick={handleSubmit}
          disabled={loading || snExists}
          className="w-full py-3 bg-brand-primary text-white rounded-xl font-medium disabled:opacity-50"
        >
          {loading ? '处理中...' : '创建乐器'}
        </Button>
      </View>
    </View>
  )
}
