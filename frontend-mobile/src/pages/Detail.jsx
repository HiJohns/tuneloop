import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { instruments } from '../data/mockData'
import { ArrowLeft, Shield, Clock, AlertCircle, MapPin, Bell } from 'lucide-react'
import { Switch } from 'antd'

export default function Detail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const instrument = instruments.find(i => i.id === parseInt(id))
  const [smsReminder, setSmsReminder] = useState(true)

  if (!instrument) {
    return <div className="p-4">乐器不存在</div>
  }

  return (
    <div className="min-h-screen bg-gray-50 pb-20">
      {/* Image */}
      <div className="relative">
        <img 
          src={instrument.image} 
          alt={instrument.name}
          className="w-full h-64 object-cover"
        />
        <button 
          onClick={() => navigate(-1)}
          className="absolute top-4 left-4 bg-black/30 text-white p-2 rounded-full"
        >
          <ArrowLeft size={20} />
        </button>
      </div>

      {/* Info */}
      <div className="bg-white p-4">
        <h1 className="text-xl font-bold text-gray-800">{instrument.name}</h1>
        <p className="text-gray-500 mt-1">{instrument.description}</p>
        
        <div className="flex items-center gap-4 mt-4">
          <div>
            <p className="text-gray-500 text-sm">月租金</p>
            <p className="text-orange-500 font-bold text-2xl">¥{instrument.monthlyRent}</p>
          </div>
          <div>
            <p className="text-gray-500 text-sm">押金</p>
            <p className="text-gray-800 font-bold text-xl">¥{instrument.deposit}</p>
          </div>
        </div>

        {/* Key Points */}
        <div className="mt-6 space-y-4">
          {/* Asset Card */}
          <div className="flex items-start gap-3 p-3 bg-gray-50 rounded-lg">
            <div className="flex-1">
              <p className="font-medium text-sm text-gray-800">资产信息</p>
              <p className="text-gray-500 text-sm mt-1">SN: {instrument.sn}</p>
              <p className="text-gray-500 text-sm">归属: {instrument.site}</p>
            </div>
            <div className="flex items-center gap-1 text-brand-primary">
              <MapPin size={16} />
              <span className="text-sm">距离我: {instrument.distance} km</span>
            </div>
          </div>

          <div className="flex items-start gap-3 p-3 bg-blue-50 rounded-lg">
            <Shield className="text-blue-500 flex-shrink-0" size={20} />
            <div>
              <p className="font-medium text-sm text-blue-800">押金说明</p>
              <p className="text-blue-600 text-sm mt-0.5">{instrument.depositNote}</p>
            </div>
          </div>

          <div className="flex items-start gap-3 p-3 bg-green-50 rounded-lg">
            <Clock className="text-green-500 flex-shrink-0" size={20} />
            <div>
              <p className="font-medium text-sm text-green-800">起租期</p>
              <p className="text-green-600 text-sm mt-0.5">最少{instrument.minRentPeriod}个月起租</p>
            </div>
          </div>

          <div className="flex items-start gap-3 p-3 bg-yellow-50 rounded-lg">
            <AlertCircle className="text-yellow-500 flex-shrink-0" size={20} />
            <div>
              <p className="font-medium text-sm text-yellow-800">损耗标准</p>
              <p className="text-yellow-700 text-sm mt-0.5">{instrument.wearStandard}</p>
            </div>
          </div>

          {/* SMS Reminder */}
          <div className="flex items-center justify-between p-3 bg-white border rounded-lg">
            <div className="flex items-center gap-2">
              <Bell size={18} className="text-gray-500" />
              <span className="text-sm text-gray-700">开启到期前7天短信通知</span>
            </div>
            <Switch checked={smsReminder} onChange={setSmsReminder} />
          </div>
        </div>
      </div>

      {/* Bottom Action */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        <button 
          onClick={() => navigate(`/checkout/${instrument.id}`)}
          className="w-full bg-orange-500 text-white py-3 rounded-lg font-medium"
        >
          立即租赁
        </button>
      </div>
    </div>
  )
}