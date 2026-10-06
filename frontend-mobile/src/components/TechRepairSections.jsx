// #2084：维修师工作台区块（待报价/维修中/已完成 + 报价/加价/完成）共享组件。
// 用于：① MyRepairs「维修服务」Tab 内联（省一跳）② 独立页 /tech-repair-workbench（RS-02 深链）。
// 自包含（挂载即拉 scope=mine 三查询）；仅用 @tarojs/components + platform + services/api（跨端）。
import { useState, useEffect } from 'react'
import { formatCents, yuanToCents as toCents } from '../utils/money'
import { View, Text, Input, Video, Button, Image } from '@tarojs/components'
import { apiFetch, resolveErrorMessage, getToken } from '../services/api'
import Taro from '@tarojs/taro'
import { dialog, env, getInputValue, uploadFile as uploadFileApi } from '../platform'
import { formatBeijingDate } from '../utils/format'
import { photoSrc } from '../utils/media'
import ImageUploader from './ImageUploader'

const svcStatusLabels = {
  pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
  shipping: '寄送中', pending_repair: '待维修', repairing: '维修中', adjust_pending: '加价待确认',
  done_repair: '待发回', closed: '已结算',
}
// RS-12：每项金额与待办提示
const svcAmount = (rr) => {
  const cents = rr.adjusted_quote_cents != null ? rr.adjusted_quote_cents
    : (rr.quote_repair_cents || 0) + (rr.quote_material_cents || 0) + (rr.quote_logistics_cents || 0) // #2085 含料钱
  return `¥${formatCents(cents)}`
}
const svcTodo = (rr) => ({
  pending_quote: '等待用户接受报价',
  pending_payment: '等待用户支付',
  paid: '已支付，等待寄出',
  shipping: '寄送中，等待收货',
  pending_repair: '已代收，待您开始维修',
  repairing: '维修进行中',
  adjust_pending: '加价待用户确认',
  done_repair: '待网点发回结算',
  closed: '已结算',
}[rr.status] || '')

const cardStyle = {
  display: 'flex', flexDirection: 'column', gap: 6,
  backgroundColor: '#FFFFFF', borderRadius: 12, padding: 12, marginBottom: 10,
}
const btnPrimaryStyle = {
  width: '100%', margin: 0, height: 40, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF',
  borderRadius: 8, fontSize: 14, fontWeight: 'bold',
}
const btnSecondaryStyle = {
  width: '100%', margin: 0, height: 36, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#F4F4F5', color: '#3F3F46',
  borderRadius: 8, fontSize: 13, fontWeight: 'bold',
}
const inputStyle = {
  width: '100%', height: 36, boxSizing: 'border-box', backgroundColor: '#FAFAFA',
  border: '1px solid #E4E4E7', borderRadius: 8, padding: '0 10px', fontSize: 13,
}
const labelStyle = { fontSize: 12, color: '#71717A' }

export default function TechRepairSections() {
  const [loading, setLoading] = useState(true)
  const [pendingQuotes, setPendingQuotes] = useState([])
  const [pendingPay, setPendingPay] = useState([]) // #2088：已报价·待付款（pending_payment）
  const [pendingReturn, setPendingReturn] = useState([]) // #2091：待发回（done_repair）
  const [shipped, setShipped] = useState([]) // #2116 修订：已寄出·待收货（shipping 独立分组）
  const [receiveFiles, setReceiveFiles] = useState([]) // #2116 修订：收货照片（多张累计统一提交）
  const [fetchError, setFetchError] = useState(false) // #2130：列表拉取失败（区别于"无数据"）
  const [working, setWorking] = useState([])
  const [doneList, setDoneList] = useState([])
  const [expanded, setExpanded] = useState('') // `${id}:quote|adjust`
  const [submitting, setSubmitting] = useState(false)
  // 报价表单（元）
  const [repairYuan, setRepairYuan] = useState('')
  const [materialYuan, setMaterialYuan] = useState('') // #2085 料钱
  const [logiYuan, setLogiYuan] = useState('')
  // 加价表单（元）
  const [newQuoteYuan, setNewQuoteYuan] = useState('')
  const [incurredYuan, setIncurredYuan] = useState('')
  const baseUrl = env.apiBaseUrl

  const yuanToCents = (v) => {
    const n = parseFloat(v)
    if (isNaN(n) || n < 0) return -1
    return toCents(n)
  }

  const fetchLists = async () => {
    setLoading(true)
    // #2128：列表范围由前端控制——当前默认最近 30 天（API 支持 start/end，扩展筛选只改前端）
    const startISO = encodeURIComponent(new Date(Date.now() - 30 * 24 * 3600 * 1000).toISOString())
    try {
      const [qRes, pRes, sRes, wRes, rRes, dRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-services?scope=mine&start=${startISO}&status=pending_quote`),
        apiFetch(`${baseUrl}/repair-services?scope=mine&start=${startISO}&status=pending_payment`), // #2088：已报价·待付款
        apiFetch(`${baseUrl}/repair-services?scope=mine&start=${startISO}&status=shipping`), // #2116 修订：已寄出独立分组
        apiFetch(`${baseUrl}/repair-services?scope=mine&start=${startISO}&status=pending_repair,repairing,adjust_pending`),
        apiFetch(`${baseUrl}/repair-services?scope=mine&start=${startISO}&status=done_repair`), // #2091：待发回
        apiFetch(`${baseUrl}/repair-services?scope=mine&start=${startISO}&status=closed`),
      ])
      const q = await qRes.json()
      const p = await pRes.json()
      const sv = await sRes.json()
      const w = await wRes.json()
      const r = await rRes.json()
      const d = await dRes.json() // #2134 修订3：缺失的解析——已完成组从未读过响应体（恒 0 的真根因）
      setPendingQuotes(q.code === 20000 ? (q.data?.list || []) : [])
      setPendingPay(p.code === 20000 ? (p.data?.list || []) : [])
      setShipped(sv.code === 20000 ? (sv.data?.list || []) : [])
      setWorking(w.code === 20000 ? (w.data?.list || []) : [])
      setPendingReturn(r.code === 20000 ? (r.data?.list || []) : [])
      setDoneList(d.code === 20000 ? (d.data?.list || []) : [])
      setFetchError(false) // #2130：任一成功响应即视为拉取成功
    } catch (e) {
      // #2130：拉取失败不再静默置空——置错误态，由面板内错误卡+重试呈现
      setFetchError(true)
      dialog.alert(resolveErrorMessage(e))
    }
    setLoading(false)
  }

  useEffect(() => { fetchLists() }, [])

  const toggle = (id, mode) => {
    if (expanded === `${id}:${mode}`) { setExpanded(''); return }
    setRepairYuan(''); setMaterialYuan(''); setLogiYuan(''); setNewQuoteYuan(''); setIncurredYuan(''); setReceiveFiles([])
    setExpanded(`${id}:${mode}`)
  }

  const submitQuote = async (id) => {
    const repairCents = yuanToCents(repairYuan)
    const materialCents = yuanToCents(materialYuan === '' ? '0' : materialYuan) // #2085 料钱（可空=0）
    const logiCents = yuanToCents(logiYuan === '' ? '0' : logiYuan)
    if (repairCents <= 0) { dialog.alert('请填写正确的修理费'); return }
    if (materialCents < 0) { dialog.alert('请填写正确的料钱'); return }
    if (logiCents < 0) { dialog.alert('请填写正确的物流费预估'); return }
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/quote`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ quote_repair_cents: repairCents, quote_material_cents: materialCents, quote_logistics_cents: logiCents }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('报价已提交，等待用户支付')
        setExpanded('')
        fetchLists()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  const submitAdjust = async (id) => {
    const newQuoteCents = yuanToCents(newQuoteYuan)
    const incurredCents = yuanToCents(incurredYuan)
    if (newQuoteCents <= 0) { dialog.alert('请填写正确的新修理费总价'); return }
    if (incurredCents < 0) { dialog.alert('请填写正确的到此为止修理费'); return }
    if (incurredCents > newQuoteCents) { dialog.alert('到此为止修理费不能超过新总价'); return }
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/adjust`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ new_quote_cents: newQuoteCents, incurred_cents: incurredCents }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        const d = result.data || {}
        dialog.alert(`加价申请已提交，用户确认后补差价 ¥${formatCents((d.payable_cents || 0))}`)
        setExpanded('')
        fetchLists()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  // #2116 修订：多张拍照统一提交——ImageUploader 累计后一键上传+收货（参照租赁收货环节）
  const submitReceive = async (id) => {
    if (!env.isMiniProgram) {
      dialog.alert('收货拍照请在小程序中操作')
      return
    }
    if (receiveFiles.length === 0) {
      dialog.alert('请先拍照留档')
      return
    }
    setSubmitting(true)
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
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/receive`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ photos: keys }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert(result.data?.status === 'repairing' ? '已确认收货，开始维修' : '已代收货，等待维修师开始维修')
        setExpanded('')
        setReceiveFiles([])
        fetchLists()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  // #2116：代收后师傅本人开始维修（pending_repair → repairing）
  const submitStart = async (id) => {
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/start`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('已开始维修')
        fetchLists()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  const submitComplete = async (id) => {
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/complete`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert('已完工，等待网点发回')
        setExpanded('')
        fetchLists()
      } else {
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  // #2119: 行点击 → 维修单详情（RS-API-3 员工可读；留痕/存档图/加价入口在详情页）
  const openDetail = (id) => {
    if (env.isMiniProgram) Taro.navigateTo({ url: `/pages-weapp/repair-service-detail/index?order_id=${id}` })
    else window.location.assign(`/repair-service-detail?order_id=${id}`)
  }

  // #2116 修订：瘦身行——编号/提交人/时间/状态 + 最多一个按钮；媒体与表单收进展开区
  const renderRow = (rr, opts = {}) => {
    return (
      <View key={rr.id} style={cardStyle}>
        <View
          onClick={opts.onInfoTap}
          style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}
        >
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>
            编码 {rr.repair_code || '-'}
          </Text>
          <Text style={{ fontSize: 12, color: '#A1A1AA' }}>{svcStatusLabels[rr.status] || rr.status}</Text>
        </View>
        <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
          提交人 {rr.user_name || '-'} · {rr.created_at ? formatBeijingDate(rr.created_at) : '-'}{opts.hint ? ` · ${opts.hint}` : ''}
        </Text>
        {opts.button && (
          <Button disabled={submitting} onClick={opts.button.onClick}
            style={{ ...(opts.button.secondary ? btnSecondaryStyle : btnPrimaryStyle), opacity: submitting ? 0.5 : 1 }}>
            {opts.button.label}
          </Button>
        )}
        {opts.panel || null}
      </View>
    )
  }

  const renderQuoteCard = (rr) => {
    const isOpen = expanded === `${rr.id}:quote`
    let photos = []
    try { photos = Array.isArray(rr.photos) ? rr.photos : JSON.parse(rr.photos || '[]') } catch { photos = [] }
    return renderRow(rr, {
      onInfoTap: () => openDetail(rr.id),
      button: { label: isOpen ? '收起' : '填写报价', onClick: () => toggle(rr.id, 'quote'), secondary: true },
      panel: !isOpen ? null : (
        <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4, borderTop: '1px solid #f4f4f5' }}>
          <Text style={{ fontSize: 12, color: '#52525B' }}>{rr.description || '（无描述）'}</Text>
          {photos.length > 0 && (
            <View style={{ display: 'flex', flexDirection: 'row', flexWrap: 'wrap', gap: 6 }}>
              {photos.map((p, i) => (
                <Image key={i} src={photoSrc(p)} style={{ width: 72, height: 72, borderRadius: 6, backgroundColor: '#f4f4f5' }} />
              ))}
            </View>
          )}
          {rr.video_url ? (
            <Video src={photoSrc(rr.video_url)} controls style={{ width: '100%', height: 160, borderRadius: 8, backgroundColor: '#000000' }} />
          ) : null}
          <Text style={labelStyle}>修理费（元）</Text>
          <Input style={inputStyle} type="digit" value={repairYuan}
            onInput={e => setRepairYuan(getInputValue(e))} placeholder="如 200" />
          <Text style={labelStyle}>料钱（元；材料/配件费，可空）</Text>
          <Input style={inputStyle} type="digit" value={materialYuan}
            onInput={e => setMaterialYuan(getInputValue(e))} placeholder="如 30" />
          <Text style={labelStyle}>物流费预估（元）</Text>
          <Input style={inputStyle} type="digit" value={logiYuan}
            onInput={e => setLogiYuan(getInputValue(e))} placeholder="如 50" />
          <Button disabled={submitting} onClick={() => submitQuote(rr.id)}
            style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
            {submitting ? '处理中...' : '提交报价'}
          </Button>
        </View>
      ),
    })
  }

  const renderWorkCard = (rr) => {
    if (rr.status === 'shipping') {
      const isOpen = expanded === `${rr.id}:receive`
      return renderRow(rr, {
        onInfoTap: () => openDetail(rr.id),
        hint: '等待收货',
        button: { label: isOpen ? '收起' : '收货确认（拍照留档）', onClick: () => toggle(rr.id, 'receive'), secondary: true },
        panel: !isOpen ? null : (
          <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4, borderTop: '1px solid #f4f4f5' }}>
            <Text style={labelStyle}>收货拍照（可拍多张，至少 1 张后统一提交）</Text>
            <ImageUploader maxImages={9} onChange={(files) => setReceiveFiles(files)} />
            <Button disabled={submitting || receiveFiles.length === 0} onClick={() => submitReceive(rr.id)}
              style={{ ...btnPrimaryStyle, opacity: (submitting || receiveFiles.length === 0) ? 0.5 : 1 }}>
              {submitting ? '处理中...' : `提交收货（${receiveFiles.length} 张）`}
            </Button>
          </View>
        ),
      })
    }
    if (rr.status === 'pending_repair') {
      return renderRow(rr, {
        onInfoTap: () => openDetail(rr.id),
        hint: '已代收，待开始维修',
        button: { label: '开始维修', onClick: () => submitStart(rr.id) },
      })
    }
    const isOpen = expanded === `${rr.id}:adjust`
    return renderRow(rr, {
      hint: rr.status === 'adjust_pending' ? '加价待用户确认，暂不能完工' : undefined,
      onInfoTap: () => openDetail(rr.id),
      button: rr.status === 'repairing' ? { label: '完成修理', onClick: () => submitComplete(rr.id) } : null,
      panel: isOpen && rr.status === 'repairing' ? (
        <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4, borderTop: '1px solid #f4f4f5' }}>
          <Text style={labelStyle}>发起加价（收货后维修中方可发起）</Text>
          <Text style={labelStyle}>新修理费总价（元）</Text>
          <Input style={inputStyle} type="digit" value={newQuoteYuan}
            onInput={e => setNewQuoteYuan(getInputValue(e))} placeholder="如 300" />
          <Text style={labelStyle}>到此为止修理费（元，用户不继续时按此结算）</Text>
          <Input style={inputStyle} type="digit" value={incurredYuan}
            onInput={e => setIncurredYuan(getInputValue(e))} placeholder="如 50" />
          <Button disabled={submitting} onClick={() => submitAdjust(rr.id)}
            style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
            {submitting ? '处理中...' : '提交加价申请'}
          </Button>
        </View>
      ) : null,
    })
  }

  // #2130：拉取失败 → 错误卡 + 重试（替代误导性的全 0 分组）
  if (fetchError) {
    return (
      <View>
        <Text style={{ fontSize: 11, color: '#A1A1AA', marginBottom: 6, display: 'block' }}>列表范围：最近 30 天</Text>
        <View style={cardStyle}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#B91C1C' }}>列表拉取失败</Text>
          <Text style={labelStyle}>网络异常或登录态失效，请检查网络后重试。</Text>
          <Button disabled={loading} onClick={fetchLists}
            style={{ ...btnPrimaryStyle, opacity: loading ? 0.5 : 1 }}>
            {loading ? '重试中...' : '重试'}
          </Button>
        </View>
      </View>
    )
  }

  return (
    <View>
      {/* #2128：列表范围声明（各分组统一「最近 30 天」，避免"总数 vs 可见数"歧义） */}
      <Text style={{ fontSize: 11, color: '#A1A1AA', marginBottom: 6, display: 'block' }}>列表范围：最近 30 天</Text>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>待报价（{pendingQuotes.length}）</Text>
            <Text onClick={fetchLists} style={{ fontSize: 12, color: '#71717A' }}>刷新</Text>
          </View>
          {loading ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
          ) : pendingQuotes.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无待报价维修单</Text>
          ) : pendingQuotes.map(renderQuoteCard)}

          {/* #2088：已报价·待付款（报价提交后单据保留可见，等待用户支付） */}
          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>已报价·待付款（{pendingPay.length}）</Text>
          </View>
          {!loading && pendingPay.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无待付款维修单</Text>
          ) : pendingPay.map(rr => renderRow(rr, { onInfoTap: () => openDetail(rr.id), hint: svcTodo(rr) }))}

          {/* #2116 修订：已寄出·待收货（shipping 独立分组） */}
          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>已寄出·待收货（{shipped.length}）</Text>
          </View>
          {!loading && shipped.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无待收货维修单</Text>
          ) : shipped.map(renderWorkCard)}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>维修中（{working.length}）</Text>
          </View>
          {!loading && working.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无维修中维修单</Text>
          ) : working.map(renderWorkCard)}

          {/* #2091：待发回（师傅完修 → 网点发回结算前；只读，发回与结算为网点职责） */}
          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>待发回（{pendingReturn.length}）</Text>
          </View>
          {!loading && pendingReturn.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无待发回维修单</Text>
          ) : pendingReturn.map(rr => renderRow(rr, { onInfoTap: () => openDetail(rr.id), hint: svcTodo(rr) }))}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>已完成（{doneList.length}）</Text>
          </View>
          {!loading && doneList.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无已完成维修单</Text>
          ) : doneList.map(rr => renderRow(rr, { onInfoTap: () => openDetail(rr.id) }))}
    </View>
  )
}
