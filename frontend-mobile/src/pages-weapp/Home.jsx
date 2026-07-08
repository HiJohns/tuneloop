import { useState, useEffect, useCallback, useRef } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Image, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env, getWindowSize, dialog } from '../platform'
import BottomNav from '../components-weapp/BottomNav'
import * as S from '../styles-weapp'

const INSTRUMENT_PLACEHOLDER = 'data:image/svg+xml,' + encodeURIComponent(
  '<svg xmlns="http://www.w3.org/2000/svg" width="96" height="96" viewBox="0 0 96 96"><rect fill="#f0f0f0" width="96" height="96"/><text x="48" y="54" text-anchor="middle" fill="#ccc" font-size="24">🎸</text></svg>'
)

function parseImages(images) {
  if (!images) return []
  if (Array.isArray(images)) return images
  if (typeof images === 'string') {
    try { return JSON.parse(images) } catch { return [] }
  }
  return []
}

function getDailyRate(instrument) {
  const pricing = instrument.pricing
  if (!pricing) return instrument.base_daily_rate || 0
  if (typeof pricing === 'object' && !Array.isArray(pricing)) {
    return pricing.daily_rent || instrument.base_daily_rate || 0
  }
  if (typeof pricing === 'string') {
    try {
      const parsed = JSON.parse(pricing)
      if (Array.isArray(parsed)) return parsed[0]?.daily_rent || instrument.base_daily_rate || 0
      return parsed.daily_rent || instrument.base_daily_rate || 0
    } catch { return instrument.base_daily_rate || 0 }
  }
  return instrument.base_daily_rate || 0
}

function InstrumentCard({ instrument, onClick }) {
  const images = parseImages(instrument.images)
  const dailyRate = getDailyRate(instrument)
  const monthlyRent = Math.round(dailyRate * 30)
  const levelName = instrument.level_name || ''
  const thumb = instrument.cover_image || instrument.thumbnail || images[0] || INSTRUMENT_PLACEHOLDER

  const levelBg = levelName.includes('大师') ? '#8A2BE2'
    : levelName.includes('专业') ? '#0084FF'
    : levelName.includes('入门') ? '#FF6B00'
    : '#71717a'

  return (
    <View style={{ backgroundColor: '#fff', borderRadius: 16, padding: 12, display: 'flex', alignItems: 'center', boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)', width: '100%', marginBottom: 16 }} onClick={onClick}>
      <View style={{ width: 80, height: 80, backgroundColor: '#fafafa', borderRadius: 12, overflow: 'hidden', flexShrink: 0, display: 'flex', alignItems: 'center', justifyContent: 'center' }}>
        <Image src={thumb} style={{ width: 72, height: 72 }} mode="widthFix" />
      </View>
      <View style={{ flex: '1 1 0%', marginLeft: 12, height: 80, display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', paddingRight: 16, overflow: 'hidden' }}>
        <View style={{ display: 'flex', flexDirection: 'column', height: '100%', justifyContent: 'space-between', paddingTop: 2, paddingBottom: 2, minWidth: 0, flex: '1 1 0%' }}>
          <View style={{ width: '100%', minWidth: 0 }}>
            <Text style={{ fontSize: '1.4rem', lineHeight: '1.6rem', fontWeight: '900', color: '#000', letterSpacing: '0.025em', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{instrument.name || instrument.sn}</Text>
            <Text style={{ fontSize: 14, color: '#71717a', fontWeight: '700', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{instrument.category_name}</Text>
          </View>
          {levelName && (
            <View style={{ backgroundColor: levelBg, color: '#fff', fontSize: 14, padding: '2px 10px', borderRadius: 999, fontWeight: '900', alignSelf: 'flex-start', boxShadow: '0 1px 2px rgba(0,0,0,0.05)', marginTop: -2 }}>
              {levelName}
            </View>
          )}
        </View>
        <View style={{ height: '100%', display: 'flex', flexDirection: 'column', justifyContent: 'flex-end', textAlign: 'right', alignSelf: 'flex-end', marginLeft: 8, flexShrink: 0, whiteSpace: 'nowrap' }}>
          {instrument.stock_status === 'available' ? (
            <Text style={{ color: '#C21838', fontWeight: '900', fontSize: 26, letterSpacing: '-0.025em' }}>
              ¥{monthlyRent}<Text style={{ fontSize: 16, fontWeight: '700', color: 'rgba(194,24,56,0.7)' }}> / 月</Text>
            </Text>
          ) : (
            <Text style={{ color: '#a1a1aa', fontWeight: '700', fontSize: 16 }}>租赁中</Text>
          )}
        </View>
      </View>
    </View>
  )
}

export default function Home() {
  const nav = (url) => { Taro.navigateTo({ url }) }
  const instance = Taro.getCurrentInstance()
  const routerParams = instance.router?.params || {}
  const tenant = routerParams.tenant || null
  const categoryFromUrl = routerParams.category_id || null

  const [categories, setCategories] = useState([])
  const [instruments, setInstruments] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedCategory, setSelectedCategory] = useState(categoryFromUrl)
  const [banners, setBanners] = useState([])
  const [currentBanner, setCurrentBanner] = useState(0)
  const [jumpReset, setJumpReset] = useState(false)
  const [catOffsetX, setCatOffsetX] = useState(0)
  const [scrollY, setScrollY] = useState(0)
  const scrolled = scrollY > 50
  const menuStuck = scrollY > 130
  const topCategories = categories.filter(c => !c.parent_id)
  const catTouchStartRef = useRef({ x: 0, offset: 0 })
  const bannerTouchStartXRef = useRef(0)

  const baseUrl = env.apiBaseUrl
  const imageBaseUrl = baseUrl.replace(/\/api$/, '')

  const fetchCategories = useCallback(async () => {
    try {
      const res = await apiFetch(`${baseUrl}/public/categories${tenant ? `?tenant=${tenant}` : ''}`)
      const result = await res.json()
      if (result.code === 20000) {
        setCategories(result.data?.list || [])
      }
    } catch {}
  }, [baseUrl, tenant])

  const fetchInstruments = useCallback(async () => {
    try {
      let url = `${baseUrl}/public/instruments?page=1&pageSize=50`
      if (selectedCategory) url += `&category_id=${selectedCategory}`
      if (tenant) url += `&tenant=${tenant}`
      const res = await apiFetch(url)
      const result = await res.json()
      if (result.code === 20000) {
        setInstruments((result.data?.list || []).filter(i => i.stock_status !== 'archived' && i.stock_status !== 'lost'))
      }
    } catch {}
  }, [baseUrl, tenant, selectedCategory])

  const fetchBanners = useCallback(async () => {
    try {
      const res = await apiFetch(`${baseUrl}/public/banners`)
      const result = await res.json()
      if (result.code === 20000 && result.data?.list?.length > 0) {
        const list = result.data.list.map(b => ({
          ...b,
          image_url: b.image_url.startsWith('http') ? b.image_url : `${imageBaseUrl}${b.image_url}`
        }))
        setBanners(list)
      } else {
        setBanners([])
      }
    } catch {
      setBanners([])
    }
  }, [baseUrl, imageBaseUrl])

  useEffect(() => {
    fetchCategories()
    fetchInstruments().finally(() => setLoading(false))
    fetchBanners()
  }, [fetchCategories, fetchInstruments, fetchBanners])

  useEffect(() => {
    if (!banners.length) return
    const timer = setInterval(() => {
      setCurrentBanner(prev => {
        const next = prev >= banners.length ? 0 : prev < banners.length - 1 ? prev + 1 : banners.length
        return next
      })
    }, 4000)
    return () => clearInterval(timer)
  }, [banners.length])

  const handleCategoryChange = (catId) => {
    setSelectedCategory(catId)
  }

  return (
    <View style={{ height: '100vh', width: '100vw', overflow: 'hidden', display: 'flex', flexDirection: 'column', position: 'relative' }}>
      {/* Z=0: Full-screen carousel background */}
      <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, width: '100%', height: '100%', zIndex: 0 }}>
        {banners.length > 0 && (
          <View style={{ display: 'flex', flexDirection: 'row', height: '100%', width: `${(banners.length + 2) * 100}%`, transform: `translateX(-${(currentBanner + 1) * (100 / (banners.length + 2))}%)`, transition: jumpReset ? 'none' : 'transform 0.5s ease-in-out' }}
            onTransitionEnd={() => {
              if (currentBanner === -1) {
                setJumpReset(true)
                setCurrentBanner(banners.length - 1)
                setTimeout(() => setJumpReset(false), 50)
              } else if (currentBanner === banners.length) {
                setJumpReset(true)
                setCurrentBanner(0)
                setTimeout(() => setJumpReset(false), 50)
              }
            }}>
            {banners.length > 0 && (
              <View key="clone-last" style={{ height: '100%', width: `${100 / (banners.length + 2)}%`, backgroundColor: banners[banners.length - 1].bg_color || '#915F38' }}>
                <Image src={banners[banners.length - 1].image_url} style={{ width: '100%', height: '100%' }} mode="aspectFill" />
              </View>
            )}
            {banners.map((item, i) => (
              <View
                key={i}
                style={{ height: '100%', width: `${100 / (banners.length + 2)}%`, backgroundColor: item.bg_color || '#915F38' }}
                onClick={() => item.link_url && nav(item.link_url)}
              >
                <Image
                  src={item.image_url}
                  style={{ width: '100%', height: '100%' }}
                  mode="aspectFill"
                />
                {item.title ? (
                  <View style={{ position: 'absolute', bottom: 0, left: 0, right: 0, backgroundColor: 'rgba(0,0,0,0.4)', paddingLeft: 16, paddingRight: 16, paddingTop: 8, paddingBottom: 8 }}>
                    <Text style={{ color: '#fff', fontSize: 14 }}>{item.title}</Text>
                  </View>
                ) : null}
              </View>
            ))}
            {banners.length > 0 && (
              <View key="clone-first" style={{ height: '100%', width: `${100 / (banners.length + 2)}%`, backgroundColor: banners[0].bg_color || '#915F38' }}>
                <Image src={banners[0].image_url} style={{ width: '100%', height: '100%' }} mode="aspectFill" />
              </View>
            )}
          </View>
        )}
      </View>

      {/* Carousel dots — hide on scroll */}
      {!scrolled && (
        <View style={{ position: 'absolute', left: 0, right: 0, zIndex: 40, display: 'flex', alignItems: 'center', justifyContent: 'center', bottom: 8 }}>
          <View style={{ display: 'flex', alignItems: 'center' }}>
            {(banners.length > 0 ? banners : Array.from({ length: 3 })).map((_, i) => {
              const r = currentBanner < 0 ? banners.length - 1 : currentBanner >= banners.length ? 0 : currentBanner
              return <View key={i} style={{
                width: i === r ? 12 : 6,
                height: 6,
                borderRadius: 999,
                backgroundColor: i === r ? '#fff' : 'rgba(255,255,255,0.4)',
                marginLeft: i > 0 ? 6 : 0
              }} />
            })}
          </View>
        </View>
      )}

      {/* Swipe layer — intercepts touch and mouse over the banner area */}
      {banners.length > 0 && (
        <View style={{ position: 'absolute', top: 0, left: 0, right: 0, zIndex: 10002, height: 240 }}
          onTouchStart={(e) => { bannerTouchStartXRef.current = e.touches[0].clientX }}
          onTouchEnd={(e) => {
            const diff = e.changedTouches[0].clientX - bannerTouchStartXRef.current
            if (Math.abs(diff) > 50) {
              if (diff < 0) {
                setCurrentBanner(prev => prev < banners.length - 1 ? prev + 1 : banners.length)
              } else {
                setCurrentBanner(prev => prev > 0 ? prev - 1 : -1)
              }
            } else {
              const currentItem = banners[currentBanner >= 0 && currentBanner < banners.length ? currentBanner : 0]
              if (currentItem?.link_url) nav(currentItem.link_url)
            }
          }}
        />
      )}

      {/* Z=1: Dark solid overlay — ensures text readability on light banner images */}
      <View style={{ position: 'fixed', top: 0, left: 0, right: 0, zIndex: 1, backgroundColor: 'rgba(0,0,0,0.4)', height: 160 }} />

      {/* E layer: frosted backdrop — transparent→blurs carousel on scroll */}
      <View style={{ position: 'fixed', top: 0, left: 0, right: 0, bottom: 0, zIndex: 5, backgroundColor: scrolled ? 'rgba(90,59,36,0.8)' : 'transparent', transition: 'background-color 0.3s' }} />

      {/* A: Search bar — fixed above carousel */}
      <View style={{ position: 'fixed', left: 0, right: 0, zIndex: 10000, display: 'flex', alignItems: 'center', justifyContent: 'center', top: '60px' }}>
        <View style={{ width: 250, height: 42, borderRadius: 999, display: 'flex', alignItems: 'center', paddingLeft: 16, paddingRight: 16, boxShadow: scrolled ? '0 1px 2px rgba(0,0,0,0.05)' : '0 1px 2px rgba(0,0,0,0.05)', backgroundColor: scrolled ? 'rgba(255,255,255,0.2)' : 'rgba(255,255,255,0.2)', border: scrolled ? '1px solid rgba(255,255,255,0.1)' : '1px solid rgba(255,255,255,0.2)' }}>
          <Text style={{ fontSize: 16, marginRight: 8, color: 'rgba(255,255,255,0.7)', textShadow: '0 1px 3px rgba(0,0,0,0.5)' }}>🔍</Text>
          <Input placeholder="搜索乐器..." placeholderStyle="color: rgba(255,255,255,0.4)"
            style={{ fontSize: 14, flex: '1 1 0%', backgroundColor: 'transparent', color: '#fff', textShadow: '0 1px 3px rgba(0,0,0,0.5)' }} />
        </View>
      </View>

      {/* Menu — fixed overlay when stuck, z above search bar */}
      {menuStuck && (
        <View style={{ position: 'fixed', left: 0, right: 0, zIndex: 10001, backgroundColor: 'transparent', top: '102px' }}>
          <MenuContent categories={topCategories} selectedCategory={selectedCategory} onCategoryChange={handleCategoryChange} catOffsetX={catOffsetX} setCatOffsetX={setCatOffsetX} scrolled={true} />
        </View>
      )}

      {/* B: clip layer — wraps both ScrollView and BottomNav, overflow:hidden clips at edges */}
      <View style={{ position: 'fixed', left: 0, right: 0, zIndex: 100, display: 'flex', flexDirection: 'column', top: '142px', bottom: 0, overflow: 'hidden' }}>
        <ScrollView style={{ flex: '1 1 0%', overflowY: 'auto', backgroundColor: 'transparent' }}
          scrollY scrollWithAnimation enhanced showScrollbar={false}
          onScroll={e => setScrollY(e.detail?.scrollTop ?? e.target?.scrollTop ?? 0)}>
          <View style={{ height: '100px' }}></View>

          <View style={{ opacity: menuStuck ? 0 : 1, backgroundColor: 'transparent' }}>
            <MenuContent categories={topCategories} selectedCategory={selectedCategory} onCategoryChange={handleCategoryChange} catOffsetX={catOffsetX} setCatOffsetX={setCatOffsetX} scrolled={false} />
          </View>

          <View>
            <View style={{ paddingLeft: 16, paddingRight: 16, paddingTop: 16, paddingBottom: 80 }}>
            {loading ? (
              Array(3).fill(0).map((_, i) => (
                <View key={i} style={{ backgroundColor: '#fff', borderRadius: 16, padding: 12, display: 'flex', boxShadow: '0 4px 6px -1px rgba(0,0,0,0.1)', marginBottom: 16 }}>
                  <View style={{ width: 80, height: 80, backgroundColor: '#e4e4e7', borderRadius: 12, flexShrink: 0 }}>
                    <View style={{ width: 80, height: 80, backgroundColor: '#d4d4d8', borderRadius: 12 }} />
                  </View>
                  <View style={{ flex: '1 1 0%', marginLeft: 12, paddingRight: 16 }}>
                    <View style={{ height: 20, backgroundColor: '#e4e4e7', borderRadius: 4, width: '75%', marginBottom: 8 }} />
                    <View style={{ height: 16, backgroundColor: '#e4e4e7', borderRadius: 4, width: '50%' }} />
                  </View>
                </View>
              ))
            ) : instruments.length > 0 ? (
              instruments.map(instrument => (
                <InstrumentCard
                  key={instrument.id}
                  instrument={instrument}
                  onClick={() => { const url = tenant ? `/pages-weapp/detail/index?id=${instrument.id}&tenant=${tenant}` : `/pages-weapp/detail/index?id=${instrument.id}`; nav(url) }}
                />
              ))
            ) : (
              <View style={{ textAlign: 'center', paddingTop: 64, paddingBottom: 64, color: 'rgba(255,255,255,0.6)' }}>
                <Text style={{ fontSize: 48, marginBottom: 16 }}>🎵</Text>
                <Text style={{ fontSize: 18 }}>暂无乐器</Text>
              </View>
            )}
            </View>
          </View>
        </ScrollView>
        <View>
          <BottomNav
            active="home"
            tabs={[
              { key: 'home', icon: '🏪', label: '首页', onClick: () => nav('/pages-weapp/home/index') },
              { key: 'rent', icon: '🪕', label: '租赁', onClick: () => dialog.toast('功能开发中') },
              { key: 'service', icon: '🛠️', label: '维修', onClick: () => dialog.toast('功能开发中') },
              { key: 'profile', icon: '👤', label: '我的', onClick: () => dialog.toast('功能开发中') },
            ]}
          />
        </View>
      </View>
    </View>
  )
}

function MenuContent({ categories, selectedCategory, onCategoryChange, catOffsetX, setCatOffsetX, scrolled }) {
  const items = [{ id: null, name: '全部' }, ...(categories || [])]
  const localTouchRef = useRef({ x: 0, offset: 0 })

  return (
    <View style={{ width: '100%', overflow: 'hidden', paddingLeft: 28, backgroundColor: 'rgba(0,0,0,0.2)', paddingTop: 4, paddingBottom: 4 }}
      onTouchStart={e => {
        localTouchRef.current = { x: e.touches[0].clientX, offset: catOffsetX }
      }}
      onTouchMove={e => {
        const dx = e.touches[0].clientX - localTouchRef.current.x
        if (Math.abs(dx) > 5) {
          setCatOffsetX(Math.min(0, Math.max(localTouchRef.current.offset + dx, -(items.length * 120 - 375))))
        }
      }}
    >
      <View style={{ display: 'inline-flex', alignItems: 'center', whiteSpace: 'nowrap', transform: `translateX(${catOffsetX}px)` }}>
        {items.map(item => (
          <Text
            key={item.id || 'all'}
            style={{
              fontSize: 18,
              whiteSpace: 'nowrap',
              textShadow: '0 1px 4px rgba(0,0,0,0.6)',
              marginRight: 32,
              fontWeight: selectedCategory === item.id ? '900' : '700',
              borderBottom: selectedCategory === item.id ? '2px solid #fff' : 'none',
              paddingBottom: selectedCategory === item.id ? 2 : 0,
              color: selectedCategory === item.id ? '#fff' : 'rgba(255,255,255,0.8)'
            }}
            onClick={() => onCategoryChange(item.id)}
          >
            {item.name}
          </Text>
        ))}
      </View>
    </View>
  )
}
