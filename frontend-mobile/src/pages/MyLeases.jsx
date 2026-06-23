import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Button, ScrollView } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { ArrowLeft, Package } from 'lucide-react'

const MAIN_TABS = [
  { key: 'active', label: '进行中' },
  { key: 'completed', label: '已完成' },
]

const SUB_FILTERS = {
  active: [
    { key: '', label: '全部' },
    { key: 'reserved', label: '未支付' },
    { key: 'paid', label: '待发货' },
    { key: 'shipped', label: '已发货' },
    { key: 'in_lease', label: '租赁中' },
    { key: 'expired', label: '已超期' },
    { key: 'returning', label: '归还中' },
  ],
  completed: [
    { key: 'returned', label: '已归还' },
    { key: 'completed', label: '已完成' },
    { key: 'cancelled', label: '已取消' },
  ],
}

const STATUS_LABELS = {
  reserved: '未支付', paid: '待发货', pending_shipment: '待发货',
  shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '超期',
}

const STATUS_COLORS = {
  reserved: 'bg-blue-100 text-blue-700',
  paid: 'bg-orange-100 text-orange-700', pending_shipment: 'bg-orange-100 text-orange-700',
  shipped: 'bg-green-100 text-green-700',
  in_lease: 'bg-indigo-100 text-indigo-700', returning: 'bg-yellow-100 text-yellow-700',
  returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600', cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700',
}

const MAIN_INCLUDE = {
  active: ['reserved', 'paid', 'pending_shipment', 'shipped', 'in_lease', 'expired', 'returning'],
  completed: ['returned', 'completed', 'cancelled'],
}

export default function MyLeases() {
  const navigate = useNavigate()
  const [mainTab, setMainTab] = useState('active')
  const [subFilter, setSubFilter] = useState('')
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)

  const baseUrl = env.apiBaseUrl

  useEffect(() => {
    const fetchOrders = async () => {
      setLoading(true)
      try {
        const statusKey = subFilter || ''
        let url = `${baseUrl}/orders`
        if (statusKey) url += `?status=${statusKey}`
        const resp = await apiFetch(url)
        const result = await resp.json()
        let list = []
        if (result.code === 20000) {
          list = result.data?.list || []
        }
        if (!subFilter) {
          list = list.filter(o => MAIN_INCLUDE[mainTab]?.includes(o.status))
        }
        setOrders(list)
      } catch (err) {
        console.error('Failed to fetch orders:', err)
      }
      setLoading(false)
    }
    fetchOrders()
  }, [baseUrl, mainTab, subFilter])

  return (
    <View className="min-h-screen bg-[#FDFBF7] pb-20">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-4">
        <View className="flex items-center gap-3">
          <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
          <Text className="text-lg font-black text-black">我的租约</Text>
        </View>
      </View>

      {/* Main Tabs */}
      <View className="px-4 pt-3 pb-1 flex gap-2">
        {MAIN_TABS.map(tab => (
          <Button
            key={tab.key}
            onClick={() => { setMainTab(tab.key); setSubFilter('') }}
            className={`px-5 py-2 rounded-full text-sm font-black ${
              mainTab === tab.key ? 'bg-black text-white' : 'bg-white text-zinc-500'
            }`}
          >
            {tab.label}
          </Button>
        ))}
      </View>

      {/* Sub Filters */}
      <ScrollView scrollX className="px-4 py-2" enhanced showScrollbar={false}>
        <View className="flex gap-2 whitespace-nowrap">
          {SUB_FILTERS[mainTab].map(f => (
            <Button
              key={f.key}
              onClick={() => setSubFilter(f.key)}
              className={`px-3 py-1 rounded-full text-xs font-bold ${
                subFilter === f.key ? 'bg-black text-white' : 'bg-white text-zinc-400'
              }`}
            >
              {f.label}
            </Button>
          ))}
        </View>
      </ScrollView>

      <View className="p-4">
        {loading ? (
          <View className="text-center py-16 text-zinc-400 font-medium">加载中...</View>
        ) : orders.length === 0 ? (
          <View className="text-center py-16">
            <Package size={48} className="mx-auto text-zinc-200 mb-4" />
            <Text className="text-zinc-400 font-medium">暂无租约</Text>
          </View>
        ) : (
          <View className="space-y-3">
            {orders.map(order => (
              <View
                key={order.id}
                className="bg-white rounded-2xl shadow-sm p-4 active:opacity-80"
                onClick={() => navigate(`/order/${order.id}`)}
              >
                <View className="flex items-center justify-between mb-2">
                  <Text className="text-sm font-black text-black flex-1 min-w-0 truncate">
                    订单 #{order.id?.slice(0, 8)}
                  </Text>
                  <Text className={`text-xs px-2 py-1 rounded-full font-bold flex-shrink-0 ml-2 ${STATUS_COLORS[order.status] || 'bg-gray-100 text-gray-600'}`}>
                    {STATUS_LABELS[order.status] || order.status}
                  </Text>
                </View>
                <View className="space-y-1 text-sm">
                  <View className="flex items-center gap-2">
                    <Text className="text-zinc-400 font-medium">月租:</Text>
                    <Text className="text-black font-black">¥{order.monthly_rent}</Text>
                    <Text className="text-zinc-400 font-medium ml-4">押金:</Text>
                    <Text className="text-black font-black">¥{order.deposit}</Text>
                  </View>
                  {order.start_date && (
                    <Text className="text-zinc-400 font-medium">
                      起: <Text className="text-black">{formatDisplayDate(order.start_date)}</Text>
                    </Text>
                  )}
                  {order.end_date && (
                    <Text className="text-zinc-400 font-medium">
                      止: <Text className="text-black">{formatDisplayDate(order.end_date)}</Text>
                    </Text>
                  )}
                </View>
                {order.status === 'in_lease' && (
                  <Button
                    onClick={(e) => {
                      e.stopPropagation()
                      navigate(`/return/${order.id}?instrument=${order.instrument_id}`)
                    }}
                    className="mt-3 w-full py-2.5 bg-black text-white rounded-xl font-black text-sm"
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
