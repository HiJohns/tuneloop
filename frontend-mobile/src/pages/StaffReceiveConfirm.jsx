import { useState, useEffect } from 'react'
import { useParams, useNavigate, useSearchParams } from 'react-router-dom'
import { ArrowLeft, CheckCircle, Camera, AlertTriangle } from 'lucide-react'
import ImageUploader from '../components/ImageUploader'
import { apiFetch } from '../services/api'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

export default function StaffReceiveConfirm() {
  const { orderId } = useParams()
  const [searchParams] = useSearchParams()
  const navigate = useNavigate()
  const instrumentId = searchParams.get('instrument')
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  const [instrument, setInstrument] = useState(null)
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [photoFiles, setPhotoFiles] = useState([])
  const [hasDamage, setHasDamage] = useState(false)
  const [damageReason, setDamageReason] = useState('')
  const [damageAmount, setDamageAmount] = useState('')

  useEffect(() => {
    const fetchData = async () => {
      try {
        const [orderResp, instResp] = await Promise.all([
          apiFetch(`${baseUrl}/orders/${orderId}`),
          apiFetch(`${baseUrl}/public/instruments/${instrumentId}`),
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
    if (hasDamage && (!damageReason.trim() || !damageAmount.trim())) {
      alert('请填写定损理由和金额')
      return
    }
    setSubmitting(true)
    try {
      const condition = hasDamage ? 'damaged' : 'good'

      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${orderId}/return-inspect`, {
        method: 'PUT',
        body: JSON.stringify({
          instrument_sn: instrument?.sn,
          scan_time: new Date().toISOString(),
          condition: condition,
          notes: hasDamage ? damageReason.trim() : '验收通过',
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        alert('接收确认成功')
        navigate(`/staff/instrument/${instrumentId}`)
      } else {
        alert('接收失败: ' + (result.message || ''))
      }
    } catch (err) {
      alert('操作失败: ' + err.message)
    }
    setSubmitting(false)
  }

  if (loading) {
    return <div className="min-h-screen bg-brand-bg flex items-center justify-center">
      <p className="text-gray-500">加载中...</p>
    </div>
  }

  const images = instrument?.images ? (Array.isArray(instrument.images) ? instrument.images : []) : []

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">接收确认</h1>
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
          </div>
        </div>

        {/* Order Info */}
        {order && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-3">租赁信息</h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between"><span className="text-gray-500">租期</span><span>{order.start_date || '-'} 至 {order.end_date || '-'}</span></div>
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
          <p className="text-sm text-gray-500 mb-3">请拍摄乐器当前状态照片作为接收留档</p>
          <ImageUploader maxImages={5} onChange={(files) => setPhotoFiles(files)} />
        </div>

        {/* Damage Assessment */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3 flex items-center gap-2">
            <AlertTriangle size={18} />
            定损
          </h3>
          <div className="flex gap-3 mb-3">
            <button
              onClick={() => { setHasDamage(false); setDamageReason(''); setDamageAmount('') }}
              className={`flex-1 py-2 rounded-lg font-medium text-sm ${!hasDamage ? 'bg-green-100 text-green-700 border-2 border-green-500' : 'bg-gray-100 text-gray-500'}`}
            >
              无损坏
            </button>
            <button
              onClick={() => setHasDamage(true)}
              className={`flex-1 py-2 rounded-lg font-medium text-sm ${hasDamage ? 'bg-red-100 text-red-700 border-2 border-red-500' : 'bg-gray-100 text-gray-500'}`}
            >
              有损坏
            </button>
          </div>
          {hasDamage && (
            <div className="space-y-3">
              <div>
                <label className="block text-sm text-gray-600 mb-1">定损理由</label>
                <input
                  type="text"
                  value={damageReason}
                  onChange={e => setDamageReason(e.target.value)}
                  placeholder="请描述损坏情况"
                  className="w-full border rounded-lg px-3 py-2 text-sm"
                />
              </div>
              <div>
                <label className="block text-sm text-gray-600 mb-1">定损金额</label>
                <input
                  type="number"
                  value={damageAmount}
                  onChange={e => setDamageAmount(e.target.value)}
                  placeholder="请输入定损金额"
                  className="w-full border rounded-lg px-3 py-2 text-sm"
                />
              </div>
            </div>
          )}
        </div>

        {/* Submit Button */}
        <button
          onClick={handleConfirmReceive}
          disabled={submitting}
          className="w-full py-3 bg-green-600 text-white rounded-xl font-medium disabled:opacity-50 flex items-center justify-center gap-2"
        >
          <CheckCircle size={20} />
          {submitting ? '提交中...' : '确认接收'}
        </button>
      </div>
    </div>
  )
}
