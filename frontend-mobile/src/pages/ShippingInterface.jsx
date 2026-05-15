import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, Camera, Scan, Plus, X } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'

export default function ShippingInterface() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [snInput, setSnInput] = useState('')
  const [items, setItems] = useState([])
  const [logistics, setLogistics] = useState({ company: '', trackingNumber: '' })
  const [submitting, setSubmitting] = useState(false)

  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  // Auto-fetch instrument(s) from URL params on mount
  useEffect(() => {
    const ids = searchParams.get('instrument')
    if (ids) {
      ids.split(',').forEach(id => {
        fetchInstrumentById(id.trim())
      })
    }
  }, [])

  const fetchInstrumentById = async (instrumentId) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/${instrumentId}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data) {
        const inst = result.data
        if (inst.stock_status !== 'reserved') {
          alert(`乐器 ${inst.sn} 未处于已预约状态（当前: ${inst.stock_status}）`)
          return
        }
        const orderResp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(inst.sn)}`)
        const orderResult = await orderResp.json()
        const orderID = orderResult.code === 20000 ? orderResult.data?.order_id : null
        setItems(prev => [...prev.filter(i => i.sn !== inst.sn), {
          sn: inst.sn,
          images: inst.images,
          pricing: inst.pricing,
          category_name: inst.category_name,
          level_name: inst.level_name,
          category_id: inst.category_id,
          order_id: orderID,
          photos: [],
        }])
      }
    } catch (err) {
      console.error('Failed to fetch instrument:', err)
    }
  }

  const checkInstrument = async (sn) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/check?sn=${encodeURIComponent(sn)}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data?.exists) {
        const inst = result.data.info
        if (inst.stock_status !== 'reserved') {
          alert(`乐器 ${sn} 未处于已预约状态（当前: ${inst.stock_status}）`)
          return
        }

        const orderResp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(sn)}`)
        const orderResult = await orderResp.json()
        const orderID = orderResult.code === 20000 ? orderResult.data?.order_id : null

        setItems(prev => [...prev.filter(i => i.sn !== sn), {
          sn,
          images: inst.images,
          pricing: inst.pricing,
          category_name: inst.category_name,
          level_name: inst.level_name,
          category_id: inst.category_id,
          order_id: orderID,
          photos: [],
        }])
        setSnInput('')
      } else {
        alert('未找到该乐器')
      }
    } catch (err) {
      console.error('Failed to check instrument:', err)
    }
  }

  const updateItemPhotos = (sn, files) => {
    setItems(prev => prev.map(i => i.sn === sn ? { ...i, photos: files } : i))
  }

  const removeItem = (sn) => {
    setItems(prev => prev.filter(i => i.sn !== sn))
  }

  const handleSubmit = async () => {
    if (items.length === 0) return
    for (const item of items) {
      if (item.photos.length === 0) {
        alert(`请为乐器 ${item.sn} 拍摄或选择照片`)
        return
      }
    }

    setSubmitting(true)
    const token = localStorage.getItem('token') || sessionStorage.getItem('token')

    for (const item of items) {
      if (!item.order_id) {
        alert(`乐器 ${item.sn} 没有活跃的订单`)
        setSubmitting(false)
        return
      }

      // Upload photos for this item
      const photoUrls = []
      try {
        for (const file of item.photos) {
          const formData = new FormData()
          formData.append('file', file)
          const resp = await fetch(`${baseUrl}/upload`, {
            method: 'POST',
            headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
            body: formData,
          })
          const result = await resp.json()
          if (result.code === 20000 && result.data?.url) {
            photoUrls.push(result.data.url)
          } else {
            alert(`照片上传失败 ${item.sn}: ${result.message || '未知错误'}`)
            setSubmitting(false)
            return
          }
        }
      } catch (err) {
        alert(`照片上传失败 ${item.sn}: ${err.message}`)
        setSubmitting(false)
        return
      }

      // Submit shipping for this item
      try {
        const resp = await apiFetch(`${baseUrl}/warehouse/orders/${item.order_id}/shipping`, {
          method: 'PUT',
          body: JSON.stringify({
            tracking_number: logistics.trackingNumber,
            company: logistics.company,
            shipped_at: new Date().toISOString(),
            photos: photoUrls,
          }),
        })
        const result = await resp.json()
        if (result.code !== 20000) {
          alert(`发货失败 ${item.sn}: ${result.message}`)
          setSubmitting(false)
          return
        }
      } catch (err) {
        alert(`发货失败 ${item.sn}: ${err.message}`)
        setSubmitting(false)
        return
      }
    }

    alert('全部发货成功')
    navigate('/staff/instruments', { replace: true })
    setSubmitting(false)
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">发货</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Logistics Info */}
        <div className="bg-white rounded-xl p-4 space-y-3">
          <h3 className="font-medium">物流信息</h3>
          <input
            type="text"
            value={logistics.company}
            onChange={e => setLogistics({ ...logistics, company: e.target.value })}
            placeholder="承运公司"
            className="w-full border rounded-lg px-3 py-2"
          />
          <input
            type="text"
            value={logistics.trackingNumber}
            onChange={e => setLogistics({ ...logistics, trackingNumber: e.target.value })}
            placeholder="快递单号"
            className="w-full border rounded-lg px-3 py-2"
          />
        </div>

        {/* Items List */}
        {items.length > 0 && (
          <div>
            <h3 className="font-medium mb-2">货品列表</h3>
            <div className="space-y-3">
              {items.map((item, idx) => (
                <div key={idx} className="bg-white rounded-xl p-3">
                  <div className="flex gap-3 items-start mb-3">
                    <img
                      src={(() => { try { const imgs = JSON.parse(item.images || '[]'); return imgs[0] || '' } catch { return '' } })()}
                      alt={item.sn}
                      className="w-14 h-14 object-cover rounded-lg flex-shrink-0 bg-gray-100"
                      onError={(e) => { (e.target).src = ''; (e.target).style.display = 'none' }}
                    />
                    <div className="flex-1 min-w-0">
                      <p className="font-medium text-sm truncate">SN: {item.sn}</p>
                      <p className="text-xs text-gray-500 truncate">
                        {[item.category_name, item.level_name].filter(Boolean).join(' | ') || '-'}
                      </p>
                      {(() => {
                        try {
                          const pricing = JSON.parse(item.pricing || '[]')
                          if (pricing[0]) {
                            const p = pricing[0]
                            return <p className="text-xs text-blue-600 font-medium mt-0.5">¥{p.daily_rent}/天 · ¥{p.monthly_rent}/月</p>
                          }
                        } catch {}
                        return null
                      })()}
                    </div>
                    <button onClick={() => removeItem(item.sn)} className="text-red-500 flex-shrink-0">
                      <X size={18} />
                    </button>
                  </div>
                  <div className="border-t pt-3">
                    <p className="text-xs text-gray-500 mb-2">拍摄乐器照片作为发货留档</p>
                    <ImageUploader
                      maxImages={5}
                      onChange={(files) => updateItemPhotos(item.sn, files)}
                    />
                  </div>
                </div>
              ))}
            </div>
          </div>
        )}

        {/* Add Instrument */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3">添加乐器</h3>
          <div className="flex gap-2">
            <input
              type="text"
              value={snInput}
              onChange={e => setSnInput(e.target.value)}
              placeholder="输入识别码或扫描二维码"
              className="flex-1 border rounded-lg px-3 py-2"
              onKeyDown={e => e.key === 'Enter' && snInput && checkInstrument(snInput)}
            />
            <button
              onClick={() => snInput && checkInstrument(snInput)}
              className="px-4 py-2 bg-brand-primary text-white rounded-lg"
            >
              <Plus size={18} />
            </button>
            <button
              onClick={() => alert('扫码功能暂不可用')}
              className="px-4 py-2 border rounded-lg"
            >
              <Scan size={18} />
            </button>
          </div>
        </div>

        {/* Submit Button */}
        <button
          onClick={handleSubmit}
          disabled={items.length === 0 || submitting}
          className="w-full py-3 bg-brand-primary text-white rounded-xl disabled:opacity-50 font-medium"
        >
          {submitting ? '提交中...' : `提交（${items.length} 件）`}
        </button>
      </div>
    </div>
  )
}
