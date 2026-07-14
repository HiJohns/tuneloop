import { useState, useEffect } from 'react'
import Taro from '@tarojs/taro'
import { View, Text, Input, ScrollView, Image } from '@tarojs/components'
import { apiFetch } from '../../services/api'
import { env, storage } from '../../platform'

const baseUrl = env.apiBaseUrl

export default function Search() {
  const [query, setQuery] = useState('')
  const [results, setResults] = useState([])
  const [loading, setLoading] = useState(false)
  const [history, setHistory] = useState([])

  useEffect(() => {
    setHistory(storage.getJSON('search_history', []))
  }, [])

  const handleSearch = async (q) => {
    const term = (q || query).trim()
    if (!term) return
    setLoading(true)
    try {
      const resp = await apiFetch(`${baseUrl}/public/instruments/search?q=${term}`)
      const result = await resp.json()
      if (result.code === 20000) setResults(result.data?.list || [])
    } catch {}
    setLoading(false)
    // Save history
    const newHistory = [term, ...history.filter(h => h !== term)].slice(0, 5)
    setHistory(newHistory)
    storage.setJSON('search_history', newHistory)
  }

  const fixImg = (url) => url && !url.startsWith('http') && !url.startsWith('data:') ? `https://wx.cadenzayueqi.com${url}` : url

  return (
    <View style={{ height: '100vh', backgroundColor: '#f4f4f5', display: 'flex', flexDirection: 'column' }}>
      {/* Search bar */}
      <View style={{ backgroundColor: '#fff', padding: '10px 16px', display: 'flex', alignItems: 'center', gap: 8, borderBottom: '1px solid #e4e4e7' }}>
        <Input value={query} onInput={e => setQuery(e.detail.value)} confirmType="search" onConfirm={e => handleSearch(e.detail.value)}
          placeholder="搜索乐器 SN" style={{ flex: 1, height: 40, border: '1px solid #d4d4d8', borderRadius: 20, padding: '0 16px', fontSize: 14 }} />
        <Text onClick={() => handleSearch()} style={{ fontSize: 14, fontWeight: '700', color: '#915F38', padding: '4px 8px' }}>搜索</Text>
      </View>

      <ScrollView style={{ flex: 1 }}>
        {/* History */}
        {results.length === 0 && history.length > 0 && (
          <View style={{ padding: 16 }}>
            <Text style={{ fontSize: 14, fontWeight: '700', color: '#71717a', marginBottom: 12 }}>历史搜索</Text>
            {history.map((h, i) => (
              <Text key={i} onClick={() => { setQuery(h); handleSearch(h) }}
                style={{ fontSize: 14, color: '#915F38', paddingVertical: 8, display: 'block' }}>{h}</Text>
            ))}
          </View>
        )}

        {/* Results */}
        {loading && <Text style={{ textAlign: 'center', color: '#a1a1aa', padding: 32 }}>搜索中...</Text>}
        {results.map(item => (
          <View key={item.id} onClick={() => Taro.navigateTo({ url: `/pages-weapp/detail/index?id=${item.id}` })}
            style={{ backgroundColor: '#fff', marginLeft: 16, marginRight: 16, marginTop: 12, borderRadius: 16, padding: 16, boxShadow: '0 1px 2px rgba(0,0,0,0.05)', display: 'flex', gap: 12 }}>
            <Image src={fixImg(item.cover_image || '')} style={{ width: 80, height: 80, borderRadius: 8, backgroundColor: '#f4f4f5' }} mode="aspectFill" />
            <View style={{ flex: 1 }}>
              <Text style={{ fontSize: 16, fontWeight: '700', color: '#000' }}>{item.category_name}</Text>
              <Text style={{ fontSize: 12, color: '#71717a', marginTop: 4 }}>SN: {item.sn}</Text>
              <Text style={{ fontSize: 12, color: '#71717a' }}>{item.level_name}</Text>
              <Text style={{ fontSize: 14, fontWeight: '700', color: '#C21838', marginTop: 8 }}>{Math.round(item.base_daily_rate || 0)}/日</Text>
            </View>
          </View>
        ))}
        {results.length === 0 && query && !loading && (
          <Text style={{ textAlign: 'center', color: '#a1a1aa', padding: 32 }}>未找到匹配的乐器</Text>
        )}
      </ScrollView>
    </View>
  )
}
