import { useLocation, useNavigate } from 'react-router-dom'
import { CheckCircle } from 'lucide-react'
import { View, Text, Button } from '@tarojs/components'

export default function PaymentComplete() {
  const location = useLocation()
  const navigate = useNavigate()
  const state = location.state || {}

  if (!state.paymentAmount && state.paymentAmount !== 0) {
    return (
      <View className="min-h-screen bg-brand-bg flex flex-col items-center justify-center p-4">
        <Text className="text-gray-500 mb-4">无效访问</Text>
        <Button onClick={() => navigate('/')} className="text-brand-primary">返回首页</Button>
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
            <Text className="font-medium text-red-500">¥{state.paymentAmount.toFixed(2)}</Text>
          </View>
          <View className="flex justify-between">
            <Text className="text-gray-500">定损金额</Text>
            <Text>¥{state.damageAmount?.toFixed(2)}</Text>
          </View>
          <View className="flex justify-between">
            <Text className="text-gray-500">押金抵扣</Text>
            <Text>¥{state.deposit?.toFixed(2)}</Text>
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
        onClick={() => navigate('/profile')}
        className="mt-6 w-full max-w-sm py-2.5 bg-brand-primary text-white rounded-lg"
      >
        返回我的订单
      </Button>
    </View>
  )
}
