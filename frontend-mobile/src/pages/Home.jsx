import { useState, useEffect, useCallback } from 'react'
import { useNavigate } from 'react-router-dom'
import { instrumentsApi, apiFetch } from '../services/api'
import { ChevronRight, Search, Heart } from 'lucide-react'

function InstrumentCard({ instrument, onClick, isFavorite, onToggleFavorite }) {
  // 调试：打印 images 数据
  console.log('[Image Debug] Instrument:', instrument.name, 'Images:', instrument.images)
  
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
             onError={(e) => {
               console.error('[Image Debug] Failed to load image:', instrument.images?.[0])
               e.target.src = '/placeholder.png'
             }}
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
           onError={(e) => {
             console.error('[Image Debug] Failed to load image:', instrument.images?.[0])
             e.target.src = '/placeholder.png'
           }}
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
  const [page, setPage] = useState(1)
  const [hasMore, setHasMore] = useState(true)
  const [loadingMore, setLoadingMore] = useState(false)

  const fetchInstruments = useCallback(async (pageNum = 1, append = false) => {
    console.log('[Infinite Scroll] Fetching page:', pageNum, 'append:', append)
    
    try {
      if (!append) setLoading(true)
      else setLoadingMore(true)
      
      const response = await apiFetch(`${import.meta.env.VITE_API_BASE_URL || '/api'}/instruments?page=${pageNum}&pageSize=20`)
      const result = await response.json()
      
      if (result.code === 20000) {
        const newData = result.data || []
        console.log('[Infinite Scroll] Received', newData.length, 'items')
        console.log('[Infinite Scroll] Pagination:', result.pagination)
        
        if (append) {
          setInstruments(prev => [...prev, ...newData])
        } else {
          setInstruments(newData)
        }
        
        const pagination = result.pagination
        if (pagination) {
          console.log('[Infinite Scroll] Total pages:', pagination.totalPages, 'Current page:', pageNum)
          setHasMore(pageNum < pagination.totalPages)
        } else if (newData.length < 20) {
          console.log('[Infinite Scroll] Less than 20 items, no more data')
          setHasMore(false)
        }
      }
      
      // Update categories only on initial load
      if (pageNum === 1) {
        const uniqueCategories = ["全部", ...new Set(
          (result.data || []).map(i => i.category).filter(cat => cat)
        )]
        setCategories(uniqueCategories)
      }
      
      if (!append) setLoading(false)
      else setLoadingMore(false)
    } catch (error) {
      console.error('[Infinite Scroll] Failed to fetch instruments:', error)
      setLoading(false)
      setLoadingMore(false)
    }
  }, [])

  useEffect(() => {
    fetchInstruments(1, false)
  }, [fetchInstruments])

  // Intersection Observer for infinite scroll
  useEffect(() => {
    const observer = new IntersectionObserver(
      (entries) => {
        if (entries[0].isIntersecting && hasMore && !loadingMore) {
          console.log('[Infinite Scroll] Sentinel intersected, loading page:', page + 1)
          setPage(prev => prev + 1)
        }
      },
      { 
        threshold: 0.1,
        rootMargin: '200px'
      }
    )

    const sentinel = document.getElementById('scroll-sentinel')
    if (sentinel) {
      console.log('[Infinite Scroll] Observer attached to sentinel, hasMore:', hasMore)
      observer.observe(sentinel)
    } else {
      console.log('[Infinite Scroll] Sentinel not found')
    }

    return () => observer.disconnect()
  }, [hasMore, loadingMore, page])

  // Load more when page changes
  useEffect(() => {
    if (page > 1) {
      console.log('[Infinite Scroll] Page changed to:', page, 'fetching more...')
      fetchInstruments(page, true)
    }
  }, [page, fetchInstruments])

  // Scroll event fallback for mobile/WeChat
  useEffect(() => {
    const handleScroll = () => {
      if (!hasMore || loadingMore) return
      
      const scrollTop = window.scrollY || document.documentElement.scrollTop
      const scrollHeight = document.documentElement.scrollHeight
      const clientHeight = window.innerHeight
      
      if (scrollTop + clientHeight >= scrollHeight - 200) {
        console.log('[Infinite Scroll] Scroll event triggered')
        setPage(prev => prev + 1)
      }
    }

    window.addEventListener('scroll', handleScroll)
    return () => window.removeEventListener('scroll', handleScroll)
  }, [hasMore, loadingMore])

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
                onClick={() => {
                  setActiveCategory(cat)
                  setPage(1)
                  setHasMore(true)
                }}
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
        ) : (
          <>
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
            
            {/* Loading indicator or sentinel for infinite scroll */}
            {loadingMore ? (
              <div className="text-center py-4 text-gray-500 col-span-2">加载中...</div>
            ) : hasMore ? (
              <div id="scroll-sentinel" className="h-40 col-span-2"></div>
            ) : (
              <div className="text-center py-4 text-gray-500 col-span-2">暂无更多乐器</div>
            )}
          </>
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