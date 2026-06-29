import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView, Input, Textarea } from '@tarojs/components'
import { warehouseApi, apiFetch, getToken } from '../services/api'
import { env, scanQRCode } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { Package, Search, Scan } from 'lucide-react'
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
    { key: 'in_transit', label: '运输中' },
    { key: 'in_lease', label: '租赁中' },
    { key: 'expired', label: '已超期' },
    { key: 'returning', label: '归还中' },
  ],
  completed: [
    { key: 'completed', label: '已完成' },
    { key: 'cancelled', label: '已取消' },
    { key: 'transferred', label: '已过户' },
  ],
}
const allStatusKeys = ['reserved', 'paid', 'pending_shipment', 'in_transit', 'shipped', 'in_lease', 'expired', 'returning', 'returned', 'completed', 'cancelled', 'transferred']

const STATUS_LABELS = {
  reserved: '未支付', paid: '待发货', pending_shipment: '待发货',
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
  const baseUrl = env.apiBaseUrl

  const token = getToken()
  const isStaff = (() => {
    try {
      if (!token) return false
      const payload = JSON.parse(atob(token.split('.')[1]))
      return payload?.role && payload.role !== 'USER'
    } catch { return false }
  })()

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
      setHasMore(result.data?.total > (pageNum * 20))
    } catch (err) {
      console.error('Failed to fetch orders:', err)
    }
    setLoading(false)
    setLoadingMore(false)
  }, [mainTab, subFilter, baseUrl])

  useEffect(() => { setPage(1); setOrders([]); setHasMore(true) }, [mainTab, subFilter])

  useEffect(() => {
    fetchOrders(page, page > 1)
  }, [page])

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
    <View className="flex flex-col h-screen bg-[#FDFBF7]">
      <View className="bg-gradient-to-b from-[#FDF4E7] to-white px-4 pt-4 pb-3">
        <Text className="text-lg font-black text-black">订单管理</Text>
      </View>

      <ScrollView scrollY className="flex-1 px-4"
        onScrollToLower={() => {
          if (!loadingMore && hasMore) {
            setLoadingMore(true)
            setPage(p => p + 1)
          }
        }}
        lowerThreshold={50}
        enableBackToTop
      >
      <View className="bg-white mt-3 rounded-2xl shadow-sm p-4">
        <View className="flex gap-2 mb-3">
          {MAIN_TABS.map(tab => (
            <View
              key={tab.key}
              onClick={() => { setMainTab(tab.key); setPage(1); setSubFilter('') }}
              className={`flex-1 py-2 rounded-lg text-sm font-black text-center ${
                mainTab === tab.key ? 'bg-black text-white' : 'bg-zinc-100 text-zinc-500'
              }`}
            >
              {tab.label}
            </View>
          ))}
        </View>
        <View className="flex gap-2 overflow-x-auto">
          {SUB_FILTERS[mainTab].map(f => (
            <View
              key={f.key}
              onClick={() => { setSubFilter(f.key); setPage(1) }}
              className={`px-3 py-1.5 rounded-full text-xs font-bold whitespace-nowrap ${
                subFilter === f.key ? 'bg-black text-white' : 'bg-zinc-100 text-zinc-600'
              }`}
            >
              {f.label}
            </View>
          ))}
        </View>
      </View>

      {/* Search Bar */}
      <View className="bg-white mt-3 rounded-2xl shadow-sm p-4">
        <View className="flex gap-2">
          <View className="flex-1 relative">
            <Search size={16} className="text-zinc-400" style={{ position: 'absolute', left: 12, top: 12 }} />
            <input
              type="text"
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              placeholder="输入订单号搜索"
              className="w-full pl-9 pr-3 py-2 bg-zinc-50 rounded-lg text-sm"
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
            />
          </View>
          {env.isWechatBrowser && (
            <View onClick={handleQRScan} className="px-3 py-2 bg-zinc-50 rounded-lg flex items-center gap-1">
              <Scan size={18} />
              <Text className="text-xs font-bold text-zinc-600">扫码</Text>
            </View>
          )}
        </View>
      </View>

      {/* Order List */}
      <View className="mt-3 space-y-3">
        {loading ? (
          <View className="text-center text-zinc-400 py-8 font-medium">加载中...</View>
        ) : orders.length === 0 ? (
          <View className="text-center text-zinc-400 py-12">
            <Package size={48} className="mx-auto mb-3 opacity-50" />
            <Text className="font-medium">暂无订单</Text>
          </View>
        ) : (
          <>
            {(mainTab === 'active' ? [...orders].sort((a, b) => new Date(b.created_at) - new Date(a.created_at)) : [...orders].sort((a, b) => new Date(b.returned_at || b.end_date || 0) - new Date(a.returned_at || a.end_date || 0))).map(order => {
              const isTerminal = ['completed', 'returned', 'cancelled', 'transferred'].includes(order.status)
              return (
              <View
                key={order.id}
                className="bg-white rounded-2xl shadow-sm p-4 active:opacity-80"
                onClick={() => navigate(`/staff/orders/${order.id}`)}
              >
                <View className="flex items-center justify-between mb-2">
                  <Text className="text-sm font-black text-black flex-1 min-w-0 truncate">#{order.id?.slice(0, 8)}</Text>
                  <Text className={`text-xs px-2 py-1 rounded-full font-bold flex-shrink-0 ml-2 ${STATUS_COLORS[order.status] || 'bg-zinc-100 text-zinc-500'}`}>
                    {STATUS_LABELS[order.status] || order.status}
                  </Text>
                </View>
                {isTerminal ? (
                  <View className="space-y-1 text-sm">
                    <Text className="text-zinc-400 font-medium">乐器: <Text className="text-black font-medium">{order.instrument_category || '-'}</Text></Text>
                    <Text className="text-zinc-400 font-medium">实际租期: <Text className="text-black font-medium">{formatDisplayDate(order.start_date)} ~ {formatDisplayDate(order.returned_at || order.end_date)}</Text></Text>
                    <Text className="text-zinc-400 font-medium">租期天数: <Text className="text-black font-medium">{Math.max(1, Math.round(((new Date(order.returned_at || order.end_date) - new Date(order.start_date)) / 86400000))) || order.lease_term * 30 || '-'}</Text> 天</Text>
                  </View>
                ) : (
                  <View className="space-y-1 text-sm">
                    <Text className="text-zinc-400 font-medium">下单日: <Text className="text-black font-medium">{formatDisplayDate(order.created_at)}</Text></Text>
                    <Text className="text-zinc-400 font-medium">乐器: <Text className="text-black font-medium">{order.instrument_category || '-'}</Text></Text>
                    <Text className="text-zinc-400 font-medium">租期天数: <Text className="text-black font-medium">{order.lease_term || Math.max(1, Math.round(((new Date(order.end_date) - new Date(order.start_date)) / 86400000))) || '-'}</Text> 天</Text>
                    <Text className="text-zinc-400 font-medium">预期归还日: <Text className="text-black font-medium">{formatDisplayDate(order.end_date)}</Text></Text>
                  </View>
                )}
              </View>
              )
            })}
            {loadingMore && <View className="text-center py-4 text-zinc-400 font-medium">加载中...</View>}
          </>
        )}
      </View>
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
