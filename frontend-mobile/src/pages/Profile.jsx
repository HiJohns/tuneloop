import { useNavigate } from 'react-router-dom'
import { myAssets } from '../data/mockData'
import { User, MapPin, Bell, HelpCircle, ChevronRight, Wrench, RefreshCw } from 'lucide-react'

function AssetCard({ asset, onRenew, onMaintain }) {
  return (
    <div className="bg-white rounded-lg p-4 shadow-sm">
      <div className="flex gap-3">
        <img 
          src={asset.image} 
          alt={asset.name}
          className="w-20 h-20 object-cover rounded-lg"
        />
        <div className="flex-1">
          <h3 className="font-medium">{asset.name}</h3>
          <p className="text-gray-500 text-sm mt-1">
            租期: {asset.startDate} 至 {asset.endDate}
          </p>
          <span className="inline-block px-2 py-0.5 bg-green-100 text-green-700 text-xs rounded mt-2">
            {asset.status}
          </span>
        </div>
      </div>
      <div className="flex gap-2 mt-4">
        <button
          onClick={onRenew}
          className="flex-1 py-2 border border-orange-500 text-orange-500 rounded-lg text-sm font-medium flex items-center justify-center gap-1"
        >
          <RefreshCw size={14} />
          续租
        </button>
        <button
          onClick={onMaintain}
          className="flex-1 py-2 bg-orange-500 text-white rounded-lg text-sm font-medium flex items-center justify-center gap-1"
        >
          <Wrench size={14} />
          申请维护
        </button>
      </div>
    </div>
  )
}

export default function Profile() {
  const navigate = useNavigate()

  return (
    <div className="min-h-screen bg-gray-50 pb-20">
      {/* Header */}
      <div className="bg-orange-500 text-white px-4 py-6">
        <div className="flex items-center gap-4">
          <div className="w-16 h-16 bg-white/20 rounded-full flex items-center justify-center">
            <User size={32} />
          </div>
          <div>
            <h1 className="text-lg font-bold">用户</h1>
            <p className="text-sm opacity-90">138****8888</p>
          </div>
        </div>
      </div>

      {/* My Assets */}
      <div className="p-4">
        <h2 className="font-medium text-gray-800 mb-3">我的乐器</h2>
        <div className="space-y-3">
          {myAssets.map(asset => (
            <AssetCard
              key={asset.id}
              asset={asset}
              onRenew={() => alert("续租功能开发中...")}
              onMaintain={() => navigate('/booking')}
            />
          ))}
        </div>
      </div>

      {/* Menu Items */}
      <div className="p-4 space-y-2">
        <h2 className="font-medium text-gray-800 mb-3">其他</h2>
        
        <div className="bg-white rounded-lg overflow-hidden">
          <button className="w-full flex items-center justify-between p-4 border-b">
            <div className="flex items-center gap-3">
              <MapPin size={20} className="text-gray-400" />
              <span className="text-gray-800">收货地址</span>
            </div>
            <ChevronRight size={20} className="text-gray-400" />
          </button>
          
          <button className="w-full flex items-center justify-between p-4 border-b">
            <div className="flex items-center gap-3">
              <Bell size={20} className="text-gray-400" />
              <span className="text-gray-800">消息通知</span>
            </div>
            <ChevronRight size={20} className="text-gray-400" />
          </button>
          
          <button className="w-full flex items-center justify-between p-4 border-b">
            <div className="flex items-center gap-3">
              <HelpCircle size={20} className="text-gray-400" />
              <span className="text-gray-800">帮助中心</span>
            </div>
            <ChevronRight size={20} className="text-gray-400" />
          </button>
          
          <button className="w-full flex items-center justify-between p-4">
            <div className="flex items-center gap-3">
              <span className="text-gray-400 text-lg">ℹ️</span>
              <span className="text-gray-800">关于我们</span>
            </div>
            <ChevronRight size={20} className="text-gray-400" />
          </button>
        </div>
      </div>

      {/* Bottom Navigation */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <div className="flex justify-around py-3 max-w-[480px] mx-auto">
          <div className="flex flex-col items-center text-gray-400">
            <span>🏠</span>
            <span className="text-xs mt-1">首页</span>
          </div>
          <div className="flex flex-col items-center text-gray-400">
            <span>📋</span>
            <span className="text-xs mt-1">订单</span>
          </div>
          <div className="flex flex-col items-center text-orange-500">
            <span>👤</span>
            <span className="text-xs mt-1">我的</span>
          </div>
        </div>
      </div>
    </div>
  )
}