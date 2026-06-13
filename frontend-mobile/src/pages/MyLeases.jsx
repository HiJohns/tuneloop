import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { ArrowLeft, Package } from 'lucide-react'

export default function MyLeases() {
  const navigate = useNavigate()
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchOrders = async () => {
      try {
        const token = getToken()
        const baseUrl = env.apiBaseUrl
        const resp = await apiFetch(`${baseUrl}/orders`)
        const result = await resp.json()
        if (result.code === 20000) {
          setOrders(result.data?.list || [])
        }
      } catch (err) {
        console.error('Failed to fetch orders:', err)
      }
      setLoading(false)
    }
    fetchOrders()
  }, [])

  const statusLabel = {
    pending: '待付款',
    paid: '待发货',
    shipped: '已发货',
    in_lease: '租赁中',
    returning: '归还中',
    returned: '已归还',
    completed: '已完成',
    cancelled: '已取消',
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </Button>
        <Text className="text-lg font-bold">我的租约</Text>
      </View>

      <View className="p-4">
        {loading ? (
          <View className="text-center py-8 text-gray-500">加载中...</View>
        ) : orders.length === 0 ? (
          <View className="text-center py-16">
            <Package size={48} className="mx-auto text-gray-300 mb-4" />
            <Text className="text-gray-500">暂无租约</Text>
          </View>
        ) : (
          <View className="space-y-3">
            {orders.map(order => (
              <View
                key={order.id}
                className="bg-white rounded-xl p-4 shadow-sm"
                onClick={() => navigate(`/instrument/${order.instrument_id}`)}
              >
                <View className="flex justify-between items-start mb-2">
                  <Text className="font-medium">订单 #{order.id?.slice(0, 8)}</Text>
                  <Text className={`text-xs px-2 py-1 rounded-full ${
                    order.status === 'in_lease' ? 'bg-green-100 text-green-700' :
                    order.status === 'pending' ? 'bg-yellow-100 text-yellow-700' :
                    'bg-gray-100 text-gray-600'
                  }`}>
                    {statusLabel[order.status] || order.status}
                  </Text>
                </View>
                <View className="text-sm text-gray-500 space-y-1">
                  <Text>月租: ¥{order.monthly_rent}</Text>
                  <Text>押金: ¥{order.deposit}</Text>
                  {order.start_date && <Text>起: {formatDisplayDate(order.start_date)}</Text>}
                  {order.end_date && <Text>止: {formatDisplayDate(order.end_date)}</Text>}
                </View>
                {order.status === 'in_lease' && (
                  <Button
                    onClick={(e) => {
                      e.stopPropagation()
                      navigate(`/return/${order.id}?instrument=${order.instrument_id}`)
                    }}
                    className="mt-3 bg-brand-primary text-white py-2 px-4 rounded-lg text-sm"
                  >
                    归还乐器
                  </Button>
                )}
              </View>
            ))}
          </View>
        )}
      </View>
    </View>
  )
}
