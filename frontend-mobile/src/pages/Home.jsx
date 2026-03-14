import { useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { instruments, categories } from '../data/mockData'
import { ChevronRight } from 'lucide-react'

function InstrumentCard({ instrument, onClick }) {
  return (
    <div 
      className="bg-white rounded-lg shadow-sm overflow-hidden cursor-pointer"
      onClick={onClick}
    >
      <img 
        src={instrument.image} 
        alt={instrument.name}
        className="w-full h-40 object-cover"
      />
      <div className="p-3">
        <h3 className="font-medium text-sm text-gray-800 truncate">{instrument.name}</h3>
        <p className="text-orange-500 font-bold mt-1">
          ¥{instrument.monthlyRent}/月
        </p>
        <p className="text-gray-500 text-xs mt-1">
          押金: ¥{instrument.deposit}
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
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <div className="bg-orange-500 text-white px-4 py-4">
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
                  ? 'bg-orange-500 text-white'
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
          <div className="flex flex-col items-center text-orange-500">
            <ChevronRight size={20} />
            <span className="text-xs mt-1">首页</span>
          </div>
        </div>
      </div>
    </div>
  )
}