// InvoiceConfirm — 发票「确认申请」独立页（#2218 从 Invoice.jsx 拆出，多步流程=独立页面）
import { useState, useEffect, useMemo } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, Input } from '@tarojs/components'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { env, dialog, getInputValue, toWeappRoute } from '../platform'
import { formatCents } from '../utils/money'

export default function InvoiceConfirm() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const orderIds = useMemo(() => {
    const q = searchParams.get('order_ids') || ''
    let r = ''
    try { r = Taro.getCurrentInstance()?.router?.params?.order_ids || '' } catch {}
    return (q || r).split(',').map(s => s.trim()).filter(Boolean)
  }, [searchParams])
  // #2225: 用稳定的字符串键做 effect 依赖，避免 searchParams 对象身份抖动导致无限 load()
  const orderIdsKey = orderIds.join(',')
  const [eligible, setEligible] = useState([])
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [formByTenant, setFormByTenant] = useState({})
  const goBack = () => { if (env.isMiniProgram) Taro.navigateBack(); else navigate(-1) }
  const nav = (to, opts) => {
    if (!env.isMiniProgram) return navigate(to, opts)
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    // #2225: weapp 分支需支持 replace——否则提交成功后的跳转会压栈，破坏「独立页面」语义
    if (opts?.replace) return Taro.redirectTo({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }

  useEffect(() => {
    const load = async () => {
      setLoading(true)
      try {
        const resp = await apiFetch(`${env.apiBaseUrl}/user/invoices/eligible`)
        const data = await resp.json()
        if (data.code === 20000) {
          const ids = new Set(orderIds)
          setEligible((data.data || []).filter(o => ids.has(o.order_id)))
        }
      } catch (e) { console.error('Failed to load eligible invoices:', e) }
      setLoading(false)
    }
    load()
  }, [orderIdsKey])

  const merchantGroups = useMemo(() => {
    const map = {}
    eligible.forEach(o => {
      if (!map[o.tenant_id]) map[o.tenant_id] = { tenant_id: o.tenant_id, merchant_name: o.merchant_name, orders: [], total: 0 }
      map[o.tenant_id].orders.push(o)
      map[o.tenant_id].total += o.total_cents
    })
    return Object.values(map)
  }, [eligible])

  const selectedTotal = useMemo(() => eligible.reduce((s, o) => s + o.total_cents, 0), [eligible])

  const emptyForm = { invoice_type: '普通', title: '', tax_number: '' }
  const getForm = (tid) => formByTenant[tid] || emptyForm
  const setFormField = (tid, key, value) => {
    setFormByTenant(prev => {
      const cur = prev[tid] || emptyForm
      return { ...prev, [tid]: { ...cur, [key]: value } }
    })
  }
  const TAX_RE = /^(?:[0-9A-Za-z]{15}|[0-9A-Za-z]{18}|[0-9A-Za-z]{20})$/
  const validateAndBuildGroups = () => {
    const groups = []
    for (const g of merchantGroups) {
      const f = getForm(g.tenant_id)
      const name = g.merchant_name || '商户'
      const title = (f.title || '').trim()
      const tax = (f.tax_number || '').replace(/\s/g, '')
      if (!title) return { error: `${name}：请填写发票抬头` }
      if (title.length > 255) return { error: `${name}：发票抬头过长` }
      if (f.invoice_type === '专用' && !tax) return { error: `${name}：专用发票需填写税号` }
      if (tax && !TAX_RE.test(tax)) return { error: `${name}：税号格式不正确` }
      groups.push({ tenant_id: g.tenant_id, order_ids: g.orders.map(o => o.order_id), invoice_type: f.invoice_type || '普通', title, tax_number: tax })
    }
    return { groups }
  }

  const handleSubmit = async () => {
    if (merchantGroups.length === 0) { dialog.toast('请先选择订单'); return }
    const built = validateAndBuildGroups()
    if (built.error) { dialog.toast(built.error); return }
    setSubmitting(true)
    try {
      const resp = await apiFetch(`${env.apiBaseUrl}/user/invoices`, { method: 'POST', body: JSON.stringify({ groups: built.groups }) })
      const data = await resp.json()
      if (data.code === 20000) {
        dialog.toast('申请已提交', { icon: 'success' })
        nav('/invoice?tab=applied', { replace: true })
      } else { dialog.toast(data.message || '提交失败') }
    } catch (e) { dialog.toast('提交失败') }
    setSubmitting(false)
  }

  return (
    <View style={{ minHeight: '100vh', background: '#FDFBF7' }}>
      <View style={{ padding: 16 }}>
        {/* 页首仅 H5（weapp 用原生导航栏，#2218） */}
        {!env.isMiniProgram && (
          <View style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
            <Button onClick={goBack} style={{ background: 'none', border: 'none', padding: 0, fontSize: 16, color: '#000' }}>‹ 返回</Button>
            <Text style={{ fontSize: 18, fontWeight: '700', marginLeft: 8 }}>确认申请</Text>
          </View>
        )}

        {loading ? (
          <View style={{ padding: 40, textAlign: 'center' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
        ) : merchantGroups.length === 0 ? (
          <View style={{ padding: 40, textAlign: 'center' }}><Text style={{ color: '#a1a1aa' }}>请从发票列表选择订单</Text></View>
        ) : (
          <>
            {merchantGroups.map(g => {
              const f = getForm(g.tenant_id)
              return (
              <View key={g.tenant_id} style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 12 }}>
                {g.merchant_name ? (<View style={{ marginBottom: 8 }}><Text style={{ fontSize: 16, fontWeight: '700' }}>{g.merchant_name}</Text></View>) : null}
                <View style={{ marginBottom: 4 }}><Text style={{ fontSize: 13, color: '#666' }}>{g.orders.length} 笔订单</Text></View>
                {g.orders.map(o => (
                  <View key={o.order_id} style={{ display: 'flex', justifyContent: 'space-between', padding: '6px 0', borderTop: '1px solid #f4f4f5' }}>
                    <Text style={{ fontSize: 13, color: '#555' }}>SN: {o.sn || '-'}</Text>
                    <Text style={{ fontSize: 13, color: '#555' }}>¥{formatCents(o.total_cents)}</Text>
                  </View>
                ))}
                <View style={{ display: 'flex', justifyContent: 'space-between', marginTop: 8, paddingTop: 8, borderTop: '1px solid #e4e4e7' }}>
                  <Text style={{ fontSize: 14, fontWeight: '700' }}>小计</Text>
                  <Text style={{ fontSize: 14, fontWeight: '700' }}>¥{formatCents(g.total)}</Text>
                </View>
                <View style={{ marginTop: 12, paddingTop: 12, borderTop: '1px solid #e4e4e7' }}>
                  <View style={{ marginBottom: 6 }}><Text style={{ fontSize: 13, color: '#71717a' }}>发票类型</Text></View>
                  <View style={{ display: 'flex', marginBottom: 12 }}>
                    {['普通', '专用'].map(tt => (
                      <View key={tt} onClick={() => setFormField(g.tenant_id, 'invoice_type', tt)} style={{ padding: '6px 16px', marginRight: 8, borderRadius: 16, border: f.invoice_type === tt ? '1px solid #D97706' : '1px solid #d4d4d8', background: f.invoice_type === tt ? '#FEF3C7' : '#fff' }}>
                        <Text style={{ fontSize: 13, color: f.invoice_type === tt ? '#D97706' : '#52525b' }}>{tt}发票</Text>
                      </View>
                    ))}
                  </View>
                  <View style={{ marginBottom: 6 }}><Text style={{ fontSize: 13, color: '#71717a' }}>发票抬头</Text><Text style={{ fontSize: 13, color: '#dc2626' }}> *</Text></View>
                  <Input value={f.title} onInput={e => setFormField(g.tenant_id, 'title', getInputValue(e))} placeholder="请输入发票抬头（个人或单位名称）" style={{ width: '100%', boxSizing: 'border-box', height: 40, padding: '0 12px', background: '#f9fafb', borderRadius: 8, fontSize: 14, marginBottom: 12 }} />
                  <View style={{ marginBottom: 6 }}><Text style={{ fontSize: 13, color: '#71717a' }}>税号</Text><Text style={{ fontSize: 13, color: f.invoice_type === '专用' ? '#dc2626' : '#a1a1aa' }}>{f.invoice_type === '专用' ? ' *' : '（选填）'}</Text></View>
                  <Input value={f.tax_number} onInput={e => setFormField(g.tenant_id, 'tax_number', getInputValue(e))} placeholder={f.invoice_type === '专用' ? '专用发票必填' : '个人抬头可不填'} style={{ width: '100%', boxSizing: 'border-box', height: 40, padding: '0 12px', background: '#f9fafb', borderRadius: 8, fontSize: 14 }} />
                </View>
              </View>
              )
            })}

            <View style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 16 }}>
              <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                <Text style={{ fontSize: 16, fontWeight: '700' }}>总开票金额</Text>
                <Text style={{ fontSize: 16, fontWeight: '700', color: '#D97706' }}>¥{formatCents(selectedTotal)}</Text>
              </View>
            </View>

            <Button onClick={handleSubmit} disabled={submitting} style={{ width: '100%', height: 48, background: '#D97706', color: '#fff', borderRadius: 12, fontSize: 16, fontWeight: '700', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: 0, padding: 0 }}>
              {submitting ? '处理中...' : '确认提交'}
            </Button>
          </>
        )}
      </View>
    </View>
  )
}
