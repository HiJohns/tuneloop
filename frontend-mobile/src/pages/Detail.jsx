import { useState, useEffect, useCallback, useRef } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { instrumentsApi, getToken, apiFetch, redirectToLogin } from '../services/api'
import { ArrowLeft, Shield, Clock, AlertCircle, MapPin, Bell, CheckCircle, X, ShoppingCart } from 'lucide-react'
import { Switch, Tag, Modal, Button as AntButton } from 'antd'
import dayjs from 'dayjs'
import { env, storage, eventBus, getWindowSize } from '../platform'
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
  const [activeOrder, setActiveOrder] = useState(null)
  const [currentUser, setCurrentUser] = useState(null)
  const [currentBanner, setCurrentBanner] = useState(0)
  const [displayMedia, setDisplayMedia] = useState(null)
  const [pricingV2, setPricingV2] = useState(null)
  const bannerTouchStartXRef = useRef(0)
  const isRentable = instrument?.stock_status === 'available'
  const isCustomer = !currentUser || currentUser?.role === 'USER'
  const baseUrl = env.apiBaseUrl
  const dailyRent = pricingV2?.base_daily_rate || instrument?.base_daily_rate || 0

  useEffect(() => {
    const fetchData = async () => {
      try {
        setLoading(true)
        const token = getToken()
        const headers = token ? { 'Authorization': `Bearer ${token}` } : {}

        const [instRes, mediaRes, pv2Res] = await Promise.all([
          apiFetch(`${baseUrl}/public/instruments/${id}`),
          apiFetch(`${baseUrl}/public/instruments/${id}/display-media`),
          apiFetch(`${baseUrl}/public/instruments/${id}/pricing-v2`),
        ])
        const instData = await instRes.json()
        if (instData.code === 20000) setInstrument(instData.data)

        const mediaData = await mediaRes.json()
        if (mediaData.code === 20000) setDisplayMedia(mediaData.data)

        const pv2Data = await pv2Res.json()
        if (pv2Data.code === 20000) setPricingV2(pv2Data.data)

        if (token) {
          try {
            const userRes = await apiFetch(`${baseUrl}/users/me`, { headers })
            const userData = await userRes.json()
            if (userData.code === 20000) setCurrentUser(userData.data)
          } catch {}
          try {
            const inst = instData.data
            if (inst?.sn) {
              const orderRes = await apiFetch(`${baseUrl}/orders/by-instrument-sn?sn=${encodeURIComponent(inst.sn)}`)
              const orderData = await orderRes.json()
              if (orderData.code === 20000 && orderData.data) setActiveOrder(orderData.data)
            }
          } catch {}
        }
      } catch {}
      setLoading(false)
    }
    fetchData()
  }, [id])

  const bannerImagesSource = Array.isArray(displayMedia?.images) && displayMedia.images.length > 0
    ? displayMedia.images.map(i => ({ url: i.url }))
    : (parseImages(instrument?.images) || []).map(url => ({ url }))
  const bannerImages = bannerImagesSource.length > 0
    ? bannerImagesSource
    : [{ url: 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="375" height="232" viewBox="0 0 375 232"><rect fill="#f0f0f0" width="375" height="232"/><text x="188" y="120" text-anchor="middle" fill="#ccc" font-size="20">暂无图片</text></svg>') }]

  if (loading) {
    return <View className="p-4">加载中...</View>
  }

  if (!instrument) {
    return <View className="p-4">乐器不存在</View>
  }

  const levelName = instrument.level_name || ''

  const levelBg = levelName.includes('大师') ? 'bg-[#8A2BE2]'
    : levelName.includes('专业') ? 'bg-[#0084FF]'
    : levelName.includes('入门') ? 'bg-[#FF6B00]'
    : 'bg-zinc-500'

  return (
    <View className="min-h-screen bg-zinc-100 pb-[140px] flex flex-col relative antialiased">
      {/* Nav bar */}
      <View className="w-full pt-3 pb-2 px-4 flex justify-between items-center bg-zinc-100">
        <Text className="text-xl font-bold text-black" onClick={() => navigate(-1)}>❮</Text>
        <Text className="text-lg font-black text-black">乐器详情</Text>
        <Text className="text-sm font-bold text-zinc-700">★ 收藏</Text>
      </View>

      {/* Banner carousel */}
      <View className="w-full overflow-hidden"
        onTouchStart={(e) => { bannerTouchStartXRef.current = e.touches[0].clientX }}
        onTouchEnd={(e) => {
          const diff = e.changedTouches[0].clientX - bannerTouchStartXRef.current
          if (Math.abs(diff) > 50) {
            setCurrentBanner(prev => {
              if (diff < 0) return (prev + 1) % bannerImages.length
              return (prev - 1 + bannerImages.length) % bannerImages.length
            })
          }
        }}
      >
        <View className="w-full" style={{ height: `${Math.round(getWindowSize().width * 0.618)}px` }}>
          <View className="flex flex-row h-full" style={{
            width: `${bannerImages.length * 100}%`,
            transform: `translateX(-${currentBanner * (100 / bannerImages.length)}%)`,
            transition: 'transform 0.5s ease-in-out'
          }}>
            {bannerImages.map((img, i) => (
              <View key={i} className="h-full px-2 box-border" style={{ width: `${100 / bannerImages.length}%` }}>
                <Image src={img.url || img} className="w-full h-full object-contain" />
              </View>
            ))}
          </View>
        </View>
        <View className="flex items-center justify-center space-x-1.5 pb-3 bg-zinc-100">
          {bannerImages.map((_, i) => (
            <View key={i} className={`${i === currentBanner ? 'w-3' : 'w-1.5'} h-1.5 rounded-full ${i === currentBanner ? 'bg-[#915F38]' : 'bg-black/15'}`} />
          ))}
        </View>
      </View>

      <ScrollView className="w-full flex-1" scrollY scrollWithAnimation showScrollbar={false}>
        <View className="px-4 mt-4 space-y-3 pb-4">

          {/* Card A: Instrument info + deposit */}
          <View className="bg-white rounded-2xl p-4 shadow-sm flex flex-col space-y-2">
            <View className="flex justify-between items-start w-full">
              <View className="flex-1 min-w-0 pr-4">
                <Text className="block text-2xl font-black text-black tracking-wide truncate">{instrument.name || instrument.sn}</Text>
              </View>
              <View className="flex-shrink-0 whitespace-nowrap text-right">
                <Text className="text-[#C21838] text-base tracking-tight">
                  押金 ¥{deposit} <Text className="text-zinc-400 font-normal">❯</Text>
                </Text>
              </View>
            </View>
            <View className="flex items-center space-x-3">
              {levelName && (
                <View className={`inline-block ${levelBg} text-white text-[10px] font-black px-2.5 py-0.5 rounded-full shadow-sm`}>
                  {levelName}
                </View>
              )}
              <Text className="text-[#C21838] text-base tracking-tight">
                月租 ¥{Math.round((dailyRent || instrument?.base_daily_rate || 0) * 25)}/月
              </Text>
            </View>
            <View className="border-t border-zinc-100 pt-3 flex justify-between items-center text-xs text-zinc-500 font-bold">
              <View className="flex items-center space-x-1"><Text>🏠</Text><Text>{instrument.site_name || '暂无网点'}</Text></View>
              <View className="flex items-center space-x-1"><Text>📍</Text><Text>{instrument.site_address || '暂无地址'}</Text></View>
              <View className="flex items-center space-x-1"><Text>📞</Text><Text>{instrument.site_phone || ''}</Text></View>
            </View>
          </View>

          {/* Video player */}
          {liveVideo && (
            <View className="px-0">
              <Video
                src={liveVideo.url}
                poster={liveVideo.thumb_url || ''}
                controls
                className="w-full rounded-2xl"
                style={{ maxHeight: 400 }}
              />
            </View>
          )}

          {/* Poster image */}
          {instrument?.poster && (
            <View className="px-0">
              <Image
                src={instrument.poster}
                className="w-full rounded-2xl object-contain"
                style={{ maxWidth: 750 }}
                mode="widthFix"
              />
            </View>
          )}

          {/* Card C: Specifications & properties */}
          <View className="bg-white rounded-2xl p-4 shadow-sm space-y-2">
            <Text className="text-base font-black text-black">规格参数</Text>
            {instrument.properties && typeof instrument.properties === 'object' && Object.keys(instrument.properties).length > 0 ? (
              Object.entries(instrument.properties).map(([key, vals]) => (
                <View key={key} className="flex justify-between items-center">
                  <Text className="text-sm font-bold text-zinc-600">{key}</Text>
                  <Text className="text-sm text-zinc-400">{(Array.isArray(vals) ? vals : [vals]).join(', ')}</Text>
                </View>
              ))
            ) : (
              <Text className="block text-sm text-zinc-400">暂无规格参数</Text>
            )}
          </View>

          {/* Pricing V2 tiers */}
          {isRentable && pricingV2?.tiers?.length > 0 && (
            <View className="bg-white rounded-2xl p-4 shadow-sm space-y-2">
              <Text className="text-base font-black text-black">定价策略</Text>
              {pricingV2.tiers.map((t, i) => {
                const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
                return (
                  <View key={i} className="flex justify-between text-sm">
                    <Text className="text-zinc-500">{range}</Text>
                    <Text className="font-bold text-black">¥{Math.round(t.daily_rate)}/天</Text>
                  </View>
                )
              })}
              {(pricingV2.deposit > 0 || pricingV2.shipping_fee > 0) && (
                <View className="border-t border-zinc-100 pt-2 space-y-1">
                  {pricingV2.deposit > 0 && (
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500">押金</Text>
                      <Text className="text-black">¥{pricingV2.deposit}</Text>
                    </View>
                  )}
                  {pricingV2.shipping_fee > 0 && (
                    <View className="flex justify-between text-sm">
                      <Text className="text-zinc-500">物流费</Text>
                      <Text className="font-bold text-black">¥{pricingV2.shipping_fee}</Text>
                    </View>
                  )}
                </View>
              )}
              <View className="border-t border-zinc-100 pt-2 mt-1">
                <Text className="text-xs text-orange-600 font-medium">
                  ⚠️ 逾期后每日自动扣款 ¥{overdueDailyFee}/日；押金归还质检通过后退还
                </Text>
              </View>
            </View>
          )}

          {/* Rent-to-own */}
          {isRentable && (
            <View className="bg-white rounded-2xl p-4 shadow-sm">
              <View className="flex items-center">
                <Text>🎁</Text>
                <Text className="font-bold text-sm text-purple-800 ml-1">租购转化</Text>
              </View>
              <Text className="text-purple-600 text-sm mt-1 font-bold">
                租满12个月可直接获得所有权
              </Text>
            </View>
          )}

          {/* Service comparison */}
          {isRentable && (
            <View className="bg-white rounded-2xl p-4 shadow-sm" onClick={() => setShowComparison(true)}>
              <View className="flex justify-between items-center">
                <Text className="text-base font-black text-black">服务权益对比</Text>
                <Text className="text-sm text-zinc-400">查看详情 ❯</Text>
              </View>
            </View>
          )}

        </View>
      </ScrollView>

      {/* Floating cart icon */}
      {cartItemCount > 0 && (
        <View onClick={() => navigate('/cart')} className="fixed bottom-24 right-4 bg-[#002140] text-white p-3 rounded-full shadow-lg z-50">
          <Text className="text-xl">🛒</Text>
          <Text className="absolute -top-1 -right-1 bg-red-500 text-white text-xs w-5 h-5 rounded-full flex items-center justify-center font-bold">
            {cartItemCount}
          </Text>
        </View>
      )}

      {/* Bottom panel */}
      <View className="fixed bottom-0 left-0 right-0 bg-[#FDFBF7] border-t border-zinc-100 p-4 flex flex-col space-y-2 z-50 shadow-2xl">
        {isRentable && isCustomer ? (
          <>
            <View className="flex w-full space-x-3">
              <View
                onClick={handleAddToCart}
                className="flex-1 h-12 rounded-full shadow-sm flex items-center justify-center"
                style={{ background: 'linear-gradient(135deg, #E2B07E, #C98E54)' }}
              >
                <Text className="text-white font-black text-base">加入购物车</Text>
              </View>
              <View
                onClick={() => navigate(`/checkout/${id}`)}
                className="flex-1 h-12 rounded-full shadow-sm flex items-center justify-center"
                style={{ background: 'linear-gradient(135deg, #FA5E3C, #E63917)' }}
              >
                <Text className="text-white font-black text-base">立即租赁</Text>
              </View>
            </View>
            <Text className="block text-center text-xs font-bold text-zinc-400 tracking-wide">
              合计金额：预付全款租金 + 固定押金 + 往返运费
            </Text>
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
              <View
                onClick={() => navigate(`/return/${activeOrder.order_id}?instrument=${id}`)}
                className="w-full py-2 bg-orange-500 text-white rounded-lg font-medium text-center"
              >
                <Text>归还乐器</Text>
              </View>
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
              {currentUser?.id === activeOrder?.user_id && (
                <View
                  onClick={() => navigate(`/receive/${activeOrder.order_id}?instrument=${id}`)}
                  className="w-full py-3 bg-green-500 text-white rounded-lg font-medium mt-2 text-center"
                >
                  <Text>确认收货</Text>
                </View>
              )}
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
              <View
                onClick={() => navigate(`/return/${activeOrder.order_id}?instrument=${id}`)}
                className="w-full py-2 bg-orange-500 text-white rounded-lg font-medium text-center"
              >
                <Text>归还乐器</Text>
              </View>
            </View>
          ) : (
            <View className="p-3 bg-cyan-50 rounded-lg text-center space-y-2">
              <Text className="text-cyan-700 font-medium">乐器物流中</Text>
              <Text className="text-gray-500 text-sm">该乐器正在运输途中</Text>
              {currentUser?.id === activeOrder?.user_id && (
                <View
                  onClick={() => navigate(`/receive/${activeOrder.order_id}?instrument=${id}`)}
                  className="w-full py-3 bg-green-500 text-white rounded-lg font-medium mt-2 text-center"
                >
                  <Text>确认收货</Text>
                </View>
              )}
            </View>
          )
        ) : (
          <View className="p-3 bg-gray-100 rounded-lg text-center">
            <Text className="text-gray-500 font-medium">该乐器目前不可租赁</Text>
            <Text className="text-gray-400 text-sm mt-1">乐器已被预约，暂时无法租赁</Text>
          </View>
        )}
      </View>

      {/* Cart toast modal */}
      {cartToast && (
        <View className="fixed inset-0 bg-black/50 z-50 flex items-center justify-center" onClick={() => setCartToast(false)}>
          <View className="bg-white rounded-xl p-6 mx-8 text-center" onClick={e => e.stopPropagation()}>
            <Text className="text-green-500 text-5xl mb-3">✓</Text>
            <Text className="text-lg font-bold mb-1">加入成功</Text>
            <Text className="text-gray-500 text-sm mb-4">该乐器已添加到购物车</Text>
            <View className="flex gap-3">
              <View
                onClick={() => { setCartToast(false); navigate('/') }}
                className="flex-1 py-3 px-6 border rounded-lg text-gray-600 text-center"
              >
                <Text>继续浏览</Text>
              </View>
              <View
                onClick={() => { setCartToast(false); navigate('/cart') }}
                className="flex-1 py-3 px-6 bg-[#002140] text-white rounded-lg text-center"
              >
                <Text>提交订单</Text>
              </View>
            </View>
          </View>
        </View>
      )}

      {/* Fullscreen image/video overlay */}
      {fullscreenImage && (
        <View className="fixed inset-0 bg-black z-50 flex items-center justify-center" onClick={() => setFullscreenImage(null)}>
          {liveVideo && fullscreenImage === liveVideo.url ? (
            <Video src={liveVideo.url} poster={liveVideo.thumb_url} controls className="w-full" style={{ maxHeight: '100%' }} />
          ) : (
            <Image src={fullscreenImage} className="max-w-full max-h-full object-contain" />
          )}
        </View>
      )}

      {/* Service comparison modal */}
      <Modal title="📊 服务权益对比" open={showComparison} onCancel={() => setShowComparison(false)} footer={null} width={600}>
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
                <Text className={`p-2 flex-1 text-center ${item.entry === '✓' ? 'text-green-600' : 'text-gray-400'}`}>{item.entry}</Text>
                <Text className={`p-2 flex-1 text-center ${item.professional === '✓' ? 'text-green-600' : 'text-gray-400'}`}>{item.professional}</Text>
                <Text className={`p-2 flex-1 text-center font-medium ${item.master === '✓' ? 'text-purple-600' : 'text-gray-400'}`}>{item.master}</Text>
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
