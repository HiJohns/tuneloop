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
  const [photoSpecs, setPhotoSpecs] = useState([])
  const [uploadedPhotos, setUploadedPhotos] = useState([])

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
          name: inst.name,
          brand: inst.brand,
          model: inst.model,
          category_id: inst.category_id,
          order_id: orderID,
        }])
        if (inst.category_id) fetchPhotoSpecs(inst.category_id)
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
          name: inst.name,
          brand: inst.brand,
          model: inst.model,
          category_id: inst.category_id,
          order_id: orderID,
        }])
        setSnInput('')
        if (inst.category_id) {
          fetchPhotoSpecs(inst.category_id)
        }
      } else {
        alert('未找到该乐器')
      }
    } catch (err) {
      console.error('Failed to check instrument:', err)
    }
  }

  const fetchPhotoSpecs = async (categoryId) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instrument-photo-specs/${categoryId}`)
      const result = await resp.json()
      if (result.code === 20000) {
        setPhotoSpecs(result.data?.photo_requirements || [])
      }
    } catch (err) {
      console.error('Failed to fetch photo specs:', err)
    }
  }

  const removeItem = (sn) => {
    setItems(prev => prev.filter(i => i.sn !== sn))
  }

  const handleSubmit = async () => {
    if (items.length === 0) return
    
    // Check if required photos are uploaded (per spec, not just any photo)
    const requiredPhotoCount = photoSpecs.filter(spec => spec.required).length
    if (requiredPhotoCount > 0 && uploadedPhotos.length < requiredPhotoCount) {
      alert(`请上传所需的 ${requiredPhotoCount} 张照片，当前已上传 ${uploadedPhotos.length} 张`)
      return
    }
    
    setSubmitting(true)
    try {
      for (const item of items) {
        if (!item.order_id) {
          alert(`乐器 ${item.sn} 没有活跃的订单`)
          setSubmitting(false)
          return
        }
        const resp = await apiFetch(`${baseUrl}/warehouse/orders/${item.order_id}/shipping`, {
          method: 'PUT',
          body: JSON.stringify({
            tracking_number: logistics.trackingNumber,
            company: logistics.company,
            shipped_at: new Date().toISOString(),
            photos: uploadedPhotos,
          }),
        })
        const result = await resp.json()
        if (result.code !== 20000) {
          alert(`发货失败 ${item.sn}: ${result.message}`)
          setSubmitting(false)
          return
        }
      }
      alert('全部发货成功')
      navigate('/staff/instruments')
    } catch (err) {
      alert('发货失败: ' + err.message)
    }
    setSubmitting(false)
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">Shipping</h1>
      </div>

      <div className="p-4 space-y-4">
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3">Add Instrument</h3>
          <div className="flex gap-2">
            <input
              type="text"
              value={snInput}
              onChange={e => setSnInput(e.target.value)}
              placeholder="Enter SN or scan QR code"
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

        {items.length > 0 && (
          <div className="space-y-2">
            {items.map((item, idx) => (
              <div key={idx} className="bg-white rounded-xl p-3 flex justify-between items-center">
                <div>
                  <p className="font-medium text-sm">{item.name}</p>
                  <p className="text-xs text-gray-500">{item.brand} {item.model} | SN: {item.sn}</p>
                </div>
                <button onClick={() => removeItem(item.sn)} className="text-red-500">
                  <X size={18} />
                </button>
              </div>
            ))}
          </div>
        )}

        {photoSpecs.length > 0 && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-2 flex items-center gap-2">
              <Camera size={18} className="text-brand-primary" />
              Photo Requirements
            </h3>
            <ul className="space-y-1 text-sm text-gray-600 mb-3">
              {photoSpecs.map((spec, idx) => (
                <li key={idx}>• {spec.position}: {spec.description} {spec.required ? '(Required)' : '(Optional)'}</li>
              ))}
            </ul>
            <ImageUploader 
              maxImages={5} 
              onUpload={(photos) => setUploadedPhotos(photos)}
            />
          </div>
        )}

        <div className="bg-white rounded-xl p-4 space-y-3">
          <h3 className="font-medium">Logistics Info</h3>
          <input
            type="text"
            value={logistics.company}
            onChange={e => setLogistics({ ...logistics, company: e.target.value })}
            placeholder="Courier company"
            className="w-full border rounded-lg px-3 py-2"
          />
          <input
            type="text"
            value={logistics.trackingNumber}
            onChange={e => setLogistics({ ...logistics, trackingNumber: e.target.value })}
            placeholder="Tracking number"
            className="w-full border rounded-lg px-3 py-2"
          />
        </div>

        <button
          onClick={handleSubmit}
          disabled={items.length === 0 || submitting || (photoSpecs.filter(spec => spec.required).length > 0 && uploadedPhotos.length < photoSpecs.filter(spec => spec.required).length)}
          className="w-full py-3 bg-brand-primary text-white rounded-xl disabled:opacity-50 font-medium"
        >
          {submitting ? 'Submitting...' : `Submit (${items.length} items)`}
        </button>
      </div>
    </div>
  )
}
