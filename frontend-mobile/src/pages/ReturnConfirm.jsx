import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowLeft, CheckCircle, Camera, Truck } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { getToken, redirectToLogin } from '../services/api'
import { dialog, env, storage, session, uploadFile } from '../platform'
import { formatDisplayDate } from '../utils/format'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

export default function ReturnConfirm() {
  const { orderId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const instrumentId = searchParams.get('instrument')
  const baseUrl = env.apiBaseUrl

  const [instrument, setInstrument] = useState(null)
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [photoFiles, setPhotoFiles] = useState([])
  const [courierCompany, setCourierCompany] = useState('')
  const [trackingNumber, setTrackingNumber] = useState('')

  useEffect(() => {
    const fetchData = async () => {
      try {
        const token = getToken()
        const headers = { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) }

        const [orderResp, instResp] = await Promise.all([
          fetch(`${baseUrl}/orders/${orderId}`, { headers }),
          fetch(`${baseUrl}/public/instruments/${instrumentId}`, { headers }),
        ])

        const orderResult = await orderResp.json()
        const instResult = await instResp.json()

        if (orderResult.code === 20000) setOrder(orderResult.data)
        if (instResult.code === 20000) setInstrument(instResult.data)
      } catch (err) {
        console.error('Failed to load data:', err)
      }
      setLoading(false)
    }
    fetchData()
  }, [orderId, instrumentId])

  const handleSubmitReturn = async () => {
    if (!courierCompany.trim() || !trackingNumber.trim()) {
      dialog.alert('请填写物流信息')
      return
    }
    setSubmitting(true)
    try {
      const token = getToken()
      if (!token) { redirectToLogin(); return }

      // Upload photos
      const photoUrls = []
      for (const file of photoFiles) {
        const fd = new FormData()
        fd.append('file', file)
        const upResp = await uploadFile(`${baseUrl}/upload`, file, {
          headers: { ...(token ? { 'Authorization': `Bearer ${token}` } : {}) },
        })
        const upResult = await upResp.json()
        if (upResult.code === 20000 && upResult.data?.url) {
          photoUrls.push(upResult.data.url)
        }
      }

      const resp = await fetch(`${baseUrl}/orders/${orderId}/return`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          courier_company: courierCompany.trim(),
          tracking_number: trackingNumber.trim(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('已提交归还申请')
        navigate('/profile')
      } else {
        dialog.alert('归还失败: ' + (result.message || ''))
      }
    } catch (err) {
      dialog.alert('操作失败: ' + err.message)
    }
    setSubmitting(false)
  }

  if (loading) {
    return <div className="min-h-screen bg-brand-bg flex items-center justify-center">
      <p className="text-gray-500">加载中...</p>
    </div>
  }

  const images = (() => {
    if (!instrument?.images) return []
    if (Array.isArray(instrument.images)) return instrument.images
    if (typeof instrument.images === 'string') {
      try { return JSON.parse(instrument.images) } catch { return [] }
    }
    return []
  })()

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">归还乐器</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Instrument Info */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3">乐器信息</h3>
          <img
            src={images[0] || PLACEHOLDER_IMAGE}
            alt={instrument?.sn}
            className="w-full h-40 object-contain bg-gray-100 rounded-lg mb-3"
          />
          <div className="space-y-2 text-sm">
            <div className="flex justify-between"><span className="text-gray-500">识别码</span><span>{instrument?.sn || '-'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">类别</span><span>{instrument?.category_name || '-'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">商户</span><span>{instrument?.tenant_name || '-'}</span></div>
            <div className="flex justify-between"><span className="text-gray-500">所属网点</span><span>{instrument?.site_name || '-'}</span></div>
          </div>
        </div>

        {/* Order Info */}
        {order && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-3">租赁信息</h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between"><span className="text-gray-500">租期</span><span>{formatDisplayDate(order.start_date)} 至 {formatDisplayDate(order.end_date)}</span></div>
              <div className="flex justify-between"><span className="text-gray-500">月租金</span><span>¥{order.monthly_rent || 0}</span></div>
              <div className="flex justify-between"><span className="text-gray-500">押金</span><span>¥{order.deposit || 0}</span></div>
              <div className="flex justify-between"><span className="text-gray-500">租赁人</span><span>{order.user_name || '-'}</span></div>
            </div>
          </div>
        )}

        {/* Logistics Info */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3 flex items-center gap-2">
            <Truck size={18} />
            物流信息
          </h3>
          <p className="text-sm text-gray-500 mb-3">请填写返程物流信息，用于归还乐器</p>
          <div className="space-y-3">
            <div>
              <label className="block text-sm text-gray-600 mb-1">承运公司</label>
              <input
                type="text"
                value={courierCompany}
                onChange={e => setCourierCompany(e.target.value)}
                placeholder="如：顺丰速运"
                className="w-full border rounded-lg px-3 py-2 text-sm"
              />
            </div>
            <div>
              <label className="block text-sm text-gray-600 mb-1">快递单号</label>
              <input
                type="text"
                value={trackingNumber}
                onChange={e => setTrackingNumber(e.target.value)}
                placeholder="请输入快递单号"
                className="w-full border rounded-lg px-3 py-2 text-sm"
              />
            </div>
          </div>
        </div>

        {/* Photo Upload */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3 flex items-center gap-2">
            <Camera size={18} />
            拍照留档
          </h3>
          <p className="text-sm text-gray-500 mb-3">请拍摄乐器当前状态照片作为归还留档</p>
          <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
        </div>

        {/* Submit Button */}
        <button
          onClick={handleSubmitReturn}
          disabled={submitting || !courierCompany.trim() || !trackingNumber.trim()}
          className="w-full py-3 bg-orange-500 text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '提交归还'}
        </button>
      </div>
    </div>
  )
}
