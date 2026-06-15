import { useState, useEffect, useCallback } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { View, Text, Image, ScrollView, Input } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { env } from '../platform'
import bannerImg from '../assets/banner.png'

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
  const monthlyRent = Math.round(dailyRate * 25)
  const levelName = instrument.level_name || ''
  const thumb = images[0] || INSTRUMENT_PLACEHOLDER

  const levelBg = levelName.includes('大师') ? 'bg-[#8A2BE2]'
    : levelName.includes('专业') ? 'bg-[#0084FF]'
    : levelName.includes('入门') ? 'bg-[#FF6B00]'
    : 'bg-zinc-500'

  return (
    <View className="bg-white rounded-l-2xl p-3 flex items-center shadow-md w-full" onClick={onClick}>
      <View className="w-28 h-28 bg-zinc-50 rounded-xl overflow-hidden flex-shrink-0 flex items-center justify-center">
        <Image src={thumb} className="w-24 h-24 object-contain" />
      </View>
      <View className="flex-1 ml-3 h-28 flex justify-between items-start pr-4 overflow-hidden">
        <View className="flex flex-col space-y-1 h-full justify-between py-0.5 min-w-0 flex-1">
          <View className="w-full min-w-0">
            <Text className="block text-3xl font-black text-black tracking-wide truncate">{instrument.name || instrument.sn}</Text>
            <Text className="block text-sm text-zinc-500 font-bold truncate">{instrument.category_name}</Text>
          </View>
          {levelName && (
            <View className={`inline-block ${levelBg} text-white text-sm px-2.5 py-0.5 rounded-full font-black self-start shadow-sm -mt-0.5`}>
              {levelName}
            </View>
          )}
        </View>
        <View className="h-full flex flex-col justify-end text-right self-end ml-2 flex-shrink-0 whitespace-nowrap">
          <Text className="text-[#C21838] font-black text-[26px] tracking-tight">
            ¥{monthlyRent}<Text className="text-base font-bold text-[#C21838]/70"> / 月</Text>
          </Text>
        </View>
      </View>
    </View>
  )
}

export default function Home() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const tenant = searchParams.get('tenant')

  const [categories, setCategories] = useState([])
  const [instruments, setInstruments] = useState([])
  const [loading, setLoading] = useState(true)
  const [selectedCategory, setSelectedCategory] = useState(null)

  const baseUrl = env.apiBaseUrl

  const fetchCategories = useCallback(async () => {
    try {
      const res = await apiFetch(`${baseUrl}/public/categories${tenant ? `?tenant=${tenant}` : ''}`)
      const result = await res.json()
      if (result.code === 20000) {
        setCategories(result.data?.list || [])
        if (result.data?.list?.length > 0) setSelectedCategory(result.data.list[0].id)
      }
    } catch {}
  }, [baseUrl, tenant])

  const fetchInstruments = useCallback(async () => {
    try {
      const res = await apiFetch(`${baseUrl}/public/instruments?page=1&pageSize=10${tenant ? `&tenant=${tenant}` : ''}`)
      const result = await res.json()
      if (result.code === 20000) {
        setInstruments((result.data?.list || []).filter(i => i.stock_status !== 'archived' && i.stock_status !== 'lost'))
      }
    } catch {}
  }, [baseUrl, tenant])

  useEffect(() => {
    fetchCategories()
    fetchInstruments().finally(() => setLoading(false))
  }, [fetchCategories, fetchInstruments])

  const navigateToCategory = (catId) => {
    const url = tenant ? `/instruments?category_id=${catId}&tenant=${tenant}` : `/instruments?category_id=${catId}`
    navigate(url)
  }

  const navigateToList = () => {
    const url = tenant ? `/instruments?tenant=${tenant}` : '/instruments'
    navigate(url)
  }

  return (
    <View className="h-screen w-screen bg-[#915F38] overflow-hidden flex flex-col relative antialiased">
      <ScrollView className="w-full flex-1 overflow-y-auto" scrollY scrollWithAnimation enhanced showScrollbar={false}>
        {/* A. Banner */}
        <View className="relative w-full h-[240px] bg-[#784A2B] flex flex-col items-center justify-center overflow-hidden">
          <Image src={bannerImg} className="absolute inset-0 w-full h-full object-cover" />
          <View className="absolute bottom-3 flex items-center space-x-1.5 justify-center w-full">
            <View className="w-1.5 h-1.5 rounded-full bg-white/40"></View>
            <View className="w-1.5 h-1.5 rounded-full bg-white/40"></View>
            <View className="w-3 h-1.5 rounded-full bg-white"></View>
          </View>
          <View className="absolute top-4 left-0 right-0 px-6 flex justify-center">
            <View className="w-[250px] h-[42px] bg-transparent rounded-full flex items-center px-4 border border-[#FFF]">
              <Text className="text-white/70 text-base mr-2">🔍</Text>
              <Input placeholder="搜索乐器..." placeholderStyle="color: rgba(255,255,255,0.4)" className="text-white text-sm flex-1 bg-transparent" />
            </View>
          </View>
        </View>

        {/* B. Sticky Category Menu */}
        <View className="sticky top-0 z-40 bg-[#FDFBF7] py-2 shadow-sm border-b border-zinc-100">
          <ScrollView className="w-full whitespace-nowrap pl-7" scrollX showScrollbar={false}>
            <View className="inline-flex items-center space-x-8 pr-4">
              {categories.map(item => (
                <Text
                  key={item.id}
                  className={`text-lg whitespace-nowrap ${selectedCategory === item.id ? 'font-black text-black border-b-2 border-black pb-0.5' : 'font-bold text-zinc-500/90'}`}
                  onClick={() => { setSelectedCategory(item.id); navigateToCategory(item.id) }}
                >
                  {item.name}
                </Text>
              ))}
            </View>
          </ScrollView>
        </View>

        {/* C. Instrument List */}
        <View className="pl-7 pr-0 pt-4 pb-20 space-y-4 bg-[#915F38]">
          {loading ? (
            Array(3).fill(0).map((_, i) => (
              <View key={i} className="bg-white rounded-l-2xl p-3 flex shadow-md">
                <View className="w-28 h-28 bg-zinc-200 rounded-xl animate-pulse flex-shrink-0" />
                <View className="flex-1 ml-3 pr-4 space-y-3">
                  <View className="h-5 bg-zinc-200 rounded w-3/4 animate-pulse" />
                  <View className="h-4 bg-zinc-200 rounded w-1/2 animate-pulse" />
                  <View className="h-6 bg-zinc-200 rounded w-1/3 animate-pulse" />
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
      </ScrollView>

      {/* D. Bottom Tabbar */}
      <View className="absolute bottom-0 left-0 right-0 bg-[#5A3B24] border-t border-[#4E321E] py-2 flex justify-around items-center z-50 shadow-2xl">
        <View className="flex flex-col items-center justify-center text-white" onClick={() => navigate('/')}>
          <Text className="text-xl mb-0.5">🏪</Text>
          <Text className="text-[10px] font-bold text-white">首页</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40" onClick={() => navigateToList()}>
          <Text className="text-xl mb-0.5">🪕</Text>
          <Text className="text-[10px] font-medium text-white/50">租赁</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40" onClick={() => { const url = tenant ? `/my-service?tenant=${tenant}` : '/my-service'; navigate(url) }}>
          <Text className="text-xl mb-0.5">🛠️</Text>
          <Text className="text-[10px] font-medium text-white/50">维修</Text>
        </View>
        <View className="flex flex-col items-center justify-center text-white/40" onClick={() => { const url = tenant ? `/profile?tenant=${tenant}` : '/profile'; navigate(url) }}>
          <Text className="text-xl mb-0.5">👤</Text>
          <Text className="text-[10px] font-medium text-white/50">我的</Text>
        </View>
      </View>
    </View>
  )
}
