import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { Card, Button, Space, Tag, Image, message, Select, Input } from 'antd'
import { ShoppingCartOutlined, FilterOutlined, EyeOutlined } from '@ant-design/icons'
import { api } from '../services/api'

const { Option } = Select
const { Search } = Input

export default function InstrumentListUser() {
  const navigate = useNavigate()
  const [instruments, setInstruments] = useState([])
  const [loading, setLoading] = useState(false)
  const [categories, setCategories] = useState([])
  const [sites, setSites] = useState([])
  const [levels, setLevels] = useState([])
  
  // Filters
  const [categoryFilter, setCategoryFilter] = useState('')
  const [siteFilter, setSiteFilter] = useState('')
  const [levelFilter, setLevelFilter] = useState('')
  const [sortBy, setSortBy] = useState('price')
  const [searchText, setSearchText] = useState('')

  useEffect(() => {
    fetchInstruments()
    fetchFilters()
  }, [categoryFilter, siteFilter, levelFilter, sortBy, searchText])

  const fetchInstruments = async () => {
    setLoading(true)
    try {
      const params = {
        page: 1,
        pageSize: 20,
        sort: sortBy
      }
      if (categoryFilter) params.category_id = categoryFilter
      if (siteFilter) params.site_id = siteFilter
      if (levelFilter) params.level_id = levelFilter
      if (searchText) params.search = searchText
      
      const data = await api.get('/user/instruments', { params })
      setInstruments(data?.list || [])
    } catch (error) {
      console.error('Failed to fetch instruments:', error)
      message.error('加载乐器失败')
    } finally {
      setLoading(false)
    }
  }

  const fetchFilters = async () => {
    try {
      // Fetch categories
      const catData = await api.get('/categories')
      setCategories(catData?.list || [])
      
      // Fetch sites
      const siteData = await api.get('/common/sites')
      setSites(siteData?.list || [])
      
      // Fetch levels
      const levelData = await api.get('/instruments/levels')
      setLevels(levelData?.list || [])
    } catch (error) {
      console.error('Failed to fetch filters:', error)
    }
  }

  const handleViewDetail = (instrumentId) => {
    navigate(`/instruments/${instrumentId}`)
  }

  const handleRent = (instrumentId, e) => {
    e.stopPropagation()
    navigate(`/instruments/${instrumentId}`)
  }

  return (
    <div className="p-6">
      {/* Filters Toolbar */}
      <Card className="mb-6">
        <Space wrap className="w-full">
          <Search
            placeholder="搜索乐器名称/品牌"
            value={searchText}
            onChange={(e) => setSearchText(e.target.value)}
            style={{ width: 250 }}
            allowClear
          />
          
          <Select
            placeholder="选择类别"
            value={categoryFilter}
            onChange={setCategoryFilter}
            allowClear
            style={{ width: 150 }}
            suffixIcon={<FilterOutlined />}
          >
            {categories.map(cat => (
              <Option key={cat.id} value={cat.id}>{cat.name}</Option>
            ))}
          </Select>
          
          <Select
            placeholder="选择网点"
            value={siteFilter}
            onChange={setSiteFilter}
            allowClear
            style={{ width: 150 }}
          >
            {sites.map(site => (
              <Option key={site.id} value={site.id}>{site.name}</Option>
            ))}
          </Select>
          
          <Select
            placeholder="选择级别"
            value={levelFilter}
            onChange={setLevelFilter}
            allowClear
            style={{ width: 120 }}
          >
            {levels.map(level => (
              <Option key={level.id} value={level.id}>{level.name}</Option>
            ))}
          </Select>
          
          <Select
            placeholder="排序方式"
            value={sortBy}
            onChange={setSortBy}
            style={{ width: 120 }}
          >
            <Option value="price">按价格</Option>
            <Option value="distance">按距离</Option>
            <Option value="rating">按评分</Option>
          </Select>
        </Space>
      </Card>

      {/* Instruments Grid */}
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-6">
        {instruments.map(instrument => (
          <Card
            key={instrument.id}
            hoverable
            className="instrument-card"
            cover={
              <div className="relative">
                {instrument.images && instrument.images.length > 0 ? (
                  <Image
                    src={instrument.images[0]}
                    alt={instrument.name}
                    className="h-48 w-full object-cover"
                    preview={false}
                    fallback="/placeholder-instrument.png"
                  />
                ) : (
                  <div className="h-48 w-full bg-gray-200 flex items-center justify-center">
                    <div className="text-gray-400 text-center">
                      <div className="text-4xl mb-2">🎸</div>
                      <div>暂无图片</div>
                    </div>
                  </div>
                )}
                <Tag 
                  color={instrument.stock_status === 'available' ? 'green' : 'red'}
                  className="absolute top-2 right-2"
                >
                  {instrument.stock_status === 'available' ? '可租' : '已租'}
                </Tag>
              </div>
            }
            actions={[
              <Button 
                type="text" 
                icon={<EyeOutlined />}
                onClick={() => handleViewDetail(instrument.id)}
              >
                查看
              </Button>,
              <Button 
                type="primary" 
                icon={<ShoppingCartOutlined />}
                disabled={instrument.stock_status !== 'available'}
                onClick={(e) => handleRent(instrument.id, e)}
              >
                立即租赁
              </Button>
            ]}
          >
            <Card.Meta
              title={
                <div className="text-lg font-semibold">
                  {instrument.brand} {instrument.name}
                </div>
              }
              description={
                <Space direction="vertical" size="small" className="w-full">
                  <div className="text-gray-600 text-sm">
                    {instrument.category_name} | {instrument.level_name}
                  </div>
                  
                  {instrument.daily_rent && (
                    <div className="mt-2">
                      <div className="text-2xl font-bold text-blue-600">
                        ¥{instrument.daily_rent}
                        <span className="text-sm text-gray-500 ml-1">/天</span>
                      </div>
                      <div className="text-xs text-gray-400">
                        周租: ¥{instrument.weekly_rent} | 月租: ¥{instrument.monthly_rent}
                      </div>
                    </div>
                  )}
                  
                  {instrument.site_name && (
                    <div className="text-xs text-gray-500 mt-1">
                      📍 {instrument.site_name}
                    </div>
                  )}
                </Space>
              }
            />
          </Card>
        ))}
      </div>

      {/* Empty State */}
      {instruments.length === 0 && !loading && (
        <div className="text-center py-12">
          <div className="text-6xl mb-4">🎵</div>
          <div className="text-lg text-gray-500 mb-2">暂无符合条件的乐器</div>
          <div className="text-sm text-gray-400">请调整筛选条件或稍后重试</div>
        </div>
      )}

      {/* Loading */}
      {loading && (
        <div className="text-center py-12">
          <div className="text-lg text-gray-500">加载中...</div>
        </div>
      )}
    </div>
  )
}