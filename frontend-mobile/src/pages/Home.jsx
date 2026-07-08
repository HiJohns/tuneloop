import { useState, useEffect, useCallback, useRef } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, ScrollView, Input } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { env, getWindowSize } from '../platform'
import BottomNav from '../components/BottomNav'

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

  const levelBg = levelName.includes('大师') ? 'bg-[#8A2BE2]'
    : levelName.includes('专业') ? 'bg-[#0084FF]'
    : levelName.includes('入门') ? 'bg-[#FF6B00]'
    : 'bg-zinc-500'

  return (
    <View className="bg-white rounded-2xl p-3 flex items-center shadow-md w-full" onClick={onClick}>
      <View className="w-20 h-20 bg-zinc-50 rounded-xl overflow-hidden flex-shrink-0 flex items-center justify-center">
        <Image src={thumb} className="w-[72px] h-[72px] object-contain" />
      </View>
      <View className="flex-1 ml-3 h-20 flex justify-between items-start pr-4 overflow-hidden">
        <View className="flex flex-col space-y-1 h-full justify-between py-0.5 min-w-0 flex-1">
          <View className="w-full min-w-0">
            <Text className="block text-[1.4rem] leading-[1.6rem] font-black text-black tracking-wide truncate">{instrument.name || instrument.sn}</Text>
            <Text className="block text-sm text-zinc-500 font-bold truncate">{instrument.category_name}</Text>
          </View>
          {levelName && (
            <View className={`inline-block ${levelBg} text-white text-sm px-2.5 py-0.5 rounded-full font-black self-start shadow-sm -mt-0.5`}>
              {levelName}
            </View>
          )}
        </View>
        <View className="h-full flex flex-col justify-end text-right self-end ml-2 flex-shrink-0 whitespace-nowrap">
          {instrument.stock_status === 'available' ? (
            <Text className="text-[#C21838] font-black text-[26px] tracking-tight">
              ¥{monthlyRent}<Text className="text-base font-bold text-[#C21838]/70"> / 月</Text>
            </Text>
          ) : (
            <Text className="text-zinc-400 font-bold text-base">租赁中</Text>
          )}
        </View>
      </View>
    </View>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const tenant = searchParams.get('tenant')
  const categoryFromUrl = searchParams.get('category_id') || null

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

  useEffect(() => {
    const urlCat = searchParams.get('category_id') || null
    if (urlCat !== selectedCategory) {
      setSelectedCategory(urlCat)
    }
  }, [searchParams.get('category_id')])

  const handleCategoryChange = (catId) => {
    setSelectedCategory(catId)
    const params = new URLSearchParams(searchParams)
    if (catId) {
      params.set('category_id', catId)
    } else {
      params.delete('category_id')
    }
    setSearchParams(params)
  }

  const navigateToCategory = (catId) => {
    const url = tenant ? `/instruments?category_id=${catId}&tenant=${tenant}` : `/instruments?category_id=${catId}`
    navigate(url)
  }

  const navigateToList = () => {
    const token = getToken()
    let isStaff = false
    try {
      if (token) {
        const payload = JSON.parse(atob(token.split('.')[1]))
        isStaff = payload?.role && payload.role !== 'USER'
      }
    } catch {}
    const url = isStaff ? '/staff/orders' : '/my-leases'
    navigate(url)
  }

  return (
    <View className="h-screen w-screen overflow-hidden flex flex-col relative antialiased">
      {/* Z=0: Full-screen carousel background */}
      <View className="fixed inset-0 w-full h-full z-0">
        {banners.length > 0 && (
          <View className="flex flex-row h-full" style={{
            width: `${(banners.length + 2) * 100}%`,
            transform: `translateX(-${(currentBanner + 1) * (100 / (banners.length + 2))}%)`,
            transition: jumpReset ? 'none' : 'transform 0.5s ease-in-out'
          }}
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
              <View key="clone-last" className="h-full" style={{ width: `${100 / (banners.length + 2)}%`, backgroundColor: banners[banners.length - 1].bg_color || '#915F38' }}>
                <Image src={banners[banners.length - 1].image_url} className="w-full h-full" mode="aspectFill" />
              </View>
            )}
            {banners.map((item, i) => (
              <View
                key={i}
                className="h-full"
                style={{
                  width: `${100 / (banners.length + 2)}%`,
                  backgroundColor: item.bg_color || '#915F38'
                }}
                onClick={() => item.link_url && navigate(item.link_url)}
              >
                <Image
                  src={item.image_url}
                  className="w-full h-full"
                  mode="aspectFill"
                />
                {item.title ? (
                  <View className="absolute bottom-0 left-0 right-0 bg-black/40 px-4 py-2">
                    <Text className="text-white text-sm">{item.title}</Text>
                  </View>
                ) : null}
              </View>
            ))}
            {banners.length > 0 && (
              <View key="clone-first" className="h-full" style={{ width: `${100 / (banners.length + 2)}%`, backgroundColor: banners[0].bg_color || '#915F38' }}>
                <Image src={banners[0].image_url} className="w-full h-full" mode="aspectFill" />
              </View>
            )}
          </View>
        )}
      </View>

      {/* Carousel dots — hide on scroll */}
      {!scrolled && (
        <View className="absolute left-0 right-0 z-[40] flex items-center justify-center" style={{ bottom: 8 }}>
          <View className="flex items-center space-x-1.5">
            {(banners.length > 0 ? banners : Array.from({ length: 3 })).map((_, i) => {
              const r = currentBanner < 0 ? banners.length - 1 : currentBanner >= banners.length ? 0 : currentBanner
              return <View key={i} className={`${i === r ? 'w-3' : 'w-1.5'} h-1.5 rounded-full ${i === r ? 'bg-white' : 'bg-white/40'}`} />
            })}
          </View>
        </View>
      )}

        {/* Swipe layer — intercepts touch and mouse over the banner area */}
        {banners.length > 0 && (
          <View className="absolute top-0 left-0 right-0 z-[10002]" style={{ height: 240 }}
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
                if (currentItem?.link_url) navigate(currentItem.link_url)
              }
            }}
            onMouseDownCapture={(e) => { bannerTouchStartXRef.current = e.clientX }}
            onMouseUp={(e) => {
              const diff = e.clientX - bannerTouchStartXRef.current
              if (Math.abs(diff) > 50) {
                if (diff < 0) {
                  setCurrentBanner(prev => prev < banners.length - 1 ? prev + 1 : banners.length)
                } else {
                  setCurrentBanner(prev => prev > 0 ? prev - 1 : -1)
                }
              }
            }}
          />
        )}

      {/* Z=1: Dark solid overlay — ensures text readability on light banner images */}
      <View className="fixed top-0 left-0 right-0 z-[1] bg-black/40" style={{ height: 160 }} />

      {/* E layer: frosted backdrop — transparent→blurs carousel on scroll */}
      <View className={`fixed inset-0 z-[5] transition-colors duration-300 ${scrolled ? 'bg-[#5A3B24]/80' : 'bg-transparent'}`} />

      {/* A: Search bar — fixed above carousel */}
      <View className="fixed left-0 right-0 z-[10003] flex items-center justify-center" style={{ top: '60px' }}>
        <View className={`w-[250px] h-[42px] rounded-full flex items-center px-4 shadow-sm transition-all duration-300 ${scrolled ? 'bg-white/20 border border-white/10' : 'bg-white/20 border border-white/20'}`}>
          <Text className="text-base mr-2 text-white/70" style={{ textShadow: '0 1px 3px rgba(0,0,0,0.5)' }}>🔍</Text>
          <Input placeholder="搜索乐器..." placeholderStyle="color: rgba(255,255,255,0.4)" className="text-sm flex-1 bg-transparent text-white"
            style={{ textShadow: '0 1px 3px rgba(0,0,0,0.5)' }} />
        </View>
      </View>

      {/* Menu — fixed overlay when stuck, z above search bar */}
      {menuStuck && (
        <View className="fixed left-0 right-0 z-[10001] bg-transparent" style={{ top: '102px' }}>
          <MenuContent categories={topCategories} selectedCategory={selectedCategory} onCategoryChange={handleCategoryChange} catOffsetX={catOffsetX} setCatOffsetX={setCatOffsetX} scrolled={true} />
        </View>
      )}

      {/* B: clip layer — wraps both ScrollView and BottomNav, overflow:hidden clips at edges */}
       <View className="fixed left-0 right-0 z-[100] flex flex-col" style={{ top: '142px', bottom: 0, overflow: 'hidden' }}>
        <ScrollView className="flex-1 overflow-y-auto bg-transparent"
          scrollY scrollWithAnimation enhanced showScrollbar={false}
          onScroll={e => setScrollY(e.detail?.scrollTop ?? e.target?.scrollTop ?? 0)}>
          <View style={{ height: '100px' }}></View>

          <View className={menuStuck ? 'opacity-0' : 'bg-transparent'}>
            <MenuContent categories={topCategories} selectedCategory={selectedCategory} onCategoryChange={handleCategoryChange} catOffsetX={catOffsetX} setCatOffsetX={setCatOffsetX} scrolled={false} />
          </View>

        <View>
          <View className="px-4 pt-4 pb-20 space-y-4">
          {loading ? (
            Array(3).fill(0).map((_, i) => (
              <View key={i} className="bg-white rounded-2xl p-3 flex shadow-md">
                <View className="w-20 h-20 bg-zinc-200 rounded-xl animate-pulse flex-shrink-0" />
                <View className="flex-1 ml-3 pr-4 space-y-2">
                  <View className="h-5 bg-zinc-200 rounded w-3/4 animate-pulse" />
                  <View className="h-4 bg-zinc-200 rounded w-1/2 animate-pulse" />
                </View>
              </View>
            ))
          ) : instruments.length > 0 ? (
            instruments.map(instrument => (
              <InstrumentCard
                key={instrument.id}
                instrument={instrument}
                onClick={() => { const url = tenant ? `/instrument/${instrument.id}?tenant=${tenant}` : `/instrument/${instrument.id}`; navigate(url) }}
              />
            ))
          ) : (
            <View className="text-center py-16 text-white/60">
              <Text className="text-5xl block mb-4">🎵</Text>
              <Text className="text-lg">暂无乐器</Text>
            </View>
          )}
          </View>
        </View>
        </ScrollView>
        <View>
        <BottomNav
          active="home"
          tabs={[
            { key: 'home', icon: '🏪', label: '首页', onClick: () => navigate('/') },
            { key: 'rent', icon: '🪕', label: '租赁', onClick: navigateToList },
            { key: 'service', icon: '🛠️', label: '维修', onClick: () => { const url = tenant ? `/my-repairs?tenant=${tenant}` : '/my-repairs'; navigate(url) } },
            { key: 'profile', icon: '👤', label: '我的', onClick: () => { const url = tenant ? `/profile?tenant=${tenant}` : '/profile'; navigate(url) } },
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
    <View className="w-full overflow-hidden pl-7 bg-black/20 py-1"
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
      <View className="inline-flex items-center space-x-8 pr-4"
        style={{ transform: `translateX(${catOffsetX}px)`, whiteSpace: 'nowrap' }}
      >
        {items.map(item => (
          <Text
            key={item.id || 'all'}
            className={`text-lg whitespace-nowrap ${selectedCategory === item.id ? `font-black border-b-2 pb-0.5 text-white border-white` : `font-bold text-white/80`}`}
            style={{ textShadow: '0 1px 4px rgba(0,0,0,0.6)' }}
            onClick={() => onCategoryChange(item.id)}
          >
            {item.name}
          </Text>
            ))}
          </View>
    </View>
  )
}
