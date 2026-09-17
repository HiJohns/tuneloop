import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Input, Button, ScrollView } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute } from '../platform'
import { formatBeijingDate } from '../utils/format'

// #1957 阶段3c：网点员工维修服务工作台（RS-API-2 scope=site）
//   待发回（done_repair）→ 发回 + 结算（dispatch）
//   进行中（paid/shipping/repairing）→ 分段物流费实填（legs）

const svcStatusLabels = {
  pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
  shipping: '寄送中', repairing: '维修中', adjust_pending: '加价待确认',
  done_repair: '待发回', closed: '已结算',
}

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

export default function StaffRepairServices() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [loading, setLoading] = useState(true)
  const [pendingDispatch, setPendingDispatch] = useState([])
  const [inProgress, setInProgress] = useState([])
  const [expanded, setExpanded] = useState('') // `${id}:dispatch` | `${id}:leg`
  const [submitting, setSubmitting] = useState(false)
  // 表单态
  const [trackingCompany, setTrackingCompany] = useState('')
  const [trackingNumber, setTrackingNumber] = useState('')
  const [feeYuan, setFeeYuan] = useState('')
  const [nextLeg, setNextLeg] = useState(1)
  const baseUrl = env.apiBaseUrl

  const yuanToCents = (v) => {
    const n = parseFloat(v)
    if (isNaN(n) || n < 0) return -1
    return Math.round(n * 100)
  }

  const fetchLists = async () => {
    setLoading(true)
    try {
      const [doneRes, activeRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair-services?scope=site&status=done_repair`),
        apiFetch(`${baseUrl}/repair-services?scope=site&status=paid,shipping,repairing`),
      ])
      const done = await doneRes.json()
      const active = await activeRes.json()
      setPendingDispatch(done.code === 20000 ? (done.data?.list || []) : [])
      setInProgress(active.code === 20000 ? (active.data?.list || []) : [])
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setLoading(false)
  }

  useEffect(() => { fetchLists() }, [])

  const toggle = async (id, mode) => {
    if (expanded === `${id}:${mode}`) { setExpanded(''); return }
    setTrackingCompany(''); setTrackingNumber(''); setFeeYuan(''); setNextLeg(1)
    setExpanded(`${id}:${mode}`)
    if (mode === 'leg') {
      // 取已有分段数 → 下一leg序号（RS-API-3 staff 可读）
      try {
        const res = await apiFetch(`${baseUrl}/user/repair-services/${id}`)
        const result = await res.json()
        if (result.code === 20000) {
          setNextLeg((result.data?.logistics_fees?.length || 0) + 1)
        }
      } catch {}
    }
  }

  const submitDispatch = async (id) => {
    const feeCents = yuanToCents(feeYuan)
    if (!trackingNumber.trim()) { dialog.alert('请填写发回物流单号'); return }
    if (feeCents < 0) { dialog.alert('请填写正确的本段物流费'); return }
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/dispatch`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          tracking_company: trackingCompany.trim(),
          tracking_number: trackingNumber.trim(),
          logistics_fee_cents: feeCents,
        }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        const d = result.data || {}
        const parts = []
        if (d.refund_cents != null) parts.push(`退款 ¥${(d.refund_cents / 100).toFixed(2)}`)
        if (d.shortfall_cents != null) parts.push(`待补缴 ¥${(d.shortfall_cents / 100).toFixed(2)}`)
        dialog.alert(parts.length ? `发回并结算完成（${parts.join('，')}）` : '发回并结算完成')
        setExpanded('')
        fetchLists()
      } else {
        // 退款失败等：502 透传错误，可重试
        dialog.alert(resolveErrorMessage(result))
      }
    } catch (e) {
      dialog.alert(resolveErrorMessage(e))
    }
    setSubmitting(false)
  }

  const submitLeg = async (id) => {
    const feeCents = yuanToCents(feeYuan)
    if (feeCents < 0) { dialog.alert('请填写正确的本段物流费'); return }
    setSubmitting(true)
    try {
      const res = await apiFetch(`${baseUrl}/repair-services/${id}/legs`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ leg: nextLeg, logistics_fee_cents: feeCents }),
      })
      const result = await res.json()
      if (result.code === 20000) {
        dialog.alert(`第 ${nextLeg} 段物流费已登记`)
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

  const renderCard = (rr, mode) => {
    const isOpen = expanded === `${rr.id}:${mode}`
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
        <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
          更新于 {rr.updated_at ? formatBeijingDate(rr.updated_at) : '-'}
        </Text>
        {mode === 'dispatch' ? (
          <Button onClick={() => toggle(rr.id, 'dispatch')} style={btnSecondaryStyle}>
            {isOpen ? '收起' : '发回并结算'}
          </Button>
        ) : (
          <Button onClick={() => toggle(rr.id, 'leg')} style={btnSecondaryStyle}>
            {isOpen ? '收起' : '实填本段物流费'}
          </Button>
        )}
        {isOpen && (
          <View style={{ display: 'flex', flexDirection: 'column', gap: 8, paddingTop: 4 }}>
            {mode === 'dispatch' ? (
              <>
                <Text style={labelStyle}>物流公司</Text>
                <Input style={inputStyle} value={trackingCompany}
                  onInput={e => setTrackingCompany(getInputValue(e))} placeholder="如 顺丰速运" />
                <Text style={labelStyle}>物流单号（必填）</Text>
                <Input style={inputStyle} value={trackingNumber}
                  onInput={e => setTrackingNumber(getInputValue(e))} placeholder="运单号" />
              </>
            ) : (
              <Text style={labelStyle}>本段为第 {nextLeg} 段</Text>
            )}
            <Text style={labelStyle}>本段实际物流费（元）</Text>
            <Input style={inputStyle} type="digit" value={feeYuan}
              onInput={e => setFeeYuan(getInputValue(e))} placeholder="如 12.50" />
            <Button disabled={submitting}
              onClick={() => (mode === 'dispatch' ? submitDispatch(rr.id) : submitLeg(rr.id))}
              style={{ ...btnPrimaryStyle, opacity: submitting ? 0.5 : 1 }}>
              {submitting ? '处理中...' : mode === 'dispatch' ? '确认发回并结算' : '登记本段物流费'}
            </Button>
          </View>
        )}
      </View>
    )
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
        <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
        <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>维修服务 · 网点工作台</Text>
      </View>

      <ScrollView scrollY className="flex-1 min-h-0 overflow-y-auto"
        style={{ flex: 1, minHeight: 0 }}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>待发回（{pendingDispatch.length}）</Text>
            <Text onClick={fetchLists} style={{ fontSize: 12, color: '#71717A' }}>刷新</Text>
          </View>
          {loading ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
          ) : pendingDispatch.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无待发回维修单</Text>
          ) : pendingDispatch.map(rr => renderCard(rr, 'dispatch'))}

          <View style={{ marginTop: 14, marginBottom: 8 }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>进行中（{inProgress.length}）</Text>
          </View>
          {!loading && inProgress.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无进行中维修单</Text>
          ) : inProgress.map(rr => renderCard(rr, 'leg'))}
        </View>
      </ScrollView>
    </View>
  )
}
