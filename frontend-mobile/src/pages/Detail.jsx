import { useState, useEffect, useCallback } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { instrumentsApi, getToken, apiFetch, redirectToLogin } from '../services/api'
import { ArrowLeft, Shield, Clock, AlertCircle, MapPin, Bell, CheckCircle, X, ShoppingCart } from 'lucide-react'
import { Switch, Tag, Modal, Button as AntButton } from 'antd'
import dayjs from 'dayjs'
import { env, storage, eventBus } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { View, Text, Image, Button, Video, ScrollView } from '@tarojs/components'

const SERVICE_ITEMS = [
  { name: '基础清洁', entry: '✓', professional: '✓', master: '✓' },
  { name: '免费调音', entry: '1次/年', professional: '2次/年', master: '无限次' },
  { name: '深度维护', entry: '✗', professional: '✓', master: '✓' },
  { name: '免费维修', entry: '✗', professional: '✓', master: '✓' },
  { name: '专家精调', entry: '✗', professional: '✗', master: '✓' },
  { name: '上门保养', entry: '✗', professional: '✗', master: '✓' },
]

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

export default function Detail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  
  const [selectedLevel, setSelectedLevel] = useState('专业级')
  const [days, setDays] = useState(30)
  const [noDeposit, setNoDeposit] = useState(false)
  const [showComparison, setShowComparison] = useState(false)
  const [userCreditScore] = useState(750)
  const [canUseDepositFree, setCanUseDepositFree] = useState(false)

  const [pricingV2, setPricingV2] = useState(null)
  const [pricingV2Loading, setPricingV2Loading] = useState(false)
  
  const [calculatedRent, setCalculatedRent] = useState(0)
  const [totalAmount, setTotalAmount] = useState(0)
  const [cartToast, setCartToast] = useState(false)
  const cartItemCount = (() => {
    try {
      const cartData = storage.getJSON('cart', {items: []})
      return cartData.items?.length || 0
    } catch { return 0 }
  })()
  
  // Load saved days from cart when instrument loads
  useEffect(() => {
    if (!instrument?.id) return
    try {
      const cart = storage.getJSON('cart', {items: []})
      const item = cart.items?.find(i => i.instrument_id === instrument.id)
      if (item?.days) {
        setDays(item.days)
      } else if (item?.end_date) {
        setDays(Math.max(dayjs(item.end_date).diff(dayjs().startOf('day'), 'day'), 1))
      }
    } catch {}
  }, [instrument?.id])
  const isInCart = (() => {
    if (!instrument) return false
    try {
      const cartData = storage.getJSON('cart', {items: []})
      return cartData.items?.some(i => i.instrument_id === instrument.id) || false
    } catch { return false }
  })()

  const handleAddToCart = () => {
    if (!instrument) return
    const existing = storage.getJSON('cart', {items: []})
    
    // Remove existing item for this instrument if present
    const filtered = existing.items.filter(i => i.instrument_id !== instrument.id)
    
    // Add new item with current lease terms
    const startDate = dayjs().format('YYYY-MM-DD')
    const returnDate = dayjs().add(days, 'day').format('YYYY-MM-DD')
    filtered.push({
      instrument_id: instrument.id,
      name: instrument.name,
      sn: instrument.sn,
      category_name: instrument.category_name,
      brand: instrument.brand,
      model: instrument.model,
      tenant_id: instrument.tenant_id,
      tenant_name: instrument.tenant_name || '',
      site_id: instrument.site_id,
      site_name: instrument.site_name,
      site_address: instrument.site_address || '',
      images: instrument.images,
      pricing: instrument.pricing,
      base_daily_rate: instrument.base_daily_rate,
      days: days,
      start_date: startDate,
      end_date: returnDate,
      pricing_v2: pricingV2,
      calculated_rent: totalRent,
    })
    
    storage.setJSON('cart', { items: filtered })
    setCartToast(true)
    eventBus.emit('cartUpdated')
  }

  const isRentable = instrument?.stock_status === 'available'
  const [activeOrder, setActiveOrder] = useState(null)

  useEffect(() => {
    const fetchInstrument = async () => {
      try {
        setLoading(true)
        const baseUrl = env.apiBaseUrl
        const endpoint = `/public/instruments/${id}`
        const response = await apiFetch(`${baseUrl}${endpoint}`)
        const result = await response.json()
        if (result.code === 20000) {
          const inst = result.data
          inst._parsedPricing = parsePricing(inst.pricing)
          setInstrument(inst)

          if (inst.sn) {
            const token = getToken()
            if (token) {
              try {
                const orderResp = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(inst.sn)}`)
                const orderResult = await orderResp.json()
                if (orderResult.code === 20000 && orderResult.data) {
                  setActiveOrder(orderResult.data)
                }
              } catch {}
            }
          }
        }
        setLoading(false)
      } catch (error) {
        console.error('Failed to fetch instrument:', error)
        setLoading(false)
      }
    }
    
    fetchInstrument()
  }, [id])

  useEffect(() => {
    if (!id) return
    const baseUrl = env.apiBaseUrl
    setPricingV2Loading(true)
    apiFetch(`${baseUrl}/public/instruments/${id}/pricing-v2`)
      .then(r => r.json())
      .then(res => {
        if (res.code === 20000) setPricingV2(res.data)
      }).catch(() => {
        // pricing-v2 not available, fallback to flat daily rate
      }).finally(() => {
        setPricingV2Loading(false)
      })
  }, [id])

  const [mediaPublic, setMediaPublic] = useState(null)

  useEffect(() => {
    if (!id) return
    const baseUrl = env.apiBaseUrl
    apiFetch(`${baseUrl}/public/instruments/${id}/media`)
      .then(r => r.json())
      .then(res => {
        if (res.code === 20000) setMediaPublic(res.data)
      }).catch(() => {
        // fallback to old instrument.images/video fields
      })
  }, [id])

  useEffect(() => {
    const creditScore = userCreditScore
    setCanUseDepositFree(creditScore >= 650)
  }, [userCreditScore])

  // Computed pricing values
  const pricing = parsePricing(instrument?.pricing)
  const dailyRent = pricing[0]?.daily_rent || instrument?.base_daily_rate || 0
  const deposit = pricing[0]?.deposit || pricingV2?.deposit || 0
  const shippingFee = pricing[0]?.shipping_fee || pricingV2?.shipping_fee || 0
  const overdueDailyFee = pricing[0]?.overdue_daily_fee || dailyRent

  const computeTieredRent = (pricingData, daysCount) => {
    if (!pricingData?.tiers?.length) {
      return (pricingData?.base_daily_rate || dailyRent) * daysCount
    }
    let remaining = daysCount
    let total = 0
    let prevMax = 0
    for (const tier of pricingData.tiers) {
      const tierDays = tier.days_max > 0 ? tier.days_max - prevMax : remaining
      const segDays = Math.min(tierDays, remaining)
      total += segDays * tier.daily_rate
      remaining -= segDays
      prevMax = tier.days_max
      if (remaining <= 0) break
    }
    return total
  }

  const totalRent = computeTieredRent(pricingV2, days)

  const calculatePrice = useCallback(() => {
    if (!instrument) return
    if (!pricingV2 && !dailyRent) return
    const total = totalRent + deposit + shippingFee
    setCalculatedRent(totalRent)
    setTotalAmount(total)
  }, [totalRent, deposit, shippingFee, pricingV2, dailyRent, instrument])

  useEffect(() => {
    calculatePrice()
  }, [calculatePrice])

  if (loading) {
    return <View className="p-4">加载中...</View>
  }

  if (!instrument) {
    return <View className="p-4">乐器不存在</View>
  }

  const publicImages = mediaPublic?.images?.length > 0 ? mediaPublic.images.map(i => i.url) : null
  const images = publicImages || parseImages(instrument.images)

  return (
    <View className="min-h-screen bg-gray-50 pb-24">
      <View className="relative">
         <Image 
           src={images[0] || PLACEHOLDER_IMAGE}
           className="w-full h-64 object-contain bg-gray-100"
         />
        {mediaPublic?.video && (
          <View className="mt-2 px-4">
            <Video
              src={mediaPublic.video.url}
              poster={mediaPublic.video.thumb_url}
              controls
              className="w-full rounded-lg"
              style={{ maxHeight: 240 }}
            />
          </View>
        )}
        <Button 
          onClick={() => navigate(-1)}
          className="absolute top-4 left-4 bg-black/30 text-white p-2 rounded-full"
        >
          <ArrowLeft size={20} />
        </Button>
      </View>

      <View className="bg-white p-4 pb-24">
        <Text className="text-xl font-bold text-gray-800">{instrument.name || instrument.category_name}</Text>
        {instrument.sn && <Text className="text-sm text-gray-400 mt-1">编号: {instrument.sn}</Text>}
        <Text className="text-gray-500 mt-1">{instrument.description}</Text>

        {/* Site/Merchant Info */}
        {(instrument.tenant_name || instrument.site_name) && (
          <View className="mt-3 p-3 bg-gray-50 rounded-lg">
            {instrument.tenant_name && (
              <Text className="text-sm text-gray-600">
                <Text className="font-medium">商户:</Text> {instrument.tenant_name}
              </Text>
            )}
            {instrument.site_name && (
              <Text className="text-sm text-gray-600 mt-1">
                <Text className="font-medium">网点:</Text> {instrument.site_name}
                {instrument.site_address && <Text className="text-gray-400"> ({instrument.site_address})</Text>}
              </Text>
            )}
          </View>
        )}

        {/* Dynamic Properties */}
        {instrument.properties && typeof instrument.properties === 'object' && Object.keys(instrument.properties).length > 0 && (
          <View className="mt-3 p-3 bg-gray-50 rounded-lg">
            <Text className="text-gray-500 text-xs block mb-1">动态属性</Text>
            {Object.entries(instrument.properties).map(([key, vals]) => (
              <View key={key} className="flex justify-between text-xs mt-1">
                <Text className="text-gray-400">{key}</Text>
                <Text>{(Array.isArray(vals) ? vals : [vals]).join(', ')}</Text>
              </View>
            ))}
          </View>
        )}

        {isRentable && pricingV2?.tiers?.length > 0 && (
        <View className="mt-4 p-3 bg-blue-50 rounded-lg border border-blue-100">
          <Text className="font-medium text-sm text-blue-800 mb-1">定价策略</Text>
          <View className="text-xs text-gray-500 space-y-0.5">
            {pricingV2.tiers.map((t, i) => {
              const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
              const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
              return <Text key={i} className="block">{range}: ¥{t.daily_rate}/天</Text>
            })}
            {pricingV2.deposit > 0 && <Text className="mt-1 block">押金: ¥{pricingV2.deposit}</Text>}
            {pricingV2.shipping_fee > 0 && <Text className="block">物流费: ¥{pricingV2.shipping_fee}</Text>}
          </View>
        </View>
        )}

        {isRentable && (
        <View className="mt-3 p-2 bg-orange-50 rounded-lg text-sm">
          <Text className="text-orange-800">
            ⚠️ 逾期后将每日自动扣款，按 ¥{overdueDailyFee}/日 计算
          </Text>
          <Text className="text-orange-700 mt-1">
            💰 押金将在乐器归还、质检通过后原路退还。如乐器损坏，将在定损后从押金中抵扣
          </Text>
        </View>
        )}

        {isRentable && (
        <View className="mt-3 p-3 bg-purple-50 rounded-lg mb-24">
          <Text className="font-medium text-sm text-purple-800 flex items-center gap-1">
            <Text>🎁</Text>
            <Text className="font-bold">租购转化</Text>
          </Text>
          <Text className="text-purple-600 text-sm mt-0.5 font-bold">
            租满12个月可直接获得所有权
          </Text>
        </View>
        )}
      </View>

      {/* Floating Cart Icon */}
      {(cartItemCount > 0) && (
        <Button
          onClick={() => navigate('/cart')}
          className="fixed bottom-24 right-4 bg-brand-primary text-white p-3 rounded-full shadow-lg z-50"
        >
          <ShoppingCart size={24} />
          <Text className="absolute -top-1 -right-1 bg-red-500 text-white text-xs w-5 h-5 rounded-full flex items-center justify-center font-bold">
            {cartItemCount}
          </Text>
        </Button>
      )}

      <View className="fixed bottom-0 left-0 right-0 bg-white border-t p-4 safe-area-pb">
        {isRentable ? (
          <>
            <View className="flex gap-2">
              <Button 
                onClick={handleAddToCart}
                className="flex-1 py-3 rounded-lg font-medium flex items-center justify-center gap-1 bg-orange-100 text-orange-600"
              >
                <ShoppingCart size={18} />
                加入购物车
              </Button>
              <Button
                onClick={() => navigate(`/checkout/${id}`)}
                className="flex-1 py-3 rounded-lg font-medium flex items-center justify-center gap-1 bg-orange-500 text-white"
              >
                立即租赁
              </Button>
            </View>
          </>
        ) : activeOrder ? (
          activeOrder.order_status === 'in_lease' ? (
            <View className="p-3 bg-green-50 rounded-lg space-y-2">
              <Text className="text-green-700 font-medium">租赁中</Text>
              <Text className="text-gray-500 text-sm">
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
              {activeOrder.end_date && new Date(activeOrder.end_date) < new Date() && (
                <Text className="text-red-600 font-bold">
                  超期 {Math.ceil((Date.now() - new Date(activeOrder.end_date).getTime()) / 86400000)} 天
                </Text>
              )}
              <Button
                onClick={() => navigate(`/return/${activeOrder.order_id}?instrument=${id}`)}
                className="w-full py-2 bg-orange-500 text-white rounded-lg font-medium"
              >
                归还乐器
              </Button>
            </View>
          ) : activeOrder.order_status === 'returning' ? (
            <View className="p-3 bg-orange-50 rounded-lg space-y-2">
              <Text className="text-orange-700 font-medium">归还中</Text>
              <Text className="text-gray-500 text-sm">该乐器正在归还流程中</Text>
              <Text className="text-gray-500 text-sm">
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
              {activeOrder.deposit_refunded && (
                <Text className="text-green-600 text-sm mt-1">押金已退还</Text>
              )}
            </View>
          ) : ['reserved', 'pending', 'paid', 'pending_shipment'].includes(activeOrder.order_status) ? (
            <View className="p-3 bg-blue-50 rounded-lg space-y-2">
              <Text className="text-blue-700 font-medium">已预约</Text>
              <Text className="text-gray-500 text-sm">
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
            </View>
          ) : ['in_transit', 'shipped'].includes(activeOrder.order_status) ? (
            <View className="p-3 bg-cyan-50 rounded-lg text-center space-y-2">
              <Text className="text-cyan-700 font-medium">乐器物流中</Text>
              <Text className="text-gray-500 text-sm">该乐器正在运输途中</Text>
              <Button
                onClick={() => navigate(`/receive/${activeOrder.order_id}?instrument=${id}`)}
                className="w-full py-3 bg-green-500 text-white rounded-lg font-medium mt-2"
              >
                <CheckCircle size={18} className="inline mr-1" />
                确认收货
              </Button>
            </View>
          ) : activeOrder.order_status === 'expired' ? (
            <View className="p-3 bg-red-50 rounded-lg space-y-2">
              <Text className="text-red-700 font-medium">已超期</Text>
              <Text className="text-gray-500 text-sm">
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
              {activeOrder.end_date && (
                <Text className="text-red-600 font-bold">
                  超期 {Math.ceil((Date.now() - new Date(activeOrder.end_date).getTime()) / 86400000)} 天
                </Text>
              )}
              <Button
                onClick={() => navigate(`/return/${activeOrder.order_id}?instrument=${id}`)}
                className="w-full py-2 bg-orange-500 text-white rounded-lg font-medium"
              >
                归还乐器
              </Button>
            </View>
          ) : (
            <View className="p-3 bg-cyan-50 rounded-lg text-center space-y-2">
              <Text className="text-cyan-700 font-medium">乐器物流中</Text>
              <Text className="text-gray-500 text-sm">该乐器正在运输途中</Text>
              <Button
                onClick={() => navigate(`/receive/${activeOrder.order_id}?instrument=${id}`)}
                className="w-full py-3 bg-green-500 text-white rounded-lg font-medium mt-2"
              >
                <CheckCircle size={18} className="inline mr-1" />
                确认收货
              </Button>
            </View>
          )
        ) : (
          <View className="p-3 bg-gray-100 rounded-lg text-center">
            <Text className="text-gray-500 font-medium">该乐器目前不可租赁</Text>
            <Text className="text-gray-400 text-sm mt-1">乐器已被预约，暂时无法租赁</Text>
          </View>
        )}
      </View>

      {cartToast && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center" onClick={() => setCartToast(false)}>
          <View className="bg-white rounded-xl p-6 mx-8 text-center" onClick={e => e.stopPropagation()}>
            <CheckCircle size={48} className="text-green-500 mx-auto mb-3" />
            <Text className="text-lg font-bold mb-1">加入成功</Text>
            <Text className="text-gray-500 text-sm mb-4">该乐器已添加到购物车</Text>
            <View className="flex gap-3">
              <Button 
                onClick={() => { setCartToast(false); navigate('/') }}
                className="flex-1 py-3 px-6 border rounded-lg text-gray-600 min-w-[100px]"
              >
                继续浏览
              </Button>
              <Button 
                onClick={() => { setCartToast(false); navigate('/cart') }}
                className="flex-1 py-3 px-6 bg-brand-primary text-white rounded-lg min-w-[100px]"
              >
                提交订单
              </Button>
            </View>
          </View>
        </View>
      )}

      <Modal
        title="📊 服务权益对比"
        open={showComparison}
        onCancel={() => setShowComparison(false)}
        footer={null}
        width={600}
      >
        <View className="overflow-x-auto">
          <View className="w-full text-sm">
            <View className="bg-gray-100 flex">
              <Text className="p-2 flex-1 font-medium">权益项</Text>
              <Text className="p-2 flex-1 text-center font-medium">入门级</Text>
              <Text className="p-2 flex-1 text-center font-medium">专业级</Text>
              <Text className="p-2 flex-1 text-center font-medium text-purple-600">大师级</Text>
            </View>
            {SERVICE_ITEMS.map((item, idx) => (
              <View key={idx} className="flex border-b">
                <Text className="p-2 flex-1">{item.name}</Text>
                <Text className={`p-2 flex-1 text-center ${item.entry === '✓' ? 'text-green-600' : 'text-gray-400'}`}>
                  {item.entry}
                </Text>
                <Text className={`p-2 flex-1 text-center ${item.professional === '✓' ? 'text-green-600' : 'text-gray-400'}`}>
                  {item.professional}
                </Text>
                <Text className={`p-2 flex-1 text-center font-medium ${item.master === '✓' ? 'text-purple-600' : 'text-gray-400'}`}>
                  {item.master}
                </Text>
              </View>
            ))}
          </View>
        </View>
        <View className="mt-4 flex justify-end">
          <AntButton onClick={() => setShowComparison(false)}>关闭</AntButton>
        </View>
      </Modal>
    </View>
  )
}
