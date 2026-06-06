import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Descriptions, Tag, Image, Row, Col, Button, Space, Divider, Tabs, Statistic, Spin, Empty } from 'antd'
import { ArrowLeftOutlined, EditOutlined, DollarOutlined, UserOutlined, EnvironmentOutlined, CalendarOutlined, TruckOutlined } from '@ant-design/icons'

function parsePricing(pricing) {
  if (!pricing) return null
  if (Array.isArray(pricing)) return pricing[0] || null
  if (typeof pricing === 'string') {
    try { const arr = JSON.parse(pricing); return Array.isArray(arr) ? (arr[0] || null) : null } catch { return null }
  }
  if (typeof pricing === 'object') return pricing
  return null
}

export default function InstrumentDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [leaseData, setLeaseData] = useState(null)
  const [leaseLoading, setLeaseLoading] = useState(false)
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchInstrument()
  }, [id])

  useEffect(() => {
    if (instrument?.sn) fetchLeaseData()
  }, [instrument?.sn])

  const fetchInstrument = async () => {
    setLoading(true)
    try {
      const response = await fetch(`${API_BASE_URL}/instruments/${id}`)
      if (!response.ok) throw new Error('Failed to fetch instrument')
      const data = await response.json()
      if (data.code === 20000) {
        setInstrument(data.data)
      }
    } catch (error) {
      console.error('Failed to fetch instrument:', error)
    } finally {
      setLoading(false)
    }
  }

  const fetchLeaseData = async () => {
    if (!instrument?.sn) return
    setLeaseLoading(true)
    try {
      const orderResp = await fetch(`${API_BASE_URL}/orders/by-instrument-sn?sn=${encodeURIComponent(instrument.sn)}`)
      const orderResult = await orderResp.json()
      if (orderResult.code !== 20000 || !orderResult.data?.order_id) {
        setLeaseData(null)
        setLeaseLoading(false)
        return
      }

      const detailResp = await fetch(`${API_BASE_URL}/orders/${orderResult.data.order_id}`)
      const detailResult = await detailResp.json()
      if (detailResult.code !== 20000) {
        setLeaseData(null)
        setLeaseLoading(false)
        return
      }

      const order = detailResult.data
      let userData = null
      if (order.user_id) {
        const userResp = await fetch(`${API_BASE_URL}/users/${order.user_id}`)
        const userResult = await userResp.json()
        if (userResult.code === 20000) userData = userResult.data
      }

      setLeaseData({ order, user: userData })
    } catch (error) {
      console.error('Failed to fetch lease data:', error)
    }
    setLeaseLoading(false)
  }

  if (loading) {
    return <div className="p-6 flex items-center justify-center h-64"><Spin size="large" /></div>
  }

  if (!instrument) {
    return <div className="p-6">乐器不存在</div>
  }

  const statusMap = {
    available: { color: 'green', text: '可租' },
    reserved: { color: 'blue', text: '已预约' },
    shipping: { color: 'cyan', text: '物流中' },
    rented: { color: 'orange', text: '租赁中' },
    returning: { color: 'orange', text: '归还中' },
    maintenance: { color: 'red', text: '维修中' },
    archived: { color: 'default', text: '已下架' }
  }
  const statusConfig = statusMap[instrument.stock_status] || { color: 'default', text: '未知' }

  const levelMap = {
    beginner: '入门级',
    intermediate: '中级',
    advanced: '高级',
    professional: '专业级'
  }

  const pricing = parsePricing(instrument.pricing)

  const activeStatuses = ['reserved', 'shipping', 'rented', 'returning']

  return (
    <div className="p-6">
      {/* Header */}
      <div className="mb-6 flex justify-between items-start">
        <div>
          <Button 
            type="link" 
            icon={<ArrowLeftOutlined />}
            onClick={() => navigate(-1)}
            className="px-0 mb-2"
          >
            返回列表
          </Button>
          <h1 className="text-2xl font-bold text-gray-900">{instrument.sn}</h1>
          <p className="text-gray-600 mt-1">
            {instrument.category_name}
          </p>
        </div>
        <Button 
          type="primary" 
          icon={<EditOutlined />}
          onClick={() => navigate(`/instruments/${instrument?.id || id}/edit`)}
        >
          编辑
        </Button>
      </div>

      <Tabs defaultActiveKey="basic" items={[
        {
          label: '基本信息',
          key: 'basic',
          children: (
            <Row gutter={16}>
              <Col span={16}>
                <Card title="乐器信息">
                  <Descriptions column={2}>
                    <Descriptions.Item label="识别码">{instrument.sn}</Descriptions.Item>
                    <Descriptions.Item label="分类">{instrument.category_name}</Descriptions.Item>
                    <Descriptions.Item label="级别">
                      <Tag color="blue">{levelMap[instrument.level] || instrument.level}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="状态">
                      <Tag color={statusConfig.color}>{statusConfig.text}</Tag>
                    </Descriptions.Item>
                    <Descriptions.Item label="评分" span={2}>
                      {instrument.rating} ⭐ ({instrument.review_count} 评价)
                    </Descriptions.Item>
                    <Descriptions.Item label="创建时间" span={2}>
                      {new Date(instrument.created_at).toLocaleString()}
                    </Descriptions.Item>
                    <Descriptions.Item label="更新时间" span={2}>
                      {new Date(instrument.updated_at).toLocaleString()}
                    </Descriptions.Item>
                  </Descriptions>
                </Card>
                
                <Card title="描述" className="mt-4">
                  <p className="text-gray-700">{instrument.description}</p>
                </Card>
              </Col>
              
              <Col span={8}>
                <Card title="多媒体">
                  {(() => {
                    const media = instrument.media
                    const displayImages = media?.display?.filter(m => m.file_type === 'image') || []
                    const images = displayImages.length > 0
                      ? displayImages
                      : (instrument.images?.length ? instrument.images.map(u => ({ url: u, file_type: 'image' })) : [])
                    const video = (media?.video && media.video.url)
                      ? media.video
                      : (instrument.video ? { url: instrument.video } : null)
                    
                    return (
                      <>
                        {images.length > 0 ? (
                          <Image.PreviewGroup>
                            {images.map((item, index) => (
                              <Image
                                key={index}
                                src={item.url || item}
                                alt={`${instrument.sn}-${index}`}
                                width="100%"
                                height={150}
                                className="mb-2 object-cover rounded"
                              />
                            ))}
                          </Image.PreviewGroup>
                        ) : (
                          <div className="text-center text-gray-500 py-8">暂无图片</div>
                        )}
                        {video && (
                          <div className="mt-4">
                            <Divider>视频</Divider>
                            <video src={video.url} controls width="100%" className="rounded" />
                          </div>
                        )}
                      </>
                    )
                  })()}
                </Card>
              </Col>
            </Row>
          )
        },
        {
          label: '价格配置',
          key: 'pricing',
          children: (
            <Card title="租金价格">
              <Row gutter={16}>
                <Col span={6}>
                  <Statistic
                    title="日租金"
                    value={`¥${pricing?.daily_rent || 0}`}
                    prefix={<DollarOutlined />}
                  />
                </Col>
                <Col span={6}>
                  <Statistic
                    title="周租金"
                    value={`¥${pricing?.weekly_rent || 0}`}
                    prefix={<DollarOutlined />}
                  />
                </Col>
                <Col span={6}>
                  <Statistic
                    title="月租金"
                    value={`¥${pricing?.monthly_rent || 0}`}
                    prefix={<DollarOutlined />}
                  />
                </Col>
                <Col span={6}>
                  <Statistic
                    title="押金"
                    value={`¥${pricing?.deposit || 0}`}
                    prefix={<DollarOutlined />}
                    valueStyle={{ color: '#cf1322' }}
                  />
                </Col>
              </Row>
            </Card>
          )
        },
        {
          label: '租赁状态',
          key: 'lease',
          children: (
            <Card title="当前租赁信息">
              {leaseLoading ? (
                <Spin />
              ) : !activeStatuses.includes(instrument.stock_status) ? (
                <Empty description="当前无租赁信息" />
              ) : !leaseData ? (
                <Empty description="未找到关联订单" />
              ) : (
                <div className="space-y-4">
                  <Descriptions column={2} bordered size="small">
                    <Descriptions.Item label="租赁人">
                      <Space>
                        <UserOutlined />
                        {leaseData.user?.name || '-'}
                      </Space>
                    </Descriptions.Item>
                    <Descriptions.Item label="电话">
                      {leaseData.user?.phone || '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="地址" span={2}>
                      <Space>
                        <EnvironmentOutlined />
                        {leaseData.user?.address || '-'}
                      </Space>
                    </Descriptions.Item>
                    <Descriptions.Item label="租期开始">
                      <Space>
                        <CalendarOutlined />
                        {leaseData.order?.start_date || '-'}
                      </Space>
                    </Descriptions.Item>
                    <Descriptions.Item label="租期结束">
                      {leaseData.order?.end_date || '-'}
                    </Descriptions.Item>
                    {(instrument.stock_status === 'shipping' || instrument.stock_status === 'returning') && (
                      <>
                        <Descriptions.Item label="物流公司">
                          <Space>
                            <TruckOutlined />
                            {leaseData.order?.courier_company || '-'}
                          </Space>
                        </Descriptions.Item>
                        <Descriptions.Item label="物流单号">
                          {leaseData.order?.tracking_number || '-'}
                        </Descriptions.Item>
                      </>
                    )}
                  </Descriptions>

                  <div className="text-sm text-gray-500 space-y-1">
                    <p>日租金: ¥{leaseData.order?.monthly_rent ? (leaseData.order.monthly_rent / 30).toFixed(0) : 0}</p>
                    <p>月租金: ¥{leaseData.order?.monthly_rent || 0}</p>
                    <p>押金: ¥{leaseData.order?.deposit || 0}</p>
                  </div>
                </div>
              )}
            </Card>
          )
        }
      ]} />
    </div>
  )
}
