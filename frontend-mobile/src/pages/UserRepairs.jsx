import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView, Button } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import BottomNav from '../components/BottomNav'

const statusLabels = {
  pending_ship: '待发送', shipping: '发送中', inspecting: '质检中',
  quoted: '待回复', pending_payment: '待付款', pending_cancel: '待取消',
  repairing: '维修中', return_pending: '待发回', returned: '已发回',
  closed: '已关闭', appealing: '申诉中',
}

export default function UserRepairs() {
  const navigate = useNavigate()
  const [requests, setRequests] = useState([])
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    apiFetch(`${baseUrl}/repair-requests`).then(r => r.json()).then(r => {
      if (r.code === 20000) setRequests(r.data?.list || [])
    }).catch(() => {}).finally(() => setLoading(false))
  }, [])

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center">我的报修</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        <View className="mt-4 space-y-3">
          {loading ? (
            <Text className="text-center text-zinc-400 py-8">加载中...</Text>
          ) : requests.length === 0 ? (
            <Text className="text-center text-zinc-400 py-8">暂无报修记录</Text>
          ) : requests.map(r => (
            <View key={r.id} className="bg-white rounded-2xl shadow-sm p-4 active:opacity-80"
              onClick={() => navigate(`/repair-request?request_id=${r.id}`)}>
              <View className="flex items-center justify-between mb-1">
                <Text className="text-sm font-bold text-black">{r.sn || '#' + r.id?.slice(0, 8)}</Text>
                <Text className="text-xs px-2 py-1 rounded-full font-bold bg-zinc-100 text-zinc-600">
                  {statusLabels[r.status] || r.status}
                </Text>
              </View>
              <Text className="text-xs text-zinc-400">{r.created_at ? new Date(r.created_at).toLocaleDateString() : ''}</Text>
              {r.quote_amount && <Text className="text-xs text-zinc-500 mt-1">报价: ¥{r.quote_amount}</Text>}
            </View>
          ))}
        </View>
        <Button onClick={() => navigate('/create-repair')}
          className="fixed bottom-20 right-4 w-14 h-14 bg-black text-white rounded-full text-2xl font-bold shadow-lg flex items-center justify-center">
          +
        </Button>
      </ScrollView>

      <BottomNav
        active="service"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => navigate('/') },
          { key: 'rent', icon: '🪕', label: '租赁', onClick: () => navigate('/my-leases') },
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => navigate('/my-repairs') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => navigate('/profile') },
        ]}
      />
    </View>
  )
}
