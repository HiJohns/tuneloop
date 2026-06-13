import { useState, useEffect, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { instrumentsApi, apiFetch, getToken, redirectToLogin } from '../services/api'
import { ChevronRight, Search, Heart, ShoppingCart } from 'lucide-react'
import { env, storage } from '../platform'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

function parseImages(images) {
  if (!images) return []
  if (Array.isArray(images)) return images
  if (typeof images === 'string') {
    try { return JSON.parse(images) } catch { return [] }
  }
  return []
}

function parsePricing(pricing) {
  if (!pricing) return {}
  if (typeof pricing === 'object') return pricing
  if (typeof pricing === 'string') {
    try { return JSON.parse(pricing) } catch { return {} }
  }
  return {}
}

function InstrumentCard({ instrument, onClick, isFavorite, onToggleFavorite }) {
  // Safe parse JSON images and pricing
  const images = parseImages(instrument.images)
  const pricing = parsePricing(instrument.pricing)
  const dailyRent = (Array.isArray(pricing) ? pricing[0]?.daily_rent : pricing.daily_rent) || instrument.base_daily_rate || 0
  const monthlyRent = Math.round(dailyRent * 25)
  const weeklyRent = Math.round(dailyRent * 6)
  const rawDeposit = Array.isArray(pricing) ? pricing[0]?.deposit : pricing?.deposit
  const fallbackDeposit = dailyRent * 2
  const displayDeposit = rawDeposit || fallbackDeposit || 0
  const isAvailable = instrument.stock_status === 'available'

  const handleFavoriteClick = (e) => {
    e.stopPropagation()
    onToggleFavorite(instrument.id)
  }
  
  return (
    <View 
      className="bg-white rounded-xl shadow-sm border border-gray-100 overflow-hidden cursor-pointer active:scale-95 transition-transform"
      onClick={onClick}
    >
        <View className="relative">
          <Image 
            src={images[0] || PLACEHOLDER_IMAGE}
            className="w-full h-40 object-contain bg-gray-100 rounded-xl"
          />
          <View className="absolute top-2 left-2 flex gap-1">
            {isAvailable ? (
              <Text className="bg-green-500 text-white text-xs px-2 py-0.5 rounded">可租</Text>
            ) : (
              <Text className="bg-gray-400 text-white text-xs px-2 py-0.5 rounded">已租</Text>
            )}
            {dailyRent > 0 && (
              <Text className="bg-brand-primary text-white text-xs px-2 py-0.5 rounded">特惠</Text>
            )}
          </View>
         <Button
           onClick={handleFavoriteClick}
           className="absolute top-2 right-2 text-white bg-black/30 rounded-full p-1"
         >
           <Heart size={16} fill={isFavorite ? "red" : "none"} color={isFavorite ? "red" : "white"} />
         </Button>
       </View>
       <View className="p-3">
         <Text className="font-bold text-base text-brand-text truncate">{instrument.name}</Text>
         <Text className="text-brand-primary text-lg font-bold">
           ¥{monthlyRent}<Text className="text-brand-unit text-sm">/月</Text>
         </Text>
         <Text className="text-gray-500 text-sm">
            押金: ¥{displayDeposit}
         </Text>
       </View>
    </View>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const tenant = searchParams.get('tenant')
  const [activeCategory, setActiveCategory] = useState("全部")
  const [loading, setLoading] = useState(true)
  const [favorites, setFavorites] = useState([])
  const [, setForceUpdate] = useState(0)
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
      
      const baseUrl = env.apiBaseUrl
      const endpoint = '/public/instruments'
      const response = await apiFetch(`${baseUrl}${endpoint}?page=${pageNum}&pageSize=20${tenant ? `&tenant=${tenant}` : ''}`)
      const result = await response.json()
      
       if (result.code === 20000) {
        let newData = (result.data?.list || []).filter(i => i.stock_status !== 'archived' && i.stock_status !== 'lost')
        console.log('[Infinite Scroll] Received', newData.length, 'items')
        console.log('[Infinite Scroll] Pagination:', result.data?.pagination)
        
        if (append) {
          setInstruments(prev => [...prev, ...newData])
        } else {
          setInstruments(newData)
        }
        
        const pagination = result.data?.pagination
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
          (result.data?.list || []).map(i => i.category_name || i.category).filter(cat => cat)
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
  }, [tenant])

  useEffect(() => {
    fetchInstruments(1, false)
  }, [fetchInstruments])

  // Listen for cart updates to refresh floating icon
  useEffect(() => {
    const handleCartUpdate = () => forceUpdate(prev => prev + 1)
    window.addEventListener('cartUpdated', handleCartUpdate)
    return () => window.removeEventListener('cartUpdated', handleCartUpdate)
  }, [])

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
    : instruments.filter(i => (i.category_name || i.category) === activeCategory)

  return (
    <View className="min-h-screen bg-brand-bg">
      {/* Header */}
      <View className="bg-brand-primary text-white px-4 py-4 flex justify-between items-center">
        <View>
          <Text className="text-lg font-bold">乐器租赁</Text>
          <Text className="text-sm opacity-90">精品乐器 轻松租回家</Text>
        </View>
        <Button className="text-white">
          <Search size={20} />
        </Button>
      </View>

      {/* Category Tabs */}
      <ScrollView className="bg-white border-b" scrollX>
        <View className="flex px-4 py-3 gap-4">
          {categories.map((cat, index) => {
            const icons = {
              "钢琴": "🎹",
              "吉他": "🎸", 
              "古筝": "🎻",
              "提琴": "🎻",
              "全部": "全部"
            }
            return (
              <Button
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
              </Button>
            )
          })}
        </View>
      </ScrollView>

      {/* Instrument Grid */}
      <View className="p-4">
        {loading ? (
          <View className="grid grid-cols-2 gap-4">
            {Array(6).fill(0).map((_, i) => (
              <View key={i} className="bg-white rounded-xl overflow-hidden">
                <View className="w-full h-40 bg-gray-200 animate-pulse"></View>
                <View className="p-3 space-y-2">
                  <View className="h-4 bg-gray-200 rounded w-3/4 animate-pulse"></View>
                  <View className="h-4 bg-gray-200 rounded w-1/2 animate-pulse"></View>
                </View>
              </View>
            ))}
          </View>
        ) : (
          <>
            <View className="grid grid-cols-2 gap-4">
              {filteredInstruments.map(instrument => (
                <InstrumentCard
                  key={instrument.id}
                  instrument={instrument}
                  onClick={() => navigate(`/instrument/${instrument.id}`)}
                  isFavorite={favorites.includes(instrument.id)}
                  onToggleFavorite={toggleFavorite}
                />
              ))}
            </View>
            
            {/* Loading indicator or sentinel for infinite scroll */}
            {loadingMore ? (
              <View className="text-center py-4 text-gray-500 col-span-2">加载中...</View>
            ) : hasMore ? (
              <View id="scroll-sentinel" className="h-40 col-span-2"></View>
            ) : (
              <View className="text-center py-4 text-gray-500 col-span-2">暂无更多乐器</View>
            )}
          </>
        )}
      </View>

      {/* Toast */}
      {toast.visible && (
        <View className="fixed top-1/2 left-1/2 transform -translate-x-1/2 -translate-y-1/2 bg-black/80 text-white px-4 py-2 rounded">
          {toast.message}
        </View>
      )}

      {/* Floating Cart Icon */}
      {(() => {
        try {
          const cartData = storage.getJSON('cart', {items: []})
          const cartCount = cartData.items?.length || 0
          if (cartCount > 0) {
            return (
              <Button
                onClick={() => navigate('/cart')}
                className="fixed bottom-24 right-4 bg-brand-primary text-white p-3 rounded-full shadow-lg z-50"
              >
                <ShoppingCart size={24} />
                <Text className="absolute -top-1 -right-1 bg-red-500 text-white text-xs w-5 h-5 rounded-full flex items-center justify-center font-bold">
                  {cartCount}
                </Text>
              </Button>
            )
          }
          return null
        } catch {
          return null
        }
      })()}

      {/* Bottom Navigation */}
      <View className="fixed bottom-0 left-0 right-0 bg-white border-t safe-area-pb">
        <View className="flex justify-around py-3 max-w-[480px] mx-auto">
          <View 
            className="flex flex-col items-center text-brand-primary cursor-pointer"
            onClick={() => navigate('/')}
          >
            <Text className="text-xl">🏠</Text>
            <Text className="text-xs mt-1">首页</Text>
          </View>
          <View 
            className="flex flex-col items-center text-gray-400 cursor-pointer"
            onClick={() => navigate('/service')}
          >
            <Text className="text-xl">🔧</Text>
            <Text className="text-xs mt-1">维修</Text>
          </View>
          {getToken() ? (
            <View 
              className="flex flex-col items-center text-gray-400 cursor-pointer"
              onClick={() => navigate('/profile')}
            >
              <Text className="text-xl">👤</Text>
              <Text className="text-xs mt-1">我的</Text>
            </View>
          ) : (
            <View 
              className="flex flex-col items-center text-brand-primary cursor-pointer"
              onClick={() => redirectToLogin()}
            >
              <Text className="text-xl">🔑</Text>
              <Text className="text-xs mt-1">登录</Text>
            </View>
          )}
        </View>
      </View>
    </View>
  )
}