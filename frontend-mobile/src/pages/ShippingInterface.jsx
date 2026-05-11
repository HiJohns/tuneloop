import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, Camera, Scan, Plus, X } from 'lucide-react'

export default function ShippingInterface() {
  const navigate = useNavigate()
  const [snInput, setSnInput] = useState('')
  const [items, setItems] = useState([])
  const [logistics, setLogistics] = useState({ company: '', trackingNumber: '' })
  const [submitting, setSubmitting] = useState(false)
  const [photoSpecs, setPhotoSpecs] = useState([])

  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  const checkInstrument = async (sn) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/check?sn=${encodeURIComponent(sn)}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data?.exists) {
        const inst = result.data.info
        if (inst.stock_status !== 'reserved') {
          alert(`Instrument ${sn} is not in reserved status (current: ${inst.stock_status})`)
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
        alert('Instrument not found')
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
    setSubmitting(true)
    try {
      for (const item of items) {
        if (!item.order_id) {
          alert(`No active order found for instrument ${item.sn}`)
          setSubmitting(false)
          return
        }
        const resp = await apiFetch(`${baseUrl}/warehouse/orders/${item.order_id}/shipping`, {
          method: 'PUT',
          body: JSON.stringify({
            tracking_number: logistics.trackingNumber,
            company: logistics.company,
            shipped_at: new Date().toISOString(),
          }),
        })
        const result = await resp.json()
        if (result.code !== 20000) {
          alert(`Failed to ship ${item.sn}: ${result.message}`)
          setSubmitting(false)
          return
        }
      }
      alert('All items shipped successfully')
      navigate('/staff/instruments')
    } catch (err) {
      alert('Shipping failed: ' + err.message)
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
              onClick={() => alert('QR scanner is not yet available')}
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
            <ul className="space-y-1 text-sm text-gray-600">
              {photoSpecs.map((spec, idx) => (
                <li key={idx}>• {spec.position}: {spec.description} {spec.required ? '(Required)' : '(Optional)'}</li>
              ))}
            </ul>
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
          disabled={items.length === 0 || submitting}
          className="w-full py-3 bg-brand-primary text-white rounded-xl disabled:opacity-50 font-medium"
        >
          {submitting ? 'Submitting...' : `Submit (${items.length} items)`}
        </button>
      </div>
    </div>
  )
}
