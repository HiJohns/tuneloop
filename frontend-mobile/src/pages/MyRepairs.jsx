import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView, Button, Input } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import BottomNav from '../components/BottomNav'

const statusLabels = {
  pending_assessment: '待评估', pending_ship: '待发送', shipping: '发送中', inspecting: '质检中',
  quoted: '待回复', pending_payment: '待付款', pending_cancel: '待取消',
  repairing: '维修中', return_pending: '待发回', returned: '已发回',
  closed: '已关闭', appealing: '申诉中',
}

export default function MyRepairs() {
  const navigate = useNavigate()
  const [snInput, setSnInput] = useState('')
  const [myRepairs, setMyRepairs] = useState([])
  const [pendingRepairs, setPendingRepairs] = useState([])
  const [repairRequests, setRepairRequests] = useState([])
  const [loading, setLoading] = useState(true)
  const [roles, setRoles] = useState([])
  const baseUrl = env.apiBaseUrl

  // Check if user is customer (no staff claims)
  const token = getToken()
  const isCustomer = (() => {
    if (!token) return true
    try {
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload?.role === 'USER' || !payload?.role
    } catch { return true }
  })()

  const fetchRepairs = async () => {
    setLoading(true)
    try {
      const [roleRes] = await Promise.all([
        apiFetch(`${baseUrl}/site-members/me`),
      ])
      const role = await roleRes.json()
      const roles = role.code === 20000 ? (role.data?.roles || []) : []
      setRoles(roles)

      const hasSiteRole = roles.some(r => ['site_admin', 'site_member', 'worker'].includes(r))
      const isPureTech = roles.includes('repair_technician') && !hasSiteRole

      const fetches = []

      // All roles: fetch repair requests
      if (isCustomer) {
        fetches.push(apiFetch(`${baseUrl}/repair-requests`).then(r => r.json()).then(r => {
          if (r.code === 20000) setRepairRequests(r.data?.list || [])
        }))
      } else {
        // Staff/tech: fetch my repairs + pending
        fetches.push(apiFetch(`${baseUrl}/repair/mine`).then(r => r.json()).then(r => {
          if (r.code === 20000) setMyRepairs(r.data?.list || [])
        }))
        fetches.push(apiFetch(`${baseUrl}/repair/pending`).then(r => r.json()).then(r => {
          if (r.code === 20000) setPendingRepairs(r.data?.list || [])
        }))
      }

      // Staff: also fetch site repair requests
      if (hasSiteRole || isPureTech) {
        const statusFilter = isPureTech ? '?status=inspecting,repairing' : '?status=pending_assessment,return_pending,pending_ship,shipping,repairing,quoted,inspecting'
        fetches.push(apiFetch(`${baseUrl}/repair-requests${statusFilter}`).then(r => r.json()).then(r => {
          if (r.code === 20000) setRepairRequests(r.data?.list || [])
        }))
      }

      await Promise.all(fetches)
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchRepairs() }, [])

  const hasSiteRole = roles.some(r => ['site_admin', 'site_member', 'worker'].includes(r))
  const isPureTech = roles.includes('repair_technician') && !hasSiteRole

  const handleSearch = () => {
    if (!snInput.trim()) return
    navigate(`/repair?instrument_id=${snInput.trim()}`)
  }

  const handleShipBack = async (id) => {
    const company = prompt('输入物流公司')
    if (!company) return
    const number = prompt('输入物流单号')
    if (!number) return
    try {
      const resp = await apiFetch(`${baseUrl}/repair-requests/${id}/return-shipping`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ return_company: company, return_tracking_number: number }),
      })
      const result = await resp.json()
      if (result.code === 20000) { await fetchRepairs() }
      else { alert(result.message) }
    } catch {}
  }

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg font-black text-black">维修</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        {/* Scan / SN search — staff and repair technicians only */}
        {!isCustomer && (
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black mb-2">扫码查找乐器</Text>
          <View className="flex gap-2">
            <input className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
              value={snInput} onChange={e => setSnInput(e.target.value)} placeholder="输入乐器编号或扫码" />
            <Button onClick={handleSearch} className="px-4 py-2 bg-black text-white rounded-lg text-sm font-bold">查找</Button>
          </View>
        </View>
        )}

        {/* Customer: My repair requests + create button */}
        {isCustomer && (
          <>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-1">
            <View><Text className="text-sm font-bold text-black">我的报修 ({repairRequests.length})</Text></View>
            {loading ? (
              <View><Text className="text-xs text-zinc-400">加载中...</Text></View>
            ) : repairRequests.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无报修记录</Text></View>
            ) : (
              <View className="space-y-2">
                {repairRequests.map(r => (
                  <View key={r.id} className="border border-zinc-100 rounded-xl p-3 space-y-1 active:opacity-80"
                    onClick={() => navigate(`/repair-request?request_id=${r.id}`)}>
                    <View className="flex justify-between items-center">
                      <Text className="text-sm font-bold text-black">{r.created_at ? new Date(r.created_at).toLocaleDateString() : '#' + r.id?.slice(0, 8)}</Text>
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
                      <Text className="text-xs text-zinc-400">商户</Text>
                      <Text className="text-xs text-zinc-600">{r.merchant_name || '-'}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">网点</Text>
                      <Text className="text-xs text-zinc-600">{r.site_name || '-'}</Text>
                    </View>
                    {r.quote_amount != null && (
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">报价</Text>
                      <Text className="text-xs text-zinc-600">¥{r.quote_amount}</Text>
                    </View>
                    )}
                  </View>
                ))}
              </View>
            )}
          </View>
          <Button onClick={() => navigate('/create-repair')}
            className="fixed bottom-20 right-4 w-14 h-14 bg-black text-white rounded-full text-2xl font-bold shadow-lg flex items-center justify-center z-50">+</Button>
          </>
        )}

        {/* Pure repair technician: My repairs + inspecting/repairing requests */}
        {isPureTech && (
          <>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-1">
            <View><Text className="text-sm font-bold text-black">我的维修 ({myRepairs.length})</Text></View>
            {loading ? (
              <View><Text className="text-xs text-zinc-400">加载中...</Text></View>
            ) : myRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无进行中的维修</Text></View>
            ) : (
              <View className="space-y-2">
                {myRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3 active:opacity-80"
                    onClick={() => navigate(`/repair?instrument_id=${inst.id}`)}>
                    <Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text>
                    <Text className="text-xs text-zinc-400 mt-1">
                      状态: {inst.repair_status === 'repair_in_progress' ? '维修中' : inst.repair_status}
                    </Text>
                  </View>
                ))}
              </View>
            )}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-1">
            <View><Text className="text-sm font-bold text-black">质检/维修中报修 ({repairRequests.length})</Text></View>
            {repairRequests.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无</Text></View>
            ) : (
              <View className="space-y-2">
                {repairRequests.map(r => (
                  <View key={r.id} className="border border-zinc-100 rounded-xl p-3 active:opacity-80"
                    onClick={() => navigate(`/repair-request?request_id=${r.id}`)}>
                    <Text className="text-sm font-bold text-black">#{r.id?.slice(0, 8)}</Text>
                    <Text className="text-xs text-zinc-400">{statusLabels[r.status] || r.status}</Text>
                  </View>
                ))}
              </View>
            )}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4 space-y-1">
            <View><Text className="text-sm font-bold text-black">待维修乐器 ({pendingRepairs.length})</Text></View>
            {pendingRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无</Text></View>
            ) : (
              <View className="space-y-2">
                {pendingRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3 active:opacity-80"
                    onClick={() => navigate(`/repair?instrument_id=${inst.id}`)}>
                    <Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text>
                    <Text className="text-xs text-zinc-400">{inst.category_name || ''}</Text>
                    <Button className="mt-2 py-1.5 bg-black text-white rounded-lg text-xs font-bold">接单</Button>
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
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 space-y-1">
            <View><Text className="text-sm font-bold text-black">本网点报修 ({repairRequests.length})</Text></View>
            {loading ? (
              <View><Text className="text-xs text-zinc-400">加载中...</Text></View>
            ) : repairRequests.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无报修</Text></View>
            ) : (
              <View className="space-y-2">
                {repairRequests.map(r => (
                  <View key={r.id} className="border border-zinc-100 rounded-xl p-3 space-y-1 active:opacity-80"
                    onClick={() => navigate(`/repair-request?request_id=${r.id}`)}>
                    <View className="flex justify-between items-center">
                      <Text className="text-sm font-bold text-black">{r.created_at ? new Date(r.created_at).toLocaleDateString() : '#' + r.id?.slice(0, 8)}</Text>
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
                      <Text className="text-xs text-zinc-400">报修人</Text>
                      <Text className="text-xs text-zinc-600">{r.reporter_name || '-'}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">商户</Text>
                      <Text className="text-xs text-zinc-600">{r.merchant_name || '-'}</Text>
                    </View>
                    <View className="flex justify-between items-center">
                      <Text className="text-xs text-zinc-400">网点</Text>
                      <Text className="text-xs text-zinc-600">{r.site_name || '-'}</Text>
                    </View>
                    {r.status === 'return_pending' && (
                      <Button onClick={(e) => { e.stopPropagation(); handleShipBack(r.id) }}
                        className="mt-1 py-1.5 bg-black text-white rounded-lg text-xs font-bold">填物流发回</Button>
                    )}
                  </View>
                ))}
              </View>
            )}
          </View>
          <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4 space-y-1">
            <View><Text className="text-sm font-bold text-black">待维修乐器 ({pendingRepairs.length})</Text></View>
            {pendingRepairs.length === 0 ? (
              <View><Text className="text-xs text-zinc-400">暂无等待维修的乐器</Text></View>
            ) : (
              <View className="space-y-2">
                {pendingRepairs.map(inst => (
                  <View key={inst.id} className="border border-zinc-100 rounded-xl p-3 active:opacity-80">
                    <Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text>
                    <Text className="text-xs text-zinc-400">{inst.category_name || ''}</Text>
                    <Button onClick={() => navigate(`/repair?instrument_id=${inst.id}`)}
                      className="mt-2 py-1.5 bg-black text-white rounded-lg text-xs font-bold">查看</Button>
                  </View>
                ))}
              </View>
            )}
          </View>
          </>
        )}
      </ScrollView>

      <BottomNav
        active="service"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => navigate('/') },
          ...(!isPureTech ? [{ key: 'rent', icon: '🪕', label: '租赁', onClick: () => navigate('/my-leases') }] : []),
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => navigate('/my-repairs') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => navigate('/profile') },
        ]}
      />
    </View>
  )
}
