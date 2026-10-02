// #2084：维修师工作台区块（待报价/维修中/已完成 + 报价/加价/完成）共享组件。
// 用于：① MyRepairs「维修服务」Tab 内联（省一跳）② 独立页 /tech-repair-workbench（RS-02 深链）。
// 自包含（挂载即拉 scope=mine 三查询）；仅用 @tarojs/components + platform + services/api（跨端）。
import { useState, useEffect } from 'react'
import { formatCents, yuanToCents as toCents } from '../utils/money'
import { View, Text, Input, Video, Button } from '@tarojs/components'
import { apiFetch, resolveErrorMessage, getToken } from '../services/api'
import Taro from '@tarojs/taro'
import { dialog, env, getInputValue, uploadFile as uploadFileApi } from '../platform'
import { formatBeijingDate } from '../utils/format'
import { photoSrc } from '../utils/media'

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
    try {
      const [qRes, pRes, wRes, rRes, dRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=pending_quote`),
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=pending_payment`), // #2088：已报价·待付款
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=paid,shipping,repairing,adjust_pending`),
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=done_repair`), // #2091：待发回
        apiFetch(`${baseUrl}/repair-services?scope=mine&status=closed`),
      ])
      const q = await qRes.json()
      const p = await pRes.json()
      const w = await wRes.json()
      const r = await rRes.json()
      setPendingQuotes(q.code === 20000 ? (q.data?.list || []) : [])
      setPendingPay(p.code === 20000 ? (p.data?.list || []) : [])
      setWorking(w.code === 20000 ? (w.data?.list || []) : [])
      setPendingReturn(r.code === 20000 ? (r.data?.list || []) : [])
      setDoneList(dRes.code === 20000 ? (dRes.data?.list || []) : [])
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setLoading(false)
  }

  useEffect(() => { fetchLists() }, [])

  const toggle = (id, mode) => {
    if (expanded === `${id}:${mode}`) { setExpanded(''); return }
    setRepairYuan(''); setMaterialYuan(''); setLogiYuan(''); setNewQuoteYuan(''); setIncurredYuan('')
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

  // #2116：收货确认（拍照留档）——师傅本人→repairing；员工代收→pending_repair（服务端按身份判定）
  const submitReceive = async (id) => {
    if (!env.isMiniProgram) {
      dialog.alert('收货拍照请在小程序中操作')
      return
    }
    setSubmitting(true)
    try {
      const choose = await Taro.chooseImage({ count: 9, sizeType: ['compressed'], sourceType: ['camera', 'album'] })
      const paths = choose.tempFilePaths || []
      if (!paths.length) { setSubmitting(false); return }
      const authHeaders = { Authorization: 'Bearer ' + (getToken() || '') }
      const keys = []
      for (const p of paths) {
        const resp = await uploadFileApi(`${baseUrl}/upload`, p, { headers: authHeaders })
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
            <Text style={labelStyle}>料钱（元；材料/配件费，可空）</Text>
            <Input style={inputStyle} type="digit" value={materialYuan}
              onInput={e => setMaterialYuan(getInputValue(e))} placeholder="如 30" />
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
    const canAdjust = rr.status === 'shipping' || rr.status === 'pending_repair' || rr.status === 'repairing'
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
        {/* #2116：收货环节——shipping 拍照收货 / pending_repair 开始维修 / repairing 才可完工 */}
        {rr.status === 'shipping' && (
          <Button disabled={submitting} onClick={() => submitReceive(rr.id)}
            style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
            确认收货（拍照留档）
          </Button>
        )}
        {rr.status === 'pending_repair' && (
          <Button disabled={submitting} onClick={() => submitStart(rr.id)}
            style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
            开始维修
          </Button>
        )}
        {rr.status === 'repairing' && (
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
    <View>
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
          ) : pendingPay.map(rr => (
            <View key={rr.id} style={cardStyle}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>编码 {rr.repair_code || '-'}</Text>
                <Text style={{ fontSize: 12, color: '#A1A1AA' }}>{svcStatusLabels[rr.status] || rr.status}</Text>
              </View>
              <Text style={{ fontSize: 12, color: '#52525B' }} numberOfLines={2}>{rr.description || '（无描述）'}</Text>
              {rr.video_url ? (
                <Video src={photoSrc(rr.video_url)} controls style={{ width: '100%', height: 160, borderRadius: 8, backgroundColor: '#000000' }} />
              ) : null}
              <Text style={{ fontSize: 11, color: '#A1A1AA' }}>金额 {svcAmount(rr)} · {svcTodo(rr)}</Text>
            </View>
          ))}

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
          ) : pendingReturn.map(rr => (
            <View key={rr.id} style={cardStyle}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>编码 {rr.repair_code || '-'}</Text>
                <Text style={{ fontSize: 12, color: '#A1A1AA' }}>{svcStatusLabels[rr.status] || rr.status}</Text>
              </View>
              <Text style={{ fontSize: 12, color: '#52525B' }} numberOfLines={2}>{rr.description || '（无描述）'}</Text>
              {rr.video_url ? (
                <Video src={photoSrc(rr.video_url)} controls style={{ width: '100%', height: 160, borderRadius: 8, backgroundColor: '#000000' }} />
              ) : null}
              <Text style={{ fontSize: 11, color: '#A1A1AA' }}>金额 {svcAmount(rr)} · {svcTodo(rr)}</Text>
            </View>
          ))}

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
  )
}
