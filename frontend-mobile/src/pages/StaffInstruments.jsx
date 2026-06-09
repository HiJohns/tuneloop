import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { apiFetch, getToken } from '../services/api'
import { ArrowLeft, Search, Truck } from 'lucide-react'

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
  const [activeCategory, setActiveCategory] = useState('全部')
  const [loading, setLoading] = useState(true)
  const [page, setPage] = useState(1)
  const [total, setTotal] = useState(0)
  const pageSize = 20

  useEffect(() => {
    const fetchInstruments = async () => {
      try {
        setLoading(true)
        const baseUrl = import.meta.env.VITE_API_BASE_URL || '/api'
        let url = `${baseUrl}/instruments?page=${page}&pageSize=${pageSize}`
        if (activeCategory !== '全部') {
          url += `&category_id=${activeCategory}`
        }
        const resp = await apiFetch(url)
        const result = await resp.json()
        if (result.code === 20000) {
          setInstruments(result.data?.list || [])
          setTotal(result.data?.total || 0)
          if (page === 1) {
            const cats = ['全部', ...new Set((result.data?.list || []).map(i => i.category_name || i.category).filter(Boolean))]
            setCategories(cats)
          }
        }
      } catch (err) {
        console.error('Failed to fetch instruments:', err)
      }
      setLoading(false)
    }
    fetchInstruments()
  }, [page, activeCategory])

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
    <div className="min-h-screen bg-brand-bg pb-20">
      <div className="bg-brand-primary text-white px-4 py-4 flex items-center gap-3">
        <button onClick={() => navigate(-1)}>
          <ArrowLeft size={20} />
        </button>
        <h1 className="text-lg font-bold">乐器管理</h1>
      </div>

      <div className="bg-white border-b overflow-x-auto">
        <div className="flex px-4 py-3 gap-2">
          {categories.map(cat => (
            <button
              key={cat}
              onClick={() => { setActiveCategory(cat); setPage(1) }}
              className={`px-3 py-1.5 rounded-full text-sm whitespace-nowrap ${
                activeCategory === cat
                  ? 'bg-brand-primary text-white'
                  : 'bg-gray-100 text-gray-600'
              }`}
            >
              {cat}
            </button>
          ))}
        </div>
      </div>

      <div className="p-4">
        {loading ? (
          <div className="text-center py-8 text-gray-500">Loading...</div>
        ) : (
          <div className="space-y-3">
            {instruments.map(inst => (
              <div
                key={inst.id}
                className="bg-white rounded-xl p-3 shadow-sm flex gap-3"
                onClick={() => navigate(`/staff/instrument/${inst.id}`)}
              >
                {(() => {
                  const instImages = parseImages(inst.images)
                  return (
                  <img
                  src={instImages[0] || PLACEHOLDER_IMAGE}
                  alt={inst.sn}
                  className="w-20 h-20 object-cover rounded-lg bg-gray-100"
                  onError={(e) => { e.target.onerror = null; e.target.src = PLACEHOLDER_IMAGE }}
                />
                )})()}
                <div className="flex-1">
                  <h3 className="font-medium text-sm">SN: {inst.sn}</h3>
                  <p className="text-xs text-gray-500">{inst.category_name || inst.level_name || '-'}</p>
                  <div className="flex items-center gap-2 mt-1">
                    <span className={`text-xs px-2 py-0.5 rounded-full ${statusColor[inst.stock_status] || 'bg-gray-100'}`}>
                      {statusLabel[inst.stock_status] || inst.stock_status}
                    </span>
                    <span className="text-xs text-gray-400">{inst.site_name}</span>
                  </div>
                  {inst.stock_status === 'rented' && inst.tracking_number && (
                    <div className="flex items-center gap-1 mt-1 text-xs text-gray-500">
                      <Truck size={12} />
                      <span>{inst.tracking_number}</span>
                    </div>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}

        {total > page * pageSize && (
          <button
            onClick={() => setPage(p => p + 1)}
            className="w-full mt-4 py-3 bg-white rounded-lg text-brand-primary text-sm"
          >
            Load More
          </button>
        )}
      </div>

      <div className="fixed bottom-6 right-6">
        {(() => {
          const mapping = JSON.parse(localStorage.getItem('permission_mapping') || '{}')
          const cusPerm = parseInt(localStorage.getItem('user_cus_perm') || '0')
          const bit = mapping['instrument:create']
          const ok = bit !== undefined && (cusPerm & (1 << bit)) !== 0
          return ok ? (
            <button
              onClick={() => navigate('/staff/instrument/new')}
              className="w-14 h-14 bg-brand-primary text-white rounded-full shadow-lg flex items-center justify-center text-2xl"
            >
              +
            </button>
          ) : null
        })()}
      </div>
    </div>
  )
}
