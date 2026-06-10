import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, CheckCircle, Camera } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { dialog, env, storage, session, uploadFile } from '../platform'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

export default function ReceiveConfirm() {
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

  useEffect(() => {
    const fetchData = async () => {
      try {
        const token = storage.getItem('token') || session.getItem('token')
        const headers = { 'Authorization': `Bearer ${token}` }

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

  const handleConfirmReceive = async () => {
    setSubmitting(true)
    try {
      const token = storage.getItem('token') || session.getItem('token')

      // 1. Upload photos first
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

      // 2. Confirm delivery
      const resp = await fetch(`${baseUrl}/warehouse/orders/${orderId}/delivery`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
          ...(token ? { 'Authorization': `Bearer ${token}` } : {}),
        },
        body: JSON.stringify({
          delivered_at: new Date().toISOString(),
          photos: photoUrls,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.alert('确认收货成功')
        navigate('/profile')
      } else {
        dialog.alert('确认收货失败: ' + (result.message || ''))
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
        <h1 className="text-lg font-bold">确认收货</h1>
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
            <div className="flex justify-between"><span className="text-gray-500">所属网点</span><span>{instrument?.site_name || '-'}</span></div>
            {instrument?.properties && Object.keys(instrument.properties).length > 0 && (
              <div className="flex justify-between"><span className="text-gray-500">动态属性</span><span>{JSON.stringify(instrument.properties)}</span></div>
            )}
          </div>
        </div>

        {/* Order Info */}
        {order && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-3">租赁信息</h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between"><span className="text-gray-500">租期</span><span>{order.start_date ? order.start_date.slice(0, 10) : '-'} 至 {order.end_date ? order.end_date.slice(0, 10) : '-'}</span></div>
              <div className="flex justify-between"><span className="text-gray-500">月租金</span><span>¥{order.monthly_rent || 0}</span></div>
              <div className="flex justify-between"><span className="text-gray-500">押金</span><span>¥{order.deposit || 0}</span></div>
              <div className="flex justify-between"><span className="text-gray-500">租赁人</span><span>{order.user_name || '-'}</span></div>
            </div>
          </div>
        )}

        {/* Photo Upload */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3 flex items-center gap-2">
            <Camera size={18} />
            拍照留档
          </h3>
          <p className="text-sm text-gray-500 mb-3">请拍摄乐器当前状态照片作为签收留档</p>
          <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
        </div>

        {/* Confirm Button */}
        <button
          onClick={handleConfirmReceive}
          disabled={submitting || photoFiles.length === 0}
          className="w-full py-3 bg-green-500 text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '确认收货'}
        </button>
      </div>
    </div>
  )
}
