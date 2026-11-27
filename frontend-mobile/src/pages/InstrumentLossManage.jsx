import { useState, useRef } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Button, ScrollView, Image } from '@tarojs/components'
import { apiFetch, resolveErrorMessage } from '../services/api'
import { dialog, env, toWeappRoute, useDidShow } from '../platform'
import { formatCents } from '../utils/money'
import { formatBeijingDate } from '../utils/format'

// #1959 阶段3 丢失-1：员工 weapp 丢失登记 / 找回 / 台账（LS-01/05/04）
// #2615 信息架构收敛：本页 = **丢失记录列表**（第一类实体是 loss record）。
//   - 删除「在管乐器」段与行内红色「丢失登记」按钮（1000 台 = 1000 个红按钮）
//   - 唯一登记入口 = 页首「＋ 登记丢失」→ 登记页（/instrument-loss-register）
//   - 行只展示 + 整行点进详情（/instrument-loss-detail?id=），恢复动作在详情页
//   D1/D6：列表不再用 /instruments?stock_status=lost 当主数据源；危险色只出现在
//   登记/恢复这两个确认动作按钮上，列表行与入口一律不用危险色。

const PAGE_SIZE = 20
const partyLabels = { user: '用户', logistics: '物流公司', platform: '平台', site: '网点' }

const cardStyle = {
  display: 'flex', flexDirection: 'column', gap: 6,
  backgroundColor: '#FFFFFF', borderRadius: 12, padding: 12, marginBottom: 10,
}
const btnPrimaryStyle = {
  width: '100%', margin: 0, height: 40, display: 'flex', alignItems: 'center',
  justifyContent: 'center', backgroundColor: '#171717', color: '#FFFFFF',
  borderRadius: 8, fontSize: 14, fontWeight: 'bold',
}
const thumbStyle = { width: 48, height: 48, borderRadius: 8, backgroundColor: '#F4F4F5', flexShrink: 0 }

export default function InstrumentLossManage() {
  const navigate = useNavigate()
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    return Taro.navigateTo({ url: route.url })
  }
  const [records, setRecords] = useState([])
  const [total, setTotal] = useState(0)
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  // 触底加载用 ref（避免 setState 异步导致的重复请求）
  const pageRef = useRef(1)
  const busyRef = useRef(false)
  const baseUrl = env.apiBaseUrl

  const hasMore = records.length < total

  const fetchPage = async (pageNum, append) => {
    if (busyRef.current) return
    busyRef.current = true
    if (append) setLoadingMore(true)
    else setLoading(true)
    try {
      const res = await apiFetch(`${baseUrl}/instrument-loss?page=${pageNum}&pageSize=${PAGE_SIZE}&status=all`)
      const d = await res.json()
      if (d.code === 20000) {
        const list = d.data?.list || []
        setRecords((prev) => (append ? [...prev, ...list] : list))
        setTotal(d.data?.total ?? 0)
        pageRef.current = pageNum
      } else {
        dialog.alert(resolveErrorMessage(d))
      }
    } catch (e) { dialog.alert(resolveErrorMessage(e)) }
    busyRef.current = false
    setLoading(false)
    setLoadingMore(false)
  }

  // D9：从登记页/详情页返回后刷新第一页（H5 幂等；weapp 用 Taro.useDidShow）
  useDidShow(() => {
    pageRef.current = 1
    fetchPage(1, false)
  })

  const loadMore = () => {
    if (!hasMore || busyRef.current) return
    fetchPage(pageRef.current + 1, true)
  }

  const openDetail = (record) => nav(`/instrument-loss-detail?id=${record.id}`)

  return (
    <View style={{ backgroundColor: '#FDFBF7', display: 'flex', flexDirection: 'column', height: '100vh' }}>
      {/* 页首仅 H5（weapp 用原生导航栏 + index.config.ts 标题，#2220） */}
      {!env.isMiniProgram && (
        <View style={{ backgroundColor: '#FFFFFF', padding: '12px 16px', borderBottom: '1px solid #F4F4F5', display: 'flex', alignItems: 'center', gap: 8 }}>
          <Text onClick={() => nav(-1)} style={{ fontSize: 20, color: '#18181B', padding: '0 6px' }}>‹</Text>
          <Text style={{ fontSize: 16, fontWeight: 'bold', color: '#18181B' }}>丢失记录</Text>
        </View>
      )}

      <ScrollView scrollY style={{ flex: 1, minHeight: 0 }} onScrollToLower={loadMore}>
        <View style={{ padding: '12px 16px 96px', boxSizing: 'border-box' }}>
          {/* 唯一登记入口（单一入口原则：列表内不再出现其它登记入口） */}
          <Button data-testid="loss-register-entry" onClick={() => nav('/instrument-loss-register')} style={btnPrimaryStyle}>
            ＋ 登记丢失
          </Button>

          <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', margin: '14px 0 8px' }}>
            <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>丢失记录（{total}）</Text>
            <Text onClick={() => fetchPage(1, false)} style={{ fontSize: 12, color: '#71717A' }}>刷新</Text>
          </View>

          {loading ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
          ) : records.length === 0 ? (
            <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无丢失记录，点击上方「＋ 登记丢失」开始登记</Text>
          ) : records.map((r) => {
            const brief = r.instrument_brief || {}
            const restored = Boolean(r.restored_at)
            return (
              <View key={r.id} data-testid="loss-row" onClick={() => openDetail(r)} style={cardStyle}>
                <View style={{ display: 'flex', gap: 10, alignItems: 'center' }}>
                  {brief.cover_image ? (
                    <Image src={brief.cover_image} mode="aspectFill" style={thumbStyle} />
                  ) : (
                    <View style={thumbStyle} />
                  )}
                  <View style={{ display: 'flex', flexDirection: 'column', gap: 2, flex: 1, minWidth: 0 }}>
                    <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                      <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>SN: {brief.sn || '-'}</Text>
                      <Text style={{ fontSize: 12, color: restored ? '#16A34A' : '#DC2626' }}>
                        {restored ? '已恢复' : '丢失中'}
                      </Text>
                    </View>
                    <Text style={{ fontSize: 12, color: '#71717A' }}>
                      {brief.category_name || '-'}{brief.level_name ? ` · ${brief.level_name}` : ''}
                    </Text>
                  </View>
                </View>
                <Text style={{ fontSize: 11, color: '#71717A' }}>
                  丢失于 {formatBeijingDate(r.created_at)}
                </Text>
                <Text style={{ fontSize: 11, color: '#71717A' }}>
                  {partyLabels[r.responsible_party] || r.responsible_party} · 比例 {r.user_ratio}% ·
                  {' '}赔偿 ¥{formatCents(r.compensation_cents || 0)} · 承担 ¥{formatCents(r.user_burden_cents || 0)}
                </Text>
              </View>
            )
          })}

          {loadingMore && (
            <Text style={{ display: 'block', textAlign: 'center', fontSize: 12, color: '#A1A1AA', padding: '8px 0' }}>加载更多...</Text>
          )}
          {!hasMore && records.length > 0 && (
            <Text style={{ display: 'block', textAlign: 'center', fontSize: 12, color: '#C4C4C8', padding: '8px 0' }}>没有更多了</Text>
          )}
        </View>
      </ScrollView>
    </View>
  )
}
