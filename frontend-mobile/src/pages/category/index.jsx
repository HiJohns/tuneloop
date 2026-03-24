import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'

export default function CategoryPage() {
  const [categories, setCategories] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const navigate = useNavigate()
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchCategories()
  }, [])

  const fetchCategories = async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/categories`)
      if (!response.ok) throw new Error('Failed to fetch categories')
      
      const data = await response.json()
      if (data.code === 20000) {
        setCategories(data.data || [])
      } else {
        throw new Error(data.message || 'API error')
      }
    } catch (err) {
      setError(err.message)
      // Fallback data if API fails
      setCategories([
        { id: 1, name: '钢琴', icon: '🎹', level: 1, sort: 1, visible: true },
        { id: 2, name: '吉他', icon: '🎸', level: 1, sort: 2, visible: true },
        { id: 3, name: '古筝', icon: '🎵', level: 1, sort: 3, visible: true },
        { id: 4, name: '小提琴', icon: '🎻', level: 1, sort: 4, visible: true }
      ])
    } finally {
      setLoading(false)
    }
  }

  const handleCategoryClick = (category) => {
    if (category.visible === false) return
    navigate(`/instruments?category=${category.id}`)
  }

  if (loading) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center">
          <div className="text-lg mb-2">加载中...</div>
          <div className="text-gray-500">正在获取分类信息</div>
        </div>
      </div>
    )
  }

  if (error) {
    return (
      <div className="flex items-center justify-center h-screen">
        <div className="text-center text-red-600">
          <div className="text-lg mb-2">加载失败</div>
          <div className="text-sm">{error}</div>
        </div>
      </div>
    )
  }

  const visibleCategories = categories.filter(cat => cat.visible !== false)
  const primaryCategories = visibleCategories.filter(cat => cat.level === 1)
  const secondaryCategories = visibleCategories.filter(cat => cat.level === 2)

  return (
    <div className="min-h-screen bg-gray-50">
      <div className="bg-white shadow-sm p-4">
        <h1 className="text-2xl font-bold text-gray-900">乐器分类</h1>
        <p className="text-sm text-gray-600 mt-1">选择乐器分类浏览</p>
      </div>

      {/* 一级分类 */}
      <div className="p-4">
        <h2 className="text-lg font-semibold text-gray-800 mb-3">主要分类</h2>
        <div className="grid grid-cols-2 gap-4">
          {primaryCategories
            .sort((a, b) => (a.sort || 0) - (b.sort || 0))
            .map(category => (
            <div
              key={category.id}
              className="bg-white rounded-lg p-4 cursor-pointer hover:shadow-md transition-shadow border border-gray-200"
              onClick={() => handleCategoryClick(category)}
            >
              <div className="text-center">
                <div className="text-4xl mb-2">
                  {category.icon || '🎵'}
                </div>
                <div className="text-lg font-semibold text-gray-900">
                  {category.name}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* 二级分类 */}
      {secondaryCategories.length > 0 && (
        <div className="p-4">
          <h2 className="text-lg font-semibold text-gray-800 mb-3">更详细分类</h2>
          <div className="grid grid-cols-2 gap-3">
            {secondaryCategories
              .sort((a, b) => (a.sort || 0) - (b.sort || 0))
              .map(category => (
              <div
                key={category.id}
                className="bg-white rounded-md p-3 cursor-pointer hover:shadow-sm transition-shadow border border-gray-200"
                onClick={() => handleCategoryClick(category)}
              >
                <div className="text-center">
                  <div className="text-2xl mb-1">
                    {category.icon || '🎶'}
                  </div>
                  <div className="text-sm font-medium text-gray-900">
                    {category.name}
                  </div>
                </div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* 无数据提示 */}
      {visibleCategories.length === 0 && (
        <div className="flex items-center justify-center h-64">
          <div className="text-center text-gray-500">
            <div className="text-6xl mb-4">🎸</div>
            <div className="text-lg">暂无分类数据</div>
            <div className="text-sm text-gray-400 mt-2">请联系管理员添加分类</div>
          </div>
        </div>
      )}
    </div>
  )
}
