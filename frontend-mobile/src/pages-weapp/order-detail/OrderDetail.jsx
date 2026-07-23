import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Image } from '@tarojs/components'
import { apiFetch, getToken } from '../../services/api'
import { env } from '../../platform'
import { formatDeliveryAddress, formatDisplayDate } from '../../utils/format'
import LeaseInfo from '../../components/LeaseInfo'

const STATUS = {
  reserved: { color: '#f59e0b', label: '未支付' },
  paid: { color: '#3b82f6', label: '待发货' },
  pending_shipment: { color: '#3b82f6', label: '待发货' },
  in_transit: { color: '#06b6d4', label: '运输中' },
  shipped: { color: '#22c55e', label: '已发货' },
  in_lease: { color: '#6366f1', label: '租赁中' },
  returning: { color: '#eab308', label: '归还中' },
  returned: { color: '#a1a1aa', label: '已归还' },
  completed: { color: '#a1a1aa', label: '已完成' },
  cancelled: { color: '#ef4444', label: '已取消' },
  expired: { color: '#ef4444', label: '超期' },
  transferred: { color: '#a855f7', label: '已过户' },
}

const EVENT_LABELS = {
  created: '下单', paid: '已付款', pending_shipment: '待发货',
  shipped: '已发货', in_transit: '运输中', delivered: '已收货',
  in_lease: '租赁中', returning: '归还中', returned: '已归还',
  completed: '已完成', cancelled: '已取消', expired: '已超期',
}

const baseUrl = env.apiBaseUrl

export default function OrderDetail() {
  const params = Taro.getCurrentInstance().router?.params || {}
  const [id, setId] = useState(params.id || null)
  const [order, setOrder] = useState(null)
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [actionLoading, setActionLoading] = useState(false)
  const [allLogs, setAllLogs] = useState([])
  const [logPage, setLogPage] = useState(1)
  const [logHasMore, setLogHasMore] = useState(false)

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
    const resolve = async () => {
      if (!params.id && params.out_trade_no) {
        try {
          const resp = await apiFetch(`${baseUrl}/orders/by-trade-no/${params.out_trade_no}`)
          const result = await resp.json()
          if (result.code === 20000 && result.data?.orders?.length > 0) {
            setId(result.data.orders[0].id)
            return
          }
        } catch {}
      }
      setId(params.id || null)
    }
    resolve()
  }, [params.id, params.out_trade_no])

  useEffect(() => {
    if (!id) return
    const load = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/orders/${id}?logs_limit=15`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrder(result.data)
          const logs = result.data?.order_logs || []
          setAllLogs(logs)
          setLogHasMore(logs.length >= 15)
          if (result.data.instrument_id) {
            const iResp = await apiFetch(`${baseUrl}/public/instruments/${result.data.instrument_id}`)
            const iResult = await iResp.json()
            if (iResult.code === 20000) setInstrument(iResult.data)
          }
        }
      } catch {}
      setLoading(false)
    }
    load()
  }, [id])

  const handlePay = () => {
    Taro.redirectTo({ url: `/pages-weapp/payment/index?type=rent&id=${id}` })
  }

  const fetchMoreLogs = async () => {
    try {
      const nextPage = logPage + 1
      const resp = await apiFetch(`${baseUrl}/orders/${id}/logs?page=${nextPage}&pageSize=15`)
      const res = await resp.json()
      if (res.code === 20000 && res.data?.logs) {
        setAllLogs(prev => [...prev, ...res.data.logs])
        setLogHasMore(nextPage * 15 < (res.data.total || 0))
        setLogPage(nextPage)
      }
    } catch (e) { console.warn('[OrderDetail] failed to fetch more logs', e) }
  }

  const handleCancel = async () => {
    Taro.showModal({
      title: '取消订单',
      content: '确认取消该订单？取消后不可恢复。',
      success: async (res) => {
        if (!res.confirm) return
        setActionLoading(true)
        try {
          const resp = await apiFetch(`${baseUrl}/orders/${id}/cancel-by-user`, { method: 'POST' })
          const result = await resp.json()
          if (result.code === 20000) {
            if (result.data?.refund_amount > 0) {
              Taro.redirectTo({ url: `/pages-weapp/payment/index?type=refund&id=${id}` })
            } else {
              setOrder(prev => ({ ...prev, status: 'cancelled' }))
            }
          } else {
            Taro.showModal({ title: '取消失败', content: result.message, showCancel: false })
          }
        } catch (err) {
          Taro.showModal({ title: '取消失败', content: err.message, showCancel: false })
        }
        setActionLoading(false)
      }
    })
  }

  const handleConfirmReceipt = async () => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${id}/delivery`, {
        method: 'PUT',
        body: JSON.stringify({ delivered_at: new Date().toISOString() }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '确认收货成功', icon: 'success' })
        const reload = await apiFetch(`${baseUrl}/orders/${id}`)
        const r = await reload.json()
        if (r.code === 20000) setOrder(r.data)
      } else {
        Taro.showModal({ title: '操作失败', content: result.message, showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '操作失败', content: err.message, showCancel: false })
    }
    setActionLoading(false)
  }

  const handleReturn = async () => {
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/orders/${id}/return`, {
        method: 'POST',
        body: JSON.stringify({}),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '归还申请已提交', icon: 'success' })
        const reload = await apiFetch(`${baseUrl}/orders/${id}`)
        const r = await reload.json()
        if (r.code === 20000) setOrder(r.data)
      } else {
        Taro.showModal({ title: '操作失败', content: result.message, showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '操作失败', content: err.message, showCancel: false })
    }
    setActionLoading(false)
  }

  if (loading) {
    return (
      <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}>
        <Text style={{ color: '#a1a1aa' }}>加载中...</Text>
      </View>
    )
  }
  if (!order) {
    return (
      <View style={{ height: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}>
        <Text style={{ color: '#a1a1aa' }}>订单不存在</Text>
      </View>
    )
  }

  const status = order.status || ''
  const statusDef = STATUS[status] || { color: '#a1a1aa', label: status }
  const startDate = formatDisplayDate(order.start_date)
  const endDate = formatDisplayDate(order.returned_at || order.end_date)
  const returnedAt = order.returned_at ? formatDisplayDate(order.returned_at) : null
  const deposit = order.deposit || 0
  const shippingFee = order.shipping_fee || 0
  const pb = order.pricing_breakdown
  const dailyRate = (pb && (pb.final_daily_rent || pb.base_daily_rent)) || order.base_daily_rate || 0
  const actualRentDays = order.returned_at && order.start_date
    ? Math.max(1, Math.round((new Date(order.returned_at) - new Date(order.start_date)) / 86400000) + 1)
    : 0

  const isOverdue = (status === 'expired' || status === 'in_lease') && endDate !== '-' && new Date(order.end_date) < new Date()
  const overdueDaysCalc = isOverdue ? Math.ceil((new Date() - new Date(order.end_date)) / 86400000) : 0
  const overdueFee = isOverdue ? (dailyRate > 0 ? dailyRate * overdueDaysCalc : 0).toFixed(2) : 0

  const totalAmount = (pb?.total_amount || 0) + deposit + shippingFee + (overdueFee > 0 ? Number(overdueFee) : 0)

  const showPayButton = !isStaff && status === 'reserved'
  const showCancelButton = !isStaff && (status === 'reserved' || status === 'paid' || status === 'pending_shipment' || status === 'in_transit')
  const showReceiveButton = !isStaff && (status === 'in_transit' || status === 'shipped')
  const showRenewButton = !isStaff && (status === 'in_lease' || status === 'expired')
  const showReturnButton = !isStaff && (status === 'in_lease' || status === 'expired')
  const terminal = ['returning', 'returned', 'completed', 'cancelled', 'transferred']
  const isTerminal = terminal.includes(status)

  const showStaffShip = isStaff && (status === 'paid' || status === 'pending_shipment')
  const showStaffTransit = isStaff && status === 'in_transit'
  const showStaffReceive = isStaff && status === 'returning'

  const deliveryAddress = (() => {
    if (!order.delivery_address) return null
    try {
      if (typeof order.delivery_address === 'string') return JSON.parse(order.delivery_address)
      return order.delivery_address
    } catch { return null }
  })()

  const orderLogs = allLogs

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#FDFBF7', paddingBottom: 120 }}>
      <ScrollView style={{ width: '100%' }}>
        {/* Order ID + Status */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', marginBottom: 8 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000' }}>订单编号</Text>
            <Text style={{ fontSize: 12, fontWeight: '700', color: statusDef.color, backgroundColor: statusDef.color + '18', padding: '4px 12px', borderRadius: 100 }}>
              {statusDef.label}
            </Text>
          </View>
          <Text style={{ fontSize: 11, fontWeight: '500', color: '#52525b', fontFamily: 'monospace' }}>{id}</Text>
        </View>

        {/* Overdue warning */}
        {isOverdue && (
          <View style={{ backgroundColor: '#fef2f2', margin: 16, borderRadius: 16, padding: 16, border: '1px solid #fecaca' }}>
            <View style={{ display: 'flex', gap: 8 }}>
              <Text style={{ fontSize: 16 }}>⚠️</Text>
              <View>
                <Text style={{ fontSize: 14, fontWeight: '700', color: '#b91c1c' }}>租约已超期</Text>
                <Text style={{ fontSize: 12, color: '#dc2626', marginTop: 4 }}>
                  超期 {overdueDaysCalc} 天 · 累计逾期费 ¥{overdueFee}
                </Text>
                <Text style={{ fontSize: 11, color: '#dc2626', marginTop: 2 }}>（¥{dailyRate}/天）</Text>
              </View>
            </View>
          </View>
        )}

        {/* Instrument card */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <View style={{ display: 'flex', gap: 12 }}>
            {instrument?.cover_image && (
              <Image src={instrument.cover_image} style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#f4f4f5' }} mode="aspectFill" />
            )}
            <View style={{ flex: 1 }}>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#000' }}>{instrument?.category_name || '乐器'}</Text>
              <Text style={{ fontSize: 12, color: '#71717a', marginTop: 4 }}>SN: {instrument?.sn || '-'}</Text>
              <Text style={{ fontSize: 12, color: '#71717a' }}>{instrument?.level_name || ''}</Text>
            </View>
          </View>
        </View>

        {/* Delivery Info */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>配送信息</Text>
          <View style={{ marginBottom: 8 }}>
            <View style={{ display: 'flex', gap: 8, marginBottom: 6 }}>
              <Text style={{ fontSize: 13, color: '#a1a1aa', width: 60 }}>👤 下单人</Text>
              <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>{order.user_name || order.user_email || order.user_phone || '-'}</Text>
            </View>
            {deliveryAddress && (
              <View style={{ display: 'flex', gap: 8 }}>
                <Text style={{ fontSize: 13, color: '#a1a1aa', width: 60 }}>📍 地址</Text>
                <Text style={{ fontSize: 13, color: '#000', flex: 1 }}>
                  {deliveryAddress.recipient_name} {deliveryAddress.phone}
                  {'\n'}
                  {deliveryAddress.province}{deliveryAddress.city}{deliveryAddress.district} {deliveryAddress.detail}
                </Text>
              </View>
            )}
          </View>
        </View>

        {/* Order Info */}
        <LeaseInfo
          status={status}
          startDate={order.start_date}
          endDate={order.returned_at || order.end_date}
          deliveredAt={order.delivered_at}
          dailyRate={pb?.final_daily_rent || pb?.base_daily_rent || order.base_daily_rate || instrument?.base_daily_rate || 0}
          rentDays={actualRentDays || pb?.rent_days || 0}
          createdAt={order.created_at}
        />

        {/* Return Info */}
        {returnedAt && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>归还信息</Text>
            <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6 }}>
              <Text style={{ fontSize: 13, color: '#71717a' }}>↩️ 归还日期</Text>
              <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>{returnedAt}</Text>
            </View>
            {order.settlement?.actual_rent_days && (
              <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6 }}>
                <Text style={{ fontSize: 13, color: '#71717a' }}>📊 实际租期</Text>
                <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>{order.settlement.actual_rent_days} 天</Text>
              </View>
            )}
          </View>
        )}

        {/* Fee Info */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>费用信息</Text>

          {pb && typeof pb === 'object' ? (
            <>
              {(() => {
                const tiers = pb.tier_segments
                const hasTiers = Array.isArray(tiers) && tiers.length > 0
                const policies = pb.applied_policies
                const policiesAfterTier = Array.isArray(policies)
                  ? policies.filter(p => p.type !== 'tier_discount')
                  : []

                return (
                  <View>
                    {hasTiers ? (
                      <View style={{ marginBottom: 4 }}>
                        <Text style={{ fontSize: 13, fontWeight: '600', color: '#52525b', marginBottom: 6 }}>阶梯定价</Text>
                        {tiers.map((seg, i) => {
                          const discountLabel = seg.discount < 1
                            ? `${Math.round((1 - seg.discount) * 100)}折`
                            : ''
                          return (
                            <View key={i} style={{ paddingVertical: 3, paddingLeft: 8 }}>
                              <Text style={{ fontSize: 12, color: '#71717a' }}>
                                第{seg.tier}阶{seg.days}天: ¥{Number(seg.rate).toFixed(2)}/天 × {seg.days}天
                                {seg.discount < 1 ? ` (${discountLabel})` : ''}
                                {' = '}
                                <Text style={{ fontWeight: '700', color: '#000' }}>
                                  ¥{Number(seg.subtotal).toFixed(2)}
                                </Text>
                              </Text>
                            </View>
                          )
                        })}
                        <View style={{ borderTop: '1px dashed #e4e4e7', marginTop: 4, paddingTop: 4, paddingLeft: 8 }}>
                          <Text style={{ fontSize: 13, fontWeight: '700', color: '#000' }}>
                            租金小计 ¥{Number(pb.total_amount || 0).toFixed(2)}
                          </Text>
                        </View>
                        {policiesAfterTier.length > 0 && (
                          <View style={{ paddingLeft: 8, paddingTop: 2 }}>
                            {policiesAfterTier.map((p, i) => (
                              <Text key={i} style={{ fontSize: 11, color: '#a1a1aa', marginTop: 1 }}>
                                {p.plan_name}: {Math.round((1 - p.rate) * 100)}折
                              </Text>
                            ))}
                          </View>
                        )}
                      </View>
                    ) : (
                      <View>
                        <Row label="日租金" value={`¥${Number(pb.final_daily_rent || pb.base_daily_rent || 0).toFixed(2)}`} />
                        {pb.base_daily_rent && pb.final_daily_rent < pb.base_daily_rent && (
                          <Row label="  原价" value={`¥${pb.base_daily_rent}/天`} color="#a1a1aa" />
                        )}
                        {pb.rent_days > 0 && <Row label="租期（天）" value={pb.rent_days} />}
                        <Row label="租金" value={`¥${Number(pb.total_amount || 0).toFixed(2)}`} />
                      </View>
                    )}
                  </View>
                )
              })()}
              {deposit > 0 && (
                <View>
                  <Row label="押金" value={`¥${Number(deposit).toFixed(2)}`} />
                  {pb?.deposit_method && (
                    <Text style={{ fontSize: 11, color: '#a1a1aa', textAlign: 'right', marginTop: -2 }}>
                      {pb.deposit_method === 'total_price'
                        ? `原价 ¥${pb.total_price || 0} × ${pb.deposit_ratio || 0}`
                        : `日租金 × ${pb.deposit_multiplier || 7}倍`}
                    </Text>
                  )}
                </View>
              )}
              {shippingFee > 0 && <Row label="物流费" value={`¥${shippingFee.toFixed(2)}`} />}
            </>
          ) : (
            <>
              <Row label="租金" value={`¥${Number(pb?.total_amount || 0).toFixed(2)}`} />
              <Row label="押金" value={`¥${deposit}`} />
              <Row label="物流费" value={`¥${shippingFee}`} />
            </>
          )}

          {overdueFee > 0 && (
            <>
              <Row label="逾期费用" value={`¥${overdueFee}`} color="#ef4444" />
              <Row label="  逾期日费" value={`¥${dailyRate}/天`} color="#a1a1aa" />
            </>
          )}

          {order.settlement?.actual_rent_amount !== undefined && (
            <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6, borderTop: '1px solid #f4f4f5' }}>
              <Text style={{ fontSize: 13, fontWeight: '700', color: '#000' }}>实收金额</Text>
              <Text style={{ fontSize: 13, fontWeight: '700', color: '#16a34a' }}>¥{order.settlement.actual_rent_amount}</Text>
            </View>
          )}

          <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6, borderTop: '1px solid #e4e4e7', marginTop: 4 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000' }}>{order.settlement ? '合计（含押金）' : '合计'}</Text>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000' }}>¥{totalAmount}</Text>
          </View>
        </View>

        {/* Settlement Detail */}
        {order.settlement && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>结算明细</Text>
            {order.settlement.original_rent_amount !== undefined && (
              <Row label="原始租金" value={`¥${order.settlement.original_rent_amount}`} />
            )}
            {order.settlement.actual_rent_amount !== undefined && (
              <Row label="实收租金" value={`¥${order.settlement.actual_rent_amount}`} color="#16a34a" />
            )}
            {order.settlement.actual_rent_days !== undefined && (
              <Row label="实际天数" value={`${order.settlement.actual_rent_days} 天`} />
            )}
            {order.settlement.overdue_charges_total !== undefined && Number(order.settlement.overdue_charges_total) > 0 && (
              <Row label="逾期费用" value={`¥${order.settlement.overdue_charges_total}`} color="#ef4444" />
            )}
            {order.settlement.cash_refundable !== undefined && (
              <Row label="可退现金" value={`¥${order.settlement.cash_refundable}`} />
            )}
            {order.settlement.prepaid_refunded !== undefined && (
              <Row label="预付款退还" value={`¥${order.settlement.prepaid_refunded}`} />
            )}
            {order.settlement.refund_method && (
              <Row label="退款方式" value={order.settlement.refund_method} />
            )}
            {order.settlement.refund_status && (
              <Row
                label="退款状态"
                value={order.settlement.refund_status === 'completed' ? '已退款' : order.settlement.refund_status === 'pending' ? '处理中' : order.settlement.refund_status}
                color={order.settlement.refund_status === 'completed' ? '#16a34a' : '#f59e0b'}
              />
            )}
          </View>
        )}

        {/* Logistics */}
        {(order.tracking_number || order.courier_company) && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>物流信息</Text>
            {order.courier_company && <Row label="🚚 物流公司" value={order.courier_company} />}
            {order.tracking_number && (
              <Row label="📦 物流单号" value={order.tracking_number} mono />
            )}
          </View>
        )}

        {/* Timeline Logs */}
        {orderLogs.length > 0 && (<>
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#000', marginBottom: 12 }}>订单动态</Text>
            {orderLogs.map((log, idx) => {
              const isCurrent = log.event === order.status
              const isFuture = (() => {
                const orderIdx = Object.keys(EVENT_LABELS).indexOf(order.status)
                const eventIdx = Object.keys(EVENT_LABELS).indexOf(log.event)
                return eventIdx >= 0 && orderIdx >= 0 && eventIdx > orderIdx
              })()
              const dotStyle = isCurrent
                ? { backgroundColor: '#000', width: 10, height: 10, borderRadius: 10 }
                : isFuture
                  ? { border: '2px solid #d4d4d8', borderRadius: 10, width: 10, height: 10 }
                  : { backgroundColor: '#d4d4d8', width: 10, height: 10, borderRadius: 10 }
              return (
                <View key={idx} style={{ display: 'flex', gap: 8, paddingBottom: idx < orderLogs.length - 1 ? 12 : 0 }}>
                  <View style={{ display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
                    <View style={dotStyle} />
                    {idx < orderLogs.length - 1 && <View style={{ width: 1, flex: 1, backgroundColor: '#e4e4e7' }} />}
                  </View>
                  <View style={{ flex: 1 }}>
                    <Text style={{
                      fontSize: 13, fontWeight: '700',
                      color: isCurrent ? '#000' : isFuture ? '#d4d4d8' : '#71717a',
                    }}>
                      {EVENT_LABELS[log.event] || log.event}
                    </Text>
                    <Text style={{ fontSize: 11, color: '#a1a1aa', marginTop: 2 }}>
                      {formatDisplayDate(log.time || log.created_at)}
                      {log.operator ? ` by ${log.operator}` : ''}
                    </Text>
                  </View>
                </View>
              )
            })}
          </View>
          {logHasMore && (
            <View onClick={fetchMoreLogs} style={{ marginTop: 12, paddingTop: 10, paddingBottom: 10, borderRadius: 12, backgroundColor: '#f4f4f5', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
              <Text style={{ fontSize: 13, fontWeight: '700', color: '#71717a' }}>加载更多</Text>
            </View>
          )}
        </>)}
      </ScrollView>

      {/* Action Buttons */}
      <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, backgroundColor: '#fff', borderTop: '1px solid #f4f4f5', padding: 16 }}>
        {isStaff ? (
          <>
            {showStaffShip && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/shipping/index?order=${id}` })}
                style={btnStyle('#000')}>📦 发货</View>
            )}
            {showStaffTransit && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/shipping/index?order=${id}` })}
                style={btnStyle('#06b6d4')}>🚚 接收并转发</View>
            )}
            {showStaffReceive && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/receiving/index?order_id=${id}` })}
                style={btnStyle('#C21838')}>↩️ 接收</View>
            )}
            {!showStaffShip && !showStaffTransit && !showStaffReceive && (
              <View style={{ ...btnStyle('#a1a1aa'), backgroundColor: '#f4f4f5', cursor: 'default' }}>
                {status === 'reserved' ? '⏳ 未支付'
                : status === 'shipped' ? '✅ 乐器已发货，等待用户签收'
                : status === 'in_lease' ? '✅ 租赁中'
                : status === 'expired' ? '⚠️ 租约已超期'
                : ['returned', 'completed'].includes(status) ? '✅ 该订单已完成'
                : status === 'cancelled' ? '❌ 该订单已取消'
                : status === 'transferred' ? '✅ 已过户'
                : status === 'returning' ? '↩️ 乐器归还中，等待验收'
                : statusDef.label}
              </View>
            )}
          </>
        ) : (
          <>
            {showPayButton && (
              <View onClick={actionLoading ? undefined : handlePay}
                style={{ ...btnStyle('#000'), opacity: actionLoading ? 0.5 : 1 }}>
                {actionLoading ? '处理中...' : '💳 支付'}
              </View>
            )}
            {showCancelButton && (
              <View onClick={actionLoading ? undefined : handleCancel}
                style={{ ...btnStyle('#ef4444'), opacity: actionLoading ? 0.5 : 1 }}>
                {actionLoading ? '处理中...' : '❌ 取消订单'}
              </View>
            )}
            {showReceiveButton && (
              <View onClick={actionLoading ? undefined : handleReceive}
                style={{ ...btnStyle('#16a34a'), opacity: actionLoading ? 0.5 : 1 }}>
                {actionLoading ? '处理中...' : '✅ 确认收货'}
              </View>
            )}
            {showRenewButton && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/renewal/index?id=${id}` })}
                style={{ ...btnStyle('#2563eb') }}>
                {'📅 续期'}
              </View>
            )}
            {showReturnButton && (
              <View onClick={actionLoading ? undefined : handleReturn}
                style={{ ...btnStyle('#f97316'), opacity: actionLoading ? 0.5 : 1 }}>
                {actionLoading ? '处理中...' : '↩️ 归还'}
              </View>
            )}
            {isTerminal && (
              <View style={{ ...btnStyle('#a1a1aa'), backgroundColor: '#f4f4f5', cursor: 'default' }}>
                {['completed', 'returned'].includes(status) ? '✅ 该订单已完成'
                : status === 'cancelled' ? '❌ 该订单已取消'
                : status === 'returning' ? '↩️ 乐器归还中，等待验收'
                : status === 'transferred' ? '✅ 已过户'
                : statusDef.label}
              </View>
            )}
          </>
        )}
      </View>
    </View>
  )
}

function Row({ label, value, color, mono }) {
  return (
    <View style={{ display: 'flex', justifyContent: 'space-between', paddingVertical: 6 }}>
      <Text style={{ fontSize: 13, color: '#71717a' }}>{label}</Text>
      <Text style={{
        fontSize: 13, fontWeight: '700',
        color: color || '#000',
        fontFamily: mono ? 'monospace' : undefined,
      }}>
        {value}
      </Text>
    </View>
  )
}

function btnStyle(bgColor) {
  return {
    width: '100%',
    padding: '14px 0',
    backgroundColor: bgColor,
    color: '#fff',
    borderRadius: 16,
    fontWeight: '700',
    fontSize: 15,
    textAlign: 'center',
    cursor: 'pointer',
  }
}
