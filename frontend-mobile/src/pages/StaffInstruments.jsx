import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import Taro from '@tarojs/taro'
import { View, Text, Image, Button, ScrollView } from '@tarojs/components'
import { apiFetch, getToken } from '../services/api'
import { ArrowLeft, Search, Truck } from 'lucide-react'
import { env, storage } from '../platform'
import { photoSrc } from '../utils/media'
import OptionSheet from '../components/OptionSheet'

const PLACEHOLDER_IMAGE = 'data:image/svg+xml,' + encodeURIComponent(`
  <svg xmlns="http://www.w3.org/2000/svg" width="200" height="160" viewBox="0 0 200 160">
    <rect fill="#f3f4f6" width="200" height="160"/>
    <text x="100" y="80" text-anchor="middle" fill="#9ca3af" font-size="14">无图片</text>
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

export default function StaffInstruments() {
  const navigate = useNavigate()
  const [instruments, setInstruments] = useState([])
  const [categories, setCategories] = useState([])
  const [selTop, setSelTop] = useState('')
  const [selSub, setSelSub] = useState('')
  const [picker, setPicker] = useState(null)
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20

  useEffect(() => {
    const fetchCategories = async () => {
      try {
        const baseUrl = env.apiBaseUrl
        const resp = await apiFetch(`${baseUrl}/categories`)
        const result = await resp.json()
        if (result.code === 20000) {
          setCategories(result.data?.list || result.data || [])
        }
      } catch {}
    }
    fetchCategories()
  }, [])

  // #1893: two-level category filter — top-level select, then its children.
  // /api/categories returns nested top-levels (children under sub_categories);
  // /public/categories (Home) is flat — support both shapes.
  const topCategories = categories.filter(c => !c.parent_id).map(cat => ({
    ...cat,
    sub_categories: (Array.isArray(cat.sub_categories) && cat.sub_categories.length > 0
      ? cat.sub_categories
      : categories.filter(c => c.parent_id === cat.id)
    ).sort((a, b) => (a.sort || 0) - (b.sort || 0)),
  }))
  const selectedTop = topCategories.find(c => c.id === selTop)
  const subCategories = selectedTop?.sub_categories || []
  const effCategory = selSub || selTop

  useEffect(() => {
    const fetchInstruments = async () => {
      try {
        setLoading(true)
        const baseUrl = env.apiBaseUrl
        let url = `${baseUrl}/instruments?page=${page}&pageSize=${pageSize}`
        if (effCategory) {
          url += `&category_id=${effCategory}`
        }
        const resp = await apiFetch(url)
        const result = await resp.json()
        if (result.code === 20000) {
          setInstruments(result.data?.list || [])
          setTotal(result.data?.total || 0)
        }
      } catch (err) {
        console.error('Failed to fetch instruments:', err)
      }
      setLoading(false)
    }
    fetchInstruments()
  }, [page, effCategory])

  const statusColor = {
    available: 'bg-green-100 text-green-700',
    rented: 'bg-blue-100 text-blue-700',
    maintenance: 'bg-orange-100 text-orange-700',
    archived: 'bg-gray-100 text-gray-700',
    lost: 'bg-gray-100 text-gray-700',
  }

  const statusLabel = {
    available: '可租',
    rented: '租赁中',
    maintenance: '维修中',
    archived: '已下架',
    lost: '已丢失',
  }

  return (
    <View className="min-h-screen pb-24" style={{backgroundColor: '#FDFBF7'}}>
      {!env.isMiniProgram && (
        <View className=" px-4 pt-4 pb-3 flex items-center gap-2" style={{ backgroundImage: 'linear-gradient(to bottom, #FDF4E7, #FFFFFF)' }}>
          <View onClick={() => navigate(-1)}><ArrowLeft size={20} className="text-black" /></View>
          <Text className="text-lg font-black text-black">乐器管理</Text>
        </View>
      )}

      <View className="bg-white mx-4 mt-3 rounded-2xl shadow-sm p-4 flex gap-2">
        <Button
          onClick={() => setPicker({ kind: 'top' })}
          style={{ margin: 0, height: 36, display: 'flex', alignItems: 'center' }}
          className={`px-3 rounded-full text-sm font-black ${
            selTop ? 'bg-black text-white' : 'bg-zinc-100 text-zinc-600'
          }`}
        >
          {selectedTop?.name || '全部分类'} ▼
        </Button>
        {subCategories.length > 0 && (
          <Button
            onClick={() => setPicker({ kind: 'sub' })}
            style={{ margin: 0, height: 36, display: 'flex', alignItems: 'center' }}
            className={`px-3 rounded-full text-sm font-black ${
              selSub ? 'bg-black text-white' : 'bg-zinc-100 text-zinc-600'
            }`}
          >
            {subCategories.find(c => c.id === selSub)?.name || '全部'} ▼
          </Button>
        )}
      </View>

      <View className="px-4 pt-3">
        {loading ? (
          <View className="text-center py-8 text-zinc-500 font-black">加载中...</View>
        ) : (
          <View style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
            {instruments.map(inst => (
              <View
                key={inst.id}
                className="bg-white rounded-2xl p-4 flex gap-3 cursor-pointer"
                onClick={() => env.isMiniProgram ? Taro.navigateTo({ url: `/pages-weapp/staff-instrument-detail/index?id=${inst.id}` }) : navigate(`/staff/instrument?id=${inst.id}`)}
              >
                {(() => {
                  const imgSrc = photoSrc(inst.cover_image || inst.thumbnail || inst.poster || parseImages(inst.images)[0]) || PLACEHOLDER_IMAGE
                  return (
                  <Image
                  src={imgSrc}
                  alt={inst.sn}
                  className="w-20 h-20 object-cover rounded-xl bg-zinc-100"
                  onError={(e) => { e.target.onerror = null; e.target.src = PLACEHOLDER_IMAGE }}
                />
                )})()}
                <View className="flex-1">
                  <Text className="font-black text-sm text-black">SN: {inst.sn}</Text>
                  <Text className="text-xs text-zinc-500 font-medium">{inst.category_name || inst.level_name || '-'}</Text>
                  <View className="flex items-center gap-2 mt-1">
                    <Text className={`text-xs px-2 py-0.5 rounded-full font-black ${statusColor[inst.stock_status] || 'bg-gray-100'}`}>
                      {statusLabel[inst.stock_status] || inst.stock_status}
                    </Text>
                    <Text className="text-xs text-zinc-400 font-medium">{inst.site_name}</Text>
                  </View>
                  {inst.stock_status === 'rented' && inst.tracking_number && (
                    <View className="flex items-center gap-1 mt-1 text-xs text-zinc-500 font-medium">
                      <Truck size={12} />
                      <Text>{inst.tracking_number}</Text>
                    </View>
                  )}
                </View>
              </View>
            ))}
          </View>
        )}

        {total > page * pageSize && (
          <Button
            onClick={() => setPage(p => p + 1)}
            className="w-full mt-4 py-3 bg-white rounded-2xl text-black font-black text-sm"
          >
            加载更多
          </Button>
        )}
      </View>

      <View className="fixed bottom-6 right-6">
        {(() => {
          const mapping = storage.getJSON('permission_mapping', {})
          const cusPerm = parseInt(storage.getItem('user_cus_perm') || '0')
          const bit = mapping['instrument:create']
          const ok = bit !== undefined && (cusPerm & (1 << bit)) !== 0
          return ok ? (
            <Button
              onClick={() => env.isMiniProgram ? Taro.navigateTo({ url: '/pages-weapp/staff-instrument-form/index' }) : navigate('/staff/instrument/new')}
              className="w-14 h-14 bg-black text-white rounded-full shadow-lg flex items-center justify-center text-2xl"
            >
              +
            </Button>
          ) : null
        })()}
      </View>

      {picker?.kind === 'top' && (
        <OptionSheet
          title="选择分类"
          options={[{ id: '', label: '全部分类' }, ...topCategories.map(c => ({ id: c.id, label: c.name }))]}
          onSelect={(id) => { setSelTop(id); setSelSub(''); setPage(1); setPicker(null) }}
          onClose={() => setPicker(null)}
        />
      )}
      {picker?.kind === 'sub' && (
        <OptionSheet
          title={selectedTop?.name || '选择二级分类'}
          options={[{ id: '', label: '全部' }, ...subCategories.map(c => ({ id: c.id, label: c.name }))]}
          onSelect={(id) => { setSelSub(id); setPage(1); setPicker(null) }}
          onClose={() => setPicker(null)}
        />
      )}
    </View>
  )
}
