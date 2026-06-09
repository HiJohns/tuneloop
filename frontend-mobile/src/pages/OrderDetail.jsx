import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { ArrowLeft, User, MapPin, Calendar, Clock, Truck, Package, RotateCcw, CreditCard, XCircle, AlertTriangle, CheckCircle } from 'lucide-react'

const STATUS_LABELS = {
  reserved: '已预约',
  paid: '待发货',
  pending_shipment: '待发货',
  in_transit: '运输中',
  shipped: '已送达',
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

export default function OrderDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
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

  const handlePay = async () => {
    if (!confirm('确认支付该订单？')) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/pay`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        setOrder(prev => ({ ...prev, status: 'paid' }))
      } else {
        alert('支付失败: ' + result.message)
      }
    } catch (err) {
      alert('支付失败: ' + err.message)
    }
    setActionLoading(false)
  }

  const handleCancel = async () => {
    if (!confirm('确认取消该订单？取消后不可恢复。')) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/cancel`, {
        method: 'POST',
      })
      const result = await resp.json()
      if (result.code === 20000) {
        setOrder(prev => ({ ...prev, status: 'cancelled' }))
      } else {
        alert('取消失败: ' + result.message)
      }
    } catch (err) {
      alert('取消失败: ' + err.message)
    }
    setActionLoading(false)
  }

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
  const deposit = order.deposit || 0
  const monthlyRent = order.monthly_rent || 0

  const isOverdue = (status === 'expired' || status === 'in_lease') && endDate !== '-' && new Date(endDate) < new Date()
  const overdueDaysCalc = isOverdue ? Math.ceil((new Date() - new Date(endDate)) / (1000 * 60 * 60 * 24)) : 0
  const overdueFee = isOverdue ? ((monthlyRent / 30) * overdueDaysCalc).toFixed(2) : 0

  const showPayButton = status === 'reserved'
  const showCancelButton = status === 'paid' || status === 'pending_shipment' || status === 'in_transit'
  const showReceiveButton = status === 'shipped'
  const showReturnButton = status === 'in_lease' || status === 'expired'
  const terminal = ['returning', 'returned', 'completed', 'cancelled', 'transferred']
  const isTerminal = terminal.includes(status)

  return (
    <div className="min-h-screen bg-brand-bg pb-24">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">订单详情</h1>
      </div>

      {/* Order ID */}
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

      {/* Overdue warning */}
      {isOverdue && (
        <div className="mx-4 mt-3 bg-red-50 border border-red-200 rounded-xl p-4">
          <div className="flex items-start gap-3">
            <AlertTriangle size={20} className="text-red-500 mt-0.5" />
            <div>
              <p className="text-sm font-medium text-red-700">租约已超期</p>
              <p className="text-xs text-red-600 mt-1">
                超期 {overdueDaysCalc} 天 · 累计逾期费 ¥{overdueFee}
                <span className="block mt-0.5">（¥{(monthlyRent / 30).toFixed(2)}/天）</span>
              </p>
            </div>
          </div>
        </div>
      )}

      {/* Contact Info */}
      <div className="mt-3 bg-white px-4 py-4">
        <h3 className="text-sm font-medium text-gray-900 mb-3">配送信息</h3>
        <div className="space-y-3">
          {order.user_name && (
            <div className="flex items-center gap-3">
              <User size={18} className="text-gray-400" />
              <div>
                <p className="text-xs text-gray-400">下单人</p>
                <p className="text-sm font-medium">{order.user_name}</p>
              </div>
            </div>
          )}
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
              <p className="text-xs text-gray-400">预计租期</p>
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

      {/* Price Info */}
      <div className="mt-3 bg-white px-4 py-4">
        <h3 className="text-sm font-medium text-gray-900 mb-3">费用信息</h3>
        <div className="space-y-2">
          <div className="flex justify-between text-sm">
            <span className="text-gray-500">月租金</span>
            <span className="font-medium">¥{monthlyRent}</span>
          </div>
          <div className="flex justify-between text-sm">
            <span className="text-gray-500">押金</span>
            <span className="font-medium">¥{deposit}</span>
          </div>
          {overdueFee > 0 && (
            <div className="flex justify-between text-sm">
              <span className="text-red-500">逾期费</span>
              <span className="font-medium text-red-500">¥{overdueFee}</span>
            </div>
          )}
        </div>
      </div>

      {/* Logistics */}
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
          {showPayButton && (
            <button
              onClick={handlePay}
              disabled={actionLoading}
              className="w-full py-3 bg-brand-primary text-white rounded-lg font-medium flex items-center justify-center gap-2 disabled:opacity-50"
            >
              <CreditCard size={20} />
              {actionLoading ? '处理中...' : '支付'}
            </button>
          )}
          {showCancelButton && (
            <button
              onClick={handleCancel}
              disabled={actionLoading}
              className="w-full py-3 bg-red-500 text-white rounded-lg font-medium flex items-center justify-center gap-2 disabled:opacity-50"
            >
              <XCircle size={20} />
              {actionLoading ? '处理中...' : '取消订单'}
            </button>
          )}
          {showReceiveButton && (
            <button
              onClick={() => navigate(`/receive/${id}`)}
              className="w-full py-3 bg-green-600 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <CheckCircle size={20} />
              确认收货
            </button>
          )}
          {showReturnButton && (
            <button
              onClick={() => navigate(`/return/${id}`)}
              className="w-full py-3 bg-orange-500 text-white rounded-lg font-medium flex items-center justify-center gap-2"
            >
              <RotateCcw size={20} />
              归还
            </button>
          )}
          {isTerminal && (
            <div className="text-center text-sm text-gray-400 py-2 flex items-center justify-center gap-2">
              {status === 'completed' || status === 'returned' ? (
                <><CheckCircle size={16} /> 该订单已完成</>
              ) : status === 'cancelled' ? (
                <><XCircle size={16} /> 该订单已取消</>
              ) : status === 'returning' ? (
                <><Truck size={16} /> 乐器归还中，等待验收</>
              ) : (
                <><CheckCircle size={16} /> 当前状态无操作</>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
