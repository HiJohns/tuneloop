import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { formatDisplayDate } from '../utils/format'
import { calculateDays } from '../utils/daycalc'
import { ArrowLeft, Package, History, Clock } from 'lucide-react'

const STATUS_LABELS = {
  pending: '待付款', paid: '待发货', shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '超期', transferred: '已过户',
  damage_appealing: '定损申诉', pending_damage_response: '待回应定损',
  deposit_refunding: '押金退款中',
}

const STATUS_COLORS = {
  pending: 'bg-yellow-100 text-yellow-700', paid: 'bg-orange-100 text-orange-700',
  shipped: 'bg-green-100 text-green-700', in_lease: 'bg-indigo-100 text-indigo-700',
  returning: 'bg-yellow-100 text-yellow-700', returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600', cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700', transferred: 'bg-purple-100 text-purple-700',
}

function getLeaseInfo(order) {
  if (!order.start_date) return null
  const isCancelled = order.status === 'cancelled'
  if (isCancelled) return null
  const isEnded = ['completed', 'returned'].includes(order.status)
  const endDate = (isEnded && order.returned_at) ? order.returned_at : order.end_date
  if (!endDate) return null
  const start = new Date(order.start_date.slice(0, 10))
  const end = new Date(endDate.slice(0, 10))
  const days = calculateDays(start, end)
  return { start: order.start_date, end: endDate, days }
}

export default function LeaseHistory() {
  const navigate = useNavigate()
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const sentinelRef = useRef(null)
  const token = getToken()

  const fetchOrders = useCallback(async (pageNum = 1, append = false) => {
    if (!append) setLoading(true)
    else setLoadingMore(true)
    try {
      const resp = await apiFetch(`/api/orders?page=${pageNum}&pageSize=20`)
      const result = await resp.json()
      if (result.code === 20000) {
        const list = result.data?.list || []
        if (append) {
          setOrders(prev => [...prev, ...list])
        } else {
          setOrders(list)
        }
        const total = result.data?.total || 0
        append && setHasMore(pageNum * 20 < total)
        !append && setHasMore(list.length === 20)
      }
    } catch (err) {
      console.error('Failed to fetch orders:', err)
    }
    setLoading(false)
    setLoadingMore(false)
  }, [])

  useEffect(() => { fetchOrders(1, false) }, [])

  useEffect(() => {
    if (page > 1) fetchOrders(page, true)
  }, [page])

  useEffect(() => {
    const observer = new IntersectionObserver(entries => {
      if (entries[0].isIntersecting && hasMore && !loadingMore) {
        setPage(p => p + 1)
      }
    }, { threshold: 0.1, rootMargin: '200px' })
    const sentinel = sentinelRef.current
    if (sentinel) observer.observe(sentinel)
    return () => observer.disconnect()
  }, [hasMore, loadingMore])

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">租赁历史</Text>
      </View>

      <View className="p-4">
        {loading ? (
          <View className="text-center py-8 text-gray-500">加载中...</View>
        ) : orders.length === 0 ? (
          <View className="text-center py-16">
            <History size={48} className="mx-auto text-gray-300 mb-4" />
            <Text className="text-gray-500">暂无租赁记录</Text>
          </View>
        ) : (
          <>
            <View className="space-y-3">
              {orders.filter(o => o.status !== 'reserved').map(order => (
                <View
                  key={order.id}
                  className="bg-white rounded-xl p-4 shadow-sm cursor-pointer"
                  onClick={() => navigate(`/order/${order.id}`)}
                >
                  <View className="flex justify-between items-start mb-2">
                    <View>
                      <Text className="text-sm font-mono font-bold">#{order.id?.slice(0, 8)}</Text>
                      {order.instrument?.category_name && (
                        <Text className="text-xs text-gray-400 mt-0.5">{order.instrument.category_name}</Text>
                      )}
                    </View>
                    <Text className={`text-xs px-2 py-1 rounded-full ${
                      STATUS_COLORS[order.status] || 'bg-gray-100'
                    }`}>
                      {STATUS_LABELS[order.status] || order.status}
                    </Text>
                  </View>
                  {(() => {
                    const li = getLeaseInfo(order)
                    if (!li) return null
                    return (
                      <View>
                        <View className="flex items-center text-xs text-gray-500 mt-1">
                          <Clock size={12} className="mr-1" />
                          <Text>实际租期: {formatDisplayDate(li.start)} ~ {formatDisplayDate(li.end)}</Text>
                        </View>
                        <Text className="text-xs text-gray-400 mt-0.5">{li.days} 天</Text>
                      </View>
                    )
                  })()}
                  <Text className="text-sm font-medium mt-1">
                    总计: ¥{((order.monthly_rent || 0) * (order.lease_term || 1) + (order.deposit || 0)).toFixed(0)}
                  </Text>
                </View>
              ))}
            </View>
            {loadingMore && <View className="text-center py-4 text-gray-500">加载中...</View>}
            {hasMore && <View ref={sentinelRef} className="h-20" />}
          </>
        )}
      </View>
    </View>
  )
}
