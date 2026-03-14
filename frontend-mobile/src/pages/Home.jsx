import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { instruments, categories } from '../data/mockData'
import { ChevronRight } from 'lucide-react'

function InstrumentCard({ instrument, onClick }) {
  const defaultLevel = instrument.levels[0]
  return (
    <div 
      className="bg-white rounded-xl shadow-lg border border-gray-100 overflow-hidden cursor-pointer"
      onClick={onClick}
    >
      <img 
        src={instrument.image} 
        alt={instrument.name}
        className="w-full h-40 object-cover"
      />
      <div className="p-3">
        <h3 className="font-medium text-sm text-brand-text truncate">{instrument.name}</h3>
        <p className="text-brand-primary text-xl font-bold mt-1">
          ¥{defaultLevel.monthlyRent}<span className="text-brand-unit text-sm">/月</span>
        </p>
        <p className="text-gray-500 text-xs mt-1">
          押金: ¥{defaultLevel.deposit}
        </p>
      </div>
    </div>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [activeCategory, setActiveCategory] = useState("全部")

  const filteredInstruments = activeCategory === "全部" 
    ? instruments 
    : instruments.filter(i => i.category === activeCategory)

  return (
    <div className="min-h-screen bg-brand-bg">
      {/* Header */}
      <div className="bg-brand-primary text-white px-4 py-4">
        <h1 className="text-lg font-bold">乐器租赁</h1>
        <p className="text-sm opacity-90">精品乐器 轻松租回家</p>
      </div>

      {/* Category Tabs */}
      <div className="bg-white border-b overflow-x-auto">
        <div className="flex px-4 py-3 gap-4">
          {categories.map(cat => (
            <button
              key={cat}
              onClick={() => setActiveCategory(cat)}
              className={`whitespace-nowrap px-3 py-1.5 rounded-full text-sm font-medium transition-colors ${
                activeCategory === cat
                  ? 'bg-brand-primary text-white'
                  : 'bg-gray-100 text-gray-600'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      </div>

      {/* Instrument Grid */}
      <div className="p-4">
        <div className="grid grid-cols-2 gap-4">
          {filteredInstruments.map(instrument => (
            <InstrumentCard
              key={instrument.id}
              instrument={instrument}
              onClick={() => navigate(`/instrument/${instrument.id}`)}
            />
          ))}
        </div>
      </div>

      {/* Bottom Navigation */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <div className="flex justify-around py-3 max-w-[480px] mx-auto">
          <div 
            className="flex flex-col items-center text-brand-primary cursor-pointer"
            onClick={() => navigate('/')}
          >
            <span className="text-xl">🏠</span>
            <span className="text-xs mt-1">首页</span>
          </div>
          <div 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
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