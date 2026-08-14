import { useState, useEffect, useRef, useMemo } from 'react'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
import { dialog, storage, eventBus, env, session } from '../platform'
import { apiFetch, getCartKey, getToken, redirectToLogin } from '../services/api'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">暂无图片</text>
  </svg>
`)

// Relative /uploads/media/... paths must be resolved to an absolute URL for
// the weapp <Image> component (same fixImg pattern as Detail.jsx).
function fixImg(url, baseUrl) {
  if (!url) return url
  if (url.startsWith('http') || url.startsWith('data:')) return url
  return baseUrl.replace(/\/api$/, '') + url
}

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

// computeTieredBreakdown returns per-tier segments (days / daily_rate / fee)
// and the total rent, per the pricing_v2 tier table (#1659/#1658 refinement).
function computeTieredBreakdown(pricingV2, days, baseDailyRate) {
  if (!pricingV2?.tiers?.length) {
    const rate = pricingV2?.base_daily_rate || baseDailyRate || 0
    return { tiers: [{ days, rate, fee: rate * days }], total: rate * days }
  }
  let remaining = days
  let total = 0
  let prevMax = 0
  const tiers = []
  for (const tier of pricingV2.tiers) {
    const tierDays = tier.days_max > 0 ? tier.days_max - prevMax : remaining
    const segDays = Math.min(tierDays, remaining)
    if (segDays <= 0) break
    tiers.push({ days: segDays, rate: tier.daily_rate, fee: segDays * tier.daily_rate })
    total += segDays * tier.daily_rate
    remaining -= segDays
    prevMax = tier.days_max
    if (remaining <= 0) break
  }
  return { tiers, total }
}

function getItemPricing(item) {
  const days = item.rent_qty || 30
  const dailyRent = item.daily_rent || 0
  const bd = computeTieredBreakdown(item.pricing_v2, days, dailyRent)
  const rent = bd.total
  const deposit = item.deposit || 0
  return { dailyRent, deposit, rent, tiers: bd.tiers }
}

export default function Cart() {
  const navigate = useNavigate()
  const [cartItems, setCartItems] = useState([])
  const [selected, setSelected] = useState(new Set())

  const getItemId = (item) => item.instrument_id || item.id

  // #1659: instrument rented out (or otherwise unavailable) in the cart
  const isRentedOut = (item) => {
    const st = item.stock_status
    return st && st !== 'available'
  }

  // #1639 购物车合并去重：按 instrument_id + 租期（rent_qty/days）去重
  const mergeDedup = (accountItems, guestItems) => {
    const merged = accountItems.slice()
    guestItems.forEach(guestItem => {
      const dup = merged.some(i =>
        getItemId(i) === getItemId(guestItem) &&
        (i.rent_qty || i.days || 30) === (guestItem.rent_qty || guestItem.days || 30)
      )
      if (!dup) merged.push(guestItem)
    })
    return merged
  }

  useEffect(() => {
    const data = storage.getJSON(getCartKey(), { items: [] }) || { items: [] }
    setCartItems(data.items)
    const sel = new Set(data.items.map(i => getItemId(i)))
    setSelected(sel)

    const token = getToken()
    if (!token) return
    // 登录后自动合并游客购物车（去重），不再弹 confirm（#1639 方案 b）
    const guestData = storage.getJSON('cart', { items: [] })
    if (guestData.items?.length > 0) {
      const merged = mergeDedup(data.items, guestData.items)
      storage.setJSON(getCartKey(), { items: merged })
      storage.setJSON('cart', { items: [] })
      setCartItems(merged)
      eventBus.emit('cartUpdated')
    }
  }, [])

  useEffect(() => {
    const enrichMissingPricing = async () => {
      const needsFetch = cartItems.filter(item => !item.pricing_v2?.tiers?.length && item.instrument_id)
      if (!needsFetch.length) return
      const updated = await Promise.all(cartItems.map(async (item) => {
        if (item.pricing_v2?.tiers?.length || !item.instrument_id) return item
        try {
          const res = await apiFetch(`${env.apiBaseUrl}/public/instruments/${item.instrument_id}/pricing-v2`)
          const data = await res.json()
          if (data.code === 20000 && data.data?.tiers?.length) {
            return {
              ...item,
              pricing_v2: { base_daily_rate: data.data.base_daily_rate, tiers: data.data.tiers },
              daily_rent: data.data.base_daily_rate || item.daily_rent,
              deposit: data.data.deposit || item.deposit,
              shipping_fee: data.data.shipping_fee || item.shipping_fee,
            }
          }
        } catch {}
        return item
      }))
      setCartItems(updated)
      storage.setJSON(getCartKey(), { items: updated })
    }
    if (cartItems.length > 0) enrichMissingPricing()
  }, [cartItems.length])

  // #1659: refresh stock_status of every cart item so rented-out instruments
  // are shown as unavailable (grayscale + disabled checkbox). Frontend check
  // is UX-only; the backend CreateOrder re-validates as the hard boundary.
  useEffect(() => {
    const checkStockStatus = async () => {
      const items = storage.getJSON(getCartKey(), { items: [] })?.items || []
      if (!items.length) return
      const stockMap = new Map()
      await Promise.all(items.map(async (item) => {
        if (!item.instrument_id) return
        try {
          const res = await apiFetch(`${env.apiBaseUrl}/public/instruments/${item.instrument_id}`)
          const data = await res.json()
          if (data.code === 20000 && data.data?.stock_status) {
            stockMap.set(getItemId(item), data.data.stock_status)
          }
        } catch {}
      }))
      // Merge stock_status into the CURRENT state (not a stale storage read)
      // so enrichMissingPricing's daily_rent/deposit completion is preserved.
      setCartItems(prev => {
        const updated = prev.map(item => {
          const st = stockMap.get(getItemId(item))
          return st !== undefined ? { ...item, stock_status: st } : item
        })
        storage.setJSON(getCartKey(), { items: updated })
        return updated
      })
      // Drop rented-out items from the selection so they never count toward
      // grandTotal / checkout (#1659).
      setSelected(prev => {
        const next = new Set(prev)
        stockMap.forEach((st, id) => {
          if (st !== 'available' && next.has(id)) next.delete(id)
        })
        return next
      })
    }
    checkStockStatus()
  }, [])

  const toggleSelect = (itemId) => {
    setSelected(prev => {
      const item = cartItems.find(i => getItemId(i) === itemId)
      if (item && isRentedOut(item)) return prev  // #1659: rented-out cannot be selected
      const next = new Set(prev)
      if (next.has(itemId)) next.delete(itemId); else next.add(itemId)
      return next
    })
  }

  const adjustRentDays = (itemId, delta) => {
    setCartItems(prev => {
      const updated = prev.map(item => {
        if (getItemId(item) === itemId) {
          const days = Math.max(1, Math.min(365, (item.rent_qty || item.days || 30) + delta))
          return { ...item, rent_qty: days }
        }
        return item
      })
      storage.setJSON(getCartKey(), { items: updated })
      return updated
    })
  }

  const groups = useMemo(() => {
    const map = {}
    cartItems.forEach(item => {
      const key = `${item.tenant_id || 'unknown'}-${item.site_id || 'unknown'}`
      if (!map[key]) {
        map[key] = {
          tenant_id: item.tenant_id,
          tenant_name: item.tenant_name || '',
          site_name: item.site_name || '',
          site_address: item.site_address || '',
          site_phone: item.site_phone || '',
          items: [],
        }
      }
      map[key].items.push(item)
    })
    return Object.values(map)
  }, [cartItems])

  const grandTotal = useMemo(() => {
    let total = 0
    groups.forEach(group => {
      let grpRent = 0
      let grpDeposit = 0
      group.items.forEach(item => {
        if (!selected.has(getItemId(item))) return
        const p = getItemPricing(item)
        grpRent += p.rent
        grpDeposit += p.deposit
      })
      if (grpRent + grpDeposit > 0) {
        total += grpRent + grpDeposit
      }
    })
    return total
  }, [groups, selected])

  const handleRemove = (itemId) => {
    if (dialog.confirm('确定要删除该乐器吗？')) {
      const updated = cartItems.filter(item => getItemId(item) !== itemId)
      setCartItems(updated)
      storage.setJSON(getCartKey(), { items: updated })
      setSelected(prev => { const next = new Set(prev); next.delete(itemId); return next })
      eventBus.emit('cartUpdated')
    }
  }

  const handleCheckout = () => {
    const token = getToken()
    if (!token) {
      session.setItem('post_auth_redirect', '/checkout')
      redirectToLogin('checkout')
      return
    }
    const selItems = cartItems.filter(item => selected.has(getItemId(item)) && !isRentedOut(item))
    if (selItems.length === 0) return
    storage.setJSON('cart_checkout', { items: selItems })
    navigate('/checkout')
  }

  const handleGoHome = () => {
    navigate('/')
  }

  return (
    <View className="container h-screen w-screen bg-[#FDFBF7] overflow-hidden flex flex-col relative antialiased">
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
                if (!selected.has(getItemId(item))) return
                const p = getItemPricing(item)
                totalRent += p.rent
                totalDeposit += p.deposit
              })
              const groupSubtotal = totalRent + totalDeposit

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
                      const rentedOut = isRentedOut(item)
                      const images = parseImages(item.images)
                      const imgSrc = fixImg(item.cover_image || images[0], env.apiBaseUrl) || PLACEHOLDER_IMAGE
                      const itemId = getItemId(item)
                      const pricing = getItemPricing(item)
                      const days = item.rent_qty || item.days || 30
                      const itemSubtotal = pricing.rent + pricing.deposit
                      return (
                        <View key={itemId} className="py-4 flex items-start">
                          {/* Checkbox column */}
                          <View className="flex-shrink-0 flex items-center" style={{ width: 28, marginTop: 24 }}>
                            <View
                              onClick={() => !rentedOut && toggleSelect(itemId)}
                              style={{
                                width: 20, height: 20, borderRadius: 4, borderWidth: 2,
                                borderColor: rentedOut ? '#e4e4e7' : (selected.has(itemId) ? '#B98E5F' : '#d1d5db'),
                                backgroundColor: rentedOut ? '#f4f4f5' : (selected.has(itemId) ? '#B98E5F' : 'transparent'),
                                display: 'flex', alignItems: 'center', justifyContent: 'center',
                                opacity: rentedOut ? 0.5 : 1,
                              }}
                            >
                              {!rentedOut && selected.has(itemId) && <Text style={{ color: 'white', fontSize: 12, fontWeight: 'bold' }}>✓</Text>}
                            </View>
                          </View>

                          {/* Left column: cover image (top-aligned) */}
                          <View className="flex flex-col items-center flex-shrink-0" style={{ width: 80 }}>
                            <View
                              className="w-20 h-20 bg-zinc-50 rounded-xl overflow-hidden flex items-center justify-center"
                              style={{ position: 'relative' }}
                              onClick={rentedOut ? undefined : () => navigate(`/instrument/${itemId}`)}
                            >
                              <Image
                                src={imgSrc}
                                className="w-16 h-16 object-contain"
                                style={rentedOut ? { filter: 'grayscale(1)', opacity: 0.55 } : undefined}
                              />
                              {rentedOut && (
                                <View
                                  style={{
                                    position: 'absolute', left: 0, right: 0, bottom: 0,
                                    backgroundColor: 'rgba(0,0,0,0.55)', paddingVertical: 2,
                                  }}
                                >
                                  <Text style={{ color: '#fff', fontSize: 9, textAlign: 'center', fontWeight: 'bold' }}>已被租出</Text>
                                </View>
                              )}
                            </View>
                            <Text className="text-xs text-red-500 font-bold mt-1" onClick={() => handleRemove(itemId)}>删除</Text>
                          </View>

                          {/* Right column: info + tier pricing */}
                          <View className="flex-1 flex flex-col min-w-0">
                            {/* Row 1: SN + category/level bubbles + days stepper */}
                            <View className="flex items-start justify-between">
                              <View className="flex-1 min-w-0">
                                <Text className="text-base font-black text-black tracking-wide truncate block">{item.sn || item.name || '未知乐器'}</Text>
                                <View className="flex items-center flex-wrap gap-1 mt-1">
                                  {item.level_name && <Text className="bg-blue-50 text-blue-600 text-[10px] font-black px-1.5 py-0.5 rounded flex-shrink-0">{item.level_name}</Text>}
                                  <Text className="text-xs text-amber-600 bg-amber-50 px-2 py-0.5 rounded font-extrabold flex-shrink-0">🔶 {item.category_name || '乐器'}</Text>
                                </View>
                              </View>
                              {!rentedOut && (
                                <View className="flex items-center border border-zinc-200 rounded-full h-7 px-1 bg-zinc-50/50 flex-shrink-0 ml-2">
                                  <Text className="px-2 text-zinc-400 font-bold text-sm select-none" onClick={() => adjustRentDays(itemId, -1)}>—</Text>
                                  <Text className="px-1 text-black font-black text-xs">{days}天</Text>
                                  <Text className="px-2 text-zinc-600 font-bold text-sm select-none" onClick={() => adjustRentDays(itemId, 1)}>+</Text>
                                </View>
                              )}
                            </View>

                            {/* Tier breakdown (小票标准：阶梯天数×日租金=费用 → 租金合计 + 押金 + 总金额) */}
                            <View className="text-[11px] text-right space-y-0.5 mt-2">
                              {pricing.tiers.map((t, i) => (
                                <Text key={i} className="block text-zinc-500">
                                  {t.days}天 × ¥{t.rate}/天 = ¥{t.fee}
                                </Text>
                              ))}
                              <Text className="block text-zinc-500">租金合计 ¥{pricing.rent.toFixed(0)}</Text>
                              <Text className="block text-zinc-500">押金 ¥{pricing.deposit.toFixed(0)}</Text>
                              <Text className="block font-bold text-black pt-0.5">总金额 ¥{itemSubtotal.toFixed(0)}</Text>
                            </View>
                          </View>
                        </View>
                      )
                    })}
                  </View>

                  <View className="bg-zinc-50/40 border-t border-zinc-100 p-4 flex justify-between items-end flex-shrink-0 mt-auto">
                    <View className="flex flex-col space-y-1 text-[11px] text-zinc-400 font-semibold min-w-0 flex-1">
                      <Text className="truncate">🗺️ 发货仓: {group.site_address || group.site_name || '-'}</Text>
                      {group.site_phone && <Text className="truncate">📞 电话: {group.site_phone}</Text>}
                    </View>

                    <View className="text-right flex-shrink-0 ml-3">
                      <Text className="text-[10px] text-zinc-400 font-bold block mb-0.5">网点小计</Text>
                      <Text className="text-black font-black text-lg tracking-tight">
                        ¥{groupSubtotal.toFixed(0)}
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
          className={grandTotal <= 0
            ? "m-0 bg-zinc-300 text-zinc-500 font-extrabold text-base px-10 h-12 rounded-full shadow-md flex items-center justify-center"
            : "m-0 text-white font-extrabold text-base px-10 h-12 rounded-full shadow-md flex items-center justify-center"}
          style={grandTotal > 0 ? { backgroundColor: '#B98E5F' } : {}}
          onClick={handleCheckout}
          disabled={grandTotal <= 0}
        >
           去结算
         </Button>
       </View>
    </View>
  )
}
