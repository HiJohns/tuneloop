import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { formatDeliveryAddress } from '../utils/format'
import { ArrowLeft, Camera, User, MapPin, Package } from 'lucide-react'

export default function ShippingInterface() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [logistics, setLogistics] = useState({ company: '', trackingNumber: '' })
  const [photos, setPhotos] = useState([])
  const [submitting, setSubmitting] = useState(false)

  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  const orderId = searchParams.get('order_id')

  useEffect(() => {
    if (orderId) {
      fetchOrder(orderId)
    } else {
      // Legacy support: ?instrument=:id
      const instId = searchParams.get('instrument')
      if (instId) {
        fetchInstrumentById(instId).then(inst => {
          if (!inst) return
          apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(inst.sn)}`)
            .then(r => r.json())
            .then(r => {
              if (r.code === 20000 && r.data) fetchOrder(r.data.order_id)
            })
        })
      }
    }
  }, [])

  const fetchOrder = async (orderId) => {
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${orderId}`)
      const result = await resp.json()
      if (result.code === 20000) {
        setOrder(result.data)
        const inst = await fetchInstrumentById(result.data.instrument_id)
        if (inst) setInstrument(inst)
      }
    } catch (err) {
      console.error('Failed to fetch order:', err)
    }
  }

  const fetchInstrumentById = async (id) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/${id}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data) return result.data
    } catch (err) {
      console.error('Failed to fetch instrument:', err)
    }
    return null
  }

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    setPhotos(prev => [...prev, ...files].slice(0, 10))
  }

  const removePhoto = (idx) => {
    setPhotos(prev => prev.filter((_, i) => i !== idx))
  }

  const canSubmit = logistics.company.trim() && logistics.trackingNumber.trim() && photos.length > 0 && orderId && !submitting

  const handleSubmit = async () => {
    if (!canSubmit || !orderId) return

    setSubmitting(true)
    const token = localStorage.getItem('token') || sessionStorage.getItem('token')

    try {
      // Upload photos
      const photoUrls = []
      for (const file of photos) {
        const fd = new FormData()
        fd.append('file', file)
        const resp = await fetch(`${baseUrl}/upload`, {
          method: 'POST',
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
          body: fd,
        })
        const result = await resp.json()
        if (result.code === 20000 && result.data?.url) {
          photoUrls.push(result.data.url)
        }
      }

      // Submit shipping
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/shipping`, {
        method: 'PUT',
        body: JSON.stringify({
          tracking_number: logistics.trackingNumber,
          company: logistics.company,
          shipped_at: new Date().toISOString(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        alert('发货成功')
        navigate(-1)
      } else {
        alert('发货失败: ' + result.message)
      }
    } catch (err) {
      alert('发货失败: ' + err.message)
    }
    setSubmitting(false)
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">发货</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Customer Info */}
        {order && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium text-gray-900 mb-3 flex items-center gap-2">
              <User size={16} className="text-brand-primary" />
              收货人信息
            </h3>
            {order.user_name && (
              <p className="text-sm font-medium">{order.user_name}</p>
            )}
            {order.delivery_address && (
              <div className="flex items-start gap-2 mt-2 text-sm text-gray-600">
                <MapPin size={14} className="mt-0.5 flex-shrink-0" />
                <span>{formatDeliveryAddress(order.delivery_address)}</span>
              </div>
            )}
          </div>
        )}

        {/* Instrument Info */}
        {instrument && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium text-gray-900 mb-3 flex items-center gap-2">
              <Package size={16} className="text-brand-primary" />
              乐器信息
            </h3>
            <div className="flex gap-3">
              {(() => {
                try {
                  const imgs = JSON.parse(instrument.images || '[]')
                  if (imgs[0]) return <img src={imgs[0]} alt="" className="w-16 h-16 object-cover rounded bg-gray-100" />
                } catch {}
                return null
              })()}
              <div>
                <p className="text-sm font-mono font-medium">SN: {instrument.sn || '-'}</p>
                <p className="text-xs text-gray-500">{instrument.category_name}{instrument.level_name ? ` · ${instrument.level_name}` : ''}</p>
              </div>
            </div>
          </div>
        )}

        {/* Logistics Info */}
        <div className="bg-white rounded-xl p-4 space-y-3">
          <h3 className="font-medium text-gray-900 flex items-center gap-2">
            <span className="w-2 h-2 bg-red-500 rounded-full inline-block" />
            物流信息
            <span className="text-xs text-gray-400 font-normal">（必填）</span>
          </h3>
          <input
            type="text"
            value={logistics.company}
            onChange={e => setLogistics({ ...logistics, company: e.target.value })}
            placeholder="承运公司"
            className="w-full border rounded-lg px-3 py-2 text-sm"
          />
          <input
            type="text"
            value={logistics.trackingNumber}
            onChange={e => setLogistics({ ...logistics, trackingNumber: e.target.value })}
            placeholder="快递单号"
            className="w-full border rounded-lg px-3 py-2 text-sm"
          />
        </div>

        {/* Photo Capture */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium text-gray-900 mb-3 flex items-center gap-2">
            <Camera size={16} className="text-brand-primary" />
            拍照留档
            <span className="text-xs text-gray-400 font-normal">（至少 1 张）</span>
          </h3>
          <div className="grid grid-cols-3 gap-2 mb-3">
            {photos.map((file, i) => (
              <div key={i} className="relative aspect-square rounded-lg overflow-hidden border">
                <img src={URL.createObjectURL(file)} alt="" className="w-full h-full object-cover" />
                <button
                  onClick={() => removePhoto(i)}
                  className="absolute top-1 right-1 bg-black/50 rounded-full w-5 h-5 flex items-center justify-center"
                >
                  <span className="text-white text-xs">✕</span>
                </button>
              </div>
            ))}
            {photos.length < 10 && (
              <label className="aspect-square border-2 border-dashed border-gray-300 rounded-lg flex flex-col items-center justify-center cursor-pointer text-gray-400 hover:text-brand-primary">
                <Camera size={24} />
                <span className="text-xs mt-1">拍摄</span>
                <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
              </label>
            )}
          </div>
          <p className="text-xs text-gray-400">已拍摄 {photos.length} 张，最多 10 张</p>
        </div>
      </div>

      {/* Submit Button */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <button
          onClick={handleSubmit}
          disabled={!canSubmit}
          className="w-full py-3 bg-brand-primary text-white rounded-xl font-medium disabled:opacity-50 text-lg"
        >
          {submitting ? '提交中...' : '提交'}
        </button>
      </div>
    </div>
  )
}
