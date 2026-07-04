import { useState, useEffect } from 'react'
import { View, Text, Button, ScrollView } from '@tarojs/components'
import { api } from '../services/api'
import { navigation } from '../platform'

export default function PointsComplete() {
  const [balance, setBalance] = useState(null)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    loadBalance()
  }, [])

  const loadBalance = async () => {
    try {
      const resp = await api.get('/user/points/balance')
      if (resp.code === 20000) setBalance(resp.data)
    } catch { /* silent */ }
    setLoading(false)
  }

  return (
    <ScrollView scrollY className="h-screen bg-gradient-to-b from-blue-50 to-white">
      <View className="px-5 pt-12 pb-8 flex flex-col items-center">
        <View className="w-16 h-16 rounded-full bg-green-500 flex items-center justify-center mb-4">
          <Text className="text-white text-3xl">✓</Text>
        </View>
        <View className="mb-2"><Text className="text-2xl font-bold text-black">感谢购买</Text></View>
        <View className="mb-8"><Text className="text-gray-500 text-sm">点数已成功充值到您的账户</Text></View>

        <View className="w-full bg-white rounded-2xl shadow-sm p-6 mb-8 text-center">
          <View className="mb-1"><Text className="text-gray-400 text-xs">当前预付点数</Text></View>
          <View><Text className="text-3xl font-bold text-blue-500">{loading ? '...' : balance?.prepaid_points ?? 0}</Text></View>
        </View>

        <Button className="w-full bg-blue-500 text-white py-4 rounded-xl text-lg font-medium"
          onClick={() => navigation.redirect('/')}>进入首页</Button>
      </View>
    </ScrollView>
  )
}
