import { useState, useEffect, useCallback } from 'react'
import Taro, { useDidShow } from '@tarojs/taro'
import { View, Text, ScrollView, Image, Button } from '@tarojs/components'
import { apiFetch, getToken } from '../../services/api'
import { env, uploadFile } from '../../platform'
import { formatDeliveryAddress, formatDisplayDate, formatLogTime } from '../../utils/format'
import { calculateDays, calculateEndDate } from '../../utils/daycalc'
import LeaseInfo from '../../components/LeaseInfo'

const fixImg = (url) => url && !url.startsWith('http') && !url.startsWith('data:') ? `${env.apiBaseUrl.replace(/\/api$/, '')}${url}` : url

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
  damage_appealing: { color: '#f59e0b', label: '定损申诉' },
  pending_damage_response: { color: '#ef4444', label: '待回应定损' },
  deposit_refunding: { color: '#f59e0b', label: '押金退款中' },
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
  const [showContract, setShowContract] = useState(false)
  const [receivePhotos, setReceivePhotos] = useState([])

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

  const loadOrder = useCallback(async () => {
    if (!id) return
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
  }, [id, baseUrl])

  useEffect(() => {
    loadOrder()
  }, [loadOrder])

  // Page-stack return (发货/收货/退款等操作后 navigateBack) does NOT remount —
  // reload the order so status/buttons reflect the latest state (#1665 教训).
  useDidShow(() => {
    if (id) loadOrder()
  })

  useEffect(() => {
    if (!order) return
    const hasSettlement = !!order.settlement && order.settlement.actual_rent_amount !== undefined
    setShowContract(!hasSettlement)
  }, [order?.id])

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
              // paid/pending_shipment: full original payment refunded — go to refund page
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

  const uploadPhotos = async (files) => {
    const urls = []
    for (const file of files) {
      const upResp = await uploadFile(`${baseUrl}/upload`, file, {
        headers: { Authorization: 'Bearer ' + getToken() },
      })
      const upResult = upResp.json ? await upResp.json() : JSON.parse(upResp.data || '{}')
      if (upResult.code === 20000 && upResult.data?.url) urls.push(upResult.data.url)
    }
    return urls
  }

  const handleConfirmReceipt = async () => {
    setActionLoading(true)
    try {
      const photos = await uploadPhotos(receivePhotos)
      const resp = await apiFetch(`${baseUrl}/warehouse/orders/${id}/delivery`, {
        method: 'PUT',
        body: JSON.stringify({ delivered_at: new Date().toISOString(), photos }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '确认收货成功', icon: 'success' })
        setTimeout(() => Taro.navigateBack(), 800)
      } else {
        Taro.showModal({ title: '操作失败', content: result.message, showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '操作失败', content: err.message, showCancel: false })
    }
    setActionLoading(false)
  }

  const handleReturn = () => {
    // L-07 jump-page mode: return logistics filled on the dedicated
    // ReturnConfirm page (consistent with H5 /return/:orderId).
    Taro.navigateTo({ url: `/pages-weapp/return-confirm/index?order_id=${id}&instrument=${instrument?.id || ''}` })
  }

  const handleStaffRefund = () => {
    Taro.showModal({
      title: '确认执行退款',
      content: '将按结算差额退回现金与赠点。',
      success: async (res) => {
        if (!res.confirm) return
        setActionLoading(true)
        try {
          const resp = await apiFetch(`${baseUrl}/orders/${id}/refund`, {
            method: 'POST',
          })
          const result = await resp.json()
          if (result.code === 20000) {
            Taro.showToast({ title: '退款成功', icon: 'success' })
            setTimeout(() => Taro.redirectTo({ url: `/pages-weapp/payment/index?type=refund&id=${id}` }), 800)
          } else {
            Taro.showModal({ title: '退款失败', content: result.message, showCancel: false })
          }
        } catch (err) {
          Taro.showModal({ title: '退款失败', content: err.message, showCancel: false })
        }
        setActionLoading(false)
      },
    })
  }

  const handleStaffCancel = async () => {
    Taro.showModal({
      title: '确认取消订单',
      content: '取消后不可恢复，已付款将原路退回。',
      success: async (res) => {
        if (!res.confirm) return
        setActionLoading(true)
        try {
          const resp = await apiFetch(`${baseUrl}/warehouse/orders/${id}/staff-cancel`, {
            method: 'POST',
            body: JSON.stringify({ reason: '担保人不符合要求' }),
          })
          const result = await resp.json()
          if (result.code === 20000) {
            Taro.showToast({ title: '订单已取消', icon: 'success' })
            setTimeout(() => Taro.navigateBack(), 800)
          } else {
            Taro.showModal({ title: '取消失败', content: result.message, showCancel: false })
          }
        } catch (err) {
          Taro.showModal({ title: '取消失败', content: err.message, showCancel: false })
        }
        setActionLoading(false)
      },
    })
  }

  // 定损接受/拒绝（#1707）：复用通知详情页的 appealsApi.agree + 申诉流程
  const handleDamageAccept = async (damage) => {
    const amt = Number(damage.damage_amount || 0)
    const dep = Number(damage.deposit || 0)
    const ok = await new Promise(resolve => {
      Taro.showModal({
        title: '确认接受定损',
        content: amt < dep
          ? `定损金额 ¥${amt.toFixed(2)}，押金 ¥${dep.toFixed(2)}，将退还差额 ¥${(dep - amt).toFixed(2)}`
          : `定损金额 ¥${amt.toFixed(2)}，押金 ¥${dep.toFixed(2)}，需补缴 ¥${(amt - dep).toFixed(2)}`,
        success: res => resolve(res.confirm),
        fail: () => resolve(false),
      })
    })
    if (!ok) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/appeals/${damage.report_id}/agree`, { method: 'POST' })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '已接受定损', icon: 'success' })
        setTimeout(() => Taro.navigateBack(), 800)
      } else {
        Taro.showModal({ title: '操作失败', content: result.message, showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '操作失败', content: err.message, showCancel: false })
    }
    setActionLoading(false)
  }

  const handleDamageReject = async (damage) => {
    const reason = await new Promise(resolve => {
      Taro.showModal({
        title: '拒绝定损',
        editable: true,
        placeholderText: '请输入申诉原因',
        success: res => resolve(res.confirm ? (res.content || '') : ''),
        fail: () => resolve(''),
      })
    })
    if (!reason) return
    setActionLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/appeals`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          damage_report_id: damage.report_id,
          appeal_reason: reason,
        }),
      })
      const result = await resp.json()
      if (result.code === 20000) {
        Taro.showToast({ title: '已提交申诉', icon: 'success' })
        setTimeout(() => Taro.navigateBack(), 800)
      } else {
        Taro.showModal({ title: '申诉失败', content: result.message, showCancel: false })
      }
    } catch (err) {
      Taro.showModal({ title: '申诉失败', content: err.message, showCancel: false })
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
  // Shipping fee is filled by staff at dispatch (#1541); it is only shown
  // from the shipped state onwards (shipped → completed) and hidden at
  // order time (reserved/paid/pending_shipment) — #1570.
  const showShippingFee = shippingFee > 0 && [
    'shipped', 'in_transit', 'in_lease', 'returning', 'returned',
    'completed', 'damage_appealing', 'pending_damage_response',
    'deposit_refunding', 'expired', 'transferred',
  ].includes(status)
  const pb = order.pricing_breakdown
  const dailyRate = (pb && (pb.final_daily_rent || pb.base_daily_rent)) || order.base_daily_rate || 0
  const actualRentDays = order.returned_at && order.start_date
    ? calculateDays(new Date(order.start_date), new Date(order.returned_at))
    : 0

  const isOverdue = (status === 'expired' || status === 'in_lease') && endDate !== '-' && new Date(order.end_date) < new Date()
  const overdueDaysCalc = isOverdue ? calculateDays(new Date(order.end_date), new Date()) : 0
  const overdueFee = isOverdue ? (dailyRate > 0 ? dailyRate * overdueDaysCalc : 0).toFixed(2) : 0

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
  // Staff cancel only on cancellable states, grouped with ship actions —
  // NOT in the guarantor panel (#1623).
  const showStaffCancel = isStaff && order.deposit_waived && (status === 'paid' || status === 'pending_shipment')

  const deliveryAddress = (() => {
    if (!order.delivery_address) return null
    try {
      if (typeof order.delivery_address !== 'string') return order.delivery_address
      // JSONB string value (e.g. "林维训 12345678 福建省...") parses to a
      // plain string — render it as-is; object shape renders field-by-field.
      try {
        const parsed = JSON.parse(order.delivery_address)
        if (typeof parsed === 'string') return parsed
        if (parsed && typeof parsed === 'object') return parsed
      } catch {}
      const parts = order.delivery_address.trim().split(/\s+/)
      if (parts.length >= 3) {
        return {
          recipient_name: parts[0],
          phone: parts[1],
          province: parts[2],
          city: parts[3] || '',
          district: '',
          detail: parts.slice(4).join(' '),
        }
      }
    } catch {}
    return null
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
                  超期 {overdueDaysCalc} 天 · 累计逾期费 ¥{(overdueFee || 0).toFixed(2)}
                </Text>
                <Text style={{ fontSize: 11, color: '#dc2626', marginTop: 2 }}>（¥{(dailyRate || 0).toFixed(2)}/天）</Text>
              </View>
            </View>
          </View>
        )}

        {/* Instrument card */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }} onClick={() => instrument?.id && Taro.navigateTo({ url: `/pages-weapp/detail/index?id=${instrument.id}` })}>
          <View style={{ display: 'flex', gap: 12 }}>
            {instrument?.cover_image && (
              <Image src={fixImg(instrument.cover_image)} style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#f4f4f5' }} mode="aspectFill" />
            )}
            <View style={{ flex: 1, display: 'flex', flexDirection: 'column', justifyContent: 'center' }}>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#000' }}>{instrument?.category_name || '乐器'}</Text>
              <Text style={{ fontSize: 12, color: '#71717a', marginTop: 4 }}>SN: {instrument?.sn || '-'}</Text>
              {instrument?.level_name && <Text style={{ fontSize: 12, color: '#71717a' }}>{instrument.level_name}</Text>}
            </View>
          </View>
        </View>

        {/* Delivery + Logistics (合并) */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 12 }}>配送物流</Text>
          <View style={{ marginBottom: 8 }}>
            <View style={{ display: 'flex', gap: 8, marginBottom: 6 }}>
              <Text style={{ fontSize: 13, color: '#a1a1aa', width: 60 }}>👤 下单人</Text>
              <View>
                <Text style={{ fontSize: 13, fontWeight: '500', color: '#000' }}>{order.user_name || '-'}</Text>
                {order.user_phone && <Text style={{ fontSize: 12, color: '#a1a1aa' }}>{order.user_phone}</Text>}
              </View>
            </View>
            {deliveryAddress && (
              <View style={{ display: 'flex', gap: 8 }}>
                <Text style={{ fontSize: 13, color: '#a1a1aa', width: 60 }}>📍 地址</Text>
                <Text style={{ fontSize: 13, color: '#000', flex: 1 }}>
                  {typeof deliveryAddress === 'string' ? deliveryAddress : (
                    <>
                      {deliveryAddress.recipient_name} {deliveryAddress.phone}
                      {'\n'}
                      {deliveryAddress.province}{deliveryAddress.city}{deliveryAddress.district} {deliveryAddress.detail}
                    </>
                  )}
                </Text>
              </View>
            )}
          </View>
          {(order.courier_company || order.tracking_number || order.shipped_at) && (
            <View style={{ borderTop: '1px dashed #e4e4e7', paddingTop: 8 }}>
              {order.courier_company && <Row label="物流公司" value={order.courier_company} />}
              {order.tracking_number && <Row label="物流单号" value={order.tracking_number} mono />}
              {order.shipped_at && <Row label="发货时间" value={formatDisplayDate(order.shipped_at)} />}
            </View>
          )}
        </View>

        {/* Order Info (合并归还信息：归还日期/实际租期由 LeaseInfo 显示) */}
        <LeaseInfo
          status={status}
          startDate={order.start_date}
          endDate={order.returned_at || order.end_date}
          deliveredAt={order.delivered_at}
          dailyRate={pb?.final_daily_rent || pb?.base_daily_rent || order.base_daily_rate || instrument?.base_daily_rate || 0}
          rentDays={actualRentDays || pb?.rent_days || 0}
          createdAt={order.created_at}
          orderId={order.id}
          paidAt={order.paid_at}
          returnedAt={order.returned_at}
        />

        {/* Fee Info (合并结算明细/收支明细) */}
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 16 }}>费用明细</Text>

          {/* ① 实付金额 */}
          {order.payment_records?.length > 0 && (
            <>
              <Text style={{ fontSize: 11, fontWeight: '700', color: '#a1a1aa', marginTop: 12, marginBottom: 4 }}>实付金额</Text>
              {order.payment_records.map(pr => (
                <Row key={pr.id} label={`${pr.method || '支付'}`} value={`¥${Number(pr.amount).toFixed(2)}`} />
              ))}
              <Row label="实付合计" value={`¥${order.payment_records.reduce((s, p) => s + Number(p.amount || 0), 0).toFixed(2)}`} />
            </>
          )}

          {/* ② 实际租期与租金 */}
          {order.settlement?.actual_rent_amount !== undefined && (
            <>
              <Text style={{ fontSize: 11, fontWeight: '700', color: '#a1a1aa', marginTop: 8, marginBottom: 4 }}>实际租期与租金</Text>
              {order.settlement.actual_rent_days !== undefined && (
                <Row label="实际租期" value={`${order.settlement.actual_rent_days} 天`} />
              )}
              <Row label="实际租金" value={`¥${(order.settlement.actual_rent_amount || 0).toFixed(2)}`} color="#16a34a" />
              {overdueFee > 0 && (
                <>
                  <Row label="逾期费用" value={`¥${(overdueFee || 0).toFixed(2)}`} color="#ef4444" />
                  <Row label="  逾期日费" value={`¥${(dailyRate || 0).toFixed(2)}/天`} color="#a1a1aa" />
                </>
              )}
            </>
          )}

          {/* ③ 退款 */}
          {order.settlement && (order.settlement.cash_refundable > 0 || order.settlement.prepaid_refunded > 0 || order.settlement.gift_points_refunded > 0) && (
            <>
              <Text style={{ fontSize: 11, fontWeight: '700', color: '#a1a1aa', marginTop: 8, marginBottom: 4 }}>退款</Text>
              {order.settlement.cash_refundable > 0 && (
                <Row label="现金退款" value={`¥${(order.settlement.cash_refundable || 0).toFixed(2)}`} color="#3b82f6" />
              )}
              {order.settlement.prepaid_refunded > 0 && (
                <Row label="退回预付点" value={`¥${(order.settlement.prepaid_refunded || 0).toFixed(2)}`} color="#3b82f6" />
              )}
              {order.settlement.gift_points_refunded > 0 && (
                <Row label="赠送积分退还" value={`¥${(order.settlement.gift_points_refunded || 0).toFixed(2)}`} color="#3b82f6" />
              )}
              <Row label="退款合计" value={`¥${(Number(order.settlement.cash_refundable) + Number(order.settlement.prepaid_refunded) + Number(order.settlement.gift_points_refunded)).toFixed(2)}`} color="#16a34a" />
            </>
          )}

          {/* ④ 结算（原结算明细面板并入） */}
          {order.settlement && (
            <View style={{ borderTop: '1px dashed #e4e4e7', marginTop: 8, paddingTop: 8 }}>
              <Text style={{ fontSize: 11, fontWeight: '700', color: '#a1a1aa', marginBottom: 4 }}>结算</Text>
              {order.settlement.original_rent_amount !== undefined && (
                <Row label="原始租金" value={`¥${(order.settlement.original_rent_amount || 0).toFixed(2)}`} />
              )}
              {order.settlement.actual_rent_days !== undefined && (
                <Row label="实际天数" value={`${order.settlement.actual_rent_days} 天`} />
              )}
              {order.settlement.overdue_charges_total !== undefined && Number(order.settlement.overdue_charges_total) > 0 && (
                <Row label="逾期费用" value={`¥${(order.settlement.overdue_charges_total || 0).toFixed(2)}`} color="#ef4444" />
              )}
              {order.settlement.cash_refundable !== undefined && (
                <Row label="可退现金" value={`¥${(order.settlement.cash_refundable || 0).toFixed(2)}`} />
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

          {/* ⑤ 收支（原收支明细面板并入：支付/退款记录 + 净支出） */}
          {['completed', 'returned'].includes(order?.status) && (order?.payment_records?.length > 0 || order?.refund_records?.length > 0) && (
            <View style={{ borderTop: '1px dashed #e4e4e7', marginTop: 8, paddingTop: 8 }}>
              <Text style={{ fontSize: 11, fontWeight: '700', color: '#a1a1aa', marginBottom: 4 }}>收支记录</Text>
              {(order.payment_records || []).map(pr => (
                <Row key={pr.id} label={`支付 · ${pr.method || ''}`.trim()} value={`¥${Number(pr.amount).toFixed(2)}`} />
              ))}
              {(order.refund_records || []).map(rf => (
                <Row key={rf.id} label={`退款 · ${rf.method === 'prepaid' ? '预付点' : rf.method === 'cash_withdrawal' ? '现金' : '微信'}`} value={`-¥${Number(rf.amount).toFixed(2)}`} color="#16a34a" />
              ))}
              {(() => {
                const paid = (order.payment_records || []).reduce((s, p) => s + Number(p.amount || 0), 0)
                const refunded = (order.refund_records || []).reduce((s, r) => s + Number(r.amount || 0), 0)
                return <Row label="净支出" value={`¥${Math.max(0, paid - refunded).toFixed(2)}`} />
              })()}
            </View>
          )}

          {/* 定损费用预览（#1707）：待回应定损态由后端 damage 对象返回 */}
          {order.damage && (
            <View style={{ borderTop: '1px dashed #e4e4e7', marginTop: 8, paddingTop: 8 }}>
              <Text style={{ fontSize: 11, fontWeight: '700', color: '#a1a1aa', marginBottom: 4 }}>定损费用预览</Text>
              {order.damage.actual_rent_days > 0 && (
                <Row label="实际租期" value={`${order.damage.actual_rent_days} 天`} />
              )}
              {Number(order.damage.actual_rent_amount) > 0 && (
                <Row label="实际租金" value={`¥${Number(order.damage.actual_rent_amount).toFixed(2)}`} />
              )}
              <Row label="赔偿金额" value={`¥${Number(order.damage.damage_amount).toFixed(2)}`} color="#ef4444" />
              <Row label="退款" value={`¥${Number(order.damage.refund).toFixed(2)}`} color="#16a34a" />
            </View>
          )}

          {/* 合同快照 (collapsed) */}
          {pb && typeof pb === 'object' && (
            <View style={{ marginTop: 12, borderTop: '1px dashed #e4e4e7', paddingTop: 10 }}>
              <View
                onClick={() => setShowContract(!showContract)}
                style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}
              >
                <Text style={{ fontSize: 13, fontWeight: '700', color: '#71717a' }}>合同快照</Text>
                <Text style={{ fontSize: 11, color: '#a1a1aa' }}>{showContract ? '收起 ▲' : '展开 ▼'}</Text>
              </View>
              {showContract && (
                <View style={{ marginTop: 8 }}>
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
                              <Row label="原价" value={`¥${(pb.base_daily_rent || 0).toFixed(2)}/天`} color="#a1a1aa" />
                            )}
                            {pb.rent_days > 0 && <Row label="合同租期（天）" value={pb.rent_days} />}
                            <Row label="租金" value={`¥${Number(pb.total_amount || 0).toFixed(2)}`} />
                          </View>
                        )}
                      </View>
                    )
                  })()}
                  {deposit > 0 && !order.deposit_waived && (
                    <View>
                      <Row label="押金" value={`¥${Number(deposit).toFixed(2)}`} />
                      {pb?.deposit_method && (
                        <Text style={{ fontSize: 11, color: '#a1a1aa', textAlign: 'right', marginTop: -2 }}>
                          {pb.deposit_method === 'total_price'
                            ? `乐器总价值 ¥${(pb.total_price || 0 || 0).toFixed(2)}`
                            : (pb.deposit_multiplier > 0
                                ? `日租金 × ${pb.deposit_multiplier}倍`
                                : '')}
                        </Text>
                      )}
                    </View>
                  )}
                  {order.deposit_waived && (
                    <Row label="押金" value="免押金" color="#16a34a" />
                  )}
                  {showShippingFee && <Row label="物流费" value={`¥${shippingFee.toFixed(2)}`} />}
                </View>
              )}
            </View>
          )}
        </View>

        {/* Guarantor info (deposit-free orders, #1557) */}
        {order.deposit_waived && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 12 }}>担保人信息</Text>
            {(order.guarantors || []).length > 0 ? order.guarantors.map((g, i) => (
              <View key={g.id || i} style={{ backgroundColor: '#fafafa', borderRadius: 8, padding: 10, marginBottom: 8 }}>
                <Text style={{ fontSize: 13, fontWeight: '600', color: '#000' }}>{g.name} · {g.phone}</Text>
                {g.company || g.title ? (
                  <Text style={{ fontSize: 12, color: '#71717a', marginTop: 2 }}>{[g.company, g.title].filter(Boolean).join(' / ')}</Text>
                ) : null}
                {g.address ? <Text style={{ fontSize: 12, color: '#71717a', marginTop: 2 }}>{g.address}</Text> : null}
              </View>
            )) : (
              <Text style={{ fontSize: 12, color: '#a1a1aa' }}>暂无担保人信息</Text>
            )}
          </View>
        )}

        {/* Staff: deposit-free warning + guarantors (#1557) */}
        {showStaffShip && order.deposit_waived && (
          <View style={{ backgroundColor: '#fef3c7', margin: 16, borderRadius: 16, padding: 16, border: '1px solid #fde68a' }}>
            <View style={{ display: 'flex', gap: 8, marginBottom: 8 }}>
              <Text style={{ fontSize: 16 }}>⚠️</Text>
              <Text style={{ fontSize: 14, fontWeight: '700', color: '#92400e' }}>本订单为免押金订单</Text>
            </View>
            <Text style={{ fontSize: 12, color: '#b45309', marginBottom: 10, lineHeight: '18px' }}>
              请核实以下担保人信息，若担保人不符合要求应取消订单：
            </Text>
            {(order.guarantors || []).map((g, i) => (
              <View key={g.id || i} style={{ backgroundColor: '#fff', borderRadius: 8, padding: 10, marginBottom: 8 }}>
                <Text style={{ fontSize: 13, fontWeight: '600', color: '#000' }}>{g.name} - {g.phone}</Text>
                <Text style={{ fontSize: 12, color: '#71717a', marginTop: 2 }}>{[g.company, g.title].filter(Boolean).join(' / ') || '-'}</Text>
                {g.address && <Text style={{ fontSize: 12, color: '#71717a', marginTop: 2 }}>{g.address}</Text>}
              </View>
            ))}
          </View>
        )}

        {/* Customer: receive photos (shipped) */}
        {!isStaff && status === 'shipped' && (
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 4 }}>收货拍照留档</Text>
            <Text style={{ fontSize: 12, color: '#a1a1aa', marginBottom: 8 }}>请拍摄乐器到达时的状态</Text>
            <PhotoPicker photos={receivePhotos} setPhotos={setReceivePhotos} />
          </View>
        )}

        {/* Timeline Logs */}
        {orderLogs.length > 0 && (<>
          <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 12 }}>订单动态</Text>
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
                    <Text style={{ fontSize: 11, color: '#a1a1aa', marginTop: 2, display: 'block' }}>
                      {formatLogTime(log.time || log.created_at)}
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

      {/* 定损信息面板（#1707）：待回应定损/定损申诉态展示 + 接受/拒绝入口 */}
      {order.damage && (
        <View style={{ backgroundColor: '#fff', margin: 16, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
          <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 12 }}>定损信息</Text>
          <Row label="定损金额" value={`¥${Number(order.damage.damage_amount).toFixed(2)}`} color="#ef4444" />
          {order.damage.description ? (
            <Row label="定损说明" value={order.damage.description} />
          ) : (
            <Row label="定损说明" value="暂无" color="#a1a1aa" />
          )}
          {(order.damage.photos || []).length > 0 && (
            <View style={{ flexDirection: 'row', flexWrap: 'wrap', marginTop: 8, marginBottom: 4 }}>
              {(order.damage.photos || []).map((p, i) => (
                <Image
                  key={i}
                  src={fixImg(p)}
                  mode="aspectFill"
                  style={{ width: 72, height: 72, borderRadius: 8, marginRight: 8, marginBottom: 8, backgroundColor: '#f4f4f5' }}
                  onClick={() => Taro.previewImage({ urls: (order.damage.photos || []).map(fixImg), current: fixImg(p) })}
                />
              ))}
            </View>
          )}
          {order.damage.status === 'pending' && !isStaff && (
            <View style={{ flexDirection: 'row', marginTop: 8 }}>
              <View
                onClick={() => handleDamageAccept(order.damage)}
                style={{ flex: 1, padding: '9px 0', backgroundColor: '#0ea5e9', borderRadius: 10, textAlign: 'center', marginRight: 6 }}
              >
                <Text style={{ color: '#fff', fontWeight: '700', fontSize: 14 }}>接受</Text>
              </View>
              <View
                onClick={() => handleDamageReject(order.damage)}
                style={{ flex: 1, padding: '9px 0', backgroundColor: '#f4f4f5', borderWidth: 1, borderColor: '#d4d4d8', borderRadius: 10, textAlign: 'center', marginLeft: 6 }}
              >
                <Text style={{ color: '#52525b', fontWeight: '700', fontSize: 14 }}>拒绝</Text>
              </View>
            </View>
          )}
          {order.damage.status !== 'pending' && (
            <Text style={{ fontSize: 13, color: '#71717a', marginTop: 4 }}>
              {order.damage.status === 'agreed' ? '已接受定损' : order.damage.status === 'appealed' ? '已提交申诉' : `定损状态：${order.damage.status}`}
            </Text>
          )}
        </View>
      )}

      {/* Action Buttons */}
      <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, backgroundColor: '#fff', borderTop: '1px solid #f4f4f5', padding: 16 }}>
        {isStaff ? (
          <>
            {showStaffShip && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/shipping-interface/index?order_id=${id}` })}
                style={btnStyle('#000')}>📦 发货</View>
            )}
            {showStaffCancel && (
              <View onClick={actionLoading ? undefined : handleStaffCancel}
                style={{ ...btnStyle('#ef4444'), opacity: actionLoading ? 0.5 : 1 }}>
                {actionLoading ? '处理中...' : '❌ 取消订单'}
              </View>
            )}
            {showStaffTransit && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/shipping-interface/index?order_id=${id}` })}
                style={btnStyle('#06b6d4')}>🚚 接收并转发</View>
            )}
            {showStaffReceive && (
              <View onClick={() => Taro.navigateTo({ url: `/pages-weapp/receiving-interface/index?order_id=${id}` })}
                style={btnStyle('#C21838')}>↩️ 接收</View>
            )}
            {showStaffRefund && (
              <View onClick={actionLoading ? undefined : handleStaffRefund}
                style={{ ...btnStyle('#000'), opacity: actionLoading ? 0.5 : 1 }}>
                {actionLoading ? '处理中...' : '💸 退款'}
              </View>
            )}
            {!showStaffShip && !showStaffTransit && !showStaffReceive && !showStaffRefund && (
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
              <View onClick={actionLoading ? undefined : handleConfirmReceipt}
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
              <View onClick={handleReturn}
                style={btnStyle('#f97316')}>
                ↩️ 归还
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

function PhotoPicker({ photos, setPhotos, max = 10 }) {
  return (
    <View style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
      {photos.map((file, i) => (
        <View key={i} style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#f4f4f5', position: 'relative', overflow: 'hidden' }}>
          <Image src={file} style={{ width: 80, height: 80, borderRadius: 8 }} mode="aspectFill" />
          <View onClick={() => setPhotos(prev => prev.filter((_, j) => j !== i))}
            style={{ position: 'absolute', top: 2, right: 2, width: 20, height: 20, borderRadius: 10, backgroundColor: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
            <Text style={{ color: '#fff', fontSize: 12 }}>✕</Text>
          </View>
        </View>
      ))}
      {photos.length < max && (
        <View onClick={() => {
          Taro.chooseImage({ count: max - photos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
            .then(res => setPhotos(prev => [...prev, ...(res.tempFilePaths || [])]))
        }}
          style={{ width: 80, height: 80, borderRadius: 8, border: '1px dashed #d4d4d8', display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#fafafa' }}>
          <Text style={{ color: '#a1a1aa', fontSize: 24 }}>+</Text>
        </View>
      )}
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
    marginBottom: 12,
  }
}
