import { useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text } from '@tarojs/components'
import { storage, eventBus } from '../platform'

export default function Success() {
  const navigate = useNavigate()

  useEffect(() => {
    storage.removeItem('cart')
    eventBus.emit('cartUpdated')
  }, [])

  return (
    <View className="container h-screen w-screen bg-zinc-50 overflow-hidden flex flex-col relative antialiased">
      <View className="flex-1 flex flex-col items-center justify-center px-8">
        <View className="w-32 h-32 rounded-full bg-green-100 flex items-center justify-center mb-6">
          <Text className="text-5xl">🎉</Text>
        </View>
        <Text className="text-2xl font-black text-black tracking-wide mb-2">付款完成！</Text>
        <Text className="text-base text-zinc-400 font-medium mb-8">感谢您的租赁，祝您使用愉快</Text>
        <Text
          className="text-blue-600 font-bold text-sm border-b border-blue-600 pb-0.5"
          onClick={() => navigate('/')}
        >
          返回首页
        </Text>
      </View>
    </View>
  )
}
