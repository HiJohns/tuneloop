import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView, Button, Input } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import BottomNav from '../components/BottomNav'

export default function MyRepairs() {
  const navigate = useNavigate()
  const [snInput, setSnInput] = useState('')
  const [myRepairs, setMyRepairs] = useState([])
  const [pendingRepairs, setPendingRepairs] = useState([])
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  const fetchRepairs = async () => {
    setLoading(true)
    try {
      const [mineRes, pendingRes] = await Promise.all([
        apiFetch(`${baseUrl}/repair/mine`),
        apiFetch(`${baseUrl}/repair/pending`),
      ])
      const mine = await mineRes.json()
      const pending = await pendingRes.json()
      if (mine.code === 20000) setMyRepairs(mine.data?.list || [])
      if (pending.code === 20000) setPendingRepairs(pending.data?.list || [])
    } catch {}
    setLoading(false)
  }

  useEffect(() => { fetchRepairs() }, [])

  const handleSearch = () => {
    if (!snInput.trim()) return
    navigate(`/staff/repair-scan?sn=${snInput.trim()}`)
  }

  return (
    <View className="flex flex-col h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg font-black text-black">维修</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        {/* Scan / SN search */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black mb-2">扫码查找乐器</Text>
          <View className="flex gap-2">
            <input
              className="flex-1 border border-zinc-300 rounded-lg px-3 py-2 text-sm"
              value={snInput} onChange={e => setSnInput(e.target.value)}
              placeholder="输入乐器编号或扫码" />
            <Button onClick={handleSearch} className="px-4 py-2 bg-black text-white rounded-lg text-sm font-bold">查找</Button>
          </View>
        </View>

        {/* My in-progress repairs */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4">
          <Text className="text-sm font-bold text-black mb-3">我的维修 ({myRepairs.length})</Text>
          {loading ? (
            <Text className="text-xs text-zinc-400">加载中...</Text>
          ) : myRepairs.length === 0 ? (
            <Text className="text-xs text-zinc-400">暂无进行中的维修</Text>
          ) : (
            <View className="space-y-2">
              {myRepairs.map(inst => (
                <View key={inst.id} className="border border-zinc-100 rounded-xl p-3 active:opacity-80"
                  onClick={() => navigate(`/repair?instrument_id=${inst.id}`)}>
                  <Text className="text-sm font-bold text-black">{inst.sn || '未知SN'}</Text>
                  <Text className="text-xs text-zinc-400">{inst.category_name || ''}</Text>
                  <Text className="text-xs text-zinc-400 mt-1">
                    状态: {inst.repair_status === 'repair_in_progress' ? '维修中' : inst.repair_status}
                  </Text>
                </View>
              ))}
            </View>
          )}
        </View>

        {/* Pending repairs (available for takeover) */}
        <View className="bg-white rounded-2xl shadow-sm p-4 mt-4 mb-4">
          <Text className="text-sm font-bold text-black mb-3">待维修乐器 ({pendingRepairs.length})</Text>
          {pendingRepairs.length === 0 ? (
            <Text className="text-xs text-zinc-400">暂无待维修乐器</Text>
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
