import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { api } from '../services/api'
import { Badge, Tag } from 'antd'
import { ArrowLeft, Phone, Calendar } from 'lucide-react'

function ServiceCard({ order }) {
  return (
    <div className="bg-white rounded-xl shadow-sm p-4">
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <h3 className="font-medium text-brand-text">{order.assetName}</h3>
          <Tag color={order.status === "处理中" ? "blue" : "orange"}>
            {order.status}
          </Tag>
        </div>
        
        <p className="text-gray-600 text-sm">故障: {order.fault}</p>
        
        {order.status === "待派单" && (
          <p className="text-gray-500 text-sm">备注: {order.site}</p>
        )}
        
        {order.status === "处理中" && (
          <div className="flex items-center gap-2">
            <span className="text-sm">服务人员: {order.technician}</span>
            <a href={`tel:${order.technicianPhone}`} className="text-brand-primary text-sm">
              📞 {order.technicianPhone}
            </a>
          </div>
        )}
        
        <p className="text-gray-400 text-xs">创建时间: {order.createdAt}</p>
      </div>
    </div>
  )
}

export default function MyService() {
  const navigate = useNavigate()
  const [myServiceOrders, setMyServiceOrders] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    const fetchServiceOrders = async () => {
      try {
        setLoading(true)
        const data = await api.get('/user/service-orders')
        setMyServiceOrders(data || [])
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch service orders:', error)
        setLoading(false)
      }
    }
    
    fetchServiceOrders()
  }, [])
  
  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      {/* Header */}
      <div className="bg-white border-b px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">我的维修</h1>
      </div>
      
      {/* Service Orders List */}
      <div className="p-4 space-y-4">
        {loading ? (
          <div className="text-center py-8 text-gray-500">加载中...</div>
        ) : (
          myServiceOrders.map(order => (
            <ServiceCard key={order.id} order={order} />
          ))
        )}
      </div>
      
      {/* Bottom Navigation */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <div className="flex justify-around py-3 max-w-[480px] mx-auto">
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/')}
          >
            <span className="text-xl">🏠</span>
            <span className="text-xs mt-1">首页</span>
          </div>
          <div 
            className="flex flex-col items-center text-brand-primary cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <span className="text-xl">🔧</span>
            <span className="text-xs mt-1">维修</span>
          </div>
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/profile')}
          >
            <span className="text-xl">👤</span>
            <span className="text-xs mt-1">我的</span>
          </div>
        </div>
      </div>
    </div>
  )
}
