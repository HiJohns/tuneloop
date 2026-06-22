import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { warehouseApi, apiFetch } from '../services/api'
import { env, scanQRCode } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { ArrowLeft, Package, Clock, Search, Scan, User, MapPin } from 'lucide-react'

const MAIN_TABS = [
  { key: 'active', label: '进行中' },
  { key: 'completed', label: '已完成' },
]

const SUB_FILTERS = {
  active: [
    { key: '', label: '全部' },
    { key: 'paid', label: '待发货' },
    { key: 'shipped', label: '已发货' },
    { key: 'in_transit', label: '运输中' },
    { key: 'in_lease', label: '租赁中' },
    { key: 'expired', label: '已超期' },
    { key: 'returning', label: '归还中' },
  ],
  completed: [
    { key: 'returned', label: '已归还' },
    { key: 'completed', label: '已完成' },
    { key: 'cancelled', label: '已取消' },
    { key: 'transferred', label: '已过户' },
  ],
}
const allStatusKeys = ['reserved', 'paid', 'pending_shipment', 'in_transit', 'shipped', 'in_lease', 'expired', 'returning', 'returned', 'completed', 'cancelled', 'transferred']

const STATUS_LABELS = {
  reserved: '待发货', paid: '待发货', pending_shipment: '待发货',
  in_transit: '运输中', shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '超期', transferred: '已过户',
}

const STATUS_COLORS = {
  paid: 'bg-orange-100 text-orange-700', pending_shipment: 'bg-orange-100 text-orange-700',
  in_transit: 'bg-cyan-100 text-cyan-700', shipped: 'bg-green-100 text-green-700',
  in_lease: 'bg-indigo-100 text-indigo-700', returning: 'bg-yellow-100 text-yellow-700',
  reserved: 'bg-blue-100 text-blue-700', returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600', cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700', transferred: 'bg-purple-100 text-purple-700',
}

const MAIN_INCLUDE = {
  active: ['reserved', 'paid', 'pending_shipment', 'in_transit', 'shipped', 'in_lease', 'expired', 'returning'],
  completed: ['returned', 'completed', 'cancelled', 'transferred'],
}

export default function StaffOrders() {
  const navigate = useNavigate()
  const [mainTab, setMainTab] = useState('active')
  const [subFilter, setSubFilter] = useState('')
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [searchInput, setSearchInput] = useState('')
  const sentinelRef = useRef(null)
  const baseUrl = env.apiBaseUrl

  const fetchOrders = useCallback(async (pageNum = 1, append = false) => {
    if (!append) setLoading(true)
    else setLoadingMore(true)
    try {
      const params = { page: pageNum, pageSize: 20 }
      const statusKey = subFilter || mainTab
      if (subFilter) {
        params.status = subFilter
      }
      const resp = await fetch(`${baseUrl}/warehouse/orders?page=${params.page}&pageSize=${params.pageSize}${params.status ? '&status=' + params.status : ''}`)
      const result = await resp.json()
      let list = []
      if (result.code === 20000) {
        list = result.data?.list || []
      } else if (Array.isArray(result)) {
        list = result.filter(o => MAIN_INCLUDE[mainTab]?.includes(o.status))
      }
      if (!subFilter) {
        list = list.filter(o => MAIN_INCLUDE[mainTab]?.includes(o.status))
      }
      if (append) {
        setOrders(prev => [...prev, ...list])
      } else {
        setOrders(list)
      }
      setHasMore(list.length === 20)
    } catch (err) {
      console.error('Failed to fetch orders:', err)
    }
    setLoading(false)
    setLoadingMore(false)
  }, [mainTab, subFilter, baseUrl])

  useEffect(() => { setPage(1); setOrders([]); fetchOrders(1, false) }, [mainTab, subFilter])

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

  const handleSearch = () => {
    const id = searchInput.trim()
    if (!id) return
    navigate(`/staff/orders/${id}`)
  }

  const handleQRScan = async () => {
    try {
      const result = await scanQRCode()
      navigate(`/staff/orders/${result}`)
    } catch {
      alert('扫码失败，请手动输入订单号')
    }
  }

  return (
    <View className="min-h-screen bg-brand-bg pb-20">
      <View className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <Button onClick={() => navigate(-1)}><ArrowLeft size={20} /></Button>
        <Text className="text-lg font-bold">订单管理</Text>
      </View>

      <View className="bg-white px-4 py-3 border-b">
        <View className="flex gap-2 mb-2">
          {MAIN_TABS.map(tab => (
            <Button
              key={tab.key}
              onClick={() => { setMainTab(tab.key); setPage(1); setSubFilter('') }}
              className={`flex-1 py-2 rounded-lg text-sm font-medium ${
                mainTab === tab.key ? 'bg-brand-primary text-white' : 'bg-gray-100 text-gray-600'
              }`}
            >
              {tab.label}
            </Button>
          ))}
        </View>
        <View className="flex gap-2 overflow-x-auto">
          {SUB_FILTERS[mainTab].map(f => (
            <Button
              key={f.key}
              onClick={() => { setSubFilter(f.key); setPage(1) }}
              className={`px-3 py-1 rounded-full text-xs whitespace-nowrap ${
                subFilter === f.key ? 'bg-brand-primary text-white' : 'bg-gray-100 text-gray-600'
              }`}
            >
              {f.label}
            </Button>
          ))}
        </View>
      </View>

      {/* Search Bar */}
      <View className="bg-white px-4 py-3 border-b">
        <View className="flex gap-2">
          <View className="flex-1 relative">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              type="text"
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              placeholder="输入订单号搜索"
              className="w-full pl-9 pr-3 py-2 border rounded-lg text-sm"
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
            />
          </View>
          {env.isWechatBrowser && (
            <Button onClick={handleQRScan} className="px-3 py-2 border rounded-lg text-gray-600 hover:text-brand-primary flex items-center gap-1">
              <Scan size={18} />
              <Text className="text-xs">扫码</Text>
            </Button>
          )}
        </View>
      </View>

      {/* Order List */}
      <View className="p-4 space-y-3">
        {loading ? (
          <View className="text-center text-gray-500 py-8">加载中...</View>
        ) : orders.length === 0 ? (
          <View className="text-center text-gray-400 py-12">
            <Package size={48} className="mx-auto mb-3 opacity-50" />
            <Text>暂无订单</Text>
          </View>
        ) : (
          <>
            {orders.map(order => (
              <View
                key={order.id}
                className="bg-white rounded-xl p-4 cursor-pointer hover:shadow-md transition-shadow"
                onClick={() => navigate(`/staff/orders/${order.id}`)}
              >
                <View className="flex justify-between items-start mb-3">
                  <View>
                    <Text className="text-sm font-mono font-bold text-gray-900">#{order.id?.slice(0, 12)}</Text>
                    <Text className="text-xs text-gray-400 mt-0.5">{order.id}</Text>
                  </View>
                  <Text className={`text-xs px-2.5 py-1 rounded-full font-medium ${STATUS_COLORS[order.status] || 'bg-gray-100'}`}>
                    {STATUS_LABELS[order.status] || order.status}
                  </Text>
                </View>
                <View className="grid grid-cols-2 gap-2 text-xs text-gray-500">
                  {order.user_name && (
                    <View className="flex items-center gap-1">
                      <User size={12} />
                      <Text>{order.user_name}</Text>
                    </View>
                  )}
                  {order.instrument?.sn && (
                    <View className="flex items-center gap-1">
                      <Package size={12} />
                      <Text>{order.instrument.sn}</Text>
                    </View>
                  )}
                  {order.end_date && (
                    <View className="flex items-center gap-1">
                      <Clock size={12} />
                      <Text>到期 {formatDisplayDate(order.end_date)}</Text>
                    </View>
                  )}
                </View>
              </View>
            ))}
            {loadingMore && <View className="text-center py-4 text-gray-500">加载中...</View>}
            {hasMore && <View ref={sentinelRef} className="h-20" />}
          </>
        )}
      </View>
    </View>
  )
}
