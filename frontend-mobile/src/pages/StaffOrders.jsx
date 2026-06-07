import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { warehouseApi } from '../services/api'
import { ArrowLeft, Package, Clock } from 'lucide-react'

const STATUS_TABS = [
  { key: 'paid', label: '待发货' },
  { key: 'shipped', label: '运输中' },
  { key: 'returning', label: '归还验收' },
]

const STATUS_LABELS = {
  paid: '待发货',
  shipped: '运输中',
  returning: '归还中',
  in_store: '已入库',
  maintenance: '维修中',
}

const STATUS_COLORS = {
  paid: 'bg-orange-100 text-orange-700',
  shipped: 'bg-blue-100 text-blue-700',
  returning: 'bg-yellow-100 text-yellow-700',
}

export default function StaffOrders() {
  const navigate = useNavigate()
  const [activeTab, setActiveTab] = useState('paid')
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchOrders = async () => {
      setLoading(true)
      try {
        const resp = await warehouseApi.listOrders({ status: activeTab })
        if (resp.code === 20000) {
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

  const handleOrderClick = (order) => {
    if (order.status === 'paid') {
      navigate(`/staff/shipping?order_id=${order.id}`)
    } else {
      navigate(`/staff/receiving?order_id=${order.id}`)
    }
  }

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}><ArrowLeft size={20} /></button>
        <h1 className="text-lg font-bold">仓库订单</h1>
      </div>

      <div className="flex bg-white border-b">
        {STATUS_TABS.map(tab => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`flex-1 py-3 text-sm font-medium text-center ${
              activeTab === tab.key
                ? 'text-brand-primary border-b-2 border-brand-primary'
                : 'text-gray-500'
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      <div className="p-4 space-y-3">
        {loading ? (
          <div className="text-center text-gray-500 py-8">加载中...</div>
        ) : orders.length === 0 ? (
          <div className="text-center text-gray-400 py-12">
            <Package size={48} className="mx-auto mb-3 opacity-50" />
            <p>暂无待处理订单</p>
          </div>
        ) : (
          orders.map(order => (
            <div
              key={order.id}
              className="bg-white rounded-xl p-4 cursor-pointer"
              onClick={() => handleOrderClick(order)}
            >
              <div className="flex justify-between items-start mb-2">
                <span className="text-sm font-medium">#{order.id?.slice(0, 8)}</span>
                <span className={`text-xs px-2 py-1 rounded-full ${STATUS_COLORS[order.status] || 'bg-gray-100'}`}>
                  {STATUS_LABELS[order.status] || order.status}
                </span>
              </div>
              <div className="text-xs text-gray-500 space-y-1">
                <p>乐器: {order.instrument?.name || order.instrument_id?.slice(0, 8) || '-'}</p>
                {order.end_date && (
                  <p className="flex items-center gap-1">
                    <Clock size={12} />
                    到期: {order.end_date}
                  </p>
                )}
              </div>
            </div>
          ))
        )}
      </div>
    </div>
  )
}