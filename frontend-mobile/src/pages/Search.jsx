import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { View, Text, Image, ScrollView } from '@tarojs/components'
import { apiFetch } from '../services/api'
import { storage, env } from '../platform'
import { ArrowLeft } from 'lucide-react'

const baseUrl = env.apiBaseUrl

export default function Search() {
  const navigate = useNavigate()
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const [loading, setLoading] = useState(false)
  const [history, setHistory] = useState([])

  useEffect(() => {
    setHistory(storage.getJSON('search_history', []))
  }, [])

  const handleSearch = (q) => {
    const term = (q || query).trim()
    if (!term) return
    setLoading(true)
    apiFetch(`${baseUrl}/public/instruments/search?q=${term}`)
      .then(r => r.json())
      .then(res => {
        if (res.code === 20000) setResults(res.data?.list || [])
      })
      .catch(() => {})
      .finally(() => setLoading(false))
    const newHistory = [term, ...history.filter(h => h !== term)].slice(0, 5)
    setHistory(newHistory)
    storage.setJSON('search_history', newHistory)
  }

  const enterSearch = (e) => {
    if (e.key === 'Enter') handleSearch()
  }

  return (
    <View className="h-screen bg-zinc-100 flex flex-col">
      <View className="bg-white px-4 py-3 flex items-center gap-2 border-b border-zinc-100">
        <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
        <input
          value={query}
          onChange={e => setQuery(e.target.value)}
          onKeyDown={enterSearch}
          placeholder="搜索乐器 SN"
          className="flex-1 h-10 border border-zinc-300 rounded-full px-4 text-sm outline-none"
          autoFocus
        />
        <Text onClick={() => handleSearch()} className="text-sm font-black text-[#915F38] px-2 cursor-pointer">搜索</Text>
      </View>

      <ScrollView className="flex-1">
        {results.length === 0 && history.length > 0 && (
          <View className="p-4">
            <Text className="text-sm font-black text-zinc-500 mb-3">历史搜索</Text>
            {history.map((h, i) => (
              <Text key={i} onClick={() => { setQuery(h); handleSearch(h) }}
                className="text-sm font-black text-[#915F38] py-2 block cursor-pointer">{h}</Text>
            ))}
          </View>
        )}

        {loading && <Text className="text-center text-zinc-400 py-8 font-black">搜索中...</Text>}
        {results.map(item => (
          <View key={item.id} onClick={() => navigate(`/instrument/${item.id}`)}
            className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4 flex gap-3 cursor-pointer active:opacity-80">
            <Image src={item.cover_image || ''} className="w-20 h-20 rounded-xl bg-zinc-100" mode="aspectFill" />
            <View className="flex-1">
              <Text className="text-base font-black text-black">{item.category_name}</Text>
              <Text className="text-xs text-zinc-500 mt-1 font-medium">SN: {item.sn}</Text>
              <Text className="text-xs text-zinc-500 font-medium">{item.level_name}</Text>
              <Text className="text-sm font-black text-[#C21838] mt-2">{Math.round(item.base_daily_rate || 0)}/日</Text>
            </View>
          </View>
        ))}
        {results.length === 0 && query && !loading && (
          <Text className="text-center text-zinc-400 py-8 font-black">未找到匹配的乐器</Text>
        )}
      </ScrollView>
    </View>
  )
}
