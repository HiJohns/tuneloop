import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { warehouseApi, apiFetch } from '../services/api'
import { ArrowLeft, Package, Clock, Search, Scan, User, MapPin } from 'lucide-react'

const STATUS_TABS = [
  { key: '', label: '全部' },
  { key: 'paid', label: '待发货' },
  { key: 'in_transit', label: '运输中' },
  { key: 'in_lease', label: '租赁中' },
  { key: 'returning', label: '归还验收' },
]

const STATUS_LABELS = {
  reserved: '已预约',
  paid: '待发货',
  pending_shipment: '待发货',
  in_transit: '运输中',
  shipped: '已送达',
  in_lease: '租赁中',
  returning: '归还中',
  returned: '已归还',
  completed: '已完成',
  cancelled: '已取消',
  expired: '超期',
  transferred: '已过户',
}

const STATUS_COLORS = {
  paid: 'bg-orange-100 text-orange-700',
  pending_shipment: 'bg-orange-100 text-orange-700',
  in_transit: 'bg-cyan-100 text-cyan-700',
  shipped: 'bg-green-100 text-green-700',
  in_lease: 'bg-indigo-100 text-indigo-700',
  returning: 'bg-yellow-100 text-yellow-700',
  reserved: 'bg-blue-100 text-blue-700',
  returned: 'bg-gray-100 text-gray-600',
  completed: 'bg-gray-100 text-gray-600',
  cancelled: 'bg-red-100 text-red-700',
  expired: 'bg-red-100 text-red-700',
  transferred: 'bg-purple-100 text-purple-700',
}

export default function StaffOrders() {
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState('')
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [searchInput, setSearchInput] = useState('')
  const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'

  useEffect(() => {
    const fetchOrders = async () => {
      setLoading(true)
      try {
        const params = {}
        if (activeTab) params.status = activeTab
        const resp = await warehouseApi.listOrders(params)
        if (Array.isArray(resp)) {
          setOrders(resp)
        } else if (resp.code === 20000) {
          setOrders(resp.data?.list || [])
        }
      } catch (err) {
        console.error('Failed to fetch orders:', err)
      } finally {
        setLoading(false)
      }
    }
    fetchOrders()
  }, [activeTab])

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
          canvas.width = img.width
          canvas.height = img.height
          ctx.drawImage(img, 0, 0)
          try {
            const blob = await new Promise(r => canvas.toBlob(r, 'image/png'))
            const bitmap = await createImageBitmap(blob)
            const detector = new BarcodeDetector({ formats: ['qr_code'] })
            const codes = await detector.detect(bitmap)
            if (codes.length > 0) {
              navigate(`/staff/orders/${codes[0].rawValue}`)
            } else {
              alert('未识别到二维码')
            }
          } catch {
            alert('二维码识别失败，请手动输入订单号')
          }
        }
        img.src = URL.createObjectURL(file)
      } catch {
        alert('扫码功能不可用，请手动输入订单号')
      }
    }
    input.click()
  }

  const handleOrderClick = (order) => {
    navigate(`/staff/orders/${order.id}`)
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">订单管理</h1>
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
          <button
            onClick={handleQRScan}
            className="px-3 py-2 border rounded-lg text-gray-600 hover:text-brand-primary flex items-center gap-1"
          >
            <Scan size={18} />
            <span className="text-xs">扫码</span>
          </button>
        </div>
      </div>

      {/* Status Tabs */}
      <div className="flex bg-white border-b overflow-x-auto">
        {STATUS_TABS.map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`flex-shrink-0 px-4 py-3 text-sm font-medium whitespace-nowrap ${
              activeTab === tab.key
                ? 'text-brand-primary border-b-2 border-brand-primary'
                : 'text-gray-500'
            }`}
          >
            {tab.label}
          </button>
        ))}
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
          orders.map(order => (
            <div
              key={order.id}
              className="bg-white rounded-xl p-4 cursor-pointer hover:shadow-md transition-shadow"
              onClick={() => handleOrderClick(order)}
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
                    <span>到期 {order.end_date}</span>
                  </div>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}
