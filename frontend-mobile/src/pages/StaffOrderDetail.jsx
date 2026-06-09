import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, User, MapPin, Calendar, Clock, Package, Truck, RotateCcw, CheckCircle, XCircle, AlertTriangle } from 'lucide-react'

const STATUS_LABELS = {
  reserved: '已预约',
  paid: '待发货',
  pending_shipment: '待发货',
  in_transit: '运输中',
  shipped: '已发货',
  in_lease: '租赁中',
  returning: '归还中',
  returned: '已归还',
  completed: '已完成',
  cancelled: '已取消',
  expired: '超期',
  transferred: '已过户',
}

const STATUS_COLORS = {
  reserved: 'bg-blue-100 text-blue-700',
  paid: 'bg-orange-100 text-orange-700',
  pending_shipment: 'bg-orange-100 text-orange-700',
  in_transit: 'bg-cyan-100 text-cyan-700',
  shipped: 'bg-green-100 text-green-700',
  in_lease: 'bg-indigo-100 text-indigo-700',
  returning: 'bg-yellow-100 text-yellow-700',
  returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600',
  cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700',
  transferred: 'bg-purple-100 text-purple-700',
}

export default function StaffOrderDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  useEffect(() => {
    const fetchOrder = async () => {
      setLoading(true)
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${id}`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrder(result.data)
        }
      } catch (err) {
        console.error('Failed to fetch order:', err)
      }
      setLoading(false)
    }
    fetchOrder()
  }, [id])

  if (loading) {
    return (
      <div className="min-h-screen bg-brand-bg pb-20">
        <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
          <h1 className="text-lg font-bold">订单详情</h1>
        </div>
        <div className="text-center text-gray-500 py-12">加载中...</div>
      </div>
    )
  }

  if (!order) {
    return (
      <div className="min-h-screen bg-brand-bg pb-20">
        <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
          <h1 className="text-lg font-bold">订单详情</h1>
        </div>
        <div className="text-center text-gray-400 py-12">
          <Package size={48} className="mx-auto mb-3 opacity-50" />
          <p>订单未找到</p>
        </div>
      </div>
    )
  }

  const status = order.status || ''
  const statusLabel = STATUS_LABELS[status] || status
  const statusColor = STATUS_COLORS[status] || 'bg-gray-100'

  const startDate = order.start_date || '-'
  const endDate = order.end_date || '-'
  const leaseTerm = order.lease_term || 0
  const rentalDays = leaseTerm * 30

  const showShipButton = status === 'paid' || status === 'pending_shipment'
  const showTransitButton = status === 'in_transit'
  const showReceiveButton = status === 'returning'

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">订单详情</h1>
      </div>

      {/* Order ID Banner */}
      <div className="bg-white px-4 py-4 border-b">
        <p className="text-xs text-gray-400 mb-1">订单编号</p>
        <p className="text-xl font-mono font-bold text-gray-900 tracking-wide">{id}</p>
      </div>

      {/* Status */}
      <div className="bg-white px-4 py-3 border-b">
        <div className="flex items-center justify-between">
          <span className="text-sm text-gray-500">当前状态</span>
          <span className={`text-sm px-3 py-1 rounded-full font-medium ${statusColor}`}>
            {statusLabel}
          </span>
        </div>
      </div>

      {/* Customer Info */}
      <div className="mt-3 bg-white px-4 py-4">
        <h3 className="text-sm font-medium text-gray-900 mb-3">客户信息</h3>
        <div className="space-y-3">
          <div className="flex items-center gap-3">
            <User size={18} className="text-gray-400" />
            <div>
              <p className="text-xs text-gray-400">下单人</p>
              <p className="text-sm font-medium">{order.user_name || order.user_id?.slice(0, 8) || '-'}</p>
            </div>
          </div>
          {order.delivery_address && (
            <div className="flex items-start gap-3">
              <MapPin size={18} className="text-gray-400 mt-0.5" />
              <div>
                <p className="text-xs text-gray-400">收货地址</p>
                <p className="text-sm font-medium">{order.delivery_address}</p>
              </div>
            </div>
          )}
        </div>
      </div>

      {/* Lease Info */}
      <div className="mt-3 bg-white px-4 py-4">
        <h3 className="text-sm font-medium text-gray-900 mb-3">租期信息</h3>
        <div className="space-y-3">
          <div className="flex items-center gap-3">
            <Calendar size={18} className="text-gray-400" />
            <div>
              <p className="text-xs text-gray-400">租期起点</p>
              <p className="text-sm font-medium">{startDate}</p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Clock size={18} className="text-gray-400" />
            <div>
              <p className="text-xs text-gray-400">预计租期天数</p>
              <p className="text-sm font-medium">{rentalDays} 天（{leaseTerm} 个月）</p>
            </div>
          </div>
          <div className="flex items-center gap-3">
            <Calendar size={18} className="text-gray-400" />
            <div>
              <p className="text-xs text-gray-400">预计到期日</p>
              <p className="text-sm font-medium">{endDate}</p>
            </div>
          </div>
        </div>
      </div>

      {/* Logistics Info */}
      {(order.tracking_number || order.courier_company) && (
        <div className="mt-3 bg-white px-4 py-4">
          <h3 className="text-sm font-medium text-gray-900 mb-3">物流信息</h3>
          <div className="space-y-3">
            {order.courier_company && (
              <div className="flex items-center gap-3">
                <Truck size={18} className="text-gray-400" />
                <div>
                  <p className="text-xs text-gray-400">物流公司</p>
                  <p className="text-sm font-medium">{order.courier_company}</p>
                </div>
              </div>
            )}
            {order.tracking_number && (
              <div className="flex items-center gap-3">
                <Package size={18} className="text-gray-400" />
                <div>
                  <p className="text-xs text-gray-400">物流单号</p>
                  <p className="text-sm font-mono">{order.tracking_number}</p>
                </div>
              </div>
            )}
          </div>
        </div>
      )}

      {/* Action Buttons */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <div className="space-y-3">
          {showShipButton && (
            <button
              onClick={() => navigate(`/staff/shipping?order_id=${id}`)}
              className="w-full py-3 bg-blue-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <Truck size={20} />
              发货
            </button>
          )}
          {showTransitButton && (
            <button
              onClick={() => navigate(`/staff/shipping?order_id=${id}`)}
              className="w-full py-3 bg-cyan-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <Truck size={20} />
              接收并转发
            </button>
          )}
          {showReceiveButton && (
            <button
              onClick={() => navigate(`/staff/receiving?order_id=${id}`)}
              className="w-full py-3 bg-green-600 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <RotateCcw size={20} />
              收货
            </button>
          )}
          {(status === 'reserved' || status === 'cancelled' || status === 'shipped' ||
            status === 'in_lease' || status === 'expired' || status === 'returned' ||
            status === 'completed' || status === 'transferred') && (
            <div className="text-center text-sm text-gray-400 py-2 flex items-center justify-center gap-2">
              {status === 'reserved' ? (
                <><Clock size={16} /> 等待用户支付</>
              ) : status === 'shipped' ? (
                <><CheckCircle size={16} /> 乐器已发货，等待用户签收</>
              ) : status === 'in_lease' ? (
                <><CheckCircle size={16} /> 租赁中</>
              ) : status === 'expired' ? (
                <><AlertTriangle size={16} className="text-red-400" /> 租约已超期</>
              ) : status === 'returned' || status === 'completed' ? (
                <><CheckCircle size={16} /> 该订单已完成</>
              ) : status === 'cancelled' ? (
                <><XCircle size={16} /> 该订单已取消</>
              ) : status === 'transferred' ? (
                <><CheckCircle size={16} /> 已过户</>
              ) : (
                <><Clock size={16} /> 当前状态无操作</>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
