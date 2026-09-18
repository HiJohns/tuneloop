import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Textarea, Image, Button } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute, uploadFile as uploadFileApi } from '../platform'
import { formatBeijingDate } from '../utils/format'

// #1955 阶段3a：维修服务详情枢纽页（RS-02~RS-09 用户侧动作）
// 状态驱动：选维修师 → 接受报价并支付 → 寄出 → 加价响应 → 待发回 → 评价

const MAX_PHOTOS = 6
const svcStatusLabels = {
  pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
  shipping: '寄送中', repairing: '维修中', adjust_pending: '加价待确认',
  done_repair: '待发回', closed: '已结算',
}
// RS-12：时间线类型 → 展示文案（与后端 appendRepairServiceTimeline 的 record_type 对应）
const timelineLabels = {
  created: '创建维修单', technician_selected: '选择维修师', quoted: '师傅报价',
  quote_accepted: '接受报价', paid: '支付成功', shipped: '乐器寄出',
  adjust_requested: '师傅发起加价', adjust_accepted: '同意加价',
  adjust_paid: '补差价到账', adjust_declined: '拒绝加价', leg_fee: '分段物流费登记',
  repair_completed: '完成修理', settled: '发回结算', reviewed: '提交评价',
  shortfall_paid: '补缴到账',
}
const cardStyle = {
  backgroundColor: '#FFFFFF', borderRadius: 12, padding: 14, marginBottom: 12,
  display: 'flex', flexDirection: 'column', gap: 8,
}
const btnPrimaryStyle = {
  width: '100%', margin: 0, height: 44, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF',
  borderRadius: 10, fontSize: 14, fontWeight: 'bold',
}
const btnWarnStyle = { ...btnPrimaryStyle, backgroundColor: '#D97706' }
const btnSecondaryStyle = {
  width: '100%', margin: 0, height: 40, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#F4F4F5', color: '#3F3F46',
  borderRadius: 10, fontSize: 13, fontWeight: 'bold',
}
const inputStyle = {
  width: '100%', height: 38, boxSizing: 'border-box', backgroundColor: '#FAFAFA',
  border: '1px solid #E4E4E7', borderRadius: 8, padding: '0 10px', fontSize: 13,
}
const labelStyle = { fontSize: 12, color: '#71717A' }
const yuan = (cents) => `¥${((cents || 0) / 100).toFixed(2)}`

export default function RepairServiceDetail() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  // #1674：参数统一 query（order_id），不使用 useParams
  const orderId = new URLSearchParams(env.isMiniProgram ? (Taro.getCurrentInstance()?.router?.params || {}) : window.location.search)
    .get('order_id') || ''
  const [detail, setDetail] = useState(null) // {repair, logistics_fees, review?, site?}
  const [loading, setLoading] = useState(true)
  const [technicians, setTechnicians] = useState([])
  const [techLoaded, setTechLoaded] = useState(false)
  const [busy, setBusy] = useState(false)
  // 寄出表单
  const [shipCompany, setShipCompany] = useState('')
  const [shipNumber, setShipNumber] = useState('')
  // 评价表单
  const [rating, setRating] = useState(0)
  const [reviewMsg, setReviewMsg] = useState('')
  const [reviewPhotos, setReviewPhotos] = useState([])
  const baseUrl = env.apiBaseUrl

  const loadDetail = async () => {
    if (!orderId) { setLoading(false); return }
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}`)
      const result = await res.json()
      if (result.code === 20000) setDetail(result.data || {})
      else dialog.alert(resolveErrorMessage(result))
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setLoading(false)
  }

  useEffect(() => { loadDetail() }, [])

  const loadTechnicians = async () => {
    if (techLoaded) return
    try {
      const res = await apiFetch(`${baseUrl}/common/repair-technicians`)
      const result = await res.json()
      if (result.code === 20000) setTechnicians(result.data?.list || [])
    } catch {}
    setTechLoaded(true)
  }

  const rr = detail?.repair || {}

  const selectTechnician = async (techId) => {
    setBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/select-technician`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ technician_id: techId }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        setTechLoaded(false)
        setTechnicians([])
        loadDetail()
      } else dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  // 支付：prepay（服务端重算金额）→ weapp 拉起微信支付；H5 引导小程序
  const payNow = async (payableCents) => {
    setBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/pay/prepay`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          ...(env.isMiniProgram ? { 'x-client-platform': 'weapp' } : {}),
        },
        body: JSON.stringify({ order_id: orderId, order_type: 'repair', amount: payableCents / 100 }),
      })
      const result = await res.json()
      if (result.code !== 20000 || !result.data) {
        dialog.alert(resolveErrorMessage(result))
        setBusy(false)
        return
      }
      if (env.isMiniProgram) {
        Taro.requestPayment({
          appId: result.data.app_id,
          timeStamp: result.data.time_stamp,
          nonceStr: result.data.nonce_str,
          package: result.data.package,
          signType: result.data.sign_type,
          paySign: result.data.pay_sign,
          success: () => {
            Taro.showToast({ title: '支付成功', icon: 'success' })
            loadDetail()
          },
          fail: (err) => {
            dialog.alert(err?.errMsg || '支付未完成，可稍后在详情页继续支付')
          },
        })
      } else {
        dialog.alert('已创建支付单，请在微信小程序内完成支付')
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setBusy(false)
  }

  const acceptQuote = async () => {
    setBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/accept`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) {
        const payable = result.data?.payable_cents || 0
        setBusy(false)
        if (payable > 0) return payNow(payable)
        loadDetail()
        return
      }
      dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const submitShip = async () => {
    if (!shipNumber.trim()) { dialog.alert('请填写物流单号'); return }
    setBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/ship`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tracking_company: shipCompany.trim(), tracking_number: shipNumber.trim() }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('寄出成功，等待维修师收货')
        loadDetail()
      } else dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const adjustAccept = async () => {
    setBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/adjust/accept`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) {
        const payable = result.data?.payable_cents || 0
        setBusy(false)
        if (payable > 0) return payNow(payable)
        loadDetail()
        return
      }
      dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const adjustDecline = async () => {
    setBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/adjust/decline`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('已选择不继续，乐器将安排发回并按已发生费用结算')
        loadDetail()
      } else dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  const uploadOne = async (file) => {
    const authHeaders = { Authorization: 'Bearer ' + (getToken() || '') }
    if (env.isMiniProgram) {
      const resp = await uploadFileApi(`${baseUrl}/upload`, file, { headers: authHeaders })
      const r = JSON.parse(resp.data)
      if (r.code === 20000) return r.data.file_key
      throw new Error(r.message || 'upload failed')
    }
    const fd = new FormData()
    fd.append('file', file)
    const resp = await fetch(`${baseUrl}/upload`, { method: 'POST', headers: authHeaders, body: fd })
    const r = await resp.json()
    if (r.code === 20000) return r.data.file_key
    throw new Error(resolveErrorMessage(r, 'upload failed'))
  }

  const submitReview = async () => {
    if (!rating) { dialog.alert('请选择评分'); return }
    setBusy(true)
    try {
      const keys = []
      for (const f of reviewPhotos) keys.push(await uploadOne(f))
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/review`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ rating, message: reviewMsg.trim(), photos: keys }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('评价成功，感谢反馈')
        loadDetail()
      } else dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  if (loading) {
    return (
      <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Text style={{ fontSize: 13, color: '#A1A1AA' }}>加载中...</Text>
      </View>
    )
  }
  if (!detail || !rr.id) {
    return (
      <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh', display: 'flex', flexDirection: 'column' }}>
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务详情</Text>
        </View>
        <View style={{ padding: 24, display: 'flex', justifyContent: 'center' }}>
          <Text style={{ fontSize: 13, color: '#A1A1AA' }}>维修单不存在或已删除</Text>
        </View>
      </View>
    )
  }

  const site = detail.site
  const photos = Array.isArray(rr.photos) ? rr.photos : (() => { try { return JSON.parse(rr.photos || '[]') } catch { return [] } })()
  const reviewPhotosParsed = detail.review && detail.review.photos
    ? (Array.isArray(detail.review.photos) ? detail.review.photos : (() => { try { return JSON.parse(detail.review.photos || '[]') } catch { return [] } })())
    : []
  const quoteTotal = (rr.quote_repair_cents || 0) + (rr.quote_logistics_cents || 0)
  const adjustDiff = rr.adjusted_quote_cents != null && rr.quote_repair_cents != null
    ? Math.max(0, rr.adjusted_quote_cents - rr.quote_repair_cents) : null

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务详情</Text>
      </View>

      <View style={{ padding: 16, display: 'flex', flexDirection: 'column' }}>
        {/* 基础信息 */}
        <View style={cardStyle}>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
            <Text style={{ fontSize: 15, fontWeight: 'bold', color: '#18181B', letterSpacing: 2 }}>
              {rr.repair_code || '-'}
            </Text>
            <Text style={{ fontSize: 12, color: '#D97706', fontWeight: 'bold' }}>
              {svcStatusLabels[rr.status] || rr.status}
            </Text>
          </View>
          <Text style={{ fontSize: 13, color: '#3F3F46' }}>{rr.description || '（无描述）'}</Text>
          {photos.length > 0 && (
            <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
              {photos.map((p, i) => (
                <Image key={i} src={p} mode="aspectFill" style={{ width: 72, height: 72, borderRadius: 8 }} />
              ))}
            </View>
          )}
          <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
            创建于 {rr.created_at ? formatBeijingDate(rr.created_at) : '-'}
          </Text>
        </View>

        {/* RS-12 费用明细 */}
        <View style={cardStyle}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>费用明细</Text>
          <View style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Text style={labelStyle}>报价修理费</Text>
            <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_repair_cents)}</Text>
          </View>
          <View style={{ display: 'flex', justifyContent: 'space-between' }}>
            <Text style={labelStyle}>物流费预估</Text>
            <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_logistics_cents)}</Text>
          </View>
          {rr.adjusted_quote_cents != null && (
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>加价后修理费</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.adjusted_quote_cents)}</Text>
            </View>
          )}
          {rr.incurred_repair_cents != null && rr.quote_status === 'declined' && (
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>到此为止修理费（按停止时结算）</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.incurred_repair_cents)}</Text>
            </View>
          )}
          {(detail.logistics_fees || []).map(f => (
            <View key={f.id} style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>第 {f.leg} 段物流（实填）</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(f.amount_cents)}</Text>
            </View>
          ))}
          {detail.payments && (
            <>
              <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>已付合计</Text>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>{yuan(detail.payments.made_cents)}</Text>
              </View>
              {detail.payments.refund_cents > 0 && (
                <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Text style={labelStyle}>已退款</Text>
                  <Text style={{ fontSize: 12, color: '#16A34A' }}>{yuan(detail.payments.refund_cents)}</Text>
                </View>
              )}
              {detail.payments.pending_shortfall_cents > 0 && (
                <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#D97706' }}>待补缴</Text>
                  <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#D97706' }}>{yuan(detail.payments.pending_shortfall_cents)}</Text>
                </View>
              )}
            </>
          )}
        </View>

        {/* RS-12 物流明细 */}
        {((rr.tracking_number || rr.return_tracking_number || (detail.logistics_fees || []).length > 0) && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>物流明细</Text>
            {rr.tracking_number && (
              <Text style={labelStyle}>寄出：{rr.tracking_company || '-'} {rr.tracking_number}</Text>
            )}
            {(detail.logistics_fees || []).map(f => (
              <Text key={f.id} style={labelStyle}>第 {f.leg} 段实填运费：{yuan(f.amount_cents)}（{f.created_at ? formatBeijingDate(f.created_at) : '-'}）</Text>
            ))}
            {rr.return_tracking_number && (
              <Text style={labelStyle}>发回：{rr.return_company || '-'} {rr.return_tracking_number}</Text>
            )}
          </View>
        ))}

        {/* RS-02 选维修师 */}
        {rr.status === 'pending_quote' && !rr.technician_id && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>选择维修师</Text>
            {!techLoaded ? (
              <Button onClick={loadTechnicians} style={btnSecondaryStyle}>加载可选择的维修师</Button>
            ) : technicians.length === 0 ? (
              <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无可选维修师</Text>
            ) : technicians.map(t => (
              <View key={t.technician_id}
                onClick={() => { if (!busy) selectTechnician(t.technician_id) }}
                style={{ border: '1px solid #E4E4E7', borderRadius: 10, padding: 10, display: 'flex', flexDirection: 'column', gap: 2 }}>
                <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>{t.name || '维修师'}</Text>
                  <Text style={{ fontSize: 12, color: '#171717', fontWeight: 'bold' }}>选择</Text>
                </View>
                <Text style={{ fontSize: 11, color: '#71717A' }}>{t.site_name || ''}</Text>
                {t.site_address ? <Text style={{ fontSize: 11, color: '#A1A1AA' }}>{t.site_address}</Text> : null}
              </View>
            ))}
          </View>
        )}

        {/* RS-03 接受报价并支付 */}
        {rr.status === 'pending_payment' && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>师傅报价</Text>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>修理费</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_repair_cents)}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>物流费预估</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_logistics_cents)}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>合计应付</Text>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#D97706' }}>{yuan(quoteTotal)}</Text>
            </View>
            {rr.quote_status === 'pending' && (
              <Button disabled={busy} onClick={acceptQuote} style={btnPrimaryStyle}>
                接受报价并支付 {yuan(quoteTotal)}
              </Button>
            )}
          </View>
        )}

        {/* RS-04 寄出 */}
        {rr.status === 'paid' && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>寄出乐器</Text>
            {site && (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Text style={labelStyle}>寄件地址：{site.name}</Text>
                {site.address ? <Text style={labelStyle}>{site.address}</Text> : null}
                {site.contact_name || site.phone ? (
                  <Text style={labelStyle}>联系人：{site.contact_name || '-'} {site.phone || ''}</Text>
                ) : null}
              </View>
            )}
            <Text style={labelStyle}>请将维修编码 {rr.repair_code} 写在物流单信息栏</Text>
            <Text style={labelStyle}>物流公司</Text>
            <Input style={inputStyle} value={shipCompany} onInput={e => setShipCompany(getInputValue(e))} placeholder="如 顺丰速运" />
            <Text style={labelStyle}>物流单号（必填）</Text>
            <Input style={inputStyle} value={shipNumber} onInput={e => setShipNumber(getInputValue(e))} placeholder="运单号" />
            <Button disabled={busy} onClick={submitShip} style={btnPrimaryStyle}>确认寄出</Button>
          </View>
        )}

        {/* RS-06 加价响应 */}
        {rr.status === 'adjust_pending' && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>师傅发起加价</Text>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>新修理费总价</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.adjusted_quote_cents)}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>到此为止修理费（不继续时按此结算）</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.incurred_repair_cents)}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>继续需补差价</Text>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#D97706' }}>{yuan(adjustDiff)}</Text>
            </View>
            <Button disabled={busy} onClick={adjustAccept} style={btnPrimaryStyle}>
              继续修理并补差价 {yuan(adjustDiff)}
            </Button>
            <Button disabled={busy} onClick={adjustDecline} style={btnWarnStyle}>
              不继续，安排发回
            </Button>
          </View>
        )}

        {/* 进行中提示 */}
        {(rr.status === 'shipping' || rr.status === 'repairing') && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>
              {rr.status === 'shipping' ? '乐器寄送中' : '维修进行中'}
            </Text>
            {rr.tracking_number ? (
              <Text style={labelStyle}>寄出物流：{rr.tracking_company || '-'} {rr.tracking_number}</Text>
            ) : null}
            <Text style={labelStyle}>
              {rr.status === 'shipping' ? '等待维修师收货' : '维修完成后将由网点安排发回'}
            </Text>
          </View>
        )}

        {/* 待发回 */}
        {rr.status === 'done_repair' && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>维修完成</Text>
            <Text style={labelStyle}>等待网点发回，发回时自动按实际费用结算（多退少补）</Text>
          </View>
        )}

        {/* RS-09 评价 */}
        {/* RS-API-7 待补缴支付（closed + 有 pending 补缴） */}
        {rr.status === 'closed' && detail.payments && detail.payments.pending_shortfall_cents > 0 && (
          <View style={cardStyle}>
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>维修服务补缴</Text>
              <Text style={{ fontSize: 14, fontWeight: 'bold', color: '#D97706' }}>
                {yuan(detail.payments.pending_shortfall_cents)}
              </Text>
            </View>
            <Text style={labelStyle}>实际费用超出预付部分，未支付将影响会员升级</Text>
            <Button disabled={busy} onClick={() => payNow(detail.payments.pending_shortfall_cents)}
              style={btnPrimaryStyle}>
              支付补缴 {yuan(detail.payments.pending_shortfall_cents)}
            </Button>
          </View>
        )}

        {rr.status === 'closed' && (
          <View style={cardStyle}>
            {detail.review && detail.review.id ? (
              <>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>我的评价</Text>
                <Text style={{ fontSize: 13, color: '#D97706' }}>{'★'.repeat(detail.review.rating || 0)}</Text>
                {detail.review.message ? <Text style={{ fontSize: 12, color: '#3F3F46' }}>{detail.review.message}</Text> : null}
                {reviewPhotosParsed.length > 0 && (
                  <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {reviewPhotosParsed.map((p, i) => (
                      <Image key={i} src={p} mode="aspectFill" style={{ width: 72, height: 72, borderRadius: 8 }} />
                    ))}
                  </View>
                )}
              </>
            ) : (
              <>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>评价本次维修服务</Text>
                <View style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                  {[1, 2, 3, 4, 5].map(n => (
                    <Text key={n} onClick={() => setRating(n)}
                      style={{ fontSize: 26, color: n <= rating ? '#F59E0B' : '#E4E4E7' }}>★</Text>
                  ))}
                </View>
                <Textarea style={{ width: '100%', boxSizing: 'border-box', minHeight: 60, backgroundColor: '#FAFAFA', border: '1px solid #E4E4E7', borderRadius: 8, padding: 8, fontSize: 13 }}
                  value={reviewMsg} maxlength={200} placeholder="留言（可选）"
                  onInput={e => setReviewMsg(getInputValue(e))} />
                <View style={{ display: 'flex', flexWrap: 'wrap', gap: 8 }}>
                  {reviewPhotos.map((f, i) => (
                    <Image key={i} src={env.isMiniProgram ? f : URL.createObjectURL(f)} mode="aspectFill"
                      style={{ width: 60, height: 60, borderRadius: 8 }}
                      onClick={() => setReviewPhotos(p => p.filter((_, j) => j !== i))} />
                  ))}
                  {reviewPhotos.length < MAX_PHOTOS && (env.isMiniProgram ? (
                    <View onClick={async () => {
                      try {
                        const res = await Taro.chooseImage({ count: MAX_PHOTOS - reviewPhotos.length, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
                        setReviewPhotos(p => [...p, ...(res.tempFilePaths || [])].slice(0, MAX_PHOTOS))
                      } catch (err) { console.error('choose image failed:', err) }
                    }}
                      style={{ width: 60, height: 60, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                      <Text style={{ fontSize: 20, color: '#A1A1AA' }}>＋</Text>
                    </View>
                  ) : (
                    <View style={{ width: 60, height: 60, borderRadius: 8, border: '1px dashed #D4D4D8', display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
                      <Text style={{ fontSize: 20, color: '#A1A1AA' }}>＋</Text>
                      <input type="file" accept="image/*" multiple className="hidden"
                        style={{ position: 'absolute', inset: 0, opacity: 0 }}
                        onChange={(e) => {
                          const files = Array.from(e.target.files || [])
                          setReviewPhotos(p => [...p, ...files].slice(0, MAX_PHOTOS))
                          e.target.value = ''
                        }} />
                    </View>
                  ))}
                </View>
                <Button disabled={busy} onClick={submitReview} style={btnPrimaryStyle}>
                  {busy ? '处理中...' : '提交评价'}
                </Button>
              </>
            )}
          </View>
        )}

        {/* RS-12 状态时间线 */}
        {(detail.timeline || []).length > 0 && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>进度记录</Text>
            {detail.timeline.map((t, i) => (
              <View key={t.id || i} style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Text style={{ fontSize: 12, fontWeight: 'bold', color: '#3F3F46' }}>
                    {timelineLabels[t.record_type] || t.record_type}
                  </Text>
                  <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
                    {t.created_at ? formatBeijingDate(t.created_at) : '-'}
                  </Text>
                </View>
                {t.comment ? <Text style={{ fontSize: 11, color: '#71717A' }}>{t.comment}</Text> : null}
              </View>
            ))}
          </View>
        )}
      </View>
    </View>
  )
}
