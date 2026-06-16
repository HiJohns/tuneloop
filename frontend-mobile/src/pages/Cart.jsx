import { useState, useEffect, useRef, useMemo } from 'react'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { dialog, storage, eventBus } from '../platform'

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
  if (!pricing) return []
  if (Array.isArray(pricing)) return pricing
  if (typeof pricing === 'string') {
    try { return JSON.parse(pricing) } catch { return [] }
  }
  return []
}

function getItemPricing(item) {
  const pricing = parsePricing(item.pricing)
  const dailyRent = pricing[0]?.daily_rent || item.base_daily_rate || 0
  const deposit = pricing[0]?.deposit || 0
  const rentQty = item.rent_qty || 1
  const rent = item.calculated_rent !== undefined ? item.calculated_rent : dailyRent * rentQty
  return { dailyRent, deposit, rent, shippingFee: pricing[0]?.shipping_fee || 0 }
}

export default function Cart() {
  const navigate = useNavigate()
  const [cartItems, setCartItems] = useState([])
  const [previewImages, setPreviewImages] = useState([])
  const [previewIndex, setPreviewIndex] = useState(-1)
  const previewTouchStartX = useRef(0)

  useEffect(() => {
    const data = storage.getJSON('cart', { items: [] }) || { items: [] }
    setCartItems(data.items)
  }, [])

  const groups = useMemo(() => {
    const map = {}
    cartItems.forEach(item => {
      const key = `${item.tenant_id || 'unknown'}-${item.site_id || 'unknown'}`
      if (!map[key]) {
        const itemShipping = parsePricing(item.pricing)[0]?.shipping_fee || 0
        map[key] = {
          tenant_id: item.tenant_id,
          tenant_name: item.tenant_name || '',
          site_name: item.site_name || '',
          site_address: item.site_address || '',
          site_phone: item.site_phone || '',
          shippingFee: itemShipping,
          items: [],
        }
      }
      map[key].items.push(item)
    })
    return Object.values(map)
  }, [cartItems])

  const grandTotal = useMemo(() => {
    let total = 0
    for (const group of groups) {
      let groupRent = 0
      let groupDeposit = 0
      for (const item of group.items) {
        const p = getItemPricing(item)
        groupRent += p.rent
        groupDeposit += p.deposit
      }
      total += groupRent + groupDeposit + (group.shippingFee || 0)
    }
    return total
  }, [groups])

  const getItemId = (item) => item.instrument_id || item.id

  const handleRemove = (itemId) => {
    if (dialog.confirm('确定要删除该乐器吗？')) {
      const updated = cartItems.filter(item => getItemId(item) !== itemId)
      setCartItems(updated)
      storage.setJSON('cart', { items: updated })
      eventBus.emit('cartUpdated')
    }
  }

  const openPreview = (images, index) => {
    setPreviewImages(images)
    setPreviewIndex(index)
  }

  const closePreview = () => {
    setPreviewIndex(-1)
    setPreviewImages([])
  }

  const increaseRentQty = (itemId) => {
    setCartItems(prev => prev.map(item => {
      if (getItemId(item) === itemId) {
        return { ...item, rent_qty: (item.rent_qty || 1) + 1 }
      }
      return item
    }))
  }

  const decreaseRentQty = (itemId) => {
    setCartItems(prev => prev.map(item => {
      if (getItemId(item) === itemId) {
        return { ...item, rent_qty: Math.max(1, (item.rent_qty || 1) - 1) }
      }
      return item
    }))
  }

  const handleCheckout = () => {
    if (cartItems.length === 0) return
    navigate('/success')
  }

  const handleGoHome = () => {
    navigate('/')
  }

  return (
    <View className="container h-screen w-screen bg-zinc-100 overflow-hidden flex flex-col relative antialiased">
      <View className="w-full pt-3 pb-2 px-4 flex justify-between items-center bg-white border-b border-zinc-100 flex-shrink-0">
        <Text className="text-xl font-bold text-black" onClick={() => navigate(-1)}>❮</Text>
        <Text className="text-lg font-black text-black">购物车</Text>
        <View className="w-6"></View>
      </View>

      <ScrollView className="w-full flex-1 pb-24" scrollY showScrollbar={false}>
        {cartItems.length === 0 ? (
          <View className="w-full flex flex-col items-center justify-center pt-24 px-6 space-y-4">
            <View className="w-48 h-48 bg-transparent flex items-center justify-center relative">
              <Text className="text-9xl opacity-20">🛒</Text>
              <Text className="text-4xl absolute bottom-6 right-8">🎸</Text>
            </View>
            <Text className="text-zinc-500 text-lg font-medium tracking-wide">购物车还是空的</Text>
            <Text className="text-blue-600 font-bold text-sm border-b border-blue-600 pb-0.5" onClick={handleGoHome}>去逛逛</Text>
          </View>
        ) : (
          <View className="p-4 space-y-4">
            {groups.map((group) => {
              let totalRent = 0
              let totalDeposit = 0
              group.items.forEach(item => {
                const p = getItemPricing(item)
                totalRent += p.rent
                totalDeposit += p.deposit
              })
              const groupSubtotal = totalRent + totalDeposit + (group.shippingFee || 0)

              return (
                <View key={group.tenant_id || 'unknown'} className="bg-white rounded-2xl shadow-sm overflow-hidden flex flex-col">
                  <View className="bg-zinc-50/80 px-4 py-2.5 flex items-center justify-between border-b border-zinc-100 text-[11px] text-zinc-400 font-bold">
                    <View className="flex items-center space-x-1">
                      <Text>🏢</Text>
                      <Text className="text-zinc-700 font-black">{group.tenant_name}</Text>
                      <Text className="mx-1 text-zinc-300">|</Text>
                      <Text>📍</Text>
                      <Text className="text-zinc-600">{group.site_name}</Text>
                    </View>
                    <Text className="text-orange-600 bg-orange-50 px-1.5 py-0.5 rounded scale-90">合并打包</Text>
                  </View>

                  <View className="divide-y divide-zinc-50 px-4">
                    {group.items.map((item) => {
                      const images = parseImages(item.images)
                      const imgSrc = images[0] || item.cover || PLACEHOLDER_IMAGE
                      const itemId = getItemId(item)
                      const pricing = getItemPricing(item)
                      return (
                        <View key={itemId} className="py-4 flex flex-col space-y-3">
                          <View className="flex items-center justify-between w-full">
                            <View
                              className="w-20 h-20 bg-zinc-50 rounded-xl overflow-hidden flex-shrink-0 flex items-center justify-center"
                              onClick={() => openPreview(images.length > 0 ? images : [imgSrc], 0)}
                            >
                              <Image src={imgSrc} className="w-16 h-16 object-contain" />
                            </View>

                            <View className="flex-1 ml-3 flex flex-col space-y-1 min-w-0">
                              <View className="flex items-center space-x-1.5 min-w-0">
                                <Text className="text-xl font-black text-black tracking-wide truncate max-w-[140px]">{item.sn || item.name || '未知乐器'}</Text>
                                <Text className="bg-blue-50 text-blue-600 text-[10px] font-black px-1.5 py-0.5 rounded flex-shrink-0">
                                  {item.level_name || '标准'}
                                </Text>
                              </View>
                              <Text className="text-xs text-amber-600 bg-amber-50 px-2 py-0.5 rounded font-extrabold self-start">
                                🔶 {item.category_name || item.brand || '乐器'}
                              </Text>
                              <Text className="text-[10px] text-zinc-400 font-medium">租金: ¥{pricing.rent}/期 · 押金: ¥{pricing.deposit}</Text>
                            </View>

                            <View className="flex-shrink-0 items-end ml-2">
                              <View className="flex items-center border border-zinc-200 rounded-full h-7 px-1 bg-zinc-50/50">
                                <Text className="px-2 text-zinc-400 font-bold text-sm select-none" onClick={() => decreaseRentQty(itemId)}>—</Text>
                                <Text className="px-2 text-black font-black text-xs">{item.rent_qty || 1}天</Text>
                                <Text className="px-2 text-zinc-600 font-bold text-sm select-none" onClick={() => increaseRentQty(itemId)}>+</Text>
                              </View>
                            </View>
                          </View>

                          <View className="flex justify-start">
                            <Button
                              className="m-0 bg-white border border-red-100 text-red-500 rounded-full px-3 h-6 text-[10px] font-bold flex items-center space-x-1 shadow-sm"
                              onClick={() => handleRemove(itemId)}
                            >
                              <Text className="text-red-600 font-black">🔴</Text>
                              <Text>删除</Text>
                            </Button>
                          </View>
                        </View>
                      )
                    })}
                  </View>

                  <View className="bg-zinc-50/40 border-t border-zinc-100 p-4 flex justify-between items-end w-full mt-auto">
                    <View className="flex flex-col space-y-1 text-[11px] text-zinc-400 font-semibold max-w-[60%]">
                      <Text className="truncate">🗺️ 发货仓: {group.site_address || '地址待确认'}</Text>
                      {group.site_phone && <Text>📞 电话: {group.site_phone}</Text>}
                      <View className="text-[10px] text-zinc-400/80 mt-1 pt-1 border-t border-zinc-200/60">
                        含租金全款 ¥{totalRent} + 累计押金 ¥{totalDeposit} + 运费 ¥{group.shippingFee || 0}
                      </View>
                    </View>

                    <View className="text-right flex-shrink-0 whitespace-nowrap">
                      <Text className="text-[10px] text-zinc-400 font-bold block mb-0.5">网点合并小计</Text>
                      <Text className="text-black font-black text-2xl tracking-tight">
                        ¥{groupSubtotal}
                      </Text>
                    </View>
                  </View>
                </View>
              )
            })}
          </View>
        )}
      </ScrollView>

      <View className="absolute bottom-0 left-0 right-0 bg-white border-t border-zinc-100 p-4 pb-6 flex justify-between items-center z-50 shadow-2xl flex-shrink-0">
        <View>
          <Text className="text-sm text-zinc-400">合计总额</Text>
          <Text className="text-xl font-black text-black tracking-wide">¥{grandTotal.toFixed(0)}</Text>
        </View>
        <Button
          className="m-0 bg-[#B98E5F] text-white font-extrabold text-base px-10 h-12 rounded-full shadow-md flex items-center justify-center"
          onClick={handleCheckout}
        >
          去结算
        </Button>
      </View>

      {previewIndex >= 0 && previewImages.length > 0 && (
        <View
          className="fixed inset-0 z-50 bg-black/70 flex items-center justify-center"
          onClick={closePreview}
        >
          <Image
            src={previewImages[previewIndex]}
            className="max-w-[90%] max-h-[80%] object-contain"
            mode="aspectFit"
            onClick={(e) => e.stopPropagation()}
            onTouchStart={(e) => { previewTouchStartX.current = e.touches[0].clientX }}
            onTouchEnd={(e) => {
              const diff = e.changedTouches[0].clientX - previewTouchStartX.current
              if (Math.abs(diff) > 50) {
                if (diff < 0 && previewIndex < previewImages.length - 1) {
                  setPreviewIndex(prev => prev + 1)
                } else if (diff > 0 && previewIndex > 0) {
                  setPreviewIndex(prev => prev - 1)
                }
              }
            }}
          />
          <Text className="absolute bottom-12 text-white/60 text-sm">点击空白区域关闭</Text>
        </View>
      )}
    </View>
  )
}
