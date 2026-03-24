import { useEffect, useState } from 'react'
import { useParams, useNavigate } from 'react-router-dom'

export default function InstrumentDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [selectedSpec, setSelectedSpec] = useState(null)
  const [currentImageIndex, setCurrentImageIndex] = useState(0)
  const [isImageZoomed, setIsImageZoomed] = useState(false)
  const [reviews, setReviews] = useState([])
  const [selectedDelivery, setSelectedDelivery] = useState('pickup')
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchInstrumentDetail()
    fetchReviews()
  }, [id])

  const fetchInstrumentDetail = async () => {
    try {
      setLoading(true)
      setError(null)
      
      const response = await fetch(`${API_BASE_URL}/instruments/${id}`)
      if (!response.ok) throw new Error('Failed to fetch instrument')
      
      const data = await response.json()
      
      if (data.code === 20000) {
        const instrumentData = data.data
        setInstrument(instrumentData)
        
        // Set default spec if available
        if (instrumentData.specs && instrumentData.specs.length > 0) {
          setSelectedSpec(instrumentData.specs[0])
        }
      } else {
        throw new Error(data.message || 'API error')
      }
    } catch (err) {
      setError(err.message)
      // Fallback demo data
      setInstrument({
        id,
        name: '雅马哈钢琴 U1',
        brand: '雅马哈',
        model: 'U1',
        material: '实木',
        size: '121cm',
        suitable: '初学者到高级玩家',
        description: '专业级立式钢琴，采用优质实木制作，音色纯正，适合从初学者到专业演奏者的各个阶段使用。',
        images: [
          '/images/piano1.jpg',
          '/images/piano2.jpg',
          '/images/piano3.jpg'
        ],
        video: '/videos/piano-demo.mp4',
        specs: [
          {
            id: 1,
            name: '标准版',
            daily_rent: 150,
            weekly_rent: 900, // 150 * 6
            monthly_rent: 3750, // 150 * 25
            deposit: 3000,
            stock: 5
          },
          {
            id: 2,
            name: '专业版',
            daily_rent: 180,
            weekly_rent: 1080, // 180 * 6
            monthly_rent: 4500, // 180 * 25
            deposit: 3500,
            stock: 3
          }
        ],
        rating: 4.8,
        review_count: 128,
        delivery_options: [
          { type: 'pickup', name: '门店自提', fee: 0 },
          { type: 'delivery', name: '送货上门', fee: 50 }
        ]
      })
      
      // Set default spec
      const defaultSpec = {
        id: 1,
        name: '标准版',
        daily_rent: 150,
        weekly_rent: 900,
        monthly_rent: 3750,
        deposit: 3000,
        stock: 5
      }
      setSelectedSpec(defaultSpec)
    } finally {
      setLoading(false)
    }
  }

  const fetchReviews = async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/instruments/${id}/reviews`)
      if (!response.ok) throw new Error('Failed to fetch reviews')
      
      const data = await response.json()
      
      if (data.code === 20000) {
        setReviews(data.data || [])
      } else {
        setReviews([
          {
            id: 1,
            user_name: '张先生',
            rating: 5,
            comment: '钢琴音色很好，孩子很喜欢，租赁流程也很方便。',
            images: ['/review1.jpg'],
            created_at: '2026-03-20'
          },
          {
            id: 2,
            user_name: '李女士',
            rating: 4,
            comment: '整体不错，就是送货时间有点长。',
            images: [],
            created_at: '2026-03-18'
          }
        ])
      }
    } catch (err) {
      console.error('Failed to load reviews:', err)
      setReviews([
        {
          id: 1,
          user_name: '演示用户',
          rating: 5,
          comment: '这是一段演示评价内容。',
          images: [],
          created_at: '2026-03-24'
        }
      ])
    }
  }

  const handleSpecChange = (spec) => {
    setSelectedSpec(spec)
  }

  const handleRentClick = () => {
    if (!instrument || !selectedSpec) return
    
    const params = new URLSearchParams()
    params.append('instrument_id', instrument.id)
    params.append('spec_id', selectedSpec.id)
    params.append('delivery', selectedDelivery)
    
    navigate(`/checkout?${params.toString()}`)
  }

  const handleImageClick = (index) => {
    setCurrentImageIndex(index)
    setIsImageZoomed(true)
  }

  const handleCloseZoom = () => {
    setIsImageZoomed(false)
  }

  const nextImage = () => {
    if (instrument?.images) {
      setCurrentImageIndex((prev) => (prev + 1) % instrument.images.length)
    }
  }

  const prevImage = () => {
    if (instrument?.images) {
      setCurrentImageIndex((prev) => (prev - 1 + instrument.images.length) % instrument.images.length)
    }
  }

  const getStockStatus = (stock) => {
    if (stock <= 0) return { text: '暂无可租', color: 'bg-red-100 text-red-800' }
    if (stock <= 3) return { text: '少量', color: 'bg-orange-100 text-orange-800' }
    return { text: '有货', color: 'bg-green-100 text-green-800' }
  }

  const formatPrice = (price) => {
    return `¥${price}`
  }

  const renderStars = (rating) => {
    const fullStars = Math.floor(rating)
    const hasHalfStar = rating % 1 !== 0
    const stars = []
    
    for (let i = 0; i < fullStars; i++) {
      stars.push(<span key={i} className="text-yellow-400">★</span>)
    }
    
    if (hasHalfStar) {
      stars.push(<span key="half" className="text-yellow-400">☆</span>)
    }
    
    const emptyStars = 5 - Math.ceil(rating)
    for (let i = 0; i < emptyStars; i++) {
      stars.push(<span key={`empty-${i}`} className="text-gray-300">★</span>)
    }
    
    return stars
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center">
          <div className="text-lg mb-2">加载中...</div>
          <div className="text-gray-500">正在获取乐器详情</div>
        </div>
      </div>
    )
  }

  if (error && !instrument) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center text-red-600">
          <div className="text-lg mb-2">加载失败</div>
          <div className="text-sm">{error}</div>
          <button 
            className="mt-4 px-4 py-2 bg-blue-600 text-white rounded"
            onClick={() => window.location.reload()}
          >
            重试
          </button>
        </div>
      </div>
    )
  }

  return (
    <div className="min-h-screen bg-gray-50 pb-20">
      {/* Image Gallery */}
      <div className="relative bg-white">
        <div className="relative h-80 overflow-hidden">
          {instrument?.images && instrument.images.length > 0 ? (
            <img
              src={instrument.images[currentImageIndex]}
              alt={instrument.name}
              className="w-full h-full object-cover"
              onClick={() => handleImageClick(currentImageIndex)}
            />
          ) : (
            <div className="w-full h-full flex items-center justify-center bg-gray-200">
              <div className="text-6xl">🎼</div>
            </div>
          )}
          
          {/* Image Indicators */}
          {instrument?.images && instrument.images.length > 1 && (
            <div className="absolute bottom-4 left-0 right-0 flex justify-center space-x-2">
              {instrument.images.map((_, index) => (
                <div
                  key={index}
                  className={`w-2 h-2 rounded-full ${
                    index === currentImageIndex ? 'bg-white' : 'bg-white/50'
                  }`}
                  onClick={() => setCurrentImageIndex(index)}
                />
              ))}
            </div>
          )}
        </div>
        
        {/* Back Button */}
        <button 
          className="absolute top-4 left-4 p-2 bg-black/50 text-white rounded-full"
          onClick={() => navigate(-1)}
        >
          ←
        </button>
      </div>

      {/* Main Content */}
      <div className="p-4 space-y-4">
        {/* Basic Info */}
        <div className="bg-white rounded-lg p-4">
          <h1 className="text-2xl font-bold text-gray-900">{instrument?.name}</h1>
          <div className="text-lg text-gray-600 mt-1">
            {instrument?.brand} {instrument?.model}
          </div>
          
          {/* Rating */}
          {instrument?.rating > 0 && (
            <div className="flex items-center mt-2">
              <div className="flex">{renderStars(instrument.rating)}</div>
              <span className="ml-2 text-sm text-gray-600">
                {instrument.rating} ({instrument.review_count} 评价)
              </span>
            </div>
          )}
          
          {/* Description */}
          <p className="mt-3 text-gray-700 leading-relaxed">
            {instrument?.description}
          </p>
          
          {/* Specifications */}
          {instrument?.material && (
            <div className="mt-4 grid grid-cols-2 gap-4 text-sm">
              <div>
                <span className="text-gray-500">材质:</span>
                <span className="ml-2 font-medium">{instrument.material}</span>
              </div>
              <div>
                <span className="text-gray-500">尺寸:</span>
                <span className="ml-2 font-medium">{instrument.size}</span>
              </div>
              <div className="col-span-2">
                <span className="text-gray-500">适用人群:</span>
                <span className="ml-2 font-medium">{instrument.suitable}</span>
              </div>
            </div>
          )}
        </div>

        {/* Pricing and Specs */}
        {selectedSpec && (
          <div className="bg-white rounded-lg p-4">
            <h3 className="text-lg font-semibold text-gray-900 mb-3">规格与价格</h3>
            
            {instrument?.specs && instrument.specs.length > 1 && (
              <div className="space-y-2 mb-4">
                {instrument.specs.map(spec => (
                  <button
                    key={spec.id}
                    className={`w-full p-3 border rounded-lg text-left ${
                      spec.id === selectedSpec.id
                        ? 'border-blue-600 bg-blue-50'
                        : 'border-gray-300'
                    }`}
                    onClick={() => handleSpecChange(spec)}
                  >
                    <div className="font-medium">{spec.name}</div>
                    <div className="text-sm text-gray-600 mt-1">
                      日租: {formatPrice(spec.daily_rent)}
                    </div>
                  </button>
                ))}
              </div>
            )}
            
            <div className="space-y-3">
              {/* Daily Price */}
              <div className="flex justify-between items-center py-2 border-b border-gray-200">
                <div className="text-gray-600">日租价格</div>
                <div className="text-right">
                  <div className="text-2xl font-bold text-blue-600">
                    {formatPrice(selectedSpec.daily_rent)}
                  </div>
                </div>
              </div>
              
              {/* Weekly Price */}
              <div className="flex justify-between items-center py-2 border-b border-gray-200">
                <div className="text-gray-600">周租优惠<br/>
                  <span className="text-xs text-gray-500">({formatPrice(selectedSpec.daily_rent)} × 6)</span>
                </div>
                <div className="text-right">
                  <div className="text-xl font-semibold text-gray-900">
                    {formatPrice(selectedSpec.weekly_rent)}
                  </div>
                  <div className="text-xs text-green-600">周租省 {formatPrice(selectedSpec.daily_rent * 6 - selectedSpec.weekly_rent)}</div>
                </div>
              </div>
              
              {/* Monthly Price */}
              <div className="flex justify-between items-center py-2 border-b border-gray-200">
                <div className="text-gray-600">月租优惠<br/>
                  <span className="text-xs text-gray-500">({formatPrice(selectedSpec.daily_rent)} × 25)</span>
                </div>
                <div className="text-right">
                  <div className="text-xl font-semibold text-gray-900">
                    {formatPrice(selectedSpec.monthly_rent)}
                  </div>
                  <div className="text-xs text-green-600">月租省 {formatPrice(selectedSpec.daily_rent * 25 - selectedSpec.monthly_rent)}</div>
                </div>
              </div>
              
              {/* Deposit */}
              <div className="flex justify-between items-center py-2">
                <div className="text-gray-600">押金</div>
                <div className="text-lg font-semibold text-gray-900">
                  {formatPrice(selectedSpec.deposit)}
                </div>
              </div>
              
              {/* Stock Status */}
              <div className="flex justify-between items-center py-3 bg-gray-50 rounded-lg">
                <div className="text-gray-600">库存状态</div>
                <span className={`px-3 py-1 rounded-full text-sm font-medium ${getStockStatus(selectedSpec.stock).color}`}>
                  {getStockStatus(selectedSpec.stock).text}
                </span>
              </div>
            </div>
          </div>
        )}

        {/* Delivery Options */}
        <div className="bg-white rounded-lg p-4">
          <h3 className="text-lg font-semibold text-gray-900 mb-3">配送方式</h3>
          
          <div className="space-y-2">
            {instrument?.delivery_options?.map(option => (
              <label
                key={option.type}
                className="flex items-center p-3 border rounded-lg cursor-pointer hover:bg-gray-50"
              >
                <input
                  type="radio"
                  name="delivery"
                  value={option.type}
                  checked={selectedDelivery === option.type}
                  onChange={(e) => setSelectedDelivery(e.target.value)}
                  className="mr-3"
                />
                <div className="flex-1">
                  <div className="font-medium">{option.name}</div>
                  <div className="text-sm text-gray-600">
                    {option.type === 'pickup' ? '到店自取，无额外费用' : '送货上门，便捷快速'}
                  </div>
                </div>
                <div className="text-sm font-medium">
                  {option.fee === 0 ? '免费' : `+${formatPrice(option.fee)}`}
                </div>
              </label>
            ))}
          </div>
        </div>

        {/* Reviews */}
        {reviews.length > 0 && (
          <div className="bg-white rounded-lg p-4">
            <div className="flex justify-between items-center mb-3">
              <h3 className="text-lg font-semibold text-gray-900">用户评价</h3>
              <span className="text-sm text-gray-600">{reviews.length} 条评价</span>
            </div>
            
            <div className="space-y-4">
              {reviews.slice(0, 3).map(review => (
                <div key={review.id} className="border-b border-gray-200 pb-4 last:border-b-0">
                  <div className="flex items-center justify-between mb-2">
                    <div className="flex items-center">
                      <div className="w-8 h-8 bg-gray-300 rounded-full mr-3" />
                      <div>
                        <div className="font-medium">{review.user_name}</div>
                        <div className="flex">{renderStars(review.rating)}</div>
                      </div>
                    </div>
                    <div className="text-xs text-gray-500">{review.created_at}</div>
                  </div>
                  <p className="text-gray-700 text-sm">{review.comment}</p>
                  
                  {review.images && review.images.length > 0 && (
                    <div className="flex space-x-2 mt-3">
                      {review.images.map((img, index) => (
                        <img
                          key={index}
                          src={img}
                          alt={`评价图片${index + 1}`}
                          className="w-16 h-16 rounded object-cover"
                        />
                      ))}
                    </div>
                  )}
                </div>
              ))}
              
              {reviews.length > 3 && (
                <button
                  className="w-full py-2 text-center text-blue-600 text-sm"
                  onClick={() => navigate(`/instrument/${id}/reviews`)}
                >
                  查看全部 {reviews.length} 条评价 →
                </button>
              )}
            </div>
          </div>
        )}
      </div>

      {/* Image Zoom Modal */}
      {isImageZoomed && instrument?.images && (
        <div
          className="fixed inset-0 bg-black bg-opacity-90 z-50 flex items-center justify-center"
          onClick={handleCloseZoom}
        >
          <div className="relative max-w-full max-h-full">
            <img
              src={instrument.images[currentImageIndex]}
              alt="放大查看"
              className="max-w-full max-h-screen object-contain"
            />
            
            {/* Navigation */}
            {instrument.images.length > 1 && (
              <>
                <button
                  className="absolute left-4 top-1/2 transform -translate-y-1/2 p-2 bg-black/50 text-white rounded-full"
                  onClick={(e) => {
                    e.stopPropagation()
                    prevImage()
                  }}
                >
                  ←
                </button>
                <button
                  className="absolute right-4 top-1/2 transform -translate-y-1/2 p-2 bg-black/50 text-white rounded-full"
                  onClick={(e) => {
                    e.stopPropagation()
                    nextImage()
                  }}
                >
                  →
                </button>
              </>
            )}
            
            {/* Close button */}
            <button
              className="absolute top-4 right-4 p-2 bg-black/50 text-white rounded-full"
              onClick={handleCloseZoom}
            >
              ✕
            </button>
          </div>
        </div>
      )}

      {/* Bottom Action Bar */}
      <div className="fixed bottom-0 left-0 right-0 bg-white border-t border-gray-200 p-4">
        <div className="flex items-center justify-between mb-2">
          <div>
            <div className="text-sm text-gray-600">日租价格</div>
            <div className="text-2xl font-bold text-blue-600">
              {formatPrice(selectedSpec?.daily_rent || 0)}
            </div>
          </div>
          
          <button
            className={`px-6 py-3 rounded-lg font-semibold ${
              selectedSpec?.stock > 0
                ? 'bg-blue-600 text-white'
                : 'bg-gray-300 text-gray-500 cursor-not-allowed'
            }`}
            onClick={handleRentClick}
            disabled={!selectedSpec || selectedSpec.stock <= 0}
          >
            {selectedSpec?.stock > 0 ? '立即租赁' : '暂无库存'}
          </button>
        </div>
      </div>
    </div>
  )
}
