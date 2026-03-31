import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { instrumentsApi } from '../services/api'
import { ChevronRight, Search, Heart } from 'lucide-react'

function InstrumentCard({ instrument, onClick, isFavorite, onToggleFavorite }) {
  // 安全检查: 确保 levels 存在且有数据
  if (!instrument.levels || !instrument.levels.length) {
    return (
      <div 
        className="bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden cursor-pointer active:scale-95 transition-transform"
        onClick={onClick}
      >
        <div className="relative">
           <img 
             src={instrument.images?.[0] || '/placeholder.png'} 
             alt={instrument.name}
             className="w-full h-40 object-contain bg-gray-100 rounded-xl"
           />
        </div>
        <div className="p-3">
          <h3 className="font-bold text-base text-brand-text truncate">{instrument.name}</h3>
          <p className="text-gray-400 text-sm">暂无报价</p>
        </div>
      </div>
    )
  }
  
  const defaultLevel = instrument.levels[0]
  const firstPayment = defaultLevel.monthlyRent + defaultLevel.deposit
  const promotionTag = defaultLevel.name === "大师级" ? "限量" : 
                       defaultLevel.name === "入门级" ? "热销" : ""
   
  const handleFavoriteClick = (e) => {
    e.stopPropagation()
    onToggleFavorite(instrument.id)
  }
  
  return (
    <div 
      className="bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden cursor-pointer active:scale-95 transition-transform"
      onClick={onClick}
    >
      <div className="relative">
         <img 
           src={instrument.images?.[0] || '/placeholder.png'} 
           alt={instrument.name}
           className="w-full h-40 object-contain bg-gray-100 rounded-xl"
         />
        {promotionTag && (
          <div className="absolute top-2 left-2 bg-brand-primary text-white text-xs px-2 py-1 rounded">
            {promotionTag}
          </div>
        )}
        <button
          onClick={handleFavoriteClick}
          className="absolute top-2 right-2 text-white bg-black/30 rounded-full p-1"
        >
          <Heart size={16} fill={isFavorite ? "red" : "none"} color={isFavorite ? "red" : "white"} />
        </button>
      </div>
      <div className="p-3">
        <h3 className="font-bold text-base text-brand-text truncate">{instrument.name}</h3>
        <p className="text-brand-primary text-lg font-bold">
          ¥{defaultLevel.monthlyRent}<span className="text-brand-unit text-sm">/月</span>
        </p>
        <p className="text-gray-500 text-sm">
          押金: ¥{defaultLevel.deposit}
        </p>
        <p className="text-gray-400 text-xs">
          首期实付 ¥{firstPayment} (含押金)
        </p>
      </div>
    </div>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [activeCategory, setActiveCategory] = useState("全部")
  const [loading, setLoading] = useState(true)
  const [favorites, setFavorites] = useState([])
  const [toast, setToast] = useState({ visible: false, message: "" })
  const [instruments, setInstruments] = useState([])
  const [categories, setCategories] = useState(["全部"])

  useEffect(() => {
    const fetchInstruments = async () => {
      try {
        setLoading(true)
        const data = await instrumentsApi.list()
        setInstruments(data)
        
        const uniqueCategories = ["全部", ...new Set(
          data.map(i => i.category).filter(cat => cat)  // 过滤空值
        )]
        setCategories(uniqueCategories)
        
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch instruments:', error)
        setLoading(false)
      }
    }
    
    fetchInstruments()
  }, [])

  const toggleFavorite = (instrumentId) => {
    setFavorites(prev => {
      const newFavorites = prev.includes(instrumentId)
        ? prev.filter(id => id !== instrumentId)
        : [...prev, instrumentId]
      
      setToast({
        visible: true,
        message: newFavorites.includes(instrumentId) ? "已加入我的收藏" : "已取消收藏"
      })
      
      // Auto hide toast
      setTimeout(() => setToast({ visible: false, message: "" }), 2000)
      
      return newFavorites
    })
  }

  const filteredInstruments = activeCategory === "全部" 
    ? instruments 
    : instruments.filter(i => i.category === activeCategory)

  return (
    <div className="min-h-screen bg-brand-bg">
      {/* Header */}
      <div className="bg-brand-primary text-white px-4 py-4 flex justify-between items-center">
        <div>
          <h1 className="text-lg font-bold">乐器租赁</h1>
          <p className="text-sm opacity-90">精品乐器 轻松租回家</p>
        </div>
        <button className="text-white">
          <Search size={20} />
        </button>
      </div>

      {/* Category Tabs */}
      <div className="bg-white border-b overflow-x-auto">
        <div className="flex px-4 py-3 gap-4">
          {categories.map((cat, index) => {
            const icons = {
              "钢琴": "🎹",
              "吉他": "🎸", 
              "古筝": "🎻",
              "提琴": "🎻",
              "全部": "全部"
            }
            return (
              <button
                key={cat || `category-${index}`}
                onClick={() => setActiveCategory(cat)}
                className={`flex items-center gap-1 whitespace-nowrap px-3 py-1.5 rounded-full text-sm font-medium transition-all ${
                  activeCategory === cat
                    ? 'bg-brand-primary text-white transform scale-105 border-b-2 border-brand-primary'
                    : 'bg-gray-100 text-gray-600'
                }`}
              >
                {icons[cat] || ""} {cat}
              </button>
            )
          })}
        </div>
      </div>

      {/* Instrument Grid */}
      <div className="p-4">
        {loading ? (
          <div className="grid grid-cols-2 gap-4">
            {Array(6).fill(0).map((_, i) => (
              <div key={i} className="bg-white rounded-xl overflow-hidden">
                <div className="w-full h-40 bg-gray-200 animate-pulse"></div>
                <div className="p-3 space-y-2">
                  <div className="h-4 bg-gray-200 rounded w-3/4 animate-pulse"></div>
                  <div className="h-4 bg-gray-200 rounded w-1/2 animate-pulse"></div>
                </div>
              </div>
            ))}
          </div>
        ) : filteredInstruments.length === 0 ? (
          <div className="text-center py-8 text-gray-500">
            暂无更多乐器
          </div>
        ) : (
          <div className="grid grid-cols-2 gap-4">
            {filteredInstruments.map(instrument => (
              <InstrumentCard
                key={instrument.id}
                instrument={instrument}
                onClick={() => navigate(`/instrument/${instrument.id}`)}
                isFavorite={favorites.includes(instrument.id)}
                onToggleFavorite={toggleFavorite}
              />
            ))}
          </div>
        )}
      </div>

      {/* Toast */}
      {toast.visible && (
        <div className="fixed top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 bg-black/80 text-white px-4 py-2 rounded">
          {toast.message}
        </div>
      )}

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