import { useState, useEffect, useMemo } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Button, ScrollView } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env, dialog } from '../platform'

function formatCents(cents) {
  return (cents / 100).toFixed(2)
}

function formatDate(dateStr) {
  if (!dateStr) return '-'
  return new Date(dateStr).toLocaleDateString('zh-CN', { year: 'numeric', month: '2-digit', day: '2-digit' })
}

export default function Invoice() {
  const [tab, setTab] = useState('eligible') // 'eligible' | 'applied'
  const [eligible, setEligible] = useState([])
  const [applied, setApplied] = useState([])
  const [loading, setLoading] = useState(true)
  const [selected, setSelected] = useState({}) // { orderId: true }
  const [confirming, setConfirming] = useState(false)
  const [submitting, setSubmitting] = useState(false)

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
    } catch (e) {
      console.error('Failed to load invoices:', e)
    }
    setLoading(false)
  }

  useEffect(() => {
    loadData()
  }, [tab])

  const toggleSelect = (orderId) => {
    setSelected(prev => ({ ...prev, [orderId]: !prev[orderId] }))
  }

  const selectedOrders = useMemo(() => {
    return eligible.filter(o => selected[o.order_id])
  }, [eligible, selected])

  const selectedTotal = useMemo(() => {
    return selectedOrders.reduce((sum, o) => sum + o.total_cents, 0)
  }, [selectedOrders])

  // Group by merchant for confirmation step
  const merchantGroups = useMemo(() => {
    const map = {}
    selectedOrders.forEach(o => {
      if (!map[o.tenant_id]) {
        map[o.tenant_id] = { tenant_id: o.tenant_id, merchant_name: o.merchant_name, orders: [], total: 0 }
      }
      map[o.tenant_id].orders.push(o)
      map[o.tenant_id].total += o.total_cents
    })
    return Object.values(map)
  }, [selectedOrders])

  const handleSubmit = async () => {
    if (selectedOrders.length === 0) {
      Taro.showToast({ title: '请先选择订单', icon: 'none' })
      return
    }
    setSubmitting(true)
    try {
      const groups = merchantGroups.map(g => ({
        tenant_id: g.tenant_id,
        order_ids: g.orders.map(o => o.order_id),
      }))
      const resp = await apiFetch(`${env.apiBaseUrl}/user/invoices`, {
        method: 'POST',
        body: JSON.stringify({ groups }),
      })
      const data = await resp.json()
      if (data.code === 20000) {
        Taro.showToast({ title: '申请已提交', icon: 'success' })
        setSelected({})
        setConfirming(false)
        setTab('applied')
      } else {
        Taro.showToast({ title: data.message || '提交失败', icon: 'none' })
      }
    } catch (e) {
      Taro.showToast({ title: '提交失败', icon: 'none' })
    }
    setSubmitting(false)
  }

  const handleViewInvoice = async (fileUrl) => {
    if (!fileUrl) return
    if (env.isMiniProgram) {
      try {
        const res = await Taro.downloadFile({ url: fileUrl })
        Taro.openDocument({ filePath: res.tempFilePath, showMenu: true })
      } catch (e) {
        Taro.showToast({ title: '打开失败', icon: 'none' })
      }
    } else {
      window.open(fileUrl)
    }
  }

  // Confirmation view
  if (confirming) {
    return (
      <View style={{ minHeight: '100vh', background: '#FDFBF7' }}>
        <View style={{ padding: 16 }}>
          <View style={{ display: 'flex', alignItems: 'center', marginBottom: 16 }}>
            <Button onClick={() => setConfirming(false)} style={{ background: 'none', border: 'none', padding: 0, fontSize: 16, color: '#000' }}>
              ‹ 返回
            </Button>
            <Text style={{ fontSize: 18, fontWeight: '700', marginLeft: 8 }}>确认申请</Text>
          </View>

          {merchantGroups.map(g => (
            <View key={g.tenant_id} style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 12 }}>
              <Text style={{ fontSize: 16, fontWeight: '700', marginBottom: 8 }}>{g.merchant_name || '未知商户'}</Text>
              <Text style={{ fontSize: 13, color: '#666', marginBottom: 4 }}>{g.orders.length} 笔订单</Text>
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
            </View>
          ))}

          <View style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 16 }}>
            <View style={{ display: 'flex', justifyContent: 'space-between' }}>
              <Text style={{ fontSize: 16, fontWeight: '700' }}>总开票金额</Text>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#D97706' }}>¥{formatCents(selectedTotal)}</Text>
            </View>
          </View>

          <Button
            onClick={handleSubmit}
            disabled={submitting}
            style={{ width: '100%', height: 48, background: '#D97706', color: '#fff', borderRadius: 12, fontSize: 16, fontWeight: '700', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: 0, padding: 0 }}
          >
            {submitting ? '提交中...' : '确认提交'}
          </Button>
        </View>
      </View>
    )
  }

  return (
    <View style={{ minHeight: '100vh', background: '#FDFBF7' }}>
      {/* Tabs */}
      <View style={{ display: 'flex', background: '#fff', borderBottom: '1px solid #f4f4f5' }}>
        {[
          { key: 'eligible', label: '未申请' },
          { key: 'applied', label: '已申请' },
        ].map(t => (
          <View
            key={t.key}
            onClick={() => { setTab(t.key); setSelected({}); setConfirming(false) }}
            style={{ flex: 1, textAlign: 'center', padding: '12px 0', fontSize: 15, fontWeight: tab === t.key ? '700' : '400', color: tab === t.key ? '#D97706' : '#71717a', borderBottom: tab === t.key ? '2px solid #D97706' : '2px solid transparent' }}
          >
            {t.label}
          </View>
        ))}
      </View>

      {loading ? (
        <View style={{ padding: 40, textAlign: 'center' }}>
          <Text style={{ color: '#a1a1aa' }}>加载中...</Text>
        </View>
      ) : tab === 'eligible' ? (
        <>
          {eligible.length === 0 ? (
            <View style={{ padding: 40, textAlign: 'center' }}>
              <Text style={{ color: '#a1a1aa' }}>暂无可申请发票的订单</Text>
            </View>
          ) : (
            <ScrollView scrollY style={{ height: 'calc(100vh - 120px)' }}>
              <View style={{ padding: 16 }}>
                {eligible.map(o => (
                  <View
                    key={o.order_id}
                    onClick={() => toggleSelect(o.order_id)}
                    style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 8, display: 'flex', alignItems: 'center' }}
                  >
                    <View style={{ width: 22, height: 22, borderRadius: 4, border: selected[o.order_id] ? 'none' : '1px solid #d4d4d8', background: selected[o.order_id] ? '#D97706' : '#fff', display: 'flex', alignItems: 'center', justifyContent: 'center', marginRight: 12, flexShrink: 0 }}>
                      {selected[o.order_id] && <Text style={{ color: '#fff', fontSize: 14, fontWeight: '700' }}>✓</Text>}
                    </View>
                    <View style={{ flex: 1 }}>
                      <View style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                        <Text style={{ fontSize: 15, fontWeight: '600' }}>{o.merchant_name || '商户'}</Text>
                        <Text style={{ fontSize: 15, fontWeight: '700', color: '#D97706' }}>¥{formatCents(o.total_cents)}</Text>
                      </View>
                      <Text style={{ fontSize: 12, color: '#a1a1aa' }}>SN: {o.sn || '-'} · {formatDate(o.created_at)}</Text>
                    </View>
                  </View>
                ))}
              </View>
            </ScrollView>
          )}

          {/* Bottom bar */}
          {selectedOrders.length > 0 && (
            <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, background: '#fff', padding: 16, borderTop: '1px solid #f4f4f5', display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
              <View>
                <Text style={{ fontSize: 14, color: '#666' }}>已选 {selectedOrders.length} 项</Text>
                <Text style={{ fontSize: 16, fontWeight: '700', color: '#D97706', marginLeft: 8 }}>¥{formatCents(selectedTotal)}</Text>
              </View>
              <Button
                onClick={() => setConfirming(true)}
                style={{ height: 40, background: '#D97706', color: '#fff', borderRadius: 8, fontSize: 14, fontWeight: '700', padding: '0 20px', display: 'flex', alignItems: 'center', justifyContent: 'center', margin: 0 }}
              >
                提出申请
              </Button>
            </View>
          )}
        </>
      ) : (
        /* Applied tab */
        <ScrollView scrollY style={{ height: 'calc(100vh - 60px)' }}>
          <View style={{ padding: 16 }}>
            {applied.length === 0 ? (
              <View style={{ padding: 40, textAlign: 'center' }}>
                <Text style={{ color: '#a1a1aa' }}>暂无发票申请记录</Text>
              </View>
            ) : (
              applied.map(app => (
                <View key={app.id} style={{ background: '#fff', borderRadius: 12, padding: 16, marginBottom: 12 }}>
                  <View style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                    <Text style={{ fontSize: 15, fontWeight: '700' }}>{app.merchant_name || '商户'}</Text>
                    <Text style={{ fontSize: 13, color: app.status === 'replied' ? '#16a34a' : '#d97706', fontWeight: '600' }}>
                      {app.status === 'replied' ? '已开票' : '待开票'}
                    </Text>
                  </View>
                  <Text style={{ fontSize: 13, color: '#666', marginBottom: 4 }}>{app.order_count} 笔订单 · ¥{formatCents(app.total_amount)}</Text>
                  <Text style={{ fontSize: 12, color: '#a1a1aa' }}>申请于 {formatDate(app.created_at)}</Text>
                  {app.replied_at && (
                    <Text style={{ fontSize: 12, color: '#a1a1aa' }}>回复于 {formatDate(app.replied_at)}</Text>
                  )}
                  {app.reply && (
                    <View style={{ marginTop: 8, padding: 8, background: '#f9fafb', borderRadius: 8 }}>
                      <Text style={{ fontSize: 13, color: '#555' }}>回复：{app.reply}</Text>
                    </View>
                  )}
                  {app.invoice_file && (
                    <Button
                      onClick={() => handleViewInvoice(app.invoice_file)}
                      style={{ marginTop: 8, height: 36, background: '#fff', border: '1px solid #D97706', color: '#D97706', borderRadius: 8, fontSize: 13, display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '8px 0 0', padding: 0 }}
                    >
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
