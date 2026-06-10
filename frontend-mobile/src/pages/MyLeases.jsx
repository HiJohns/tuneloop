import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch, getToken } from '../services/api'
import { env } from '../platform'
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
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">我的租约</h1>
      </div>

      <div className="p-4">
        {loading ? (
          <div className="text-center py-8 text-gray-500">加载中...</div>
        ) : orders.length === 0 ? (
          <div className="text-center py-16">
            <Package size={48} className="mx-auto text-gray-300 mb-4" />
            <p className="text-gray-500">暂无租约</p>
          </div>
        ) : (
          <div className="space-y-3">
            {orders.map(order => (
              <div
                key={order.id}
                className="bg-white rounded-xl p-4 shadow-sm"
                onClick={() => navigate(`/instrument/${order.instrument_id}`)}
              >
                <div className="flex justify-between items-start mb-2">
                  <h3 className="font-medium">订单 #{order.id?.slice(0, 8)}</h3>
                  <span className={`text-xs px-2 py-1 rounded-full ${
                    order.status === 'in_lease' ? 'bg-green-100 text-green-700' :
                    order.status === 'pending' ? 'bg-yellow-100 text-yellow-700' :
                    'bg-gray-100 text-gray-600'
                  }`}>
                    {statusLabel[order.status] || order.status}
                  </span>
                </div>
                <div className="text-sm text-gray-500 space-y-1">
                  <p>月租: ¥{order.monthly_rent}</p>
                  <p>押金: ¥{order.deposit}</p>
                  {order.start_date && <p>起: {order.start_date}</p>}
                  {order.end_date && <p>止: {order.end_date}</p>}
                </div>
                {order.status === 'in_lease' && (
                  <button
                    onClick={(e) => {
                      e.stopPropagation()
                      navigate(`/return/${order.id}`)
                    }}
                    className="mt-3 bg-brand-primary text-white py-2 px-4 rounded-lg text-sm"
                  >
                    归还乐器
                  </button>
                )}
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
