import { useNavigate } from 'react-router-dom'
import { myAssets, myLeases } from '../data/mockData'
import { User, MapPin, Bell, HelpCircle, ChevronRight, Wrench, RefreshCw } from 'lucide-react'
import { Badge, Tag } from 'antd'

function LeaseCard({ lease, onRenew }) {
  const isUrgent = lease.status === 'urgent'
  
  return (
    <div className={`${isUrgent ? 'bg-white rounded-xl shadow-lg border border-red-200 p-4' : 'bg-white rounded-xl shadow-sm p-4'}`}>
      <div className="flex gap-3 mb-4">
        <img 
          src={lease.image} 
          alt={lease.instrumentName}
          className="w-20 h-20 object-cover rounded-lg"
        />
        <div className="flex-1">
          <div className="flex items-center gap-2 mb-1">
            <h3 className="font-medium">{lease.instrumentName}</h3>
            {isUrgent && <Badge count={lease.daysLeft} overflowCount={7} style={{ backgroundColor: '#ff4d4f' }} />}
          </div>
          <p className="text-gray-500 text-sm">
            租期: {lease.startDate} 至 {lease.endDate}
          </p>
          {isUrgent && (
            <p className="text-red-500 text-xs font-medium mt-1">
              {lease.daysLeft}天后到期
            </p>
          )}
        </div>
      </div>
      
      {isUrgent ? (
        <div className="flex items-center gap-2">
          <Tag color="orange">{lease.daysLeft}天后到期</Tag>
          <button
            onClick={onRenew}
            className="flex-1 bg-brand-primary text-white py-2 rounded-lg font-medium text-sm"
          >
            一键续租
          </button>
        </div>
      ) : (
        <Tag color="green">租约正常</Tag>
      )}
    </div>
  )
}

export default function Profile() {
  const navigate = useNavigate()

  return (
    <div className="min-h-screen bg-brand-bg pb-20">
      {/* Header */}
      <div className="bg-brand-primary text-white px-4 py-6">
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

      {/* My Leases */}
      <div className="p-4">
        <h2 className="font-medium text-gray-800 mb-3">我的租约</h2>
        <div className="space-y-4">
          {myLeases.map(lease => (
            <LeaseCard
              key={lease.id}
              lease={lease}
              onRenew={() => alert("续租功能开发中...")}
            />
          ))}
        </div>
      </div>

      {/* Common Functions */}
      <div className="p-4 space-y-2">
        <h2 className="font-medium text-gray-800 mb-3">常用功能</h2>
        <div className="bg-white rounded-lg p-4">
          <div className="grid grid-cols-4 gap-4">
            <button className="flex flex-col items-center p-2">
              <MapPin size={24} className="text-brand-primary" />
              <span className="text-xs mt-1 text-gray-600">地址管理</span>
            </button>
            <button className="flex flex-col items-center p-2">
              <Bell size={24} className="text-brand-primary" />
              <span className="text-xs mt-1 text-gray-600">消息通知</span>
            </button>
            <button className="flex flex-col items-center p-2">
              <HelpCircle size={24} className="text-brand-primary" />
              <span className="text-xs mt-1 text-gray-600">帮助中心</span>
            </button>
            <button className="flex flex-col items-center p-2">
              <span className="text-xl">ℹ️</span>
              <span className="text-xs mt-1 text-gray-600">关于我们</span>
            </button>
          </div>
        </div>
      </div>

      {/* Bottom Navigation */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <div className="flex justify-around py-3 max-w-[480px] mx-auto">
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/')}
          >
            <span>🏠</span>
            <span className="text-xs mt-1">首页</span>
          </div>
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <span>🔧</span>
            <span className="text-xs mt-1">维修</span>
          </div>
          <div className="flex flex-col items-center text-brand-primary">
            <span>👤</span>
            <span className="text-xs mt-1">我的</span>
          </div>
        </div>
      </div>
    </div>
  )
}