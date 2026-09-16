// 中转网点工作台（#1931/#1934 Sub4）—— 会话查找
// 输入 6 位短码 / 订单号，命中会话卡 → 收货页 / 发货页
import { useState, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import { Input, Text, View } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue } from '../platform'

export default function TransitWorkbench() {
  const navigate = useNavigate()
  const baseUrl = env.apiBaseUrl
  const [keyword, setKeyword] = useState('')
  const [sessions, setSessions] = useState([])
  const [searched, setSearched] = useState(false)
  const [loading, setLoading] = useState(false)
  const debounceTimer = useRef(null)

  const handleSearch = (val) => {
    setKeyword(val)
    if (debounceTimer.current) clearTimeout(debounceTimer.current)
    debounceTimer.current = setTimeout(async () => {
      const q = (getInputValue(val) || '').trim()
      if (!q) { setSessions([]); setSearched(false); return }
      setLoading(true)
      try {
        let resp = await apiFetch(`${baseUrl}/forwarding/sessions?session_code=${encodeURIComponent(q)}`)
        let r = await resp.json()
        if (r.code !== 20000 || !(r.data?.list || []).length) {
          resp = await apiFetch(`${baseUrl}/forwarding/sessions?order_id=${encodeURIComponent(q)}`)
          r = await resp.json()
        }
        if (r.code === 20000) { setSessions(r.data?.list || []); setSearched(true) }
        else dialog.alert(resolveErrorMessage(r, '查询失败'))
      } catch (e) {
        dialog.alert('查询失败: ' + (e.message || ''))
      } finally {
        setLoading(false)
      }
    }, 400)
  }

  const goDetail = (s) => {
    // last mile 已发货后跳发货查证/详情；ready 状态 → 发货页；其余 → 收貨页
    if (s.status === 'ready') navigate(`/transit-ship?session=${s.id}`)
    else navigate(`/transit-receive?session=${s.id}`)
  }

  return (
    <View style={{ backgroundColor: '#FDFBF7', minHeight: '100vh' }}>
      {!env.isMiniProgram && (
      <View style={{ padding: '16px 16px 8px', backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
        <Text style={{ fontSize: 20, fontWeight: 900, color: '#000' }}>中转工作台</Text>
      </View>
      )}
      <View style={{ padding: 16, display: 'flex', flexDirection: 'column' }}>
        <View style={{ marginBottom: 12 }}>
          <Input
            style={{ backgroundColor: '#fff', border: '1px solid #e4e4e7', borderRadius: 12, padding: '12px', fontSize: 14 }}
            value={keyword}
            onInput={(e) => handleSearch(getInputValue(e))}
            placeholder="输入 6 位短码 / 订单号搜索"
          />
          {loading && <Text style={{ fontSize: 12, color: '#a1a1aa', marginTop: 4 }}>查询中...</Text>}
        </View>

        {searched && sessions.length === 0 && !loading && (
          <Text style={{ fontSize: 13, color: '#a1a1aa', textAlign: 'center', marginTop: 24 }}>未找到会话，检查编号或联系调度</Text>
        )}

        {sessions.map(s => (
          <View key={s.id}
            onClick={() => goDetail(s)}
            style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, marginBottom: 12, boxShadow: '0 1px 2px rgba(0,0,0,0.05)' }}>
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
              <Text style={{ fontSize: 14, fontWeight: 900, color: '#000' }}>短码 {s.session_code || '-'}</Text>
              <StatusBadge status={s.status} direction={s.direction} />
            </View>
            <Row label="订单" value={String(s.order_id || '').slice(0, 8)} />
            <Row label="方向" value={s.direction === 'return' ? '归还（→商户）' : '外发（→顾客）'} />
            <Row label="物流" value={[s.tracking_company, s.tracking_number].filter(Boolean).join(' ') || '-'} />
          </View>
        ))}
      </View>
    </View>
  )
}

function Row({ label, value }) {
  return (
    <View style={{ display: 'flex', justifyContent: 'space-between', marginTop: 6 }}>
      <Text style={{ fontSize: 12, color: '#a1a1aa' }}>{label}</Text>
      <Text style={{ fontSize: 12, color: '#000' }}>{value}</Text>
    </View>
  )
}

function StatusBadge({ status, direction }) {
  const map = {
    pending: { label: '待发运', color: '#a1a1aa' },
    in_transit: { label: '运输中', color: '#06b6d4' },
    received: { label: '已收货', color: '#0ea5e9' },
    ready: { label: '等待转发', color: '#f59e0b' },
    last_mile: { label: '派送中', color: '#3b82f6' },
    completed: { label: '已送达', color: '#16a34a' },
    lost: { label: '丢失', color: '#ef4444' },
  }
  const info = map[status] || { label: status, color: '#a1a1aa' }
  return (
    <Text style={{ fontSize: 12, color: '#fff', backgroundColor: info.color, padding: '2px 8px', borderRadius: 999, fontWeight: '700' }}>
      {(direction === 'return' ? '归·' : '') + info.label}
    </Text>
  )
}
