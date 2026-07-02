import { useState, useEffect } from 'react'
import { View, Text, ScrollView } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'

export default function UserWarnings() {
  const [warnings, setWarnings] = useState([])
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  const token = getToken()
  const isStaff = (() => {
    if (!token) return false
    try { const p = JSON.parse(atob(token.split('.')[1])); return p?.role && p.role !== 'USER' } catch { return false }
  })()

  useEffect(() => {
    if (!isStaff) { setLoading(false); return }
    apiFetch(`${baseUrl}/warnings?status=open`).then(r => r.json()).then(r => {
      if (r.code === 20000) setWarnings(r.data?.list || [])
    }).catch(() => {}).finally(() => setLoading(false))
  }, [])

  if (!isStaff) return (
    <View className="h-screen flex items-center justify-center">
      <Text className="text-zinc-400">员工/管理员可查看警告</Text>
    </View>
  )

  return (
    <View className="h-screen bg-zinc-50">
      <View className="bg-white px-4 py-3 border-b border-zinc-100">
        <Text className="text-lg font-bold">警告（{warnings.length}）</Text>
      </View>
      <ScrollView scrollY className="flex-1 px-4 min-h-0">
        {loading ? (
          <Text className="text-center py-8 text-zinc-400">加载中...</Text>
        ) : warnings.length === 0 ? (
          <Text className="text-center py-8 text-zinc-400">暂无警告</Text>
        ) : warnings.map(w => (
          <View key={w.id} className="bg-white rounded-2xl shadow-sm p-4 mt-4">
            <View className="flex items-center justify-between mb-1">
              <Text className={`text-xs px-2 py-0.5 rounded-full font-bold ${w.level === 'high' ? 'bg-red-100 text-red-700' : w.level === 'medium' ? 'bg-yellow-100 text-yellow-700' : 'bg-blue-100 text-blue-700'}`}>
                {w.level}
              </Text>
              <Text className="text-xs text-zinc-400">{w.created_at ? new Date(w.created_at).toLocaleString() : ''}</Text>
            </View>
            <Text className="text-sm font-bold text-black">{w.reason}</Text>
            {w.description && <Text className="text-xs text-zinc-500 mt-1">{w.description}</Text>}
          </View>
        ))}
      </ScrollView>
    </View>
  )
}
