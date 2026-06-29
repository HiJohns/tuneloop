import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Button, ScrollView, Image } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { Package } from 'lucide-react'
import BottomNav from '../components/BottomNav'

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
    { key: 'completed', label: '已完成' },
    { key: 'cancelled', label: '已取消' },
  ],
}

const STATUS_LABELS = {
  reserved: '未支付', paid: '待发货', pending_shipment: '待发货',
  shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '超期', transferred: '已过户',
}

const STATUS_COLORS = {
  reserved: 'bg-blue-100 text-blue-700',
  paid: 'bg-orange-100 text-orange-700', pending_shipment: 'bg-orange-100 text-orange-700',
  shipped: 'bg-green-100 text-green-700',
  in_lease: 'bg-indigo-100 text-indigo-700', returning: 'bg-yellow-100 text-yellow-700',
  returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600', cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700', transferred: 'bg-purple-100 text-purple-700',
}

const getActualRent = (order) => {
  if (!order.pricing_breakdown) return order.monthly_rent
  try {
    const pb = typeof order.pricing_breakdown === 'string'
      ? JSON.parse(order.pricing_breakdown)
      : order.pricing_breakdown
    return pb?.actual_rent_amount || order.monthly_rent
  } catch { return order.monthly_rent }
}

const isScheduledPeriod = (status) =>
  ['completed', 'returned', 'returning', 'cancelled'].includes(status)

const MAIN_INCLUDE = {
  active: ['reserved', 'paid', 'pending_shipment', 'shipped', 'in_lease', 'expired', 'returning'],
  completed: ['returned', 'completed', 'cancelled', 'transferred'],
}

export default function MyLeases() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const initStatus = searchParams.get('status') || ''
  const initTab = initStatus && ['returned', 'completed', 'cancelled'].includes(initStatus) ? 'completed' : 'active'
  const [mainTab, setMainTab] = useState(initTab)
  const [subFilter, setSubFilter] = useState(initStatus)
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1)
  const [loadingMore, setLoadingMore] = useState(false)
  const [hasMore, setHasMore] = useState(true)

  const baseUrl = env.apiBaseUrl

  const token = getToken()
  const isStaff = (() => {
    try {
      if (!token) return false
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload?.role && payload.role !== 'USER'
    } catch { return false }
  })()

  // Reset pagination when tab or filter changes
  useEffect(() => {
    setPage(1)
    setOrders([])
    setHasMore(true)
  }, [baseUrl, mainTab, subFilter])

  // Fetch orders with pagination
  useEffect(() => {
    const fetchOrders = async () => {
      if (page === 1) setLoading(true)
      else setLoadingMore(true)
      try {
        const statusKey = subFilter || ''
        let url = `${baseUrl}/orders?page=${page}&page_size=10`
        if (statusKey) url += `&status=${statusKey}`
        const resp = await apiFetch(url)
        const result = await resp.json()
        let list = []
        if (result.code === 20000) {
          list = result.data?.list || []
        }
        if (!subFilter) {
          list = list.filter(o => MAIN_INCLUDE[mainTab]?.includes(o.status))
        }
        setOrders(prev => page === 1 ? list : [...prev, ...list])
        setHasMore((result.data?.total || 0) > (page * 10))
      } catch (err) {
        console.error('Failed to fetch orders:', err)
      }
      setLoading(false)
      setLoadingMore(false)
    }
    fetchOrders()
  }, [page, baseUrl, mainTab, subFilter])

  return (
    <View className="flex flex-col h-screen bg-[#FDFBF7]">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-4">
        <Text className="text-lg font-black text-black">我的租约</Text>
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

      <ScrollView scrollY className="flex-1 px-4"
        onScrollToLower={() => {
          if (!loadingMore && hasMore) {
            setLoadingMore(true)
            setPage(prev => prev + 1)
          }
        }}
        lowerThreshold={50}
        enableBackToTop
      >
        {loading ? (
          <View className="text-center py-16 text-zinc-400 font-medium">加载中...</View>
        ) : orders.length === 0 ? (
          <View className="text-center py-16">
            <Package size={48} className="mx-auto text-zinc-200 mb-4" />
            <Text className="text-zinc-400 font-medium">暂无租约</Text>
          </View>
        ) : (
          <>
          <View className="space-y-3">
              {orders.map(order => {
              const showReturn = order.status === 'in_lease'
              const showPay = order.status === 'reserved'
              const showCancel = ['paid', 'pending_shipment'].includes(order.status)
              const showConfirm = order.status === 'shipped'
              const isTerminal = ['completed', 'returned', 'cancelled'].includes(order.status)

              return (
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
                <View className="flex-row">
                  <View className="flex-1">
                <View className="space-y-1 text-sm">
                  {order.instrument_name && (
                    <View><Text className="text-zinc-400 font-medium">
                      乐器: <Text className="text-black font-medium">{order.instrument_name}</Text>
                      {order.instrument_category && <Text className="text-zinc-300 ml-1">({order.instrument_category})</Text>}
                    </Text></View>
                  )}
                  {order.created_at && (
                    <View><Text className="text-zinc-400 font-medium">
                      下单日: <Text className="text-black font-medium">{formatDisplayDate(order.created_at)}</Text>
                    </Text></View>
                  )}
                  <View className="flex items-center gap-2">
                    <Text className="text-zinc-400 font-medium">总金额:</Text>
                    <Text className="text-black font-black">¥{(order.monthly_rent || 0) + (order.deposit || 0) + (order.shipping_fee || 0)}</Text>
                  </View>
                </View>
                  </View>
                  {order.cover_image && <Image src={order.cover_image} className="w-20 h-20 rounded-lg ml-3 self-start" mode="aspectFill" />}
                </View>
                <View className="mt-3 flex gap-2">
                  {!isTerminal && (
                    <>
                      {showPay && (
                        <Button
                          onClick={(e) => { e.stopPropagation(); navigate(`/order/${order.id}`) }}
                          className="flex-1 py-2.5 bg-black text-white rounded-xl font-black text-sm"
                        >
                          立即支付
                        </Button>
                      )}
                      {showConfirm && (
                        <Button
                          onClick={(e) => { e.stopPropagation(); navigate(`/order/${order.id}`) }}
                          className="flex-1 py-2.5 bg-black text-white rounded-xl font-black text-sm"
                        >
                          确认收货
                        </Button>
                      )}
                      {showReturn && (
                        <Button
                          onClick={(e) => {
                            e.stopPropagation()
                            navigate(`/return/${order.id}?instrument=${order.instrument_id}`)
                          }}
                          className="flex-1 py-2.5 bg-black text-white rounded-xl font-black text-sm"
                        >
                          归还乐器
                        </Button>
                      )}
                      {showCancel && (
                        <Button
                          onClick={(e) => { e.stopPropagation(); navigate(`/order/${order.id}`) }}
                          className="flex-1 py-2.5 bg-zinc-100 text-zinc-600 rounded-xl font-black text-sm"
                        >
                          取消订单
                        </Button>
                      )}
                      {!showPay && !showConfirm && !showReturn && !showCancel && (
                        <View className="w-full py-2.5 bg-zinc-100 rounded-xl text-center">
                          <Text className="text-zinc-400 font-black text-sm">等待处理</Text>
                        </View>
                      )}
                    </>
                  )}
                </View>
              </View>
              )
            })}
          </View>
          {loadingMore && (
            <View className="text-center py-4">
              <Text className="text-zinc-400 text-sm">加载更多...</Text>
            </View>
          )}
          </>
        )}
      </ScrollView>

      <BottomNav
        active="rent"
        tabs={[
          { key: 'home', icon: '🏪', label: '首页', onClick: () => navigate('/') },
          { key: 'rent', icon: '🪕', label: '租赁', onClick: () => navigate(isStaff ? '/staff/orders' : '/my-leases') },
          { key: 'service', icon: '🛠️', label: '维修', onClick: () => navigate(isStaff ? '/my-repairs' : '/my-repairs') },
          { key: 'profile', icon: '👤', label: '我的', onClick: () => navigate('/profile') },
        ]}
      />
    </View>
  )
}
