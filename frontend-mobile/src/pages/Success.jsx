import { useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { View, Text, Button } from '@tarojs/components'
import { CheckCircle, Calendar, Package, Hash, MapPin, User } from 'lucide-react'
import { formatDisplayDate } from '../utils/format'

export default function Success() {
  const navigate = useNavigate()
  const location = useLocation()
  const orderData = location.state || {}
  const isBatch = Array.isArray(orderData.orders)

  useEffect(() => {
    localStorage.removeItem('cart')
    window.dispatchEvent(new Event('cartUpdated'))
  }, [])

  const handleDone = () => {
    navigate('/')
  }

  if (isBatch) {
    return (
      <View className="min-h-screen bg-green-50 flex flex-col p-4">
        <View className="flex-1 flex flex-col justify-center">
          <View className="text-center mb-8">
            <CheckCircle className="text-green-500 mx-auto" size={80} />
          </View>
          
          <Text className="text-2xl font-bold text-center text-gray-800 mb-2">租赁成功</Text>
          <Text className="text-gray-500 text-center mb-8">您的订单已创建成功</Text>

          <View className="bg-white rounded-xl p-4 shadow-sm space-y-3">
            {orderData.orders.map((order, i) => (
              <View key={i} className="pb-3 border-b border-gray-100 last:border-b-0">
                <View className="flex justify-between items-center">
                  <Text className="text-sm text-gray-500">订单 #{i + 1}</Text>
                  <Text className="text-sm font-medium">{order.order_id?.slice(0, 8)}</Text>
                </View>
                <View className="flex justify-between items-center mt-1">
                  <Text className="text-sm text-gray-500">金额</Text>
                  <Text className="text-sm font-bold text-orange-500">¥{order.amount?.toFixed(0) || 0}</Text>
                </View>
              </View>
            ))}
            <View className="flex justify-between items-center pt-2">
              <Text className="font-medium">合计</Text>
              <Text className="text-xl font-bold text-orange-500">¥{orderData.total_amount?.toFixed(0) || 0}</Text>
            </View>
          </View>
        </View>

        <Button onClick={handleDone} className="w-full py-4 bg-brand-primary text-white rounded-xl font-bold text-lg">
          完成
        </Button>
      </View>
    )
  }

  return (
    <View className="min-h-screen bg-green-50 flex flex-col p-4">
      <View className="flex-1 flex flex-col justify-center">
        <View className="text-center mb-8">
          <CheckCircle className="text-green-500 mx-auto" size={80} />
        </View>
        
        <Text className="text-2xl font-bold text-center text-gray-800 mb-2">租赁成功</Text>
        <Text className="text-gray-500 text-center mb-8">您的订单已创建成功</Text>

        <View className="bg-white rounded-xl p-4 shadow-sm">
          <View className="space-y-3 text-sm">
            <View className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Hash size={18} className="text-gray-400" />
              <View>
                <Text className="text-gray-500 text-xs">订单号</Text>
                <Text className="font-medium">{orderData.order_id || 'TL' + Date.now()}</Text>
              </View>
            </View>
            
            <View className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Package size={18} className="text-gray-400" />
              <View>
                <Text className="text-gray-500 text-xs">乐器</Text>
                <Text className="font-medium">{orderData.category_name || '-'}</Text>
                <Text className="text-xs text-gray-400">{orderData.instrument_sn || '-'}</Text>
              </View>
            </View>

            <View className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <User size={18} className="text-gray-400" />
              <View>
                <Text className="text-gray-500 text-xs">商户</Text>
                <Text className="font-medium">{orderData.tenant_name || '-'}</Text>
              </View>
            </View>
            
            <View className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <MapPin size={18} className="text-gray-400" />
              <View>
                <Text className="text-gray-500 text-xs">取琴网点</Text>
                <Text className="font-medium">{orderData.site_name || '-'}</Text>
                <Text className="text-xs text-gray-400">{orderData.site_address || ''}</Text>
              </View>
            </View>
            
            <View className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Calendar size={18} className="text-gray-400" />
              <View>
                <Text className="text-gray-500 text-xs">租赁期间</Text>
                <Text className="font-medium">{orderData.lease_term || '-'}</Text>
                <Text className="text-xs text-gray-400">预期归还: {formatDisplayDate(orderData.return_date) || '待确定'}</Text>
              </View>
            </View>
            
            <View className="flex items-center gap-3 pt-2">
              <View className="text-gray-400 text-xl">¥</View>
              <View>
                <Text className="text-gray-500 text-xs">支付金额</Text>
                <Text className="text-xl font-bold text-orange-500">¥{orderData.total_amount?.toFixed(0) || 0}</Text>
              </View>
            </View>
          </View>
        </View>
      </View>

      <Button
        onClick={handleDone}
        className="w-full py-4 bg-brand-primary text-white rounded-xl font-bold text-lg"
      >
        完成
      </Button>
    </View>
  )
}