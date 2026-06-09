import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, Camera, Scan, CheckCircle, AlertTriangle, Upload, User, MapPin, Package } from 'lucide-react'

export default function ReceivingInterface() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const preloadedOrderId = searchParams.get('order_id')
  const [snInput, setSnInput] = useState('')
  const [currentItem, setCurrentItem] = useState(null)
  const [currentSN, setCurrentSN] = useState('')
  const [orderData, setOrderData] = useState(null)
  const [condition, setCondition] = useState('')
  const [damageDesc, setDamageDesc] = useState('')
  const [damageAmount, setDamageAmount] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [photoSpecs, setPhotoSpecs] = useState([])
  const [orderID, setOrderID] = useState(null)
  const [outboundPhotos, setOutboundPhotos] = useState([])
  const [capturedPhotos, setCapturedPhotos] = useState([])

  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  // Auto-load order when order_id is provided via URL
  useEffect(() => {
    if (!preloadedOrderId) return
    const loadOrder = async () => {
      try {
        const orderResp = await apiFetch(`${baseUrl}/orders/${preloadedOrderId}`)
        const orderResult = await orderResp.json()
        if (orderResult.code !== 20000) return
        setOrderData(orderResult.data)
        setOrderID(preloadedOrderId)

        const instResp = await apiFetch(`${baseUrl}/instruments/${orderResult.data.instrument_id}`)
        const instResult = await instResp.json()
        if (instResult.code !== 20000) return
        const inst = instResult.data
        setCurrentItem(inst)
        setCurrentSN(inst.sn || '')
        if (inst.category_id) {
          const specResp = await apiFetch(`${baseUrl}/instrument-photo-specs/${inst.category_id}`)
          const specResult = await specResp.json()
          if (specResult.code === 20000) setPhotoSpecs(specResult.data?.photo_requirements || [])
        }
      } catch (err) {
        console.error('Failed to load order:', err)
      }
    }
    loadOrder()
  }, [preloadedOrderId])

  useEffect(() => {
    if (orderID) {
      apiFetch(`${baseUrl}/orders/${orderID}/outbound-photos`)
        .then(r => r.json())
        .then(res => {
          if (res.code === 20000) {
            setOutboundPhotos(res.data.outbound_photos || [])
          }
        })
        .catch(() => {})
    }
  }, [orderID])

  const handlePhotoCapture = (e) => {
    const files = Array.from(e.target.files || [])
    files.forEach(f => {
      const reader = new FileReader()
      reader.onload = () => {
        setCapturedPhotos(prev => [...prev, reader.result])
      }
      reader.readAsDataURL(f)
    })
  }

  const checkInstrument = async (sn) => {
    try {
      const resp = await apiFetch(`${baseUrl}/instruments/check?sn=${encodeURIComponent(sn)}`)
      const result = await resp.json()
      if (result.code === 20000 && result.data?.exists) {
        const inst = result.data.info
        setCurrentItem(inst)
        setCurrentSN(sn)
        setSnInput('')
        if (inst.category_id) {
          const specResp = await apiFetch(`${baseUrl}/instrument-photo-specs/${inst.category_id}`)
          const specResult = await specResp.json()
          if (specResult.code === 20000) {
            setPhotoSpecs(specResult.data?.photo_requirements || [])
          }
        }
        const orderResp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(sn)}`)
        const orderResult = await orderResp.json()
        setOrderID(orderResult.code === 20000 ? orderResult.data?.order_id : null)
      } else {
        alert('未找到该乐器')
      }
    } catch (err) {
      console.error('Failed to check instrument:', err)
    }
  }

  const handleSubmit = async () => {
    if (!currentItem) return
    if (!orderID) {
      alert('未找到该乐器的活跃订单')
      return
    }
    setSubmitting(true)
    try {
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderID}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: currentSN,
          scan_time: new Date().toISOString(),
          condition: condition,
          notes: condition === 'damaged' ? damageDesc : '',
          photos: capturedPhotos,
        }),
      })
      const result = await resp.json()

      if (result.code === 20000 && condition === 'damaged') {
        const damageResp = await apiFetch(`${baseUrl}/warehouse/orders/${orderID}/damage`, {
          method: 'PUT',
          body: JSON.stringify({
            damage_description: damageDesc,
            damage_amount: parseFloat(damageAmount) || 0,
          }),
        })
        const damageResult = await damageResp.json()
        if (damageResult.code === 20000) {
          alert('定损评估已记录，通知已发送给用户')
        } else {
          alert('定损评估失败: ' + damageResult.message)
        }
      } else if (result.code === 20000) {
        alert('验收成功，押金退还已发起')
      } else {
        alert('失败: ' + result.message)
      }

      setCurrentItem(null)
      setCurrentSN('')
      setCondition('')
      setDamageDesc('')
      setDamageAmount('')
      setOrderID(null)
      setOutboundPhotos([])
      setCapturedPhotos([])
    } catch (err) {
      alert('错误: ' + err.message)
    }
    setSubmitting(false)
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">收货确认</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Customer & Instrument Info (preloaded from order) */}
        {orderData && (
          <>
            <div className="bg-white rounded-xl p-4">
              <h3 className="font-medium text-gray-900 mb-3 flex items-center gap-2">
                <User size={16} className="text-brand-primary" />
                租赁人信息
              </h3>
              <p className="text-sm font-medium">{orderData.user_name || '-'}</p>
              {orderData.delivery_address && (
                <div className="flex items-start gap-2 mt-2 text-sm text-gray-600">
                  <MapPin size={14} className="mt-0.5 flex-shrink-0" />
                  <span>{orderData.delivery_address}</span>
                </div>
              )}
            </div>

            {currentItem && (
              <div className="bg-white rounded-xl p-4">
                <h3 className="font-medium text-gray-900 mb-3 flex items-center gap-2">
                  <Package size={16} className="text-brand-primary" />
                  乐器信息
                </h3>
                <div className="flex gap-3">
                  {(() => {
                    try {
                      const imgs = JSON.parse(currentItem.images || '[]')
                      if (imgs[0]) return <img src={imgs[0]} alt="" className="w-16 h-16 object-cover rounded bg-gray-100" />
                    } catch {}
                    return null
                  })()}
                  <div>
                    <p className="text-sm font-mono font-medium">SN: {currentItem.sn || '-'}</p>
                    <p className="text-xs text-gray-500">{currentItem.category_name}{currentItem.level_name ? ` · ${currentItem.level_name}` : ''}</p>
                  </div>
                </div>
              </div>
            )}

            <div className="bg-white rounded-xl p-4">
              <h3 className="font-medium text-gray-900 mb-2">租赁信息</h3>
              <div className="text-sm space-y-1 text-gray-600">
                <p>租期: {orderData.start_date || '-'} 至 {orderData.end_date || '-'}</p>
                {orderData.deposit > 0 && <p>押金: ¥{orderData.deposit}</p>}
              </div>
            </div>
          </>
        )}

        {/* QR Scan / SN Entry — only when not preloaded */}
        {!preloadedOrderId && (
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3">扫码或输入识别码</h3>
          <div className="flex gap-2">
            <input
              type="text"
              value={snInput}
              onChange={e => setSnInput(e.target.value)}
              placeholder="输入识别码或扫码"
              className="flex-1 border rounded-lg px-3 py-2"
              onKeyDown={e => e.key === 'Enter' && snInput && checkInstrument(snInput)}
            />
            <button
              onClick={() => alert('扫码功能暂不可用')}
              className="px-4 py-2 border rounded-lg"
            >
              <Scan size={18} />
            </button>
          </div>
        </div>
        )}

        {currentItem && (
          <div className="bg-white rounded-xl p-4">
            <div className="flex justify-between items-start mb-3">
              <div>
                <p className="font-medium">{currentItem.name}</p>
                <p className="text-sm text-gray-500">{currentItem.brand} {currentItem.model}</p>
              </div>
              <span className="text-xs bg-blue-100 text-blue-700 px-2 py-1 rounded-full">
                {currentItem.stock_status}
              </span>
            </div>

            {photoSpecs.length > 0 && (
              <div className="mb-4">
                <h4 className="text-sm font-medium flex items-center gap-1 mb-1">
                  <Camera size={14} className="text-brand-primary" />
                  拍照要求
                </h4>
                <ul className="text-xs text-gray-500 space-y-0.5">
                  {photoSpecs.map((spec, idx) => (
                    <li key={idx}>• {spec.position}: {spec.description}</li>
                  ))}
                </ul>
              </div>
            )}

            {outboundPhotos.length > 0 && (
              <div className="mb-4">
                <h4 className="text-sm font-medium text-gray-700 mb-2">出库照片（供对比）</h4>
                <div className="grid grid-cols-2 gap-2">
                  {outboundPhotos.map((p, i) => (
                    <img key={i} src={p.url} alt="outbound" className="w-full rounded border object-cover h-24" />
                  ))}
                </div>
              </div>
            )}

            <div className="mb-4">
              <h4 className="text-sm font-medium text-gray-700 mb-2">归还拍照</h4>
              <label className="flex items-center gap-2 px-4 py-3 border-2 border-dashed rounded-lg cursor-pointer text-gray-500 hover:text-brand-primary">
                <Upload size={18} />
                <span className="text-sm">拍照上传</span>
                <input type="file" accept="image/*" capture="environment" multiple className="hidden" onChange={handlePhotoCapture} />
              </label>
              {capturedPhotos.length > 0 && (
                <div className="grid grid-cols-3 gap-2 mt-2">
                  {capturedPhotos.map((url, i) => (
                    <div key={i} className="relative">
                      <img src={url} alt="captured" className="w-full rounded border object-cover h-20" />
                    </div>
                  ))}
                </div>
              )}
            </div>

            <div className="space-y-3">
              <div className="flex gap-2">
                <button
                  onClick={() => setCondition('good')}
                  className={`flex-1 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-1 ${
                    condition === 'good' ? 'bg-green-500 text-white' : 'border text-gray-600'
                  }`}
                >
                  <CheckCircle size={16} /> 无损坏
                </button>
                <button
                  onClick={() => setCondition('damaged')}
                  className={`flex-1 py-2 rounded-lg text-sm font-medium flex items-center justify-center gap-1 ${
                    condition === 'damaged' ? 'bg-red-500 text-white' : 'border text-gray-600'
                  }`}
                >
                  <AlertTriangle size={16} /> 有损坏
                </button>
              </div>

              {condition === 'damaged' && (
                <div className="space-y-2">
                  <textarea
                    value={damageDesc}
                    onChange={e => setDamageDesc(e.target.value)}
                    placeholder="请描述损坏情况"
                    className="w-full border rounded-lg px-3 py-2 text-sm"
                    rows={3}
                  />
                  <input
                    type="number"
                    value={damageAmount}
                    onChange={e => setDamageAmount(e.target.value)}
                    placeholder="定损金额"
                    className="w-full border rounded-lg px-3 py-2 text-sm"
                  />
                </div>
              )}

              {condition && (
                <button
                  onClick={handleSubmit}
                  disabled={submitting}
                  className="w-full py-3 bg-brand-primary text-white rounded-lg disabled:opacity-50 font-medium"
                >
                  {submitting ? '提交中...' : '提交'}
                </button>
              )}
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
