import { useEffect, useState, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'

export default function InstrumentList() {
  const [instruments, setInstruments] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [hasMore, setHasMore] = useState(true)
  const [page, setPage] = useState(1)
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'
  
  // Filters from URL params
  const category = searchParams.get('category')
  const brand = searchParams.get('brand')
  const minPrice = searchParams.get('minPrice')
  const maxPrice = searchParams.get('maxPrice')
  const stockStatus = searchParams.get('stock')
  const sort = searchParams.get('sort') || 'newest'

  const fetchInstruments = useCallback(async (pageNum = 1) => {
    try {
      setLoading(true)
      setError(null)
      
      // Build query params
      const params = new URLSearchParams()
      params.append('page', pageNum.toString())
      params.append('per_page', '20')
      
      if (category) params.append('category', category)
      if (brand) params.append('brand', brand)
      if (minPrice) params.append('min_price', minPrice)
      if (maxPrice) params.append('max_price', maxPrice)
      if (stockStatus) params.append('stock_status', stockStatus)
      if (sort) params.append('sort', sort)
      
      const response = await fetch(`${API_BASE_URL}/instruments?${params.toString()}`)
      
      if (!response.ok) throw new Error('Failed to fetch instruments')
      
      const data = await response.json()
      
      if (data.code === 20000) {
        const newInstruments = data.data || []
        
        if (pageNum === 1) {
          setInstruments(newInstruments)
        } else {
          setInstruments(prev => [...prev, ...newInstruments])
        }
        
        setHasMore(newInstruments.length >= 20)
      } else {
        throw new Error(data.message || 'API error')
      }
    } catch (err) {
      setError(err.message)
      // Fallback demo data
      if (pageNum === 1) {
        setInstruments([
          { id: 1, name: '雅马哈钢琴 U1', brand: '雅马哈', price: 150, deposit: 3000, stock: 5, image: '/images/piano1.jpg', rent_type: 'daily' },
          { id: 2, name: '卡马吉他 D1C', brand: '卡马', price: 50, deposit: 1000, stock: 10, image: '/images/guitar1.jpg', rent_type: 'daily' },
          { id: 3, name: '敦煌古筝 696D', brand: '敦煌', price: 80, deposit: 2000, stock: 3, image: '/images/guzheng1.jpg', rent_type: 'daily' },
          { id: 4, name: '雅马哈小提琴 V5', brand: '雅马哈', price: 120, deposit: 2500, stock: 8, image: '/images/violin1.jpg', rent_type: 'daily' },
          { id: 5, name: '珠江钢琴 UP118', brand: '珠江', price: 100, deposit: 2200, stock: 12, image: '/images/piano2.jpg', rent_type: 'daily' }
        ])
      }
    } finally {
      setLoading(false)
    }
  }, [category, brand, minPrice, maxPrice, stockStatus, sort])
  
  // Initial load
  useEffect(() => {
    fetchInstruments(1)
    setPage(1)
  }, [fetchInstruments])
  
  // Infinite scroll
  useEffect(() => {
    const handleScroll = () => {
      if (loading || !hasMore) return
      
      const scrollPosition = window.innerHeight + window.scrollY
      const threshold = document.body.offsetHeight - 100
      
      if (scrollPosition >= threshold) {
        const nextPage = page + 1
        setPage(nextPage)
        fetchInstruments(nextPage)
      }
    }
    
    window.addEventListener('scroll', handleScroll)
    return () => window.removeEventListener('scroll', handleScroll)
  }, [loading, hasMore, page, fetchInstruments])
  
  const handleInstrumentClick = (instrument) => {
    if (instrument.stock <= 0) return
    navigate(`/instrument/${instrument.id}`)
  }
  
  const getRentTypeLabel = (type) => {
    const labels = { daily: '日租', weekly: '周租', monthly: '月租' }
    return labels[type] || '日租'
  }
  
  const getStockStatus = (stock) => {
    if (stock <= 0) return { text: '暂无', color: 'text-red-600' }
    if (stock <= 3) return { text: '少量', color: 'text-orange-600' }
    return { text: '有货', color: 'text-green-600' }
  }
  
  if (loading && instruments.length === 0) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center">
          <div className="text-lg mb-2">加载中...</div>
          <div className="text-gray-500">正在获取乐器信息</div>
        </div>
      </div>
    )
  }
  
  if (error && instruments.length === 0) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center text-red-600">
          <div className="text-lg mb-2">加载失败</div>
          <div className="text-sm">{error}</div>
          <button 
            className="mt-4 px-4 py-2 bg-blue-600 text-white rounded"
            onClick={() => fetchInstruments(1)}
          >
            重试
          </button>
        </div>
      </div>
    )
  }
  
  return (
    <div className="min-h-screen bg-gray-50">
      {/* Header */}
      <div className="bg-white shadow-sm p-4 sticky top-0 z-10">
        <h1 className="text-2xl font-bold text-gray-900">乐器列表</h1>
        <div className="flex gap-2 mt-3 overflow-x-auto pb-2">
          <button className="flex-shrink-0 px-3 py-1 bg-blue-600 text-white text-sm rounded-full">
            全部
          </button>
          <button className="flex-shrink-0 px-3 py-1 bg-gray-200 text-gray-700 text-sm rounded-full">
            钢琴
          </button>
          <button className="flex-shrink-0 px-3 py-1 bg-gray-200 text-gray-700 text-sm rounded-full">
            日租 < ¥100
          </button>
          <button className="flex-shrink-0 px-3 py-1 bg-gray-200 text-gray-700 text-sm rounded-full">
            有货
          </button>
        </div>
      </div>
      
      {/* Instrument List */}
      <div className="p-4">
        {instruments.length === 0 ? (
          <div className="flex items-center justify-center h-64">
            <div className="text-center text-gray-500">
              <div className="text-6xl mb-4">🎵</div>
              <div className="text-lg">暂无乐器</div>
              <div className="text-sm text-gray-400 mt-2">请调整筛选条件或稍后再试</div>
            </div>
          </div>
        ) : (
          <div className="space-y-4">
            {instruments.map(instrument => {
              const stockStatus = getStockStatus(instrument.stock)
              
              return (
                <div
                  key={instrument.id}
                  className="bg-white rounded-lg p-4 cursor-pointer hover:shadow-md transition-shadow border border-gray-200"
                  onClick={() => handleInstrumentClick(instrument)}
                >
                  <div className="flex">
                    <div className="w-20 h-20 bg-gray-200 rounded mr-4 overflow-hidden">
                      {instrument.image ? (
                        <img src={instrument.image} alt={instrument.name} className="w-full h-full object-cover" />
                      ) : (
                        <div className="w-full h-full flex items-center justify-center text-4xl">🎼</div>
                      )}
                    </div>
                    <div className="flex-1">
                      <div className="flex justify-between items-start">
                        <div>
                          <div className="font-bold text-gray-900 text-lg">{instrument.name}</div>
                          <div className="text-sm text-gray-600">{instrument.brand}</div>
                        </div>
                        <span className={`text-sm font-medium ${stockStatus.color}`}>
                          {stockStatus.text}
                        </span>
                      </div>
                      
                      <div className="mt-3 flex items-center justify-between">
                        <div>
                          <div className="text-xl font-bold text-blue-600">
                            ¥{instrument.price}/天
                          </div>
                          <div className="text-xs text-gray-500">
                            押金: ¥{instrument.deposit}
                          </div>
                        </div>
                        
                        <div className="text-right">
                          <div className="text-sm text-gray-600">
                            {getRentTypeLabel(instrument.rent_type)}
                          </div>
                          <div className="text-xs text-gray-500">
                            库存: {instrument.stock} 件
                          </div>
                        </div>
                      </div>
                    </div>
                  </div>
                </div>
              )
            })}
            
            {loading && (
              <div className="flex justify-center py-4">
                <div className="text-center text-gray-500">
                  <div className="text-sm">加载更多...</div>
                </div>
              </div>
            )}
            
            {!hasMore && instruments.length > 0 && (
              <div className="text-center text-gray-500 text-sm py-4">
                已显示所有乐器
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
