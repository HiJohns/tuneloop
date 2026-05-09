import { useEffect } from 'react'
import { useNavigate, useLocation } from 'react-router-dom'
import { CheckCircle, Calendar, Package, Hash } from 'lucide-react'

export default function Success() {
  const navigate = useNavigate()
  const location = useLocation()
  const orderData = location.state || {}

  useEffect(() => {
    localStorage.removeItem('cart')
    window.dispatchEvent(new Event('cartUpdated'))
  }, [])

  const handleDone = () => {
    navigate('/')
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
                <p className="font-medium">{orderData.instrument_name || '-'}</p>
                <p className="text-xs text-gray-400">{orderData.instrument_sn || '-'}</p>
              </div>
            </div>
            
            <div className="flex items-center gap-3 pb-3 border-b border-gray-100">
              <Calendar size={18} className="text-gray-400" />
              <div>
                <p className="text-gray-500 text-xs">租赁期间</p>
                <p className="font-medium">{orderData.lease_term || 12} 个月</p>
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