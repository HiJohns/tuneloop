import { useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { CheckCircle, Calendar, Package, Hash, MapPin, User } from 'lucide-react'

export default function Success() {
  const navigate = useNavigate()
  const location = useLocation()
  const orderData = location.state || {}
  const isBatch = Array.isArray(orderData.orders)

  useEffect(() => {
    localStorage.removeItem('cart')
    window.dispatchEvent(new Event('cartUpdated'))
  }, [])

  const handleDone = () => {
    navigate('/')
  }

  if (isBatch) {
    return (
      <div className="min-h-screen bg-green-50 flex flex-col p-4">
        <div className="flex-1 flex flex-col justify-center">
          <div className="text-center mb-8">
            <CheckCircle className="text-green-500 mx-auto" size={80} />
          </div>
          
          <h1 className="text-2xl font-bold text-center text-gray-800 mb-2">租赁成功</h1>
          <p className="text-gray-500 text-center mb-8">您的订单已创建成功</p>

          <div className="bg-white rounded-xl p-4 shadow-sm space-y-3">
            {orderData.orders.map((order, i) => (
              <div key={i} className="pb-3 border-b border-gray-100 last:border-b-0">
                <div className="flex justify-between items-center">
                  <span className="text-sm text-gray-500">订单 #{i + 1}</span>
                  <span className="text-sm font-medium">{order.order_id?.slice(0, 8)}</span>
                </div>
                <div className="flex justify-between items-center mt-1">
                  <span className="text-sm text-gray-500">金额</span>
                  <span className="text-sm font-bold text-orange-500">¥{order.amount?.toFixed(0) || 0}</span>
                </div>
              </div>
            ))}
            <div className="flex justify-between items-center pt-2">
              <span className="font-medium">合计</span>
              <span className="text-xl font-bold text-orange-500">¥{orderData.total_amount?.toFixed(0) || 0}</span>
            </div>
          </div>
        </div>

        <button onClick={handleDone} className="w-full py-4 bg-brand-primary text-white rounded-xl font-bold text-lg">
          完成
        </button>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-green-50 flex flex-col p-4">
      <div className="flex-1 flex flex-col justify-center">
        <div className="text-center mb-8">
          <CheckCircle className="text-green-500 mx-auto" size={80} />
        </div>
        
        <h1 className="text-2xl font-bold text-center text-gray-800 mb-2">租赁成功</h1>
        <p className="text-gray-500 text-center mb-8">您的订单已创建成功</p>

        <div className="bg-white rounded-xl p-4 shadow-sm">
          <div className="space-y-3 text-sm">
            <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Hash size={18} className="text-gray-400" />
              <div>
                <p className="text-gray-500 text-xs">订单号</p>
                <p className="font-medium">{orderData.order_id || 'TL' + Date.now()}</p>
              </div>
            </div>
            
            <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Package size={18} className="text-gray-400" />
              <div>
                <p className="text-gray-500 text-xs">乐器</p>
                <p className="font-medium">{orderData.category_name || '-'}</p>
                <p className="text-xs text-gray-400">{orderData.instrument_sn || '-'}</p>
              </div>
            </div>

            <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <User size={18} className="text-gray-400" />
              <div>
                <p className="text-gray-500 text-xs">商户</p>
                <p className="font-medium">{orderData.tenant_name || '-'}</p>
              </div>
            </div>
            
            <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <MapPin size={18} className="text-gray-400" />
              <div>
                <p className="text-gray-500 text-xs">取琴网点</p>
                <p className="font-medium">{orderData.site_name || '-'}</p>
                <p className="text-xs text-gray-400">{orderData.site_address || ''}</p>
              </div>
            </div>
            
            <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Calendar size={18} className="text-gray-400" />
              <div>
                <p className="text-gray-500 text-xs">租赁期间</p>
                <p className="font-medium">{orderData.lease_term || '-'}</p>
                <p className="text-xs text-gray-400">预期归还: {orderData.return_date || '待确定'}</p>
              </div>
            </div>
            
            <div className="flex items-center gap-3 pt-2">
              <div className="text-gray-400 text-xl">¥</div>
              <div>
                <p className="text-gray-500 text-xs">支付金额</p>
                <p className="text-xl font-bold text-orange-500">¥{orderData.total_amount?.toFixed(0) || 0}</p>
              </div>
            </div>
          </div>
        </div>
      </div>

      <button
        onClick={handleDone}
        className="w-full py-4 bg-brand-primary text-white rounded-xl font-bold text-lg"
      >
        完成
      </button>
    </div>
  )
}