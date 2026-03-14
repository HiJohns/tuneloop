import { useNavigate } from 'react-router-dom'
import { myServiceOrders } from '../data/mockData'
import { Badge } from 'antd'
import { ArrowLeft, Phone, Calendar } from 'lucide-react'

function ServiceCard({ order }) {
  const isProcessing = order.status === "处理中"
  
  return (
    <div className="bg-white rounded-xl shadow-sm p-4">
      <div className="flex justify-between items-start mb-3">
        <div>
          <h3 className="font-medium text-brand-text">{order.assetName}</h3>
          <p className="text-gray-500 text-sm mt-1">故障: {order.fault}</p>
        </div>
        <Badge 
          status={isProcessing ? "processing" : "default"} 
          text={order.status} 
        />
      </div>
      
      <div className="text-sm text-gray-600 space-y-2">
        {order.status === "待派单" && (
          <p className="text-gray-500">
            备注: {order.site}
          </p>
        )}
        
        {order.status === "处理中" && (
          <div className="space-y-1">
            <p>服务人员: {order.technician}</p>
            <div className="flex items-center gap-2">
              <Phone size={14} />
              <span>{order.technicianPhone}</span>
            </div>
          </div>
        )}
        
        <div className="flex items-center gap-2 text-gray-500 text-xs pt-2 border-t">
          <Calendar size={14} />
          <span>创建时间: {order.createdAt}</span>
        </div>
      </div>
    </div>
  )
}

export default function MyService() {
  const navigate = useNavigate()
  
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
        {myServiceOrders.map(order => (
          <ServiceCard key={order.id} order={order} />
        ))}
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
