import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch, getToken } from '../services/api'
import { formatDisplayDate } from '../utils/format'
import { ArrowLeft, Package, History, Clock } from 'lucide-react'

const STATUS_LABELS = {
  pending: '待付款', paid: '待发货', shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '超期', transferred: '已过户',
}

const STATUS_COLORS = {
  pending: 'bg-yellow-100 text-yellow-700', paid: 'bg-orange-100 text-orange-700',
  shipped: 'bg-green-100 text-green-700', in_lease: 'bg-indigo-100 text-indigo-700',
  returning: 'bg-yellow-100 text-yellow-700', returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600', cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700', transferred: 'bg-purple-100 text-purple-700',
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
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">租赁历史</h1>
      </div>

      <div className="p-4">
        {loading ? (
          <div className="text-center py-8 text-gray-500">加载中...</div>
        ) : orders.length === 0 ? (
          <div className="text-center py-16">
            <History size={48} className="mx-auto text-gray-300 mb-4" />
            <p className="text-gray-500">暂无租赁记录</p>
          </div>
        ) : (
          <>
            <div className="space-y-3">
              {orders.filter(o => o.status !== 'reserved').map(order => (
                <div
                  key={order.id}
                  className="bg-white rounded-xl p-4 shadow-sm cursor-pointer"
                  onClick={() => navigate(`/order/${order.id}`)}
                >
                  <div className="flex justify-between items-start mb-2">
                    <div>
                      <p className="text-sm font-mono font-bold">#{order.id?.slice(0, 8)}</p>
                      {order.instrument?.category_name && (
                        <p className="text-xs text-gray-400 mt-0.5">{order.instrument.category_name}</p>
                      )}
                    </div>
                    <span className={`text-xs px-2 py-1 rounded-full ${
                      STATUS_COLORS[order.status] || 'bg-gray-100'
                    }`}>
                      {STATUS_LABELS[order.status] || order.status}
                    </span>
                  </div>
                  {order.start_date && (
                    <p className="text-xs text-gray-500">
                      <Clock size={12} className="inline mr-1" />
                      {formatDisplayDate(order.start_date)} ~ {formatDisplayDate(order.end_date)}
                    </p>
                  )}
                  <p className="text-sm font-medium mt-1">
                    总计: ¥{((order.monthly_rent || 0) * (order.lease_term || 1) + (order.deposit || 0)).toFixed(0)}
                  </p>
                </div>
              ))}
            </div>
            {loadingMore && <div className="text-center py-4 text-gray-500">加载中...</div>}
            {hasMore && <div ref={sentinelRef} className="h-20" />}
          </>
        )}
      </div>
    </div>
  )
}
