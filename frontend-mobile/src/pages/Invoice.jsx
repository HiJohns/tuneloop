import { useState, useEffect, useMemo } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, ScrollView } from '@tarojs/components'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '../services/api'
import { env, dialog, toWeappRoute } from '../platform'
import { formatBeijingDate } from '../utils/format'
import { formatCents } from '../utils/money'

// Invoice — 发票列表页（#2218：「确认申请」已拆为独立页 InvoiceConfirm）
export default function Invoice() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const [tab, setTab] = useState(searchParams.get('tab') || 'eligible')
  const [eligible, setEligible] = useState([])
  const [applied, setApplied] = useState([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState({})

  const nav = (to, opts) => {
    if (!env.isMiniProgram) return navigate(to, opts)
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }

  const loadData = async () => {
    setLoading(true)
    try {
      if (tab === 'eligible') {
        const resp = await apiFetch(`${env.apiBaseUrl}/user/invoices/eligible`)
        const data = await resp.json()
        if (data.code === 20000) setEligible(data.data || [])
      } else {
        const resp = await apiFetch(`${env.apiBaseUrl}/user/invoices`)
        const data = await resp.json()
        if (data.code === 20000) setApplied(data.data || [])
      }
    } catch (e) { console.error('Failed to load invoices:', e) }
    setLoading(false)
  }

  useEffect(() => { loadData() }, [tab])

  const toggleSelect = (orderId) => setSelected(prev => ({ ...prev, [orderId]: !prev[orderId] }))
  const selectedOrders = useMemo(() => eligible.filter(o => selected[o.order_id]), [eligible, selected])
  const selectedTotal = useMemo(() => selectedOrders.reduce((sum, o) => sum + o.total_cents, 0), [selectedOrders])

  const handleViewInvoice = async (fileUrl) => {
    if (!fileUrl) return
    if (env.isMiniProgram) {
      try {
        const res = await Taro.downloadFile({ url: fileUrl })
        Taro.openDocument({ filePath: res.tempFilePath, showMenu: true })
      } catch (e) { dialog.toast('打开失败') }
    } else { window.open(fileUrl) }
  }

  return (
    <View style={{ minHeight: '100vh', background: '#FDFBF7' }}>
      {/* Tabs */}
      <View style={{ display: 'flex', background: '#fff', borderBottom: '1px solid #f4f4f5' }}>
        {[{ key: 'eligible', label: '未申请' }, { key: 'applied', label: '已申请' }].map(t => (
          <View key={t.key} onClick={() => { setTab(t.key); setSelected({}) }}
            style={{ flex: 1, textAlign: 'center', padding: '12px 0', fontSize: 15, fontWeight: tab === t.key ? '700' : '400', color: tab === t.key ? '#D97706' : '#71717a', borderBottom: tab === t.key ? '2px solid #D97706' : '2px solid transparent' }}>
            {t.label}
          </View>
        ))}
      </View>

      {loading ? (
        <View style={{ padding: 40, textAlign: 'center' }}><Text style={{ color: '#a1a1aa' }}>加载中...</Text></View>
      ) : tab === 'eligible' ? (
        <>
          {eligible.length === 0 ? (
            <View style={{ padding: 40, textAlign: 'center' }}><Text style={{ color: '#a1a1aa' }}>暂无可申请发票的订单</Text></View>
          ) : (
            <ScrollView scrollY style={{ height: 'calc(100vh - 120px)' }}>
              <View style={{ padding: 16 }}>
                {eligible.map(o => (
                  <View key={o.order_id} onClick={() => toggleSelect(o.order_id)} style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 8, display: 'flex', alignItems: 'center' }}>
                    <View style={{ width: 22, height: 22, borderRadius: 4, border: selected[o.order_id] ? 'none' : '1px solid #d4d4d8', background: selected[o.order_id] ? '#D97706' : '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 12, flexShrink: 0 }}>
                      {selected[o.order_id] && <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>✓</Text>}
                    </View>
                    <View style={{ flex: 1 }}>
                      <View style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                        <Text style={{ fontSize: 15, fontWeight: '600' }}>{o.merchant_name || '商户'}</Text>
                        <Text style={{ fontSize: 15, fontWeight: '700', color: '#D97706' }}>¥{formatCents(o.total_cents)}</Text>
                      </View>
                      <Text style={{ fontSize: 12, color: '#a1a1aa' }}>SN: {o.sn || '-'} · {formatBeijingDate(o.created_at)}</Text>
                    </View>
                  </View>
                ))}
              </View>
            </ScrollView>
          )}

          {/* Bottom bar — 提出申请 → 独立确认页（#2218） */}
          {selectedOrders.length > 0 && (
            <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, background: '#fff', padding: 16, borderTop: '1px solid #f4f4f5', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <View>
                <Text style={{ fontSize: 14, color: '#666' }}>已选 {selectedOrders.length} 项</Text>
                <Text style={{ fontSize: 16, fontWeight: '700', color: '#D97706', marginLeft: 8 }}>¥{formatCents(selectedTotal)}</Text>
              </View>
              <Button onClick={() => nav(`/invoice-confirm?order_ids=${selectedOrders.map(o => o.order_id).join(',')}`)} style={{ height: 40, background: '#D97706', color: '#fff', borderRadius: 8, fontSize: 14, fontWeight: '700', padding: '0 20px', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: 0 }}>
                提出申请
              </Button>
            </View>
          )}
        </>
      ) : (
        <ScrollView scrollY style={{ height: 'calc(100vh - 60px)' }}>
          <View style={{ padding: 16 }}>
            {applied.length === 0 ? (
              <View style={{ padding: 40, textAlign: 'center' }}><Text style={{ color: '#a1a1aa' }}>暂无发票申请记录</Text></View>
            ) : (
              applied.map(app => (
                <View key={app.id} style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 12 }}>
                  <View style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                    <Text style={{ fontSize: 15, fontWeight: '700' }}>{app.merchant_name || '商户'}</Text>
                    <Text style={{ fontSize: 13, color: app.status === 'replied' ? '#16a34a' : '#d97706', fontWeight: '600' }}>
                      {app.status === 'replied' ? '已开票' : '待开票'}
                    </Text>
                  </View>
                  <View style={{ marginBottom: 4 }}>
                    <Text style={{ fontSize: 13, color: '#666' }}>{app.order_count} 笔订单 · ¥{formatCents(app.total_amount)}</Text>
                  </View>
                  <Text style={{ fontSize: 12, color: '#a1a1aa' }}>申请于 {formatBeijingDate(app.created_at)}</Text>
                  {app.replied_at && (
                    <Text style={{ fontSize: 12, color: '#a1a1aa' }}>回复于 {formatBeijingDate(app.replied_at)}</Text>
                  )}
                  {app.reply && (
                    <View style={{ marginTop: 8, padding: 8, background: '#f9fafb', borderRadius: 8 }}>
                      <Text style={{ fontSize: 13, color: '#555' }}>回复：{app.reply}</Text>
                    </View>
                  )}
                  {app.invoice_file && (
                    <Button onClick={() => handleViewInvoice(app.invoice_file)} style={{ marginTop: 8, height: 36, background: '#fff', border: '1px solid #D97706', color: '#D97706', borderRadius: 8, fontSize: 13, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '8px 0 0', padding: 0 }}>
                      查看发票
                    </Button>
                  )}
                </View>
              ))
            )}
          </View>
        </ScrollView>
      )}
    </View>
  )
}
