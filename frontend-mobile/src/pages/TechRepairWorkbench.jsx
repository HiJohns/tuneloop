import { useState, useEffect } from 'react'
import { formatCents, yuanToCents as toCents } from '../utils/money'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Input, Video, Button, ScrollView } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute } from '../platform'
import { formatBeijingDate } from '../utils/format'
import { photoSrc } from '../utils/media'

// #1956 阶段3b：师傅工作台（RS-API-2 scope=mine）
//   待报价（pending_quote）→ 报价（修理费 + 物流费预估）
//   维修中（paid/shipping/repairing）→ 加价发起（RS-06 双字段）/ 完成修理

const svcStatusLabels = {
  pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
  shipping: '寄送中', repairing: '维修中', adjust_pending: '加价待确认',
  done_repair: '待发回', closed: '已结算',
}
// RS-12：每项金额与待办提示
const svcAmount = (rr) => {
  const cents = rr.adjusted_quote_cents != null ? rr.adjusted_quote_cents
    : (rr.quote_repair_cents || 0) + (rr.quote_logistics_cents || 0)
  return `¥${formatCents(cents)}`
}
const svcTodo = (rr) => ({
  pending_quote: '等待用户接受报价',
  pending_payment: '等待用户支付',
  paid: '已支付，等待寄出',
  shipping: '寄送中，等待收货',
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

export default function TechRepairWorkbench() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [loading, setLoading] = useState(true)
  const [pendingQuotes, setPendingQuotes] = useState([])
  const [working, setWorking] = useState([])
  const [doneList, setDoneList] = useState([])
  const [expanded, setExpanded] = useState('') // `${id}:quote|adjust`
  const [submitting, setSubmitting] = useState(false)
  // 报价表单（元）
  const [repairYuan, setRepairYuan] = useState('')
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
    try {
      const [qRes, wRes, dRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=pending_quote`),
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=paid,shipping,repairing,adjust_pending`),
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=closed`),
      ])
      const q = await qRes.json()
      const w = await wRes.json()
      setPendingQuotes(q.code === 20000 ? (q.data?.list || []) : [])
      setWorking(w.code === 20000 ? (w.data?.list || []) : [])
      setDoneList(dRes.code === 20000 ? (dRes.data?.list || []) : [])
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setLoading(false)
  }

  useEffect(() => { fetchLists() }, [])

  const toggle = (id, mode) => {
    if (expanded === `${id}:${mode}`) { setExpanded(''); return }
    setRepairYuan(''); setLogiYuan(''); setNewQuoteYuan(''); setIncurredYuan('')
    setExpanded(`${id}:${mode}`)
  }

  const submitQuote = async (id) => {
    const repairCents = yuanToCents(repairYuan)
    const logiCents = yuanToCents(logiYuan === '' ? '0' : logiYuan)
    if (repairCents <= 0) { dialog.alert('请填写正确的修理费'); return }
    if (logiCents < 0) { dialog.alert('请填写正确的物流费预估'); return }
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/quote`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ quote_repair_cents: repairCents, quote_logistics_cents: logiCents }),
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

  const renderQuoteCard = (rr) => {
    const isOpen = expanded === `${rr.id}:quote`
    return (
      <View key={rr.id} style={cardStyle}>
        <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>
            编码 {rr.repair_code || '-'}
          </Text>
          <Text style={{ fontSize: 12, color: '#A1A1AA' }}>待报价</Text>
        </View>
        <Text style={{ fontSize: 12, color: '#52525B' }} numberOfLines={2}>
          {rr.description || '（无描述）'}
        </Text>
        {/* #2060: 试奏视频（有则播放） */}
        {rr.video_url ? (
          <Video src={photoSrc(rr.video_url)} controls style={{ width: '100%', height: 160, borderRadius: 8, backgroundColor: '#000000' }} />
        ) : null}
        <Text style={{ fontSize: 11, color: '#71717A' }}>金额 {svcAmount(rr)} · {svcTodo(rr)}</Text>
        <Button onClick={() => toggle(rr.id, 'quote')} style={btnSecondaryStyle}>
          {isOpen ? '收起' : '填写报价'}
        </Button>
        {isOpen && (
          <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
            <Text style={labelStyle}>修理费（元）</Text>
            <Input style={inputStyle} type="digit" value={repairYuan}
              onInput={e => setRepairYuan(getInputValue(e))} placeholder="如 200" />
            <Text style={labelStyle}>物流费预估（元；受控组合为 3 段受管物流的预估合计）</Text>
            <Input style={inputStyle} type="digit" value={logiYuan}
              onInput={e => setLogiYuan(getInputValue(e))} placeholder="如 50" />
            <Button disabled={submitting} onClick={() => submitQuote(rr.id)}
              style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
              {submitting ? '处理中...' : '提交报价'}
            </Button>
          </View>
        )}
      </View>
    )
  }

  const renderWorkCard = (rr) => {
    const isOpen = expanded === `${rr.id}:adjust`
    const canAdjust = rr.status === 'shipping' || rr.status === 'repairing'
    return (
      <View key={rr.id} style={cardStyle}>
        <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>
            编码 {rr.repair_code || '-'}
          </Text>
          <Text style={{ fontSize: 12, color: '#A1A1AA' }}>
            {svcStatusLabels[rr.status] || rr.status}
          </Text>
        </View>
        <Text style={{ fontSize: 12, color: '#52525B' }} numberOfLines={2}>
          {rr.description || '（无描述）'}
        </Text>
        {/* #2060: 试奏视频（有则播放） */}
        {rr.video_url ? (
          <Video src={photoSrc(rr.video_url)} controls style={{ width: '100%', height: 160, borderRadius: 8, backgroundColor: '#000000' }} />
        ) : null}
        <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
          金额 {svcAmount(rr)} · {svcTodo(rr)} · 更新于 {rr.updated_at ? formatBeijingDate(rr.updated_at) : '-'}
        </Text>
        {canAdjust && (
          <Button onClick={() => toggle(rr.id, 'adjust')} style={btnSecondaryStyle}>
            {isOpen ? '收起' : '发起加价'}
          </Button>
        )}
        {isOpen && (
          <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
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
        )}
        {rr.status !== 'adjust_pending' && (
          <Button disabled={submitting} onClick={() => submitComplete(rr.id)}
            style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
            完成修理
          </Button>
        )}
        {rr.status === 'adjust_pending' && (
          <Text style={{ fontSize: 12, color: '#D97706' }}>加价待用户确认，暂不能完工</Text>
        )}
      </View>
    )
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务 · 维修师工作台</Text>
      </View>

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>待报价（{pendingQuotes.length}）</Text>
            <Text onClick={fetchLists} style={{ fontSize: 12, color: '#71717A' }}>刷新</Text>
          </View>
          {loading ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
          ) : pendingQuotes.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无待报价维修单</Text>
          ) : pendingQuotes.map(renderQuoteCard)}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>维修中（{working.length}）</Text>
          </View>
          {!loading && working.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无维修中维修单</Text>
          ) : working.map(renderWorkCard)}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>已完成（{doneList.length}）</Text>
          </View>
          {!loading && doneList.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无已完成维修单</Text>
          ) : doneList.map(rr => (
            <View key={rr.id} style={cardStyle}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>
                  编码 {rr.repair_code || '-'}
                </Text>
                <Text style={{ fontSize: 12, color: '#A1A1AA' }}>已结算</Text>
              </View>
              <Text style={{ fontSize: 12, color: '#52525B' }} numberOfLines={2}>
                {rr.description || '（无描述）'}
              </Text>
              {/* #2060: 试奏视频（有则播放） */}
              {rr.video_url ? (
                <Video src={photoSrc(rr.video_url)} controls style={{ width: '100%', height: 160, borderRadius: 8, backgroundColor: '#000000' }} />
              ) : null}
              <Text style={{ fontSize: 11, color: '#71717A' }}>金额 {svcAmount(rr)}</Text>
            </View>
          ))}
        </View>
      </ScrollView>
    </View>
  )
}
