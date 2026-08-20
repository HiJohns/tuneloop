import { useLocation, useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { CheckCircle } from 'lucide-react'
import { View, Text, Button } from '@tarojs/components'
import { dialog, env, toWeappRoute } from '../platform'

export default function PaymentComplete() {
  const location = useLocation()
  const navigate = useNavigate()
  const state = location.state || {}

  const nav = (to) => {
    if (!env.isMiniProgram) return navigate(to)
    const route = toWeappRoute(to)
    if (!route) { dialog.alert('该功能请在 H5 端使用'); return }
    if (route.type === 'switchTab') return Taro.switchTab({ url: route.url })
    return Taro.navigateTo({ url: route.url })
  }

  if (!state.paymentAmount && state.paymentAmount !== 0) {
    return (
      <View className="min-h-screen bg-brand-bg flex flex-col items-center justify-center p-4">
        <Text className="text-gray-500 mb-4">无效访问</Text>
        <Button onClick={() => nav('/')} className="text-brand-primary">返回首页</Button>
      </View>
    )
  }

  return (
    <View className="min-h-screen bg-brand-bg flex flex-col items-center justify-center p-4">
      <CheckCircle size={64} className="text-green-500 mb-4" />
      <Text className="text-xl font-bold mb-6">支付完成</Text>
      <View className="bg-white rounded-xl p-6 w-full max-w-sm shadow-sm">
        <View className="space-y-3 text-sm">
          <View className="flex justify-between">
            <Text className="text-gray-500">支付金额</Text>
            <Text className="font-medium text-red-500">¥{(Number(state.paymentAmount || 0) / 100).toFixed(2)}</Text>
          </View>
          <View className="flex justify-between">
            <Text className="text-gray-500">定损金额</Text>
            <Text>¥{(Number(state.damageAmount || 0) / 100).toFixed(2)}</Text>
          </View>
          <View className="flex justify-between">
            <Text className="text-gray-500">押金抵扣</Text>
            <Text>¥{(Number(state.deposit || 0) / 100).toFixed(2)}</Text>
          </View>
          <View className="border-t" />
          <View className="flex justify-between">
            <Text className="text-gray-500">商户</Text>
            <Text>{state.merchantName}</Text>
          </View>
          <View className="flex justify-between">
            <Text className="text-gray-500">订单号</Text>
            <Text className="font-mono">#{state.orderId?.slice(0, 8)}</Text>
          </View>
        </View>
      </View>
      <Button
        onClick={() => nav('/profile')}
        className="mt-6 w-full max-w-sm py-2.5 bg-brand-primary text-white rounded-lg"
      >
        返回我的订单
      </Button>
    </View>
  )
}
