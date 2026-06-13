import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { api, getToken, redirectToLogin } from '../services/api'
import { Badge, Tag } from 'antd'
import { ArrowLeft, Phone, Calendar } from 'lucide-react'

function ServiceCard({ order }) {
  return (
    <View className="bg-white rounded-xl shadow-sm p-4">
      <View className="space-y-2">
        <View className="flex items-center justify-between">
          <Text className="font-medium text-brand-text">{order.assetName}</Text>
          <Tag color={order.status === "处理中" ? "blue" : "orange"}>
            {order.status}
          </Tag>
        </View>
        
        <Text className="text-gray-600 text-sm">故障: {order.fault}</Text>
        
        {order.status === "待派单" && (
          <Text className="text-gray-500 text-sm">备注: {order.site}</Text>
        )}
        
        {order.status === "处理中" && (
          <View className="flex items-center gap-2">
            <Text className="text-sm">服务人员: {order.technician}</Text>
            <a href={`tel:${order.technicianPhone}`} className="text-brand-primary text-sm">
              📞 {order.technicianPhone}
            </a>
          </View>
        )}
        
        <Text className="text-gray-400 text-xs">创建时间: {order.createdAt}</Text>
      </View>
    </View>
  )
}

export default function MyService() {
  const navigate = useNavigate()
  const [myServiceOrders, setMyServiceOrders] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchServiceOrders = async () => {
      try {
        setLoading(true)
        const data = await api.get('/user/service-orders')
        setMyServiceOrders(data || [])
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch service orders:', error)
        setLoading(false)
      }
    }
    
    fetchServiceOrders()
  }, [])
  
  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      {/* Header */}
      <View className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </Button>
        <Text className="text-lg font-bold">我的维修</Text>
      </View>
      
      {/* Service Orders List */}
      <View className="p-4 space-y-4">
        {loading ? (
          <View className="text-center py-8 text-gray-500">加载中...</View>
        ) : (
          myServiceOrders.map(order => (
            <ServiceCard key={order.id} order={order} />
          ))
        )}
      </View>
      
      {/* Bottom Navigation */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <View className="flex justify-around py-3 max-w-[480px] mx-auto">
          <View 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/')}
          >
            <Text className="text-xl">🏠</Text>
            <Text className="text-xs mt-1">首页</Text>
          </View>
          <View 
            className="flex flex-col items-center text-brand-primary cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <Text className="text-xl">🔧</Text>
            <Text className="text-xs mt-1">维修</Text>
          </View>
          {getToken() ? (
            <View 
              className="flex flex-col items-center text-gray-400 cursor-pointer"
              onClick={() => navigate('/profile')}
            >
              <Text className="text-xl">👤</Text>
              <Text className="text-xs mt-1">我的</Text>
            </View>
          ) : (
            <View 
              className="flex flex-col items-center text-gray-400 cursor-pointer"
              onClick={() => redirectToLogin()}
            >
              <Text className="text-xl">👤</Text>
              <Text className="text-xs mt-1">登录</Text>
            </View>
          )}
        </View>
      </View>
    </View>
  )
}
