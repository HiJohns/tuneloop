import { useState, useEffect, useRef, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { warehouseApi, apiFetch } from '../services/api'
import { env } from '../platform'
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
  reserved: '已预约', paid: '待发货', pending_shipment: '待发货',
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

  const handleQRScan = () => {
    const input = document.createElement('input')
    input.type = 'file'
    input.accept = 'image/*'
    input.capture = 'environment'
    input.onchange = async (e) => {
      const file = e.target.files[0]
      if (!file) return
      try {
        const canvas = document.createElement('canvas')
        const ctx = canvas.getContext('2d')
        const img = new Image()
        img.onload = async () => {
          canvas.width = img.width; canvas.height = img.height
          ctx.drawImage(img, 0, 0)
          try {
            const blob = await new Promise(r => canvas.toBlob(r, 'image/png'))
            const bitmap = await createImageBitmap(blob)
            const detector = new BarcodeDetector({ formats: ['qr_code'] })
            const codes = await detector.detect(bitmap)
            if (codes.length > 0) navigate(`/staff/orders/${codes[0].rawValue}`)
            else alert('未识别到二维码')
          } catch { alert('二维码识别失败，请手动输入订单号') }
        }
        img.src = URL.createObjectURL(file)
      } catch { alert('扫码功能不可用，请手动输入订单号') }
    }
    input.click()
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">订单管理</h1>
      </div>

      <div className="bg-white px-4 py-3 border-b">
        <div className="flex gap-2 mb-2">
          {MAIN_TABS.map(tab => (
            <button
              key={tab.key}
              onClick={() => { setMainTab(tab.key); setPage(1); setSubFilter('') }}
              className={`flex-1 py-2 rounded-lg text-sm font-medium ${
                mainTab === tab.key ? 'bg-brand-primary text-white' : 'bg-gray-100 text-gray-600'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
        <div className="flex gap-2 overflow-x-auto">
          {SUB_FILTERS[mainTab].map(f => (
            <button
              key={f.key}
              onClick={() => { setSubFilter(f.key); setPage(1) }}
              className={`px-3 py-1 rounded-full text-xs whitespace-nowrap ${
                subFilter === f.key ? 'bg-brand-primary text-white' : 'bg-gray-100 text-gray-600'
              }`}
            >
              {f.label}
            </button>
          ))}
        </div>
      </div>

      {/* Search Bar */}
      <div className="bg-white px-4 py-3 border-b">
        <div className="flex gap-2">
          <div className="flex-1 relative">
            <Search size={16} className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400" />
            <input
              type="text"
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              placeholder="输入订单号搜索"
              className="w-full pl-9 pr-3 py-2 border rounded-lg text-sm"
              onKeyDown={e => e.key === 'Enter' && handleSearch()}
            />
          </div>
          {env.isWechatBrowser && (
            <button onClick={handleQRScan} className="px-3 py-2 border rounded-lg text-gray-600 hover:text-brand-primary flex items-center gap-1">
              <Scan size={18} />
              <span className="text-xs">扫码</span>
            </button>
          )}
        </div>
      </div>

      {/* Order List */}
      <div className="p-4 space-y-3">
        {loading ? (
          <div className="text-center text-gray-500 py-8">加载中...</div>
        ) : orders.length === 0 ? (
          <div className="text-center text-gray-400 py-12">
            <Package size={48} className="mx-auto mb-3 opacity-50" />
            <p>暂无订单</p>
          </div>
        ) : (
          <>
            {orders.map(order => (
              <div
                key={order.id}
                className="bg-white rounded-xl p-4 cursor-pointer hover:shadow-md transition-shadow"
                onClick={() => navigate(`/staff/orders/${order.id}`)}
              >
                <div className="flex justify-between items-start mb-3">
                  <div>
                    <p className="text-sm font-mono font-bold text-gray-900">#{order.id?.slice(0, 12)}</p>
                    <p className="text-xs text-gray-400 mt-0.5">{order.id}</p>
                  </div>
                  <span className={`text-xs px-2.5 py-1 rounded-full font-medium ${STATUS_COLORS[order.status] || 'bg-gray-100'}`}>
                    {STATUS_LABELS[order.status] || order.status}
                  </span>
                </div>
                <div className="grid grid-cols-2 gap-2 text-xs text-gray-500">
                  {order.user_name && (
                    <div className="flex items-center gap-1">
                      <User size={12} />
                      <span>{order.user_name}</span>
                    </div>
                  )}
                  {order.instrument?.sn && (
                    <div className="flex items-center gap-1">
                      <Package size={12} />
                      <span>{order.instrument.sn}</span>
                    </div>
                  )}
                  {order.end_date && (
                    <div className="flex items-center gap-1">
                      <Clock size={12} />
                      <span>到期 {formatDisplayDate(order.end_date)}</span>
                    </div>
                  )}
                </div>
              </div>
            ))}
            {loadingMore && <div className="text-center py-4 text-gray-500">加载中...</div>}
            {hasMore && <div ref={sentinelRef} className="h-20" />}
          </>
        )}
      </div>
    </div>
  )
}
