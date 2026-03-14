import { useNavigate } from 'react-router-dom'
import { CheckCircle, Bell, Calendar } from 'lucide-react'

export default function Success() {
  const navigate = useNavigate()

  return (
    <div className="min-h-screen bg-gray-50 flex flex-col">
      {/* Success Icon */}
      <div className="bg-white p-8 text-center">
        <div className="flex justify-center mb-4">
          <CheckCircle className="text-green-500" size={64} />
        </div>
        <h1 className="text-xl font-bold text-gray-800">支付成功</h1>
        <p className="text-gray-500 mt-2">您的订单已创建成功</p>
      </div>

      {/* Order Info */}
      <div className="bg-white mx-4 mt-4 rounded-lg p-4">
        <h2 className="font-medium text-gray-800 mb-3">订单信息</h2>
        <div className="space-y-2 text-sm">
          <div className="flex justify-between">
            <span className="text-gray-500">订单号</span>
            <span className="text-gray-800">TL202603140001</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">乐器</span>
            <span className="text-gray-800">雅马哈 U1 立式钢琴</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">租期</span>
            <span className="text-gray-800">3个月</span>
          </div>
          <div className="flex justify-between">
            <span className="text-gray-500">支付金额</span>
            <span className="text-orange-500 font-medium">¥19,400</span>
          </div>
        </div>
      </div>

      {/* Reminder Status */}
      <div className="bg-green-50 mx-4 mt-4 rounded-lg p-4 flex items-center gap-3">
        <Bell className="text-green-500 flex-shrink-0" size={24} />
        <div>
          <p className="font-medium text-green-800">到期提醒已开启</p>
          <p className="text-green-600 text-sm mt-0.5">
            我们将在租赁到期前3天、1天提醒您
          </p>
        </div>
      </div>

      {/* Dates Info */}
      <div className="bg-white mx-4 mt-4 rounded-lg p-4">
        <div className="flex items-center gap-2 text-gray-600 mb-2">
          <Calendar size={18} />
          <span className="font-medium">预计到期日</span>
        </div>
        <p className="text-gray-800">2026年6月14日</p>
      </div>

      {/* Bottom Actions */}
      <div className="mt-auto p-4 bg-white border-t">
        <div className="flex gap-3">
          <button 
            onClick={() => navigate('/')}
            className="flex-1 py-3 border border-gray-300 rounded-lg font-medium text-gray-600"
          >
            返回首页
          </button>
          <button 
            className="flex-1 py-3 bg-orange-500 text-white rounded-lg font-medium"
          >
            查看订单
          </button>
        </div>
      </div>
    </div>
  )
}