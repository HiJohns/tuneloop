import { useState, useEffect } from 'react'
import { formatCents } from '../utils/money'
import { useNavigate, useSearchParams } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, ScrollView, Button, Input } from '@tarojs/components'
import { apiFetch, getToken, resolveErrorMessage } from '../services/api'
import { dialog, env, getInputValue, toWeappRoute } from '../platform'
import { isStaffRole } from '../utils/role'
import BottomNav from '../components/BottomNav'
import TechRepairSections from '../components/TechRepairSections'
import SegmentedTabs from '../components/SegmentedTabs' // #2150 统一分段控制器
import BottomNavWeapp from '../components-weapp/BottomNav'
import { formatBeijingDate, repairStatusLabel } from '../utils/format'

const statusLabels = {
  pending_assessment: '待评估', transit_processing: '中转处理中',
  pending_ship: '待发送', shipping: '发送中', transit_in: '转入中',
  pending_payment: '待付款', repairing: '维修中',
  return_pending: '待发回', transit_out: '转出中', returned: '已发回',
  closed: '已关闭', appealing: '申诉中',
}

export default function MyRepairs() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  // #2050 维修区角色互斥：员工=内部维修工单，顾客=维修服务（含 oid/tid 非空的顾客仍是顾客）
  const token = getToken()
  const isStaff = isStaffRole(token)
  const isCustomer = !isStaff
  // Cross-end navigation (issue-1673): weapp has no react-router short paths;
  // central toWeappRoute maps H5 paths → /pages-weapp/... page urls.
  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    if (to === -1) return Taro.navigateBack()
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }
  const [snInput, setSnInput] = useState('')
  const [myRepairs, setMyRepairs] = useState([])
  const [pendingRepairs, setPendingRepairs] = useState([])
  const [acceptanceRepairs, setAcceptanceRepairs] = useState([])
  const [repairRequests, setRepairRequests] = useState([])
  const [loading, setLoading] = useState(true)
  const [roles, setRoles] = useState([])
  const [shippingBack, setShippingBack] = useState(false)
  const [showSiteRepairs, setShowSiteRepairs] = useState(true)
  const [showPending, setShowPending] = useState(true)
  // #1957 入口区分：维修工单（v3）/ 维修服务（RS-10）；#2050 顾客锁定「维修服务」
  const [svcTab, setSvcTab] = useState((!isStaff || searchParams.get('tab') === 'service') ? 'service' : 'legacy')
  const [myServices, setMyServices] = useState([])
  const [servicesLoaded, setServicesLoaded] = useState(false)
  // 排序：创建时间(created_at)/最后更新时间(updated_at) × 逆序/正序；默认创建时间逆序
  const [sortField, setSortField] = useState('created_at')
  const [sortDir, setSortDir] = useState('desc')
  const baseUrl = env.apiBaseUrl

  const fetchRepairs = async () => {
    setLoading(true)
    try {
      const [roleRes] = await Promise.all([
        apiFetch(`${baseUrl}/site-members/me`),
      ])
      const role = await roleRes.json()
      const roles = role.code === 20000 ? (role.data?.roles || []) : []
      setRoles(roles)

      const hasSiteRole = roles.some(r => ['site_admin', 'site_member'].includes(r))
      const isPureTech = roles.includes('repair_technician') && !hasSiteRole

      const fetches = []

      // Staff/tech: fetch my repairs + pending
      fetches.push(apiFetch(`${baseUrl}/repair/mine`).then(r => r.json()).then(r => {
        if (r.code === 20000) setMyRepairs(r.data?.list || [])
      }))
      fetches.push(apiFetch(`${baseUrl}/repair/pending`).then(r => r.json()).then(r => {
        if (r.code === 20000) setPendingRepairs(r.data?.list || [])
      }))

      // Staff: also fetch site repair requests
      if (hasSiteRole || isPureTech) {
        const statusFilter = isPureTech ? '?status=pending_assessment,repairing,return_pending' : '?status=pending_assessment,shipping,transit_in,repairing,return_pending,transit_out,returned,appealing'
        fetches.push(apiFetch(`${baseUrl}/repair-requests${statusFilter}`).then(r => r.json()).then(r => {
          if (r.code === 20000) setRepairRequests(r.data?.list || [])
        }))
      }

      // Staff: instruments awaiting acceptance at their sites (#1892)
      if (hasSiteRole) {
        fetches.push(apiFetch(`${baseUrl}/repair/acceptance`).then(r => r.json()).then(r => {
          if (r.code === 20000) setAcceptanceRepairs(r.data?.list || [])
        }))
      }

      await Promise.all(fetches)
    } catch {}
    setLoading(false)
  }

  // #1957：维修服务（type='service'）我的单（顾客视角，只读摘要；完整流程见阶段3a）
  const fetchMyServices = async () => {
    try {
      const sortParam = sortDir === 'desc' ? `-${sortField}` : sortField
      const res = await apiFetch(`${baseUrl}/user/repair-services?sort=${sortParam}`)
      const result = await res.json()
      if (result.code === 20000) setMyServices(result.data?.list || [])
    } catch {}
    setServicesLoaded(true)
  }

  // #2050：顾客侧历史维修工单（v3 legacy，type != 'service'）——只读，跟进存量/在途单
  const fetchLegacyRepairs = async () => {
    try {
      const res = await apiFetch(`${baseUrl}/repair-requests`)
      const result = await res.json()
      if (result.code === 20000) {
        setRepairRequests((result.data?.list || []).filter(r => r.type !== 'service'))
      }
    } catch {}
  }

  // #2050：顾客=维修服务 + 只读历史维修工单；员工=内部维修工单（互不拉取对方数据）
  useEffect(() => {
    if (isCustomer) { fetchMyServices(); fetchLegacyRepairs() }
    else fetchRepairs()
  }, [sortField, sortDir])

  const hasSiteRole = roles.some(r => ['site_admin', 'site_member'].includes(r))
  const isPureTech = roles.includes('repair_technician') && !hasSiteRole

  const handleSearch = () => {
    if (!snInput.trim()) return
    nav(`/repair?instrument_id=${snInput.trim()}`)
  }

  const handleShipBack = async (id) => {
    let company = ''
    let number = ''
    if (env.isMiniProgram) {
      const res = await Taro.showModal({ title: '发回物流', editable: true, placeholderText: '输入物流公司' })
      if (!res.confirm) return
      company = res.content || ''
      const res2 = await Taro.showModal({ title: '发回物流', editable: true, placeholderText: '输入物流单号' })
      if (!res2.confirm) return
      number = res2.content || ''
    } else {
      company = prompt('输入物流公司')
      if (!company) return
      number = prompt('输入物流单号')
      if (!number) return
    }
    setShippingBack(true)
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${id}/return-shipping`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ return_company: company, return_tracking_number: number }),
      })
      const result = await resp.json()
      if (result.code === 20000) { await fetchRepairs() }
      else { dialog.alert(resolveErrorMessage(result)) }
    } catch {}
    setShippingBack(false)
  }

  const svcStatusLabels = {
    pending_quote: '待报价', pending_payment: '待付款', paid: '已支付·待寄出',
    shipping: '寄送中', repairing: '维修中', adjust_pending: '加价待确认',
    done_repair: '待发回', closed: '已结算',
  }
  // RS-12：分状态分组 + 待办提示（金额/标志来自列表接口 reviewed/pending_shortfall_cents）
  const svcTodo = (s) => {
    if (s.status === 'pending_quote') return s.technician_id ? '等待维修师报价' : '待选择维修师'
    if (s.status === 'pending_payment') return '待接受报价并支付'
    if (s.status === 'adjust_pending') {
      const diff = (s.adjusted_quote_cents || 0) - (s.quote_repair_cents || 0)
      return `待决定：继续需补差价 ¥${formatCents(Math.max(0, diff))}`
    }
    if (s.status === 'closed') {
      if (s.pending_shortfall_cents > 0) return `待补缴 ¥${formatCents(s.pending_shortfall_cents)}`
      return s.reviewed ? '' : '待评价'
    }
    return ''
  }
  const svcGroups = [
    { key: 'todo', label: '待我处理', match: s => ['pending_quote', 'pending_payment', 'adjust_pending'].includes(s.status) },
    { key: 'doing', label: '进行中', match: s => ['paid', 'shipping', 'repairing', 'done_repair'].includes(s.status) },
    { key: 'done', label: '已完成', match: s => s.status === 'closed' },
  ]

  const openServiceTab = () => {
    setSvcTab('service')
    // #2128：「我的维修服务」为顾客专属数据源——员工/技师不得拉取
    if (isCustomer && !servicesLoaded) fetchMyServices()
  }

  const serviceSection = (
    <View style={{ display: 'flex', flexDirection: 'column' }}>
      {!isCustomer && roles.includes('repair_technician') && (
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B', marginBottom: 8, display: 'block' }}>维修师工作台</Text>
          {/* #2084：工作台内容内联（省一跳）；独立页 /tech-repair-workbench 保留（RS-02 深链） */}
          <TechRepairSections />
        </View>
      )}
      {/* #2128：「我的维修服务」= 顾客专属面板（员工/技师不显示，避免空列表误导） */}
      {isCustomer && (
      <View className="bg-white rounded-2xl shadow-sm p-4 mt-4" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>我的维修服务 ({myServices.length})</Text>
        {/* 排序：创建时间/最后更新时间 × 逆序/正序（默认创建时间逆序） */}
        <View style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <SegmentedTabs
            options={[{ key: 'created_at', label: '创建时间' }, { key: 'updated_at', label: '最后更新' }]}
            value={sortField}
            onChange={(k) => setSortField(k)}
            style={{ flex: 1 }}
          />
          <View onClick={() => setSortDir(d => (d === 'desc' ? 'asc' : 'desc'))}
            style={{ paddingTop: 6, paddingBottom: 6, paddingLeft: 10, paddingRight: 10, borderRadius: 8, backgroundColor: '#F4F4F5' }}>
            <Text style={{ fontSize: 12, color: '#52525B' }}>{sortDir === 'desc' ? '↓ 逆序' : '↑ 正序'}</Text>
          </View>
        </View>
        {!servicesLoaded ? (
          <Text style={{ fontSize: 12, color: '#A1A1AA' }}>加载中...</Text>
        ) : myServices.length === 0 ? (
          <Text style={{ fontSize: 12, color: '#A1A1AA' }}>暂无维修服务记录</Text>
        ) : (
          svcGroups.map(g => {
            const items = myServices.filter(g.match)
            if (items.length === 0) return null
            return (
              <View key={g.key} style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                <Text style={{ fontSize: 12, fontWeight: 'bold', color: '#71717A' }}>{g.label}（{items.length}）</Text>
                {items.map(s => {
                  const todo = svcTodo(s)
                  return (
                    <View key={s.id} className="border border-zinc-100 rounded-xl p-3" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}
                      onClick={() => nav(`/repair-service-detail?order_id=${s.id}`)}>
                      <View className="flex justify-between items-center">
                        <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>
                          编码 {s.repair_code || '-'}
                        </Text>
                        <Text style={{ fontSize: 12, color: '#A1A1AA' }}>{svcStatusLabels[s.status] || s.status}</Text>
                      </View>
                      <Text style={{ fontSize: 12, color: '#52525B' }}>{s.description || '（无描述）'}</Text>
                      {todo ? (
                        <Text style={{ fontSize: 11, fontWeight: 'bold', color: '#D97706' }}>{todo}</Text>
                      ) : null}
                      <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
                        {s.created_at ? formatBeijingDate(s.created_at) : '-'}
                      </Text>
                    </View>
                  )
                })}
              </View>
            )
          })
        )}
      </View>
      )}
      {/* #2050 顾客侧历史维修工单（v3 legacy，只读）：创建入口已废弃，存量/在途单仍可查看跟进 */}
      {isCustomer && repairRequests.length > 0 && (
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4" style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
          <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>历史维修工单（{repairRequests.length}）</Text>
          {repairRequests.map(r => (
            <View key={r.id} className="border border-zinc-100 rounded-xl p-3" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}
              onClick={() => nav(`/repair-request?request_id=${r.id}`)}>
              <View className="flex justify-between items-center">
                <Text style={{ fontSize: 13, fontWeight: 'bold', color: '#18181B' }}>{r.instrument_sn || '#' + (r.id || '').slice(0, 8)}</Text>
                <Text style={{ fontSize: 12, color: '#A1A1AA' }}>{statusLabels[r.status] || r.status}</Text>
              </View>
              <Text style={{ fontSize: 11, color: '#A1A1AA' }}>
                {r.created_at ? formatBeijingDate(r.created_at) : '-'}
              </Text>
            </View>
          ))}
        </View>
      )}
    </View>
  )

  return (
    <View style={{ backgroundColor: "#FDFBF7" }} className="flex flex-col h-screen">
      {!env.isMiniProgram && (
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg font-black text-black">{isCustomer ? '我的维修服务' : isPureTech ? '维修工作台' : '维修工单管理'}</Text>
      </View>
      )}

      {/* #1957 RS-10 入口区分：维修工单（v3 legacy）/ 维修服务；#2050 顾客锁定「维修服务」（维修工单仅员工） */}
      {isStaff && (
      <View style={{ backgroundColor: '#FFFFFF', padding: '10px 16px 0' }}>
        <SegmentedTabs
          options={[{ key: 'legacy', label: '维修工单' }, { key: 'service', label: '维修服务' }]}
          value={svcTab}
          onChange={(k) => (k === 'service' ? openServiceTab() : setSvcTab('legacy'))}
        />
      </View>
      )}

      {svcTab === 'service' ? (
      <ScrollView scrollY className="flex-1 min-h-0 overflow-y-auto">
        <View style={{ padding: '0 16px 96px', boxSizing: 'border-box' }}>
          {serviceSection}
        </View>
      </ScrollView>
      ) : (
      <ScrollView scrollY className="flex-1 min-h-0 overflow-y-auto">
        <View style={{ padding: '0 16px 96px', boxSizing: 'border-box' }}>
        {/* Scan / SN search — staff and repair technicians only */}
        {!isCustomer && (
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black mb-2">扫码查找乐器</Text>
          <View className="flex gap-2">
            <Input className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
              value={snInput} onInput={e => setSnInput(getInputValue(e))} placeholder="输入乐器编号或扫码" />
            <Button onClick={handleSearch} className="px-4 py-2 bg-black text-white rounded-lg text-sm font-bold">查找</Button>
          </View>
          {hasSiteRole && (
            <Button onClick={() => nav('/receiving-repair-scan')}
              className="w-full mt-2 py-2 bg-zinc-100 text-zinc-700 rounded-lg text-sm font-bold">
              扫码收货
            </Button>
          )}
        </View>
        )}

        {/* Pure repair technician: My repairs + pending_assessment/repairing requests */}
        {isPureTech && (
          <>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <View><Text className="text-sm font-bold text-black">我的维修 ({myRepairs.length})</Text></View>
            {loading ? (
              <View><Text className="text-xs text-zinc-400">加载中...</Text></View>
            ) : myRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无进行中的维修</Text></View>
            ) : (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {myRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3"
                    onClick={() => nav(`/repair?instrument_id=${inst.id}`)}>
                    <View><Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text></View>
                    <View className="mt-1">
                      <Text className="text-xs text-zinc-400">
                        状态: {repairStatusLabel(inst.repair_status)}
                      </Text>
                    </View>
                  </View>
                ))}
              </View>
            )}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <View><Text className="text-sm font-bold text-black">质检/维修中乐器 ({repairRequests.length})</Text></View>
            {repairRequests.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无</Text></View>
            ) : (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {repairRequests.map(r => (
                  <View key={r.id} className="border border-zinc-100 rounded-xl p-3"
                    onClick={() => nav(`/repair-request?request_id=${r.id}`)}>
                    <Text className="text-sm font-bold text-black">#{r.id?.slice(0, 8)}</Text>
                    <Text className="text-xs text-zinc-400">{statusLabels[r.status] || r.status}</Text>
                  </View>
                ))}
              </View>
            )}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <View><Text className="text-sm font-bold text-black">待维修乐器 ({pendingRepairs.length})</Text></View>
            {pendingRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无</Text></View>
            ) : (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {pendingRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3"
                    onClick={() => nav(`/repair?instrument_id=${inst.id}`)}
                    style={{ display: 'flex', flexDirection: 'column', gap: 4, cursor: 'pointer' }}>
                    <View><Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text></View>
                    <View><Text className="text-xs text-zinc-400">{inst.category_name || ''}</Text></View>
                    <Button
                      style={{ height: 40, margin: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
                      className="bg-black text-white rounded-lg text-xs font-bold"
                      onClick={(e) => { e.stopPropagation(); nav(`/repair?instrument_id=${inst.id}`) }}
                    >接单</Button>
                  </View>
                ))}
              </View>
            )}
          </View>
          </>
        )}

        {/* Staff: Site repair requests + return-pending logistics */}
        {hasSiteRole && (
          <>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <View className="flex justify-between items-center" onClick={() => setShowSiteRepairs(v => !v)}>
              <Text className="text-sm font-bold text-black">维修工单（本网点全部）({repairRequests.length})</Text>
              <Text className="text-xs text-zinc-400">{showSiteRepairs ? '▾' : '▸'}</Text>
            </View>
            {showSiteRepairs && (loading ? (
              <View><Text className="text-xs text-zinc-400">加载中...</Text></View>
            ) : repairRequests.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无维修工单</Text></View>
            ) : (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {repairRequests.map(r => (
                  <View key={r.id} className="border border-zinc-100 rounded-xl p-3" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}
                    onClick={() => nav(`/repair-request?request_id=${r.id}`)}>
                    <View className="flex justify-between items-center">
                      <Text className="text-sm font-bold text-black">{r.created_at ? formatBeijingDate(r.created_at) : '#' + r.id?.slice(0, 8)}</Text>
                      <Text className="text-xs text-zinc-400">{statusLabels[r.status] || r.status}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">识别码</Text>
                      <Text className="text-xs text-zinc-600">{r.instrument_sn || '-'}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">类别</Text>
                      <Text className="text-xs text-zinc-600">{r.instrument_type || '-'}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">品牌/型号</Text>
                      <Text className="text-xs text-zinc-600">{r.brand && r.model ? `${r.brand} ${r.model}` : r.brand || r.model || '-'}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">顾客</Text>
                      <Text className="text-xs text-zinc-600">{r.reporter_name || '-'}</Text>
                    </View>
                    {r.status === 'return_pending' && (
                      <Button onClick={(e) => { e.stopPropagation(); handleShipBack(r.id) }} disabled={shippingBack}
                        className="mt-1 py-1.5 bg-black text-white rounded-lg text-xs font-bold">{shippingBack ? '处理中...' : '填物流发回'}</Button>
                    )}
                  </View>
                ))}
              </View>
            ))}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <View><Text className="text-sm font-bold text-black">待验收乐器 ({acceptanceRepairs.length})</Text></View>
            {loading ? (
              <View><Text className="text-xs text-zinc-400">加载中...</Text></View>
            ) : acceptanceRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无待验收乐器</Text></View>
            ) : (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {acceptanceRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3"
                    onClick={() => nav(`/repair?instrument_id=${inst.id}`)}
                    style={{ display: 'flex', flexDirection: 'column', gap: 4, cursor: 'pointer' }}>
                    <View><Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text></View>
                    <View><Text className="text-xs text-zinc-400">{inst.repair_worker_name ? `维修师: ${inst.repair_worker_name}` : '已修复，等待验收'}</Text></View>
                    <Button onClick={(e) => { e.stopPropagation(); nav(`/repair?instrument_id=${inst.id}`) }}
                      style={{ height: 40, margin: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}
                      className="bg-black text-white rounded-lg text-xs font-bold">去验收</Button>
                  </View>
                ))}
              </View>
            )}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4" style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
            <View className="flex justify-between items-center" onClick={() => setShowPending(v => !v)}>
              <Text className="text-sm font-bold text-black">待维修乐器 ({pendingRepairs.length})</Text>
              <Text className="text-xs text-zinc-400">{showPending ? '▾' : '▸'}</Text>
            </View>
            {showPending && (pendingRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无等待维修的乐器</Text></View>
            ) : (
              <View style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                {pendingRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3"
                    onClick={() => nav(`/repair?instrument_id=${inst.id}`)}
                    style={{ display: 'flex', flexDirection: 'column', gap: 4, cursor: 'pointer' }}>
                    <View><Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text></View>
                    <View><Text className="text-xs text-zinc-400">{inst.category_name || ''}</Text></View>
                  </View>
                ))}
              </View>
            ))}
          </View>
          </>
        )}
        </View>
      </ScrollView>
      )}

      {env.isMiniProgram ? (
        <BottomNavWeapp active="service" />
      ) : (
      <BottomNav active="service" navigate={navigate} />
      )}
    </View>
  )
}
