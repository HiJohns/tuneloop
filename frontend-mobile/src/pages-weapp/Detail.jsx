import { useState, useEffect, useCallback, useRef } from 'react'
import Taro from '@tarojs/taro'
import { instrumentsApi, getToken, apiFetch, redirectToLogin } from '../services/api'
import dayjs from 'dayjs'
import { env, storage, session, eventBus, getWindowSize, previewImage } from '../platform'
import { formatDisplayDate } from '../utils/format'
import { View, Text, Image, Button, Video, ScrollView } from '@tarojs/components'
import * as S from '../styles-weapp'

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
  const instance = Taro.getCurrentInstance()
  const { id } = instance.router?.params || {}
  const nav = (url) => { Taro.navigateTo({ url }) }
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [activeOrder, setActiveOrder] = useState(null)
  const [currentUser, setCurrentUser] = useState(null)
  const [currentBanner, setCurrentBanner] = useState(0)
  const [jumpReset, setJumpReset] = useState(false)
  const [displayMedia, setDisplayMedia] = useState(null)
  const [pricingV2, setPricingV2] = useState(null)
  const [showComparison, setShowComparison] = useState(false)
  const [auditLogs, setAuditLogs] = useState([])
  const [cartToast, setCartToast] = useState(false)
  const [fullscreenImage, setFullscreenImage] = useState(null)
  const bannerTouchStartXRef = useRef(0)
  const isRentable = instrument?.stock_status === 'available'
  const isCustomer = !currentUser || currentUser?.role === 'USER'
  const baseUrl = env.apiBaseUrl
  const fixImg = (url) => url && !url.startsWith('http') && !url.startsWith('data:') ? baseUrl.replace(/\/api$/, '') + url : url
  const dailyRent = pricingV2?.base_daily_rate || instrument?.base_daily_rate || 0
  const deposit = instrument?.deposit || pricingV2?.deposit || 0
  const liveVideo = displayMedia?.video
  const overdueDailyFee = pricingV2?.overdue_daily_fee || dailyRent || 0
  const shippingFee = pricingV2?.shipping_fee || 0

  const cartItemCount = (() => {
    try {
      const cartData = storage.getJSON('cart', {items: []})
      return cartData.items?.length || 0
    } catch { return 0 }
  })()

  const handleAddToCart = () => {
    try {
      const cartData = storage.getJSON('cart', {items: []}) || {items: []}
      if (!cartData.items.find(i => i.id === id)) {
        cartData.items.push({
          id,
          instrument_id: id,
          name: instrument?.name,
          sn: instrument?.sn,
          cover_image: instrument?.cover_image || '',
          category_name: instrument?.category_name || '',
          daily_rent: dailyRent,
          deposit,
          site_id: instrument?.site_id || '',
          site_name: instrument?.site_name || '',
          site_address: instrument?.site_address || '',
          site_phone: instrument?.site_phone || '',
          tenant_id: instrument?.tenant_id || '',
          tenant_name: instrument?.tenant_name || '',
          level_name: levelName || '',
          shipping_fee: pricingV2?.shipping_fee || 0,
          rent_qty: 30,
        })
        storage.setJSON('cart', cartData)
      }
      setCartToast(true)
      setTimeout(() => setCartToast(false), 2000)
    } catch {}
  }

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
          const role = (() => { try { return JSON.parse(atob((token || '').split('.')[1]))?.role } catch { return null } })()
          if (role && role !== 'USER') {
            try {
              const logRes = await apiFetch(`${baseUrl}/admin/audit-logs?resource_type=instrument&resource_id=${id}&pageSize=20`, { headers })
              const logData = await logRes.json()
              if (logData.code === 20000) setAuditLogs(logData.data?.list || [])
            } catch {}
          }
        }
      } catch {}
      setLoading(false)
    }
    fetchData()
  }, [id])

  const bannerImagesSource = Array.isArray(displayMedia?.images) && displayMedia.images.length > 0
    ? displayMedia.images.map(i => ({
        url: fixImg(i.url)
      }))
    : (parseImages(instrument?.images) || []).map(url => ({ url }))
  const bannerImages = bannerImagesSource.length > 0
    ? bannerImagesSource
    : [{ url: 'data:image/svg+xml,' + encodeURIComponent('<svg xmlns="http://www.w3.org/2000/svg" width="375" height="232" viewBox="0 0 375 232"><rect fill="#f0f0f0" width="375" height="232"/><text x="188" y="120" text-anchor="middle" fill="#ccc" font-size="20">暂无图片</text></svg>') }]

  if (loading) {
    return <View style={{ padding: 16 }}>加载中...</View>
  }

  if (!instrument) {
    return <View style={{ padding: 16 }}>乐器不存在</View>
  }

  const levelName = instrument.level_name || ''

  const levelBg = levelName.includes('大师') ? '#8A2BE2'
    : levelName.includes('专业') ? '#0084FF'
    : levelName.includes('入门') ? '#FF6B00'
    : '#71717a'

  return (
    <View style={{ minHeight: '100vh', backgroundColor: '#f4f4f5', paddingBottom: 140, display: 'flex', flexDirection: 'column', position: 'relative' }}>
      {/* Banner carousel */}
      <View style={{ width: '100%', overflow: 'hidden' }}
        onTouchStart={(e) => { bannerTouchStartXRef.current = e.touches[0].clientX }}
        onTouchEnd={(e) => {
          const diff = e.changedTouches[0].clientX - bannerTouchStartXRef.current
          if (Math.abs(diff) > 50 && bannerImages.length > 1) {
            setCurrentBanner(prev => {
              if (diff < 0) {
                if (prev >= bannerImages.length - 1) return prev + 1
                return prev + 1
              }
              if (prev <= 0) return prev - 1
              return prev - 1
            })
          }
        }}
      >
        <View style={{ width: '100%', overflow: 'hidden', height: Math.round(getWindowSize().width * 4 / 3) }}>
          <View style={{ display: 'flex', flexDirection: 'row', height: '100%', width: `${(bannerImages.length + 2) * 100}%`, transform: `translateX(-${(currentBanner + 1) * (100 / (bannerImages.length + 2))}%)`, transition: jumpReset ? 'none' : 'transform 0.5s ease-in-out' }}
            onTransitionEnd={() => {
              if (currentBanner === -1) {
                setJumpReset(true)
                setCurrentBanner(bannerImages.length - 1)
                setTimeout(() => setJumpReset(false), 50)
              } else if (currentBanner === bannerImages.length) {
                setJumpReset(true)
                setCurrentBanner(0)
                setTimeout(() => setJumpReset(false), 50)
              }
            }}>
            {bannerImages.length > 0 && (
              <View key="clone-last" style={{ height: '100%', width: `${100 / (bannerImages.length + 2)}%` }}>
                <Image src={bannerImages[bannerImages.length - 1].url || bannerImages[bannerImages.length - 1]} style={{ width: '100%', height: '100%' }}
                  mode="aspectFill"
                  onClick={() => {
                    try {
                      const urls = bannerImages.map(img => img.url || img)
                      previewImage({ urls, current: urls[urls.length - 1] })
                    } catch (e) { console.warn('[Preview] failed:', e) }
                  }} />
              </View>
            )}
            {bannerImages.map((img, i) => (
              <View key={i} style={{ height: '100%', width: `${100 / (bannerImages.length + 2)}%` }}>
                <Image src={fixImg(img.url || img)} style={{ width: '100%', height: '100%' }}
                  mode="aspectFill"
                  onClick={() => {
                    try {
                      const urls = bannerImages.map(img => fixImg(img.url || img))
                      previewImage({ urls, current: fixImg(img.url || img) })
                    } catch (e) { console.warn('[Preview] previewImage failed:', e) }
                  }} />
              </View>
            ))}
            {bannerImages.length > 0 && (
              <View key="clone-first" style={{ height: '100%', width: `${100 / (bannerImages.length + 2)}%` }}>
                <Image src={bannerImages[0].url || bannerImages[0]} style={{ width: '100%', height: '100%' }}
                  mode="aspectFill"
                  onClick={() => {
                    try {
                      const urls = bannerImages.map(img => img.url || img)
                      previewImage({ urls, current: urls[0] })
                    } catch (e) { console.warn('[Preview] previewImage failed:', e) }
                  }} />
              </View>
            )}
          </View>
        </View>
        <View style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', paddingBottom: 12, backgroundColor: '#f4f4f5' }}>
          {bannerImages.map((_, i) => (
            <View key={i} style={{
              width: i === currentBanner ? 12 : 6,
              height: 6,
              borderRadius: 999,
              backgroundColor: i === currentBanner ? '#915F38' : 'rgba(0,0,0,0.15)',
              marginLeft: i > 0 ? 6 : 0
            }} />
          ))}
        </View>
      </View>

      <ScrollView style={{ width: '100%', flex: '1 1 0%' }} scrollY scrollWithAnimation showScrollbar={false}>
        <View style={{ paddingLeft: 16, paddingRight: 16, marginTop: 16, paddingBottom: 16 }}>

          {/* Card A: Instrument info + deposit */}
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', display: 'flex', flexDirection: 'column', marginBottom: 12 }}>
            <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', width: '100%' }}>
              <View style={{ flex: '1 1 0%', minWidth: 0, paddingRight: 16 }}>
                <Text style={{ fontSize: 24, fontWeight: '900', color: '#000', letterSpacing: '0.025em', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{instrument.name || instrument.sn}</Text>
              </View>
              <View style={{ flexShrink: 0, whiteSpace: 'nowrap', textAlign: 'right' }}>
                <Text style={{ color: '#C21838', fontSize: 16, letterSpacing: '-0.025em' }}>
                  押金 ¥{deposit} <Text style={{ color: '#a1a1aa', fontWeight: '400' }}>❯</Text>
                </Text>
              </View>
            </View>
            <View style={{ display: 'flex', alignItems: 'center', marginTop: 8 }}>
              {levelName && (
                <View style={{ backgroundColor: levelBg, color: '#fff', fontSize: 10, fontWeight: '900', padding: '2px 10px', borderRadius: 999, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', marginRight: 12 }}>
                  {levelName}
                </View>
              )}
              <Text style={{ color: '#C21838', fontSize: 16, letterSpacing: '-0.025em' }}>
                日租 ¥{Number(dailyRent || instrument?.base_daily_rate || 0).toFixed(2)}/日
              </Text>
            </View>
            <View style={{ borderTop: '1px solid #f4f4f5', paddingTop: 12, marginTop: 12, display: 'flex', justifyContent: 'space-between', alignItems: 'center', fontSize: 12, color: '#71717a', fontWeight: '700' }}>
              <View style={{ display: 'flex', alignItems: 'center' }}><Text style={{ marginRight: 4 }}>🏠</Text><Text>{instrument.site_name || '暂无网点'}</Text></View>
              <View style={{ display: 'flex', alignItems: 'center' }}><Text style={{ marginRight: 4 }}>📍</Text><Text>{instrument.site_address || '暂无地址'}</Text></View>
              <View style={{ display: 'flex', alignItems: 'center' }}><Text style={{ marginRight: 4 }}>📞</Text><Text>{instrument.site_phone || ''}</Text></View>
            </View>
          </View>

          {/* Video player */}
          {liveVideo && (
            <View style={{ marginBottom: 12 }}>
              <Video
                src={fixImg(liveVideo.url)}
                poster={fixImg(liveVideo.thumb_url || '')}
                controls
                style={{ width: '100%', borderRadius: 16, maxHeight: 400 }}
              />
            </View>
          )}

          {/* Poster image */}
          {instrument?.poster && (
            <View style={{ marginBottom: 12 }}>
              <Image
                src={fixImg(instrument.poster)}
                style={{ width: '100%', borderRadius: 16, maxWidth: 750 }}
                mode="widthFix"
              />
            </View>
          )}

          {/* Card C: Specifications & properties */}
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginBottom: 12 }}>
            <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 8 }}>规格参数</Text>
            {instrument.properties && typeof instrument.properties === 'object' ? (
              Object.entries(instrument.properties).map(([key, vals]) => (
                <View key={key} style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 8 }}>
                  <Text style={{ fontSize: 14, fontWeight: '700', color: '#52525b' }}>{key}</Text>
                  <Text style={{ fontSize: 14, color: '#a1a1aa' }}>
                    {(Array.isArray(vals) ? vals : [vals]).join(', ') || '-'}
                  </Text>
                </View>
              ))
            ) : (
              <Text style={{ fontSize: 14, color: '#a1a1aa' }}>暂无规格参数</Text>
            )}
          </View>

          {/* Pricing V2 tiers */}
          {isRentable && pricingV2?.tiers?.length > 0 && (
            <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginBottom: 12 }}>
              <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 8 }}>定价策略</Text>
              {pricingV2.tiers.map((t, i) => {
                const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                const range = t.days_max > 0 ? `${prevMax + 1}-${t.days_max}天` : `${prevMax + 1}天以上`
                return (
                  <View key={i} style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 8 }}>
                    <Text style={{ color: '#71717a', fontSize: 14 }}>{range}</Text>
                    <Text style={{ fontWeight: '700', color: '#000', fontSize: 14 }}>¥{Math.round(t.daily_rate)}/天</Text>
                  </View>
                )
              })}
              {(pricingV2.deposit > 0 || pricingV2.shipping_fee > 0) && (
                <View style={{ borderTop: '1px solid #f4f4f5', paddingTop: 8, marginTop: 4 }}>
                  {pricingV2.deposit > 0 && (
                    <View style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                      <Text style={{ color: '#71717a', fontSize: 14 }}>押金</Text>
                      <Text style={{ color: '#000', fontSize: 14 }}>¥{pricingV2.deposit}</Text>
                    </View>
                  )}
                  {pricingV2.shipping_fee > 0 && (
                    <View style={{ display: 'flex', justifyContent: 'space-between', marginBottom: 4 }}>
                      <Text style={{ color: '#71717a', fontSize: 14 }}>物流费</Text>
                      <Text style={{ fontWeight: '700', color: '#000', fontSize: 14 }}>¥{pricingV2.shipping_fee}</Text>
                    </View>
                  )}
                </View>
              )}
              <View style={{ borderTop: '1px solid #f4f4f5', paddingTop: 8, marginTop: 4 }}>
                <View style={{ display: 'flex', justifyContent: 'space-between' }}>
                  <Text style={{ color: '#71717a', fontSize: 14 }}>合计金额</Text>
                  <Text style={{ color: '#000', fontWeight: '700', fontSize: 14 }}>预付全款租金 + 固定押金 + 往返运费</Text>
                </View>
              </View>
              <View style={{ borderTop: '1px solid #f4f4f5', paddingTop: 8, marginTop: 4 }}>
                <Text style={{ fontSize: 12, color: '#ea580c', fontWeight: '500' }}>
                  ⚠️ 逾期后每日自动扣款 ¥{overdueDailyFee}/日；押金归还质检通过后退还
                </Text>
              </View>
            </View>
          )}

          {/* Rent-to-own */}
          {isRentable && (
            <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginBottom: 12 }}>
              <View style={{ display: 'flex', alignItems: 'center' }}>
                <Text>🎁</Text>
                <Text style={{ fontWeight: '700', fontSize: 14, color: '#6b21a8', marginLeft: 4 }}>租购转化</Text>
              </View>
              <Text style={{ color: '#9333ea', fontSize: 14, marginTop: 4, fontWeight: '700' }}>
                租满12个月可直接获得所有权
              </Text>
            </View>
          )}

          {/* Service comparison */}
          {isRentable && (
            <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginBottom: 12 }} onClick={() => setShowComparison(true)}>
              <View style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                <Text style={{ fontSize: 16, fontWeight: '900', color: '#000' }}>服务权益对比</Text>
                <Text style={{ fontSize: 14, color: '#a1a1aa' }}>查看详情 ❯</Text>
              </View>
            </View>
          )}

          {/* Audit log section (staff only) */}
          {!isCustomer && currentUser && auditLogs.length > 0 && (
            <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 16, boxShadow: '0 1px 2px 0 rgba(0,0,0,0.05)', marginBottom: 12 }}>
              <Text style={{ fontSize: 16, fontWeight: '900', color: '#000', marginBottom: 12 }}>操作日志</Text>
              <View style={{ maxHeight: 192, overflowY: 'auto' }}>
                {auditLogs.map((log, i) => (
                  <View key={log.id || i} style={{ display: 'flex', alignItems: 'center', padding: '6px 8px', backgroundColor: '#f9fafb', borderRadius: 4, fontSize: 12, marginBottom: 8 }}>
                    <Text style={{ color: '#9ca3af', width: 112, flexShrink: 0 }}>{new Date(log.created_at).toLocaleString()}</Text>
                    <View style={{ padding: '2px 6px', borderRadius: 4, backgroundColor: '#fff', color: '#4b5563', fontWeight: '500', marginRight: 8 }}>
                      {{'CREATE': '创建', 'UPDATE': '编辑', 'DELETE': '删除', 'SHIP': '发货', 'RECEIVE': '收货', 'RETURN': '归还', 'INSPECT': '验收'}[log.action] || log.action}
                    </View>
                    <Text style={{ color: '#9ca3af' }}>{log.actor_name || ''}</Text>
                  </View>
                ))}
              </View>
            </View>
          )}

        </View>
      </ScrollView>

      {/* Floating cart icon */}
      {cartItemCount > 0 && (
        <View onClick={() => nav('/pages-weapp/cart/index')} style={{ position: 'fixed', bottom: 96, right: 16, backgroundColor: '#002140', color: '#fff', padding: 12, borderRadius: 999, boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)', zIndex: 50 }}>
          <Text style={{ fontSize: 20 }}>🛒</Text>
          <Text style={{ position: 'absolute', top: -4, right: -4, backgroundColor: '#ef4444', color: '#fff', fontSize: 12, width: 20, height: 20, borderRadius: 999, display: 'flex', alignItems: 'center', justifyContent: 'center', fontWeight: '700' }}>
            {cartItemCount}
          </Text>
        </View>
      )}

      {/* Bottom panel */}
      <View style={{ position: 'fixed', bottom: 0, left: 0, right: 0, backgroundColor: '#FDFBF7', borderTop: '1px solid #f4f4f5', padding: 16, display: 'flex', flexDirection: 'column', zIndex: 50, boxShadow: '0 -4px 6px -1px rgba(0,0,0,0.1)' }}>
        {isRentable ? (
          <>
            <View style={{ display: 'flex', width: '100%' }}>
              <View
                onClick={handleAddToCart}
                style={{ flex: '1 1 0%', height: 48, borderRadius: 999, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'linear-gradient(135deg, #E2B07E, #C98E54)', marginRight: 12 }}
              >
                <Text style={{ color: '#fff', fontWeight: '900', fontSize: 16 }}>加入购物车</Text>
              </View>
              <View
                onClick={() => {
                  const token = getToken()
                  let role = ''
                  try {
                    if (token) {
                      const payload = JSON.parse(atob(token.split('.')[1]))
                      role = payload.role || ''
                    }
                  } catch {}
                  if (!token || role === 'GUEST') {
                    session.setItem('post_auth_redirect', `/pages-weapp/detail/index?id=${id}`)
                    Taro.navigateTo({ url: '/pages-weapp/login/index' })
                    return
                  }
                  nav(`/pages-weapp/checkout/index?id=${id}`)
                }}
                style={{ flex: '1 1 0%', height: 48, borderRadius: 999, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', display: 'flex', alignItems: 'center', justifyContent: 'center', background: 'linear-gradient(135deg, #FA5E3C, #E63917)' }}
              >
                <Text style={{ color: '#fff', fontWeight: '900', fontSize: 16 }}>立即租赁</Text>
              </View>
            </View>
          </>
        ) : activeOrder ? (
          activeOrder.order_status === 'in_lease' ? (
            <View style={{ padding: 12, backgroundColor: '#f0fdf4', borderRadius: 8, marginBottom: 8 }}>
              <Text style={{ color: '#15803d', fontWeight: '500' }}>租赁中</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
              {activeOrder.end_date && new Date(activeOrder.end_date) < new Date() && (
                <Text style={{ color: '#dc2626', fontWeight: '700' }}>
                  超期 {Math.ceil((Date.now() - new Date(activeOrder.end_date).getTime()) / 86400000)} 天
                </Text>
              )}
              {isCustomer && (
              <View
                onClick={() => nav(`/pages-weapp/return-confirm/index?order_id=${activeOrder.order_id}&instrument=${id}`)}
                style={{ width: '100%', paddingTop: 8, paddingBottom: 8, backgroundColor: '#f97316', color: '#fff', borderRadius: 8, fontWeight: '500', textAlign: 'center', marginTop: 8 }}
              >
                <Text>归还乐器</Text>
              </View>
              )}
            </View>
          ) : activeOrder.order_status === 'returning' ? (
            <View style={{ padding: 12, backgroundColor: '#fff7ed', borderRadius: 8, marginBottom: 8 }}>
              <Text style={{ color: '#c2410c', fontWeight: '500' }}>归还中</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>该乐器正在归还流程中</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
              {activeOrder.deposit_refunded && (
                <Text style={{ color: '#16a34a', fontSize: 14, marginTop: 4 }}>押金已退还</Text>
              )}
            </View>
          ) : ['reserved', 'pending', 'paid', 'pending_shipment'].includes(activeOrder.order_status) ? (
            <View style={{ padding: 12, backgroundColor: '#eff6ff', borderRadius: 8, marginBottom: 8 }}>
              <Text style={{ color: '#1d4ed8', fontWeight: '500' }}>已预约</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
            </View>
          ) : ['in_transit', 'shipped'].includes(activeOrder.order_status) ? (
            <View style={{ padding: 12, backgroundColor: '#ecfeff', borderRadius: 8, marginBottom: 8, textAlign: 'center' }}>
              <Text style={{ color: '#0e7490', fontWeight: '500' }}>乐器物流中</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>该乐器正在运输途中</Text>
              {currentUser?.id === activeOrder?.user_id && (
                <View
                  onClick={() => nav(`/pages-weapp/receive-confirm/index?order_id=${activeOrder.order_id}&instrument=${id}`)}
                  style={{ width: '100%', paddingTop: 12, paddingBottom: 12, backgroundColor: '#22c55e', color: '#fff', borderRadius: 8, fontWeight: '500', marginTop: 8, textAlign: 'center' }}
                >
                  <Text>确认收货</Text>
                </View>
              )}
            </View>
          ) : activeOrder.order_status === 'expired' ? (
            <View style={{ padding: 12, backgroundColor: '#fef2f2', borderRadius: 8, marginBottom: 8 }}>
              <Text style={{ color: '#dc2626', fontWeight: '500' }}>已超期</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>
                租期：{formatDisplayDate(activeOrder.start_date)} 至 {formatDisplayDate(activeOrder.end_date)}
              </Text>
              {activeOrder.end_date && (
                <Text style={{ color: '#dc2626', fontWeight: '700' }}>
                  超期 {Math.ceil((Date.now() - new Date(activeOrder.end_date).getTime()) / 86400000)} 天
                </Text>
              )}
              <View
                onClick={() => nav(`/pages-weapp/return-confirm/index?order_id=${activeOrder.order_id}&instrument=${id}`)}
                style={{ width: '100%', paddingTop: 8, paddingBottom: 8, backgroundColor: '#f97316', color: '#fff', borderRadius: 8, fontWeight: '500', textAlign: 'center', marginTop: 8 }}
              >
                <Text>归还乐器</Text>
              </View>
            </View>
          ) : (
            <View style={{ padding: 12, backgroundColor: '#ecfeff', borderRadius: 8, marginBottom: 8, textAlign: 'center' }}>
              <Text style={{ color: '#0e7490', fontWeight: '500' }}>乐器物流中</Text>
              <Text style={{ color: '#6b7280', fontSize: 14 }}>该乐器正在运输途中</Text>
              {currentUser?.id === activeOrder?.user_id && (
                <View
                  onClick={() => nav(`/pages-weapp/receive-confirm/index?order_id=${activeOrder.order_id}&instrument=${id}`)}
                  style={{ width: '100%', paddingTop: 12, paddingBottom: 12, backgroundColor: '#22c55e', color: '#fff', borderRadius: 8, fontWeight: '500', marginTop: 8, textAlign: 'center' }}
                >
                  <Text>确认收货</Text>
                </View>
              )}
            </View>
          )
        ) : (
          <View style={{ padding: 12, backgroundColor: '#f3f4f6', borderRadius: 8, textAlign: 'center' }}>
            <Text style={{ color: '#6b7280', fontWeight: '500' }}>该乐器目前不可租赁</Text>
            <Text style={{ color: '#9ca3af', fontSize: 14, marginTop: 4 }}>乐器已被预约，暂时无法租赁</Text>
          </View>
        )}
      </View>

      {/* Cart toast modal */}
      {cartToast && (
        <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: 'rgba(0,0,0,0.5)', zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }} onClick={() => setCartToast(false)}>
          <View style={{ backgroundColor: '#fff', borderRadius: 12, padding: 24, marginLeft: 32, marginRight: 32, textAlign: 'center' }} onClick={e => e.stopPropagation()}>
            <Text style={{ color: '#22c55e', fontSize: 48, marginBottom: 12 }}>✓</Text>
            <Text style={{ fontSize: 18, fontWeight: '700', marginBottom: 4 }}>加入成功</Text>
            <Text style={{ color: '#6b7280', fontSize: 14, marginBottom: 16 }}>该乐器已添加到购物车</Text>
            <View style={{ display: 'flex' }}>
              <View
                onClick={() => { setCartToast(false); Taro.navigateBack() }}
                style={{ flex: '1 1 0%', paddingTop: 12, paddingBottom: 12, paddingLeft: 24, paddingRight: 24, border: '1px solid #d4d4d8', borderRadius: 8, color: '#52525b', textAlign: 'center', marginRight: 12 }}
              >
                <Text>继续浏览</Text>
              </View>
              <View
                onClick={() => { setCartToast(false); nav('/pages-weapp/cart/index') }}
                style={{ flex: '1 1 0%', paddingTop: 12, paddingBottom: 12, paddingLeft: 24, paddingRight: 24, backgroundColor: '#002140', color: '#fff', borderRadius: 8, textAlign: 'center' }}
              >
                <Text>提交订单</Text>
              </View>
            </View>
          </View>
        </View>
      )}

      {/* Fullscreen image/video overlay */}
      {fullscreenImage && (
        <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, backgroundColor: '#000', zIndex: 50, display: 'flex', alignItems: 'center', justifyContent: 'center' }} onClick={() => setFullscreenImage(null)}>
          {liveVideo && fullscreenImage === liveVideo.url ? (
            <Video src={fixImg(liveVideo.url)} poster={fixImg(liveVideo.thumb_url)} controls style={{ width: '100%', maxHeight: '100%' }} />
          ) : (
            <Image src={fixImg(fullscreenImage)} style={{ maxWidth: '100%', maxHeight: '100%' }} mode="aspectFit" />
          )}
        </View>
      )}

      {/* Service comparison modal */}
      {showComparison && (
        <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 9999, backgroundColor: 'rgba(0,0,0,0.5)', display: 'flex', alignItems: 'center', justifyContent: 'center' }}
          onClick={() => setShowComparison(false)}>
          <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 24, maxWidth: 560, width: '85%', maxHeight: '80%', overflow: 'auto' }}
            onClick={(e) => e.stopPropagation()}>
            <Text style={{ fontSize: 18, fontWeight: '900', marginBottom: 16 }}>📊 服务权益对比</Text>
            <View style={{ fontSize: 14 }}>
              <View style={{ backgroundColor: '#f3f4f6', display: 'flex' }}>
                <Text style={{ padding: 8, flex: '1 1 0%', fontWeight: '500' }}>权益项</Text>
                <Text style={{ padding: 8, flex: '1 1 0%', textAlign: 'center', fontWeight: '500' }}>入门级</Text>
                <Text style={{ padding: 8, flex: '1 1 0%', textAlign: 'center', fontWeight: '500' }}>专业级</Text>
                <Text style={{ padding: 8, flex: '1 1 0%', textAlign: 'center', fontWeight: '500', color: '#9333ea' }}>大师级</Text>
              </View>
              {SERVICE_ITEMS.map((item, idx) => (
                <View key={idx} style={{ display: 'flex', borderBottom: '1px solid #f3f4f6' }}>
                  <Text style={{ padding: 8, flex: '1 1 0%' }}>{item.name}</Text>
                  <Text style={{ padding: 8, flex: '1 1 0%', textAlign: 'center', color: item.entry === '✓' ? '#16a34a' : '#9ca3af' }}>{item.entry}</Text>
                  <Text style={{ padding: 8, flex: '1 1 0%', textAlign: 'center', color: item.professional === '✓' ? '#16a34a' : '#9ca3af' }}>{item.professional}</Text>
                  <Text style={{ padding: 8, flex: '1 1 0%', textAlign: 'center', fontWeight: '500', color: item.master === '✓' ? '#9333ea' : '#9ca3af' }}>{item.master}</Text>
                </View>
              ))}
            </View>
            <View style={{ marginTop: 16, display: 'flex', justifyContent: 'flex-end' }}>
              <View onClick={() => setShowComparison(false)}
                style={{ backgroundColor: '#915F38', color: '#fff', padding: '8px 24px', borderRadius: 8, fontSize: 14, fontWeight: '700' }}>
                <Text>关闭</Text>
              </View>
            </View>
          </View>
        </View>
      )}
    </View>
  )
}
