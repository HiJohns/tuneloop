import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { formatDeliveryAddress, formatDisplayDate } from '../utils/format'
import { dialog, env } from '../platform'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'
import { ArrowLeft, User, MapPin, Truck, Package, RotateCcw, CreditCard, XCircle, AlertTriangle, CheckCircle, Clock } from 'lucide-react'

const STATUS_LABELS = {
  reserved: '未支付',
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

export default function OrderDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [order, setOrder] = useState(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [instrument, setInstrument] = useState(null)
  const baseUrl = env.apiBaseUrl

  const token = getToken()
  const isStaff = (() => {
    try {
      if (!token) return false
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload?.role && payload.role !== 'USER'
    } catch { return false }
  })()

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

  useEffect(() => {
    if (!order?.instrument_id) return
    apiFetch(`${baseUrl}/public/instruments/${order.instrument_id}`)
      .then(r => r.json())
      .then(res => {
        if (res.code === 20000) setInstrument(res.data)
      })
      .catch(() => {})
  }, [order?.instrument_id])

  const handlePay = async () => {
    if (!dialog.confirm('确认支付该订单？')) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/pay`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
          navigate('/my-leases')
      } else {
        dialog.alert('支付失败: ' + result.message)
      }
    } catch (err) {
      dialog.alert('支付失败: ' + err.message)
    }
    setActionLoading(false)
  }

  const handleCancel = async () => {
    if (!dialog.confirm('确认取消该订单？取消后不可恢复。')) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/cancel`, {
        method: 'POST',
      })
      const result = await resp.json()
      if (result.code === 20000) {
        setOrder(prev => ({ ...prev, status: 'cancelled' }))
      } else {
        dialog.alert('取消失败: ' + result.message)
      }
    } catch (err) {
      dialog.alert('取消失败: ' + err.message)
    }
    setActionLoading(false)
  }

  if (loading) {
    return (
      <View className="min-h-screen bg-brand-bg pb-20">
        <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
          <Text className="text-lg font-bold">订单详情</Text>
        </View>
        <View className="text-center text-gray-500 py-12">加载中...</View>
      </View>
    )
  }

  if (!order) {
    return (
      <View className="min-h-screen bg-brand-bg pb-20">
        <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
          <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
          <Text className="text-lg font-bold">订单详情</Text>
        </View>
        <View className="text-center text-gray-400 py-12">
          <Package size={48} className="mx-auto mb-3 opacity-50" />
          <Text>订单未找到</Text>
        </View>
      </View>
    )
  }

  const status = order.status || ''
  const statusLabel = STATUS_LABELS[status] || status
  const statusColor = STATUS_COLORS[status] || 'bg-gray-100'

  const startDate = formatDisplayDate(order.start_date)
  const endDate = formatDisplayDate(order.end_date)
  const leaseTerm = order.lease_term || 0
  const rentalDays = leaseTerm * 30
  const deposit = order.deposit || 0
  const monthlyRent = order.monthly_rent || 0
  const shippingFee = order.shipping_fee || 0

  const isOverdue = (status === 'expired' || status === 'in_lease') && endDate !== '-' && new Date(endDate) < new Date()
  const overdueDaysCalc = isOverdue ? Math.ceil((new Date() - new Date(endDate)) / (1000 * 60 * 60 * 24)) : 0
  const overdueFee = isOverdue ? ((monthlyRent / 30) * overdueDaysCalc).toFixed(2) : 0
  const totalAmount = monthlyRent + deposit + shippingFee + (overdueFee > 0 ? Number(overdueFee) : 0)

  const showPayButton = status === 'reserved'
  const showCancelButton = status === 'paid' || status === 'pending_shipment' || status === 'in_transit'
  const showReceiveButton = status === 'shipped'
  const showReturnButton = status === 'in_lease' || status === 'expired'
  const terminal = ['returning', 'returned', 'completed', 'cancelled', 'transferred']
  const isTerminal = terminal.includes(status)

  const showStaffShip = isStaff && (status === 'paid' || status === 'pending_shipment')
  const showStaffTransit = isStaff && status === 'in_transit'
  const showStaffReceive = isStaff && status === 'returning'

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-24">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
        <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
        <Text className="text-lg font-black text-black">订单详情</Text>
      </View>

      <ScrollView>
      {/* Order ID + Status */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <View className="flex items-center justify-between min-w-0 mb-2">
          <Text className="text-base font-black text-black">订单编号</Text>
          <Text className={`text-xs px-3 py-1 rounded-full font-black flex-shrink-0 ${statusColor}`}>
            {statusLabel}
          </Text>
        </View>
        <Text className="text-xs font-black text-black tracking-wide truncate">{id}</Text>
      </View>

      {/* Overdue warning */}
      {isOverdue && (
        <View className="mx-4 mt-3 bg-red-50 border border-red-200 rounded-xl p-4">
          <View className="flex items-start gap-3">
            <AlertTriangle size={20} className="text-red-500 mt-0.5" />
            <View>
              <Text className="text-sm font-black text-red-700">租约已超期</Text>
              <Text className="text-xs text-red-600 mt-1">
                超期 {overdueDaysCalc} 天 · 累计逾期费 ¥{overdueFee}
                <Text className="block mt-0.5">（¥{(monthlyRent / 30).toFixed(2)}/天）</Text>
              </Text>
            </View>
          </View>
        </View>
      )}

      {/* Instrument Info */}
      {instrument && <InstrumentInfo instrument={instrument} onClick={() => navigate(`/instrument/${instrument.id}`)} />}

      {/* Customer Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">配送信息</Text>
        <View className="space-y-3">
          <View className="flex items-start gap-3">
            <User size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">下单人</Text>
              <Text className="text-sm font-black text-black truncate">{order.user_name || order.user_email || order.user_phone || '-'}</Text>
            </View>
          </View>
          <View className="flex items-start gap-3">
            <MapPin size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">收货地址</Text>
              <Text className="text-sm font-medium text-black">{formatDeliveryAddress(order.delivery_address) || '-'}</Text>
            </View>
          </View>
        </View>
      </View>

      {/* Lease Info */}
      <LeaseInfo startDate={startDate} endDate={endDate} leaseTerm={leaseTerm} rentalDays={rentalDays} />

      {/* Fee Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">费用信息</Text>
        <View className="space-y-2">
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">租金</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{monthlyRent}</Text>
          </View>
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">押金</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{deposit}</Text>
          </View>
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">物流费</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{shippingFee}</Text>
          </View>
          {overdueFee > 0 && (
          <>
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">逾期费用</Text>
            <Text className="text-red-500 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{overdueFee}</Text>
          </View>
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-400">  逾期日费</Text>
            <Text className="text-zinc-400 flex-shrink-0 ml-auto whitespace-nowrap">¥{(monthlyRent / 30).toFixed(2)}/天</Text>
          </View>
          </>
          )}
          <View className="flex justify-between text-sm border-t border-zinc-100 pt-2 mt-2">
            <Text className="text-zinc-900 font-bold">合计</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{totalAmount}</Text>
          </View>
        </View>
      </View>

      {/* Logistics */}
      {(order.tracking_number || order.courier_company) && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3">物流信息</Text>
          <View className="space-y-3">
            {order.courier_company && (
            <View className="flex items-start gap-3">
              <Truck size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">物流公司</Text>
                <Text className="text-sm font-black text-black">{order.courier_company}</Text>
              </View>
            </View>
            )}
            {order.tracking_number && (
            <View className="flex items-start gap-3">
              <Package size={18} className="text-zinc-400 mt-0.5" />
              <View className="flex items-start flex-1 min-w-0">
                <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">物流单号</Text>
                <Text className="text-sm font-mono font-black text-black">{order.tracking_number}</Text>
              </View>
            </View>
            )}
          </View>
        </View>
      )}
      </ScrollView>

      {/* Action Buttons */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 safe-area-pb shadow-2xl">
        <View className="space-y-3 max-w-[480px] mx-auto">
          {isStaff ? (
            <>
              {showStaffShip && (
                <View onClick={() => navigate(`/staff/shipping?order=${id}`)}
                  className="w-full py-3 bg-black text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <Truck size={20} /><Text>发货</Text>
                </View>
              )}
              {showStaffTransit && (
                <View onClick={() => navigate(`/staff/shipping?order=${id}`)}
                  className="w-full py-3 bg-cyan-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <Truck size={20} /><Text>接收并转发</Text>
                </View>
              )}
              {showStaffReceive && (
                <View onClick={() => navigate(`/staff/receiving?order_id=${id}`)}
                  className="w-full py-3 bg-[#C21838] text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <RotateCcw size={20} /><Text>收货</Text>
                </View>
              )}
              {(status === 'reserved' || status === 'cancelled' || status === 'shipped' ||
                status === 'in_lease' || status === 'expired' || status === 'returned' ||
                status === 'completed' || status === 'transferred') && (
                <View className="w-full py-3 rounded-2xl font-black text-zinc-500 flex items-center justify-center gap-2">
                  {status === 'reserved' ? (<><Clock size={16} /> 未支付</>)
                  : status === 'shipped' ? (<><CheckCircle size={16} /> 乐器已发货，等待用户签收</>)
                  : status === 'in_lease' ? (<><CheckCircle size={16} /> 租赁中</>)
                  : status === 'expired' ? (<><AlertTriangle size={16} /> 租约已超期</>)
                  : status === 'returned' || status === 'completed' ? (<><CheckCircle size={16} /> 该订单已完成</>)
                  : status === 'cancelled' ? (<><XCircle size={16} /> 该订单已取消</>)
                  : status === 'transferred' ? (<><CheckCircle size={16} /> 已过户</>) : null}
                </View>
              )}
            </>
          ) : (
            <>
              {showPayButton && (
                <Button onClick={handlePay} disabled={actionLoading}
                  className="w-full py-3 bg-black text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
                  <CreditCard size={20} />{actionLoading ? '处理中...' : '支付'}
                </Button>
              )}
              {showCancelButton && (
                <Button onClick={handleCancel} disabled={actionLoading}
                  className="w-full py-3 bg-red-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 disabled:opacity-50">
                  <XCircle size={20} />{actionLoading ? '处理中...' : '取消订单'}
                </Button>
              )}
              {showReceiveButton && (
                <View onClick={() => navigate(`/receive/${id}?instrument=${order.instrument_id}`)}
                  className="w-full py-3 bg-green-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <CheckCircle size={20} />确认收货
                </View>
              )}
              {showReturnButton && (
                <View onClick={() => navigate(`/return/${id}?instrument=${order.instrument_id}`)}
                  className="w-full py-3 bg-orange-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <RotateCcw size={20} />归还
                </View>
              )}
              {isTerminal && (
                <View className="w-full py-3 rounded-2xl font-black text-zinc-500 flex items-center justify-center gap-2">
                  {status === 'completed' || status === 'returned' ? (<><CheckCircle size={16} /> 该订单已完成</>)
                  : status === 'cancelled' ? (<><XCircle size={16} /> 该订单已取消</>)
                  : status === 'returning' ? (<><RotateCcw size={16} /> 乐器归还中，等待验收</>)
                  : status === 'transferred' ? (<><CheckCircle size={16} /> 已过户</>)
                  : (<>{statusLabel}</>)}
                </View>
              )}
            </>
          )}
        </View>
      </View>
    </View>
  )
}
