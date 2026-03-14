import { useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { instruments } from '../data/mockData'
import { ArrowLeft, Shield, Clock, AlertCircle, MapPin, Bell, CheckCircle } from 'lucide-react'
import { Switch, Segmented, Tag } from 'antd'

export default function Detail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const instrument = instruments.find(i => i.id === parseInt(id))
  const [smsReminder, setSmsReminder] = useState(true)
  const [selectedLevel, setSelectedLevel] = useState("入门级")

  const currentLevel = instrument.levels.find(l => l.name === selectedLevel)

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
        
        {/* Level Selector */}
        <div className="mt-4">
          <Segmented
            options={["入门级", "专业级", "大师级"]}
            value={selectedLevel}
            onChange={setSelectedLevel}
            className="w-full"
          />
        </div>
        
        <div className="flex items-center gap-4 mt-4">
          <div>
            <p className="text-gray-500 text-sm">月租金</p>
            <p className="text-brand-primary font-bold text-2xl">¥{currentLevel.monthlyRent}</p>
          </div>
          <div>
            <p className="text-gray-500 text-sm">押金</p>
            <p className="text-gray-800 font-bold text-xl">¥{currentLevel.deposit}</p>
          </div>
        </div>

        {/* Maintenance Services */}
        <div className="mt-4 p-3 bg-green-50 rounded-lg">
          <p className="font-medium text-sm text-green-800 mb-2">服务明细</p>
          <div className="flex flex-wrap gap-2">
            {currentLevel.maintenance.map((item, idx) => (
              <Tag key={idx} color="green" icon={<CheckCircle size={12} />}>{item}</Tag>
            ))}
          </div>
        </div>

        {/* Rent-to-Own */}
        <div className="mt-3 p-3 bg-purple-50 rounded-lg">
          <p className="font-medium text-sm text-purple-800">租购转化</p>
          <p className="text-purple-600 text-sm mt-0.5">租满12个月可直接获得所有权</p>
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
              <p className="text-blue-500 text-xs mt-1">租约期满后，网点(Site)验收无误后3个工作日内原路退还</p>
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