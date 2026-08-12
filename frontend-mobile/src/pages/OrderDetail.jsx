import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { formatDeliveryAddress, formatDisplayDate } from '../utils/format'
import { dialog, env } from '../platform'
import { calculateDays, calculateEndDate } from '../utils/daycalc'
import InstrumentInfo from '../components/InstrumentInfo'
import LeaseInfo from '../components/LeaseInfo'
import { ArrowLeft, User, MapPin, Truck, Package, RotateCcw, CreditCard, XCircle, AlertTriangle, CheckCircle, Clock, Calendar, Banknote } from 'lucide-react'

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

const LIFECYCLE_ORDER = [
  'created', 'paid', 'pending_shipment', 'shipped', 'in_transit',
  'delivered', 'in_lease', 'returning', 'returned', 'completed',
  'settlement_confirmed', 'cancelled', 'expired', 'pickup_confirmed',
  'damage_assessed', 'return_inspected',
]

const EVENT_LABELS = {
  created: '下单',
  paid: '已付款',
  pending_shipment: '待发货',
  shipped: '已发货',
  in_transit: '运输中',
  delivered: '已收货',
  in_lease: '租赁中',
  returning: '归还中',
  returned: '已归还',
  completed: '已完成',
  cancelled: '已取消',
  expired: '已超期',
  settlement_confirmed: '结算确认',
  pickup_confirmed: '已提货',
  damage_assessed: '定损完成',
  return_inspected: '验货完成',
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
  const [refunding, setRefunding] = useState(false)
  const [instrument, setInstrument] = useState(null)
  const [orderLogs, setOrderLogs] = useState([])
  const [logPage, setLogPage] = useState(1)
  const [logHasMore, setLogHasMore] = useState(false)
  const [showContract, setShowContract] = useState(false)
  const baseUrl = env.apiBaseUrl

  const token = getToken()
  const isStaff = (() => {
    try {
      if (!token) return false
      const payload = JSON.parse(atob(token.split('.')[1]))
      const hasOrg = !!(payload?.oid && payload.oid !== '')
      const hasTenant = !!(payload?.tid && payload.tid !== '')
      const hasStaffRole = payload?.role && payload.role !== 'USER'
      return hasOrg || hasTenant || hasStaffRole
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
    if (id) fetchOrder()
  }, [id])

  useEffect(() => {
    if (!order) return
    const hasSettlement = !!order.settlement && order.settlement.actual_rent_amount !== undefined
    setShowContract(!hasSettlement)
  }, [order?.id])

  const fetchLogs = async (page, append) => {
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/logs?page=${page}&pageSize=15`)
      const res = await resp.json()
      if (res.code === 20000 && res.data) {
        setOrderLogs(prev => append ? [...prev, ...(res.data.logs || [])] : (res.data.logs || []))
        setLogHasMore(page * 15 < (res.data.total || 0))
        setLogPage(page)
      }
    } catch {}
  }

  useEffect(() => {
    if (!id) return
    fetchLogs(1, false)
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

  const handlePay = () => {
    navigate(`/payment?type=rent&id=${id}`, { replace: true })
  }

  const handleCancel = async () => {
    if (!dialog.confirm('确认取消该订单？取消后不可恢复。')) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/cancel-by-user`, {
        method: 'POST',
      })
      const result = await resp.json()
      if (result.code === 20000) {
        if (result.data?.refund_amount > 0) {
          // paid/pending_shipment: full original payment refunded — go to refund page
          navigate(`/payment?type=refund&id=${id}`, { replace: true })
        } else {
          setOrder(prev => ({ ...prev, status: 'cancelled' }))
          // Reload order to sync cancelled state + hide cancel button (#1623)
          try {
            const reload = await apiFetch(`${baseUrl}/orders/${id}`)
            const reloadData = await reload.json()
            if (reloadData.code === 20000) setOrder(reloadData.data)
          } catch { /* keep local state */ }
        }
      } else {
        dialog.alert('取消失败: ' + result.message)
      }
    } catch (err) {
      dialog.alert('取消失败: ' + err.message)
    }
    setActionLoading(false)
  }

  // Staff-triggered refund for deposit_refunding orders (L-04 path 2/3).
  // Executes the differential settlement on the backend and jumps to the
  // refund receipt page.
  // Staff-triggered refund for deposit_refunding orders (L-04 path 2/3).
  // Executes the differential settlement on the backend and jumps to the
  // refund receipt page.
  const handleStaffRefund = async () => {
    if (!dialog.confirm('确认执行退款？将按结算差额退回现金与赠点。')) return
    setRefunding(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/refund`, {
        method: 'POST',
      })
      const result = await resp.json()
      if (result.code === 20000) {
        navigate(`/payment?type=refund&id=${id}`, { replace: true })
      } else {
        dialog.alert('退款失败: ' + (result.message || '请重试'))
      }
    } catch (err) {
      dialog.alert('退款失败: ' + err.message)
    }
    setRefunding(false)
  }

  // Staff cancels a deposit-waived order when the guarantor fails the
  // requirement (#1557 / L-07 seq6). Aligns with the weapp OrderDetail.
  const handleStaffCancel = async () => {
    if (!dialog.confirm('确认取消订单？取消后不可恢复，已付款将原路退回。')) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${id}/staff-cancel`, {
        method: 'POST',
        body: JSON.stringify({ reason: '担保人不符合要求' }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        dialog.toast('订单已取消')
        // Reload order to reflect cancelled status + refund info
        try {
          const reload = await apiFetch(`${baseUrl}/orders/${id}`)
          const reloadData = await reload.json()
          if (reloadData.code === 20000) setOrder(reloadData.data)
        } catch { /* keep current state */ }
      } else {
        dialog.alert('取消失败: ' + (result.message || '请重试'))
      }
    } catch (err) {
      dialog.alert('取消失败: ' + err.message)
    }
    setActionLoading(false)
  }

  if (loading) {
    return (
      <View className="min-h-screen pb-20" style={{backgroundColor: '#FDFBF7'}}>
        {!env.isMiniProgram && (
          <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
            <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
            <Text className="text-lg font-bold">订单详情</Text>
          </View>
        )}
        <View className="text-center text-zinc-500 py-12 font-black">加载中...</View>
      </View>
    )
  }

  if (!order) {
    return (
      <View className="min-h-screen pb-20" style={{backgroundColor: '#FDFBF7'}}>
        {!env.isMiniProgram && (
          <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
            <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
            <Text className="text-lg font-bold">订单详情</Text>
          </View>
        )}
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
  // Shipping fee is filled by staff at dispatch (#1541); shown only from
  // shipped state onwards, hidden at order time — #1570.
  const pbShippingFee = Number(order.pricing_breakdown?.shipping_fee || 0)
  const showShippingFee = pbShippingFee > 0 && [
    'shipped', 'in_transit', 'in_lease', 'returning', 'returned',
    'completed', 'damage_appealing', 'pending_damage_response',
    'deposit_refunding', 'expired', 'transferred',
  ].includes(status)

  const startDate = formatDisplayDate(order.start_date)
  const endDate = formatDisplayDate(order.end_date)
  const leaseTerm = order.lease_term || 0
  const effStartDate = order.delivered_at || order.start_date
  const rentalDays = (effStartDate && (order.end_date || order.returned_at))
    ? calculateDays(new Date(effStartDate), new Date(order.returned_at || order.end_date))
    : leaseTerm * 30
   const deposit = order.deposit || 0
   const pb = order.pricing_breakdown
   const rentSubtotal = (pb && pb.total_amount) || 0
   const dailyRate = (pb && pb.final_daily_rent) || (pb && pb.base_daily_rent) || 0

   const settlement = order.settlement

   // Actual rental figures: settlement is authoritative once created; before
   // that (returning status with returned_at set) compute from breakdown (#1635).
   const actualReturnedAt = order.returned_at
   const actualDays = settlement?.actual_rent_days != null ? settlement.actual_rent_days
     : ((order.start_date && actualReturnedAt) ? calculateDays(new Date(order.start_date), new Date(actualReturnedAt)) : 0)
   const actualRent = settlement?.actual_rent_amount != null ? settlement.actual_rent_amount
     : (() => {
         if (actualDays < 1) return 0
         const pbO = order.pricing_breakdown
         const segs = pbO?.tier_segments || []
         if (segs.length > 0) {
           let rent = 0
           let cursor = 1
           for (const seg of segs) {
             if (cursor > actualDays) break
             const segDays = Math.min(seg.days, actualDays - cursor + 1)
             if (segDays > 0) rent += segDays * (seg.rate || 0) * (seg.discount ?? 1)
             cursor += seg.days
           }
           return Math.round(rent * 100) / 100
         }
         const rate = pbO?.final_daily_rent || pbO?.base_daily_rent || 0
         return Math.round(rate * actualDays * 100) / 100
       })()
   const showActualRent = (settlement && settlement.actual_rent_amount !== undefined) || (!settlement && !!actualReturnedAt)

   const isOverdue = (status === 'expired' || status === 'in_lease') && order.end_date && order.end_date.slice(0, 10) <= new Date().toISOString().slice(0, 10)
   const overdueDaysCalc = isOverdue ? calculateDays(new Date(order.end_date.slice(0, 10)), new Date()) : 0
   const overdueFee = isOverdue ? (dailyRate > 0 ? dailyRate * overdueDaysCalc : (rentSubtotal / 30) * overdueDaysCalc).toFixed(2) : 0

  const showPayButton = !isStaff && status === 'reserved'
  const showCancelButton = !isStaff && (status === 'reserved' || status === 'paid' || status === 'pending_shipment')
  const showReceiveButton = !isStaff && (status === 'in_transit' || status === 'shipped')
  const showRenewButton = !isStaff && (status === 'in_lease' || status === 'expired')
  const showReturnButton = !isStaff && (status === 'in_lease' || status === 'expired')
  const terminal = ['returning', 'returned', 'completed', 'cancelled', 'transferred']
  const isTerminal = terminal.includes(status)

  const showStaffShip = isStaff && (status === 'paid' || status === 'pending_shipment')
  const showStaffTransit = isStaff && status === 'in_transit'
  const showStaffReceive = isStaff && status === 'returning'
  const showStaffRefund = isStaff && status === 'deposit_refunding'
  // Staff cancel only on cancellable states (paid/pending_shipment) —
  // grouped with ship action, NOT in the guarantor panel (#1623).
  const showStaffCancel = isStaff && order.deposit_waived && (status === 'paid' || status === 'pending_shipment')

  return (
    <View className="h-screen flex flex-col" style={{backgroundColor: '#FDFBF7'}}>
      {!env.isMiniProgram && (
        <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3 flex items-center gap-2">
          <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
          <Text className="text-lg font-black text-black">订单详情</Text>
        </View>
      )}

      <ScrollView className="flex-1 overflow-y-auto">
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
                <Text className="block mt-0.5">（¥{dailyRate.toFixed(2)}/天）</Text>
              </Text>
            </View>
          </View>
        </View>
      )}

      {/* Instrument Info */}
      <View className="mx-4">{instrument && <InstrumentInfo instrument={instrument} onClick={() => navigate(`/instrument/${instrument.id}`)} />}</View>

      {/* Customer Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">配送信息</Text>
        <View className="space-y-3">
          <View className="flex items-start gap-3">
            <User size={18} className="text-zinc-400 mt-0.5" />
            <View className="flex items-start flex-1 min-w-0">
              <Text className="text-xs font-bold text-zinc-400 w-16 flex-shrink-0">下单人</Text>
              <View>
                <Text className="text-sm font-black text-black">{order.user_name || '-'}</Text>
                {order.user_phone && <Text className="text-xs text-zinc-500">{order.user_phone}</Text>}
              </View>
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
      <LeaseInfo
        status={status}
        startDate={order.start_date}
        endDate={order.returned_at || order.end_date}
        deliveredAt={order.delivered_at}
        dailyRate={pb?.final_daily_rent || pb?.base_daily_rent || 0}
        rentDays={rentalDays || pb?.rent_days || 0}
        createdAt={order.created_at}
        orderId={order.id}
        paidAt={order.paid_at}
      />

      {/* Fee Info */}
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">费用信息</Text>
        <View className="space-y-2">
          {/* ① 实付金额 */}
          {order.payment_records?.length > 0 && (
            <>
              <Text className="text-xs font-bold text-zinc-400">实付金额</Text>
              {order.payment_records.map(pr => (
                <View key={pr.id} className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">{pr.method || '支付'}</Text>
                  <Text className="text-zinc-400 text-xs flex-shrink-0 ml-auto mr-2">{pr.created_at ? String(pr.created_at).slice(5, 16) : ''}</Text>
                  <Text className="text-black font-black flex-shrink-0 whitespace-nowrap">¥{Number(pr.amount).toFixed(2)}</Text>
                </View>
              ))}
              <View className="flex justify-between text-sm border-t border-zinc-100 pt-1">
                <Text className="text-zinc-500 font-medium">实付合计</Text>
                <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{order.payment_records.reduce((s, p) => s + Number(p.amount || 0), 0).toFixed(2)}</Text>
              </View>
            </>
          )}

          {/* ② 实际租期与租金 */}
          {showActualRent && (
            <>
              <Text className="text-xs font-bold text-zinc-400 mt-2">实际租期与租金</Text>
              {actualReturnedAt && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">实际结束日期</Text>
                  <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">{formatDisplayDate(actualReturnedAt)}</Text>
                </View>
              )}
              {actualDays > 0 && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">实际租期</Text>
                  <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">{actualDays} 天</Text>
                </View>
              )}
              {actualRent > 0 && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">实际租金</Text>
                  <Text className="text-green-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{Number(actualRent).toFixed(2)}</Text>
                </View>
              )}
              {overdueFee > 0 && (
                <>
                  <View className="flex justify-between text-sm">
                    <Text className="text-zinc-500 font-medium">逾期费用</Text>
                    <Text className="text-red-500 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{overdueFee}</Text>
                  </View>
                  <View className="flex justify-between text-sm">
                    <Text className="text-zinc-400">  逾期日费</Text>
                    <Text className="text-zinc-400 flex-shrink-0 ml-auto whitespace-nowrap">¥{dailyRate.toFixed(2)}/天</Text>
                  </View>
                </>
              )}
            </>
          )}

          {/* ③ 退款 */}
          {settlement && (settlement.cash_refundable > 0 || settlement.prepaid_refunded > 0 || settlement.gift_points_refunded > 0) && (
            <>
              <Text className="text-xs font-bold text-zinc-400 mt-2">退款</Text>
              {settlement.cash_refundable > 0 && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">现金退款</Text>
                  <Text className="text-blue-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.cash_refundable}</Text>
                </View>
              )}
              {settlement.prepaid_refunded > 0 && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">退回预付点</Text>
                  <Text className="text-blue-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.prepaid_refunded}</Text>
                </View>
              )}
              {settlement.gift_points_refunded > 0 && (
                <View className="flex justify-between text-sm">
                  <Text className="text-zinc-500 font-medium">赠送积分退还</Text>
                  <Text className="text-blue-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.gift_points_refunded}</Text>
                </View>
              )}
              <View className="flex justify-between text-sm border-t border-zinc-100 pt-1">
                <Text className="text-zinc-500 font-medium">退款合计</Text>
                <Text className="text-green-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{(Number(settlement.cash_refundable) + Number(settlement.prepaid_refunded) + Number(settlement.gift_points_refunded)).toFixed(2)}</Text>
              </View>
            </>
          )}

          {/* 合同快照 (collapsed) */}
          {(order.pricing_breakdown && typeof order.pricing_breakdown === 'object') && (
            <View className="mt-3 border-t border-dashed border-zinc-200 pt-2">
              <View
                className="flex justify-between items-center cursor-pointer active:opacity-70"
                onClick={() => setShowContract(!showContract)}
              >
                <Text className="text-sm font-bold text-zinc-500">合同快照</Text>
                <Text className="text-xs text-zinc-400">{showContract ? '收起 ▲' : '展开 ▼'}</Text>
              </View>
              {showContract && (
                <View className="space-y-2 mt-2">
                  {order.pricing_breakdown.rent_days && (
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500 font-medium">合同租期（天）</Text>
                      <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">{order.pricing_breakdown.rent_days}</Text>
                    </View>
                  )}
                  {/* Tier-by-tier breakdown */}
                  {(order.pricing_breakdown.pricing_tiers?.length > 0 || (pb?.rent_days && pb?.base_daily_rent)) && (() => {
                    const tiers = order.pricing_breakdown.pricing_tiers || []
                    const days = order.pricing_breakdown.rent_days
                    const baseRate = order.pricing_breakdown.base_daily_rent || order.pricing_breakdown.final_daily_rent || 0
                    let remaining = days
                    let prevMax = 0
                    const rows = []
                    const tierList = tiers.length > 0 ? tiers : [{ days_max: -1, daily_rate: baseRate }]
                    for (const t of tierList) {
                      const tierDays = t.days_max > 0 ? t.days_max - prevMax : remaining
                      const segDays = Math.min(tierDays, remaining)
                      if (segDays <= 0) break
                      const rate = t.daily_rate || baseRate
                      const segAmount = segDays * rate
                      const startDay = prevMax + 1
                      const endDay = startDay + segDays - 1
                      const range = segDays === 1 ? `${startDay}天` : `${startDay}-${endDay}天`
                      rows.push({ range, rate, segDays, segAmount })
                      remaining -= segDays
                      prevMax = t.days_max > 0 ? t.days_max : prevMax + segDays
                    }
                    return (
                      <View className="text-xs text-zinc-400 pl-2 pb-1 border-b border-dashed">
                        {rows.map((r, i) => (
                          <Text key={i} className="block">
                            {r.range}: ¥{r.rate.toFixed(2)}/天 × {r.segDays}天 = ¥{r.segAmount.toFixed(2)}
                          </Text>
                        ))}
                      </View>
                    )
                  })()}
                  {order.pricing_breakdown.total_amount && (
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500 font-medium">合同总额</Text>
                      <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{order.pricing_breakdown.total_amount}</Text>
                    </View>
                  )}
                  {deposit > 0 && !order.deposit_waived && (
                    <>
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500 font-medium">押金</Text>
                      <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{deposit}</Text>
                    </View>
                    {order.pricing_breakdown.deposit_method && (
                      <Text className="text-[10px] text-zinc-400 text-right -mt-1">
                        {order.pricing_breakdown.deposit_method === 'total_price'
                          ? `原价 ¥${order.pricing_breakdown.total_price || 0} × ${order.pricing_breakdown.deposit_ratio || 0}`
                          : (order.pricing_breakdown.deposit_multiplier > 0
                              ? `日租金 ¥${order.pricing_breakdown.base_daily_rent || 0} × ${order.pricing_breakdown.deposit_multiplier}`
                              : '')}
                      </Text>
                    )}
                    </>
                  )}
                  {order.deposit_waived && (
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500 font-medium">押金</Text>
                      <Text className="text-green-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">免押金</Text>
                    </View>
                  )}
                  {showShippingFee && (
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500 font-medium">物流费</Text>
                      <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{pbShippingFee.toFixed(2)}</Text>
                    </View>
                  )}
                </View>
              )}
            </View>
          )}
        </View>
      </View>

      {/* Guarantor info (deposit-free orders, #1557) */}
      {order.deposit_waived && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3">担保人信息</Text>
          {(order.guarantors || []).length > 0 ? order.guarantors.map((g, i) => (
            <View key={g.id || i} className="bg-gray-50 rounded-lg p-2.5 mb-2">
              <Text className="block text-sm font-semibold text-black">{g.name} · {g.phone}</Text>
              {(g.company || g.title) && <Text className="block text-xs text-zinc-500 mt-0.5">{[g.company, g.title].filter(Boolean).join(' / ')}</Text>}
              {g.address && <Text className="block text-xs text-zinc-500 mt-0.5">{g.address}</Text>}
            </View>
          )) : (
            <Text className="text-xs text-zinc-400">暂无担保人信息</Text>
          )}
        </View>
      )}

      {/* Settlement Detail (for completed orders) */}
      {settlement && (
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-3">结算明细</Text>
        <View className="space-y-2">
          {settlement.original_rent_amount !== undefined && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">原始租金</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.original_rent_amount}</Text>
          </View>
          )}
          {settlement.actual_rent_amount !== undefined && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">实收租金</Text>
            <Text className="text-green-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.actual_rent_amount}</Text>
          </View>
          )}
          {settlement.actual_rent_days !== undefined && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">实际天数</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">{settlement.actual_rent_days} 天</Text>
          </View>
          )}
          {settlement.overdue_charges_total !== undefined && Number(settlement.overdue_charges_total) > 0 && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">逾期费用</Text>
            <Text className="text-red-500 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.overdue_charges_total}</Text>
          </View>
          )}
          {settlement.cash_refundable !== undefined && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">可退现金</Text>
            <Text className="text-blue-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.cash_refundable}</Text>
          </View>
          )}
          {settlement.prepaid_refunded !== undefined && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">预付款退还</Text>
            <Text className="text-blue-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.prepaid_refunded}</Text>
          </View>
          )}
          {settlement.gift_points_refunded !== undefined && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">赠送积分退还</Text>
            <Text className="text-blue-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{settlement.gift_points_refunded}</Text>
          </View>
          )}
          {settlement.refund_method && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">退款方式</Text>
            <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">{settlement.refund_method}</Text>
          </View>
          )}
          {settlement.refund_status && (
          <View className="flex justify-between text-sm">
            <Text className="text-zinc-500 font-medium">退款状态</Text>
            <Text className={`font-black flex-shrink-0 ml-auto whitespace-nowrap ${settlement.refund_status === 'completed' ? 'text-green-600' : 'text-orange-500'}`}>
              {settlement.refund_status === 'completed' ? '已退款' : settlement.refund_status === 'pending' ? '处理中' : settlement.refund_status}
            </Text>
          </View>
          )}
        </View>
      </View>
      )}

      {/* 收支明细 — completed/returned orders */}
      {['completed', 'returned'].includes(order?.status) && (order?.payment_records?.length > 0 || order?.refund_records?.length > 0) && (
        <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
          <Text className="text-base font-black text-black mb-3">收支明细</Text>
          <View className="space-y-2">
            {order?.payment_records?.length > 0 && (
              <>
                <Text className="text-xs font-bold text-zinc-400">支付记录</Text>
                {order.payment_records.map(pr => (
                  <View key={pr.id} className="flex justify-between text-sm">
                    <Text className="text-zinc-500 font-medium">{pr.method || '支付'}</Text>
                    <Text className="text-zinc-400 text-xs flex-shrink-0 ml-auto mr-2">{pr.created_at ? String(pr.created_at).slice(5, 16) : ''}</Text>
                    <Text className="text-black font-black flex-shrink-0 whitespace-nowrap">¥{Number(pr.amount).toFixed(2)}</Text>
                  </View>
                ))}
                <View className="flex justify-between text-sm border-t border-zinc-100 pt-1">
                  <Text className="text-zinc-500 font-medium">支付合计</Text>
                  <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{order.payment_records.reduce((s, p) => s + Number(p.amount || 0), 0).toFixed(2)}</Text>
                </View>
              </>
            )}
            {order?.refund_records?.length > 0 && (
              <>
                <Text className="text-xs font-bold text-zinc-400 mt-2">退款记录</Text>
                {order.refund_records.map(rf => (
                  <View key={rf.id} className="flex justify-between text-sm">
                    <Text className="text-zinc-500 font-medium">{rf.method === 'prepaid' ? '退回预付点' : rf.method === 'cash_withdrawal' ? '退回现金' : '退款'}</Text>
                    <Text className="text-zinc-400 text-xs flex-shrink-0 ml-auto mr-2">{rf.created_at ? String(rf.created_at).slice(5, 16) : ''}</Text>
                    <Text className="text-green-600 font-black flex-shrink-0 whitespace-nowrap">-¥{Number(rf.amount).toFixed(2)}</Text>
                  </View>
                ))}
                <View className="flex justify-between text-sm border-t border-zinc-100 pt-1">
                  <Text className="text-zinc-500 font-medium">退款合计</Text>
                  <Text className="text-green-600 font-black flex-shrink-0 ml-auto whitespace-nowrap">-¥{order.refund_records.reduce((s, r) => s + Number(r.amount || 0), 0).toFixed(2)}</Text>
                </View>
              </>
            )}
            {(order?.payment_records?.length > 0 || order?.refund_records?.length > 0) && (() => {
              const paid = (order.payment_records || []).reduce((s, p) => s + Number(p.amount || 0), 0)
              const refunded = (order.refund_records || []).reduce((s, r) => s + Number(r.amount || 0), 0)
              return (
                <View className="flex justify-between text-sm border-t border-zinc-100 pt-2 mt-2">
                  <Text className="text-zinc-900 font-bold">净支出</Text>
                  <Text className="text-black font-black flex-shrink-0 ml-auto whitespace-nowrap">¥{Math.max(0, paid - refunded).toFixed(2)}</Text>
                </View>
              )
            })()}
          </View>
        </View>
      )}

      {/* Order Logs Timeline */}
      {orderLogs.length > 0 && (
      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4">
        <Text className="text-base font-black text-black mb-4">订单日志</Text>
        <View className="space-y-0">
          {orderLogs.map((log, idx) => {
            const statusIdx = LIFECYCLE_ORDER.indexOf(order?.status)
            const eventIdx = LIFECYCLE_ORDER.indexOf(log.event)
            const isFuture = eventIdx >= 0 && statusIdx >= 0 && eventIdx > statusIdx
            const isCurrent = log.event === order?.status
            const dotClass = isCurrent
              ? 'bg-black ring-2 ring-black ring-offset-2'
              : isFuture
                ? 'border-2 border-zinc-300 bg-transparent'
                : 'bg-zinc-300'
          return (
          <View key={idx} className="flex gap-3">
            <View className="flex flex-col items-center">
              <View className={`w-3 h-3 rounded-full mt-1.5 ${dotClass}`} />
              {idx < orderLogs.length - 1 && <View className="w-0.5 flex-1 bg-zinc-200 mt-0.5" />}
            </View>
            <View className="flex-1 pb-4">
              <Text className={`text-sm font-black ${isCurrent ? 'text-black' : isFuture ? 'text-zinc-300' : 'text-zinc-500'}`}>
                {EVENT_LABELS[log.event] || log.event}
              </Text>
              <Text className="text-xs text-zinc-400 mt-0.5">
                {formatDisplayDate(log.time || log.created_at)}
                {log.operator && <Text className="ml-2">by {log.operator}</Text>}
              </Text>
            </View>
          </View>
          )
          })}
        </View>
        {logHasMore && (
          <View onClick={() => fetchLogs(logPage + 1, true)}
            className="mt-3 py-2.5 rounded-xl bg-zinc-100 text-center cursor-pointer active:opacity-70">
            <Text className="text-sm font-black text-zinc-500">加载更多</Text>
          </View>
        )}
      </View>
      )}

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
      <View className="bg-white border-t-2 border-zinc-200 p-4 safe-area-pb" style={{boxShadow:'0 -4px 12px rgba(0,0,0,0.08)'}}>
        <View className="space-y-3 max-w-[480px] mx-auto">
          {isStaff ? (
            <>
              {showStaffShip && (
                <View onClick={() => navigate(`/staff/shipping?order=${id}`)}
                  className="w-full py-3 bg-black text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <Truck size={20} /><Text>发货</Text>
                </View>
              )}
              {showStaffCancel && (
                <View onClick={actionLoading ? undefined : handleStaffCancel}
                  className="w-full py-3 bg-red-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80"
                  style={{ opacity: actionLoading ? 0.5 : 1 }}>
                  {actionLoading ? '处理中...' : '❌ 取消订单'}
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
                  className="w-full py-3 bg-rose-700 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <RotateCcw size={20} /><Text>接收</Text>
                </View>
              )}
              {showStaffRefund && (
                <View onClick={handleStaffRefund}
                  className="w-full py-3 bg-amber-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                  <Banknote size={20} /><Text>{refunding ? '退款处理中...' : '退款'}</Text>
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
              {(showRenewButton || showReturnButton) && (
                <View className="flex gap-3">
                  {showRenewButton && (
                    <View onClick={() => navigate(`/renewal/${id}`)}
                      className="flex-1 py-3 bg-blue-600 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                      <Calendar size={20} />续期
                    </View>
                  )}
                  {showReturnButton && (
                    <View onClick={() => navigate(`/return/${id}?instrument=${order.instrument_id}`)}
                      className="flex-1 py-3 bg-orange-500 text-white rounded-2xl font-black flex items-center justify-center gap-2 cursor-pointer active:opacity-80">
                      <RotateCcw size={20} />归还
                    </View>
                  )}
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
