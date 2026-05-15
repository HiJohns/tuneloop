import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, MapPin, Package, Truck, Wrench, RotateCcw, CheckCircle, User, Archive } from 'lucide-react'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">No Image</text>
  </svg>
`)

function parseImages(images) {
  if (!images) return []
  if (Array.isArray(images)) return images
  if (typeof images === 'string') {
    try { return JSON.parse(images) } catch { return [] }
  }
  return []
}

function parsePricing(pricing) {
  if (!pricing) return []
  if (Array.isArray(pricing)) return pricing
  if (typeof pricing === 'string') {
    try { return JSON.parse(pricing) } catch { return [] }
  }
  return []
}

export default function StaffInstrumentDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [activeOrder, setActiveOrder] = useState(null)

  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  useEffect(() => {
    const fetchInstrument = async () => {
      try {
        setLoading(true)
        const resp = await apiFetch(`${baseUrl}/instruments/${id}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setInstrument(result.data)
          const inst = result.data
          if (inst.sn && (inst.stock_status === 'reserved' || inst.stock_status === 'returning')) {
            try {
              const orderResp = await fetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(inst.sn)}`)
              const orderResult = await orderResp.json()
              if (orderResult.code === 20000 && orderResult.data) {
                setActiveOrder(orderResult.data)
              }
            } catch {}
          }
        }
      } catch (err) {
        console.error('Failed to fetch instrument:', err)
      }
      setLoading(false)
    }
    fetchInstrument()
  }, [id])

  const statusColor = {
    available: 'bg-green-100 text-green-700',
    reserved: 'bg-blue-100 text-blue-700',
    shipping: 'bg-cyan-100 text-cyan-700',
    rented: 'bg-indigo-100 text-indigo-700',
    returning: 'bg-yellow-100 text-yellow-700',
    maintenance: 'bg-orange-100 text-orange-700',
    archived: 'bg-gray-100 text-gray-700',
  }

  const statusLabel = {
    available: '可租',
    reserved: '已预约',
    shipping: '物流中',
    rented: '租赁中',
    returning: '归还中',
    maintenance: '维修中',
    archived: '已下架',
  }

  const handleShip = async () => {
    if (instrument.stock_status !== 'reserved') {
      alert('乐器不在已预约状态，无法发货')
      return
    }
    navigate(`/staff/shipping?instrument=${instrument.id}`)
  }

  const handleReceive = async () => {
    if (instrument.stock_status !== 'returning') {
      alert('乐器不在归还中状态')
      return
    }
    if (activeOrder) {
      navigate(`/staff/receiving/${activeOrder.order_id}?instrument=${instrument.id}`)
    } else {
      alert('未找到关联订单')
    }
  }

  const handleCompleteMaintenance = async () => {
    if (instrument.stock_status !== 'maintenance') {
      alert('乐器不在维修中状态')
      return
    }
    try {
      setActionLoading(true)
      const maintResp = await apiFetch(`${baseUrl}/instruments/${id}/status`, {
        method: 'PUT',
        body: JSON.stringify({ stock_status: 'available' }),
      })
      const maintResult = await maintResp.json()
      if (maintResult.code === 20000) {
        alert('维修完成')
        navigate('/staff/instruments')
      } else {
        alert('操作失败: ' + maintResult.message)
      }
    } catch (err) {
      alert('操作失败: ' + err.message)
    }
    setActionLoading(false)
  }

  const handleArchive = async () => {
    try {
      setActionLoading(true)
      const resp = await apiFetch(`${baseUrl}/instruments/${id}/status`, {
        method: 'PUT',
        body: JSON.stringify({ stock_status: 'archived' }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        alert('已下架')
        navigate('/staff/instruments')
      } else {
        alert('操作失败: ' + result.message)
      }
    } catch (err) {
      alert('操作失败: ' + err.message)
    }
    setActionLoading(false)
  }

  if (loading) {
    return <div className="p-4">加载中...</div>
  }

  if (!instrument) {
    return <div className="p-4">乐器不存在</div>
  }

  const images = parseImages(instrument.images)
  const pricing = parsePricing(instrument.pricing)
  const pricingInfo = pricing[0] || {}

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">乐器详情</h1>
      </div>

      <div className="p-4 space-y-4">
        {/* Image */}
        <div className="bg-white rounded-xl overflow-hidden">
          <img
            src={images[0] || PLACEHOLDER_IMAGE}
            alt={instrument.name}
            className="w-full h-48 object-contain bg-gray-100"
            onError={(e) => { e.target.onerror = null; e.target.src = PLACEHOLDER_IMAGE }}
          />
        </div>

        {/* Basic Info */}
        <div className="bg-white rounded-xl p-4">
          <div className="flex items-center justify-between mb-3">
            <h2 className="text-lg font-bold">{instrument.name}</h2>
            <span className={`px-3 py-1 rounded-full text-sm font-medium ${statusColor[instrument.stock_status] || 'bg-gray-100'}`}>
              {statusLabel[instrument.stock_status] || instrument.stock_status}
            </span>
          </div>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">SN</span>
              <span className="font-mono">{instrument.sn || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">分类</span>
              <span>{instrument.category_name || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">分级</span>
              <span>{instrument.level_name || instrument.level || '-'}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">网点</span>
              <span>{instrument.site_name || '-'}</span>
            </div>
            {instrument.properties && Object.keys(instrument.properties).length > 0 && (
              <div className="pt-2 border-t">
                <span className="text-gray-500 text-xs block mb-1">动态属性</span>
                {Object.entries(instrument.properties).map(([key, vals]) => (
                  <div key={key} className="flex justify-between text-xs mt-1">
                    <span className="text-gray-400">{key}</span>
                    <span>{(Array.isArray(vals) ? vals : [vals]).join(', ')}</span>
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Lease Info - Only for reserved/returning */}
        {activeOrder && (
          <div className="bg-white rounded-xl p-4">
            <h3 className="font-medium mb-3">租期信息</h3>
            <div className="space-y-2 text-sm">
              <div className="flex justify-between">
                <span className="text-gray-500">租期</span>
                <span>{activeOrder.start_date || '-'} 至 {activeOrder.end_date || '-'}</span>
              </div>
              <div className="flex justify-between">
                <span className="text-gray-500">状态</span>
                <span className="font-medium">{statusLabel[instrument.stock_status] || instrument.stock_status}</span>
              </div>
              {activeOrder.monthly_rent && (
                <div className="flex justify-between">
                  <span className="text-gray-500">月租金</span>
                  <span>¥{activeOrder.monthly_rent}</span>
                </div>
              )}
            </div>
          </div>
        )}

        {/* Pricing Info */}
        <div className="bg-white rounded-xl p-4">
          <h3 className="font-medium mb-3">租赁设置</h3>
          <div className="space-y-2 text-sm">
            <div className="flex justify-between">
              <span className="text-gray-500">日租金</span>
              <span>¥{pricingInfo.daily_rent || 0}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">押金</span>
              <span>¥{pricingInfo.deposit || 0}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">物流费</span>
              <span>¥{pricingInfo.shipping_fee || 0}</span>
            </div>
            <div className="flex justify-between">
              <span className="text-gray-500">逾期日费</span>
              <span>¥{pricingInfo.overdue_daily_fee || pricingInfo.daily_rent || 0}</span>
            </div>
          </div>
        </div>

        {/* Booker Info Card - Only show for reserved status */}
        {instrument.stock_status === 'reserved' && (instrument.booker_name || instrument.booker_phone) && (
          <div className="mb-4 p-4 bg-yellow-50 border border-yellow-200 rounded-lg">
            <div className="flex items-center gap-2 mb-3">
              <User size={18} className="text-yellow-600" />
              <span className="font-medium text-yellow-800">预约人信息</span>
            </div>
            {instrument.booker_name && (
              <div className="mb-2 text-sm">
                <span className="text-gray-500">姓名：</span>
                <span className="text-gray-800">{instrument.booker_name}</span>
              </div>
            )}
            {instrument.booker_phone && (
              <div className="mb-2 text-sm">
                <span className="text-gray-500">电话：</span>
                <span className="text-gray-800">{instrument.booker_phone}</span>
              </div>
            )}
            {instrument.booker_email && (
              <div className="mb-2 text-sm">
                <span className="text-gray-500">邮箱：</span>
                <span className="text-gray-800">{instrument.booker_email}</span>
              </div>
            )}
            {instrument.delivery_address && (
              <div className="text-sm">
                <span className="text-gray-500">收货地址：</span>
                <span className="text-gray-800">{instrument.delivery_address}</span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Action Buttons */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <div className="grid grid-cols-3 gap-3">
          {instrument.stock_status === 'available' && (
            <button
              onClick={handleArchive}
              disabled={actionLoading}
              className="py-3 bg-gray-600 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <Archive size={18} />
              下架
            </button>
          )}
          {instrument.stock_status === 'reserved' && (
            <button
              onClick={handleShip}
              className="py-3 bg-blue-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <Truck size={18} />
              发货
            </button>
          )}
          {instrument.stock_status === 'returning' && (
            <button
              onClick={handleReceive}
              disabled={actionLoading || !activeOrder}
              className="py-3 bg-green-600 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <RotateCcw size={18} />
              接收确认
            </button>
          )}
          {instrument.stock_status === 'maintenance' && (
            <button
              onClick={handleCompleteMaintenance}
              disabled={actionLoading}
              className="py-3 bg-purple-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <CheckCircle size={18} />
              维修完成
            </button>
          )}
        </div>
      </div>
    </div>
  )
}
