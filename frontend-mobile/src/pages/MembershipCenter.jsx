import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, ScrollView } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'

export default function MembershipCenter() {
  const navigate = useNavigate()
  const [user, setUser] = useState(null)
  const [loading, setLoading] = useState(true)
  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchUser = async () => {
      try {
        const resp = await apiFetch(`${baseUrl}/users/me`)
        const result = await resp.json()
        if (result.code === 20000) setUser(result.data)
      } catch {}
      setLoading(false)
    }
    fetchUser()
  }, [baseUrl])

  return (
    <ScrollView className="h-screen w-screen bg-zinc-50">
      {/* Navigation bar */}
      <View className="flex items-center px-4 py-3 bg-white border-b border-zinc-100">
        <Text className="text-lg mr-2" onClick={() => navigate(-1)}>{'<'}</Text>
        <Text className="text-lg font-bold flex-1 text-center mr-4">会员中心</Text>
      </View>

      {/* Membership level card */}
      <View className="mx-4 mt-4 bg-white rounded-2xl shadow-sm p-6">
        <View className="items-center">
          <Text className="text-2xl font-bold text-amber-700">
            {user?.membership_level_name || '普通会员'}
          </Text>
        </View>
      </View>

      {/* Stats cards */}
      <View className="mx-4 mt-4">
        <View className="bg-white rounded-2xl shadow-sm p-4">
          <View className="flex justify-between items-center py-3 border-b border-zinc-50">
            <Text className="text-sm text-zinc-500">消费总额</Text>
            <Text className="text-base font-bold text-zinc-800">
              ¥{user?.total_spending ? Number(user.total_spending).toLocaleString() : '0'}
            </Text>
          </View>
          <View className="flex justify-between items-center py-3 border-b border-zinc-50">
            <Text className="text-sm text-zinc-500">预付点数</Text>
            <Text className="text-base font-bold text-zinc-800">
              {user?.prepaid_points ? Number(user.prepaid_points).toLocaleString() : '0'} 点
            </Text>
          </View>
          <View className="flex justify-between items-center py-3">
            <Text className="text-sm text-zinc-500">赠点数</Text>
            <Text className="text-base font-bold text-zinc-800">
              {user?.promo_points ? Number(user.promo_points).toLocaleString() : '0'} 点
            </Text>
          </View>
        </View>
      </View>

      {loading && (
        <View className="flex-1 items-center justify-center mt-20">
          <Text className="text-zinc-400">加载中...</Text>
        </View>
      )}
    </ScrollView>
  )
}
