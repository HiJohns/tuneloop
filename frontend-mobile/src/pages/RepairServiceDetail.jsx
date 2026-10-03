import { useState, useEffect } from 'react'
import { formatCents } from '../utils/money'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Textarea, Image, Video, Button, Input } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute, uploadFile as uploadFileApi } from '../platform'
import { parseJWT } from '../platform/init'
import { formatBeijingDate } from '../utils/format'
import { photoSrc } from '../utils/media'
import ImageUploader from '../components/ImageUploader'

// #1955 阶段3a：维修服务详情枢纽页（RS-02~RS-09 用户侧动作）
// 状态驱动：选维修师 → 接受报价并支付 → 寄出 → 加价响应 → 待发回 → 评价

const MAX_PHOTOS = 6
const svcStatusLabels = {
  pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
  shipping: '寄送中', pending_repair: '待维修', repairing: '维修中', adjust_pending: '加价待确认',
  done_repair: '待发回', closed: '已结算', cancelled: '已取消',
}
// #2093：拒绝报价理由（与后端 repairQuoteDeclineReasons 枚举一致）
const DECLINE_REASONS = [
  { code: 'too_expensive', label: '太贵了' },
  { code: 'found_other', label: '已找别人修了' },
  { code: 'solved', label: '问题已解决' },
  { code: 'other', label: '其他' },
]
// RS-12：时间线类型 → 展示文案（与后端 appendRepairServiceTimeline 的 record_type 对应）
const timelineLabels = {
  created: '创建维修单', technician_selected: '选择维修师', quoted: '维修师报价',
  quote_accepted: '接受报价', paid: '支付成功', shipped: '乐器寄出',
  adjust_requested: '维修师发起加价', adjust_accepted: '同意加价',
  adjust_paid: '补差价到账', adjust_declined: '拒绝加价', leg_fee: '分段物流费登记',
  quote_declined: '拒绝报价',
  received_by_tech: '维修师收货', received_by_staff: '网点代收货', repair_started: '开始维修', // #2116
  repair_completed: '完成修理', settled: '发回结算', reviewed: '提交评价',
  shortfall_paid: '补缴到账', payment_timeout: '支付超时关闭', // #2094
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
const yuan = (cents) => `¥${formatCents((cents || 0))}`

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
  // #2119: 技师/员工加价（详情页入口）
  const [showAdj, setShowAdj] = useState(false)
  const [receiveFiles, setReceiveFiles] = useState([]) // #2121: 详情页收货拍照（多张）
  const [actBusy, setActBusy] = useState(false)
  // #2122: 寄回物流（done_repair）
  const [retCompany, setRetCompany] = useState('')
  const [retNumber, setRetNumber] = useState('')
  const [retFeeYuan, setRetFeeYuan] = useState('')
  const [retFeeEditing, setRetFeeEditing] = useState(false)
  const [adjYuan, setAdjYuan] = useState('')
  const [adjIncurredYuan, setAdjIncurredYuan] = useState('')
  const [adjBusy, setAdjBusy] = useState(false)
  const [technicians, setTechnicians] = useState([])
  const [techLoaded, setTechLoaded] = useState(false)
  const [busy, setBusy] = useState(false)
  // #2093：拒绝报价弹层
  const [showDecline, setShowDecline] = useState(false)
  const [declineReason, setDeclineReason] = useState('')
  const [declineNote, setDeclineNote] = useState('')
  const [declining, setDeclining] = useState(false)
  // 寄出表单
  const [shipCompany, setShipCompany] = useState('')
  const [shipNumber, setShipNumber] = useState('')
  // 评价表单
  const [rating, setRating] = useState(0)
  const [reviewMsg, setReviewMsg] = useState('')
  const [reviewPhotos, setReviewPhotos] = useState([])
  const baseUrl = env.apiBaseUrl
  // #2119: 员工/技师视图判定（JWT role 非 USER）
  const isStaffView = (() => { try { const t = getToken(); return !!t && (parseJWT(t)?.role || '') !== 'USER' && (parseJWT(t)?.role || '') !== '' } catch { return false } })()


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

  // #2122: 寄回并结算（done_repair；技师/员工）——物流费可编辑为实际
  const doDispatch = async () => {
    if (!retNumber.trim()) { dialog.alert('请填写物流单号'); return }
    const fee = retFeeYuan === '' ? 0 : Math.round(parseFloat(retFeeYuan) * 100)
    if (isNaN(fee) || fee < 0) { dialog.alert('请填写正确的物流费'); return }
    setActBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${orderId}/dispatch`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tracking_company: retCompany, tracking_number: retNumber.trim(), logistics_fee_cents: fee }),
      })
      const result = await res.json()
      if (result.code === 20000) { dialog.alert('已发回并结算'); loadDetail() }
      else { dialog.alert(resolveErrorMessage(result)) }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setActBusy(false)
  }

  // #2121: 详情页可执行操作（收货/开始维修/完成修理）——服务端守卫权威
  const doReceive = async () => {
    if (receiveFiles.length === 0) { dialog.alert('请先拍照留档'); return }
    setActBusy(true)
    try {
      const authHeaders = { Authorization: 'Bearer ' + (getToken() || '') }
      const keys = []
      for (const f of receiveFiles) {
        const resp = await uploadFileApi(`${baseUrl}/upload`, f, { headers: authHeaders })
        if (resp.statusCode === 401) throw new Error('登录态已失效，请重新登录')
        const r = JSON.parse(resp.data)
        if (r.code !== 20000) throw new Error(r.message || '上传失败')
        keys.push(r.data.file_key)
      }
      const res = await apiFetch(`${baseUrl}/repair-services/${orderId}/receive`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ photos: keys }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert(result.data?.status === 'repairing' ? '已确认收货，开始维修' : '已代收货，等待维修师开始维修')
        setReceiveFiles([])
        loadDetail()
      } else { dialog.alert(resolveErrorMessage(result)) }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setActBusy(false)
  }

  const doStart = async () => {
    setActBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${orderId}/start`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) { dialog.alert('已开始维修'); loadDetail() }
      else { dialog.alert(resolveErrorMessage(result)) }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setActBusy(false)
  }

  const doComplete = async () => {
    setActBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${orderId}/complete`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) { dialog.alert('已完工，等待网点发回'); loadDetail() }
      else { dialog.alert(resolveErrorMessage(result)) }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setActBusy(false)
  }

  // #2119: 加价提交（收货后维修中；服务端守卫权威）
  const submitAdjust = async () => {
    const toC = (v) => { const n = parseFloat(v); return isNaN(n) || n < 0 ? -1 : Math.round(n * 100) }
    const newQ = toC(adjYuan)
    const incurred = adjIncurredYuan === '' ? 0 : toC(adjIncurredYuan)
    if (newQ <= 0) { dialog.alert('请填写正确的新修理费总价'); return }
    if (incurred < 0) { dialog.alert('请填写正确的到此为止修理费'); return }
    setAdjBusy(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${orderId}/adjust`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ new_quote_cents: newQ, incurred_cents: incurred }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('加价申请已提交，等待用户确认')
        setShowAdj(false)
        loadDetail()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setAdjBusy(false)
  }

  useEffect(() => { loadDetail() }, [])
  // #2122: 寄回物流费默认按预估（物流费预估分→元），可编辑为实际
  useEffect(() => {
    if (detail?.repair && retFeeYuan === '') {
      setRetFeeYuan(((detail.repair.quote_logistics_cents || 0) / 100).toFixed(2))
    }
  }, [detail])

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
      // #2092：prepay 响应为双层 data（{code, data:{success, data:{app_id,time_stamp,...}}}），
      // 与 Payment.jsx 约定一致：参数在 result.data.data。
      const prepay = result.data?.data
      if (result.code !== 20000 || !prepay) {
        dialog.alert(resolveErrorMessage(result, '无法获取支付参数，请稍后重试'))
        setBusy(false)
        return
      }
      if (env.isMiniProgram) {
        // 有金额的真实支付必须有 prepay_id；缺失禁止假成功（2026-09-06 incident 口径）。
        // H5 路径为 Native 指引（data.data 仅 code_url），不在此守卫范围。
        if (!prepay.prepay_id) {
          dialog.alert('无法获取支付参数，请稍后重试')
          setBusy(false)
          return
        }
        Taro.requestPayment({
          appId: prepay.app_id || 'wxcb44a1be70e356ed',
          timeStamp: prepay.time_stamp,
          nonceStr: prepay.nonce_str,
          package: prepay.package,
          signType: prepay.sign_type,
          paySign: prepay.pay_sign,
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
        if (payable > 0) {
          // #2096：跳标准支付页（明细 + 优惠码 + 确认）——不再直拉微信支付
          if (env.isMiniProgram) {
            Taro.navigateTo({ url: `/pages-weapp/payment/index?type=repair_service&order_id=${orderId}` })
          } else {
            dialog.alert('请在微信小程序内完成支付')
          }
          return
        }
        loadDetail()
        return
      }
      dialog.alert(resolveErrorMessage(result))
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setBusy(false)
  }

  // #2093：提交拒绝报价 → 终态 cancelled
  const submitDecline = async () => {
    if (!declineReason) return
    setDeclining(true)
    try {
      const res = await apiFetch(`${baseUrl}/user/repair-services/${orderId}/quote/decline`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ reason: declineReason, note: declineNote.trim() }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        setShowDecline(false)
        dialog.alert('已拒绝报价，维修服务已关闭')
        loadDetail()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    setDeclining(false)
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
        {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务详情</Text>
        </View>
        )}
        <View style={{ padding: 24, display: 'flex', justifyContent: 'center' }}>
          <Text style={{ fontSize: 13, color: '#A1A1AA' }}>维修单不存在或已删除</Text>
        </View>
      </View>
    )
  }

  const site = detail.site
  const cc = detail.merchant // 商户地址（无 site 的新单寄件地址来源）
  const photos = Array.isArray(rr.photos) ? rr.photos : (() => { try { return JSON.parse(rr.photos || '[]') } catch { return [] } })()
  // #2094：真结算判定——closed 且时间线含 settled（发回结算）；未含则为超时/异常关闭 → 不显示评价
  const hasSettledTimeline = (detail.timeline || []).some(t => t.record_type === 'settled')
  const reviewPhotosParsed = detail.review && detail.review.photos
    ? (Array.isArray(detail.review.photos) ? detail.review.photos : (() => { try { return JSON.parse(detail.review.photos || '[]') } catch { return [] } })())
    : []
  const quoteTotal = (rr.quote_repair_cents || 0) + (rr.quote_material_cents || 0) + (rr.quote_logistics_cents || 0) // #2085 含料钱
  const adjustDiff = rr.adjusted_quote_cents != null && rr.quote_repair_cents != null
    ? Math.max(0, rr.adjusted_quote_cents - rr.quote_repair_cents) : null

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      {!env.isMiniProgram && (
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务详情</Text>
      </View>
      )}

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
                <Image key={i} src={photoSrc(p)} mode="aspectFill"
                onClick={() => Taro.previewImage({ urls: photos.map(x => photoSrc(x)), current: photoSrc(p) })}
                style={{ width: 72, height: 72, borderRadius: 8 }} />
              ))}
            </View>
          )}
          {/* #2060: 试奏视频（有则播放） */}
          {rr.video_url ? (
            <View style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              <Text style={{ fontSize: 12, color: '#71717A' }}>试奏视频</Text>
              <Video src={photoSrc(rr.video_url)} controls
                style={{ width: '100%', height: 200, borderRadius: 8, backgroundColor: '#000000' }} />
            </View>
          ) : null}
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
          {rr.quote_material_cents != null && (
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>料钱</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_material_cents)}</Text>
            </View>
          )}
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
          {detail.payments && (() => {
            const couponTotal = (detail.payments.records || [])
              .filter(r => r.status === 'paid')
              .reduce((acc, r) => acc + Number(r.coupon_discount_cents || 0), 0)
            return (
            <>
              {couponTotal > 0 && (
                <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Text style={labelStyle}>优惠码抵扣</Text>
                  <Text style={{ fontSize: 12, color: '#16A34A' }}>−{yuan(couponTotal)}</Text>
                </View>
              )}
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
            )
          })()}
        </View>

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
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>维修师报价</Text>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>修理费</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_repair_cents)}</Text>
            </View>
            {rr.quote_material_cents != null && (
              <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text style={labelStyle}>料钱</Text>
                <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_material_cents)}</Text>
              </View>
            )}
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={labelStyle}>物流费预估</Text>
              <Text style={{ fontSize: 12, color: '#18181B' }}>{yuan(rr.quote_logistics_cents)}</Text>
            </View>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>合计应付</Text>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#D97706' }}>{yuan(quoteTotal)}</Text>
            </View>
            {rr.quote_status === 'accepted' && (
              <>
                {/* #2113: 已接受未支付——恢复支付入口（此前该状态无任何按钮，死路） */}
                <Button disabled={busy} onClick={() => {
                  if (env.isMiniProgram) {
                    Taro.navigateTo({ url: `/pages-weapp/payment/index?type=repair_service&order_id=${orderId}` })
                  } else {
                    dialog.alert('请在微信小程序内完成支付')
                  }
                }} style={btnPrimaryStyle}>
                  去支付 {yuan(quoteTotal)}
                </Button>
              </>
            )}
            {rr.quote_status === 'pending' && (
              <>
                <Button disabled={busy} onClick={acceptQuote} style={btnPrimaryStyle}>
                  接受报价并支付 {yuan(quoteTotal)}
                </Button>
                {/* #2093：拒绝报价（理由选择） */}
                <Button disabled={busy} onClick={() => { setDeclineReason(''); setDeclineNote(''); setShowDecline(true) }} style={btnSecondaryStyle}>
                  拒绝报价
                </Button>
              </>
            )}
          </View>
        )}

        {/* RS-04 寄出 */}
        {rr.status === 'paid' && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>寄出乐器</Text>
            {/* #2122：维修师直属商户 → 收件信息 = 商户地址/电话 + 指派维修师名 */}
            {(cc || site) && (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <Text style={labelStyle}>收件地址：{(cc || site).name}</Text>
                {(cc || site).address ? <Text style={labelStyle}>{(cc || site).address}</Text> : null}
                {(cc || site).phone || (cc || site).contact_name ? (
                  <Text style={labelStyle}>联系电话：{(cc || site).phone || (cc || site).contact_name}</Text>
                ) : null}
                {detail.technician?.name ? (
                  <Text style={labelStyle}>收件人（维修师）：{detail.technician.name}</Text>
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
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>维修师发起加价</Text>
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
              {rr.status === 'shipping' ? '等待网点/维修师收货确认' : '维修完成后将由网点安排发回'}
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

        {rr.status === 'cancelled' && (
          <View style={cardStyle}>
            {/* #2094：未支付超时取消/拒绝取消 → 关闭信息卡（不得出现评价表单） */}
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#A1A1AA' }}>服务已关闭</Text>
            <Text style={labelStyle}>该维修服务未完成支付，已自动关闭；如有需要可重新发起维修。</Text>
          </View>
        )}

        {rr.status === 'closed' && hasSettledTimeline && (
          <View style={cardStyle}>
            {detail.review && detail.review.id ? (
              <>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>我的评价</Text>
                <Text style={{ fontSize: 13, color: '#D97706' }}>{'★'.repeat(detail.review.rating || 0)}</Text>
                {detail.review.message ? <Text style={{ fontSize: 12, color: '#3F3F46' }}>{detail.review.message}</Text> : null}
                {reviewPhotosParsed.length > 0 && (
                  <View style={{ display: 'flex', flexWrap: 'wrap', gap: 6 }}>
                    {reviewPhotosParsed.map((p, i) => (
                      <Image key={i} src={photoSrc(p)} mode="aspectFill" style={{ width: 72, height: 72, borderRadius: 8 }} />
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

        {/* #2119: 收货存档照（收货环节拍照留档） */}
        {(() => {
          let rp = []
          try { rp = Array.isArray(rr.receive_photos) ? rr.receive_photos : JSON.parse(rr.receive_photos || '[]') } catch { rp = [] }
          if (rp.length === 0) return null
          return (
            <View style={cardStyle}>
              <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>收货存档照</Text>
              {rr.received_at ? (
                <Text style={{ fontSize: 11, color: '#A1A1AA' }}>收货时间：{formatBeijingDate(rr.received_at)}</Text>
              ) : null}
              <View style={{ display: 'flex', flexDirection: 'row', flexWrap: 'wrap', gap: 6 }}>
                {rp.map((p, i) => (
                  <Image key={i} src={photoSrc(p)} mode="aspectFill"
                    onClick={() => Taro.previewImage({ urls: rp.map(x => photoSrc(x)), current: photoSrc(p) })}
                    style={{ width: 96, height: 96, borderRadius: 6, backgroundColor: '#f4f4f5' }} />
                ))}
              </View>
            </View>
          )
        })()}

        {/* #2121: 状态×角色「可执行操作」卡（服务端守卫权威） */}
        {isStaffView && ['shipping', 'pending_repair', 'repairing', 'done_repair'].includes(rr.status) && (
          <View style={cardStyle}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>可执行操作</Text>
            {rr.status === 'shipping' && (
              <>
                <Text style={labelStyle}>收货拍照（可拍多张，至少 1 张后统一提交）</Text>
                <ImageUploader maxImages={9} onChange={(files) => setReceiveFiles(files)} />
                <Button disabled={actBusy || receiveFiles.length === 0} onClick={doReceive}
                  style={{ width: '100%', margin: 0, height: 44, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 14, fontWeight: 'bold', opacity: (actBusy || receiveFiles.length === 0) ? 0.5 : 1 }}>
                  {actBusy ? '处理中...' : `确认收货（${receiveFiles.length} 张）`}
                </Button>
              </>
            )}
            {rr.status === 'pending_repair' && (
              <Button disabled={actBusy} onClick={doStart}
                style={{ width: '100%', margin: 0, height: 44, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 14, fontWeight: 'bold', opacity: actBusy ? 0.5 : 1 }}>
                {actBusy ? '处理中...' : '开始维修'}
              </Button>
            )}
            {rr.status === 'done_repair' && (
              <>
                <Text style={labelStyle}>寄回物流（顾客收件信息）</Text>
                {detail.customer ? (
                  <View style={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                    <Text style={labelStyle}>收件人：{detail.customer.name || '-'} {detail.customer.phone || ''}</Text>
                    {detail.customer.address ? <Text style={labelStyle}>{detail.customer.address}</Text> : null}
                  </View>
                ) : null}
                <Text style={labelStyle}>物流公司</Text>
                <Input style={inputStyle} value={retCompany} onInput={e => setRetCompany(getInputValue(e))} placeholder="如 顺丰速运" />
                <Text style={labelStyle}>物流单号（必填）</Text>
                <Input style={inputStyle} value={retNumber} onInput={e => setRetNumber(getInputValue(e))} placeholder="运单号" />
                <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                  <Text style={labelStyle}>物流费（元）{retFeeEditing ? '· 实际' : '· 预估'}</Text>
                  <Text onClick={() => setRetFeeEditing(!retFeeEditing)} style={{ fontSize: 12, color: '#2563EB', fontWeight: 'bold' }}>
                    {retFeeEditing ? '取消编辑' : '编辑'}
                  </Text>
                </View>
                <Input style={inputStyle} type="digit" value={retFeeYuan} disabled={!retFeeEditing}
                  onInput={e => setRetFeeYuan(getInputValue(e))} placeholder="按预估" />
                <Button disabled={actBusy} onClick={doDispatch}
                  style={{ width: '100%', margin: 0, height: 44, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 14, fontWeight: 'bold', opacity: actBusy ? 0.5 : 1 }}>
                  {actBusy ? '处理中...' : '确认发回并结算'}
                </Button>
              </>
            )}
            {rr.status === 'repairing' && (
              <>
                <Button disabled={actBusy} onClick={doComplete}
                  style={{ width: '100%', margin: 0, height: 44, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 14, fontWeight: 'bold', opacity: actBusy ? 0.5 : 1 }}>
                  {actBusy ? '处理中...' : '完成修理'}
                </Button>
                <View onClick={() => setShowAdj(!showAdj)} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', paddingTop: 4 }}>
                  <Text style={{ fontSize: 12, color: '#3F3F46', fontWeight: 'bold' }}>发起加价</Text>
                  <Text style={{ fontSize: 12, color: '#71717A' }}>{showAdj ? '收起' : '展开'}</Text>
                </View>
                {showAdj && (
                  <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
                    <Text style={labelStyle}>新修理费总价（元）</Text>
                    <Input style={inputStyle} type="digit" value={adjYuan}
                      onInput={e => setAdjYuan(getInputValue(e))} placeholder="如 300" />
                    <Text style={labelStyle}>到此为止修理费（元，用户不继续时按此结算）</Text>
                    <Input style={inputStyle} type="digit" value={adjIncurredYuan}
                      onInput={e => setAdjIncurredYuan(getInputValue(e))} placeholder="如 50" />
                    <Button disabled={adjBusy} onClick={submitAdjust}
                      style={{ width: '100%', margin: 0, height: 44, display: 'flex', alignItems: 'center', justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF', borderRadius: 10, fontSize: 14, fontWeight: 'bold', opacity: adjBusy ? 0.5 : 1 }}>
                      {adjBusy ? '处理中...' : '提交加价申请'}
                    </Button>
                  </View>
                )}
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
                    {t.operator ? ` · ${t.operator}` : ''}
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

      {/* #2093 拒绝报价弹层（自绘；weapp 安全：View/Text/Textarea） */}
      {showDecline && (
        <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.45)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: '0 24px', boxSizing: 'border-box' }}>
          <View style={{ width: '100%', backgroundColor: '#FFFFFF', borderRadius: 12, padding: 16, display: 'flex', flexDirection: 'column', gap: 10 }}>
            <Text style={{ fontSize: 15, fontWeight: 'bold', color: '#18181B' }}>拒绝报价</Text>
            <Text style={labelStyle}>请选择拒绝原因</Text>
            {DECLINE_REASONS.map(opt => (
              <View key={opt.code} onClick={() => setDeclineReason(opt.code)}
                style={{ display: 'flex', flexDirection: 'row', alignItems: 'center', gap: 8, paddingTop: 10, paddingBottom: 10, paddingLeft: 12, paddingRight: 12, border: declineReason === opt.code ? '1px solid #915F38' : '1px solid #E4E4E7', borderRadius: 8, backgroundColor: declineReason === opt.code ? '#FDF6F0' : '#FFFFFF' }}>
                <View style={{ width: 16, height: 16, borderRadius: 999, boxSizing: 'border-box', border: declineReason === opt.code ? '5px solid #915F38' : '1px solid #D4D4D8' }} />
                <Text style={{ fontSize: 13, color: '#3F3F46' }}>{opt.label}</Text>
              </View>
            ))}
            <Text style={labelStyle}>备注（可选，≤200 字）</Text>
            <Textarea value={declineNote} maxlength={200} onInput={e => setDeclineNote(getInputValue(e))} placeholder="补充说明（可选）"
              style={{ width: '100%', boxSizing: 'border-box', minHeight: 60, backgroundColor: '#FFFFFF', border: '1px solid #E4E4E7', borderRadius: 8, padding: 10, fontSize: 13 }} />
            <View style={{ display: 'flex', flexDirection: 'row', gap: 8 }}>
              <Button disabled={declining} onClick={() => setShowDecline(false)} style={{ ...btnSecondaryStyle, flex: 1 }}>取消</Button>
              <Button disabled={declining || !declineReason} onClick={submitDecline}
                style={{ ...btnPrimaryStyle, flex: 1, opacity: (declining || !declineReason) ? 0.5 : 1 }}>
                {declining ? '处理中...' : '确认拒绝报价'}
              </Button>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
