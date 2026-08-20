import { useLocation, useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Button } from '@tarojs/components'
import { dialog, env, toWeappRoute } from '../platform'

export default function RepairPaymentComplete() {
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

  return (
    <View className="flex flex-col h-screen bg-[#FDFBF7]">
      <View className="flex-1 flex flex-col items-center justify-center px-8">
        <View className="w-24 h-24 rounded-full bg-green-100 flex items-center justify-center mb-6">
          <Text className="text-5xl">🎉</Text>
        </View>
        <Text className="text-2xl font-black text-black tracking-wide mb-2">支付完成！</Text>
        {state.amount != null && (
          <Text className="text-lg text-zinc-500 mb-8">支付金额：<Text className="text-red-500 font-bold">¥{((state.amount || 0) / 100).toFixed(2)}</Text></Text>
        )}
        <View className="w-full max-w-sm space-y-3 px-4">
          <Button onClick={() => nav(`/repair-request?request_id=${state.requestId}`)}
            className="w-full py-3 bg-black text-white rounded-xl font-bold text-sm text-center">
            查看报修单
          </Button>
          <Button onClick={() => nav('/my-repairs')}
            className="w-full py-3 border border-zinc-300 rounded-xl font-bold text-sm text-zinc-600 text-center">
            返回维修列表
          </Button>
        </View>
      </View>
    </View>
  )
}
