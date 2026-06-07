import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Descriptions, Tag, Image, Row, Col, Button, Space, Divider, Tabs, Table, Spin, Empty, message, Popconfirm } from 'antd'
import { ArrowLeftOutlined, EditOutlined, DollarOutlined, UserOutlined, EnvironmentOutlined, CalendarOutlined, TruckOutlined } from '@ant-design/icons'
import { pricingApi, instrumentsApi } from '../../../services/api'

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

  const [pricingV2, setPricingV2] = useState(null)
  const [pricingV2Loading, setPricingV2Loading] = useState(false)

  useEffect(() => {
    if (id) {
      setPricingV2Loading(true)
      pricingApi.getInstrumentPricingV2(id).then(res => {
        if (res.code === 20000) setPricingV2(res.data)
      }).catch(err => {
        console.warn('Failed to load pricing-v2:', err)
      }).finally(() => {
        setPricingV2Loading(false)
      })
    }
  }, [id])

  const [mediaDetail, setMediaDetail] = useState(null)
  const [mediaLoading, setMediaLoading] = useState(false)

  useEffect(() => {
    if (!id) return
    setMediaLoading(true)
    instrumentsApi.getMedia(id).then(res => {
      if (res.code === 20000) setMediaDetail(res.data)
    }).catch(err => {
      console.warn('Failed to load media detail:', err)
    }).finally(() => {
      setMediaLoading(false)
    })
  }, [id])

  const handleSetDisplay = async (batchId) => {
    try {
      const res = await instrumentsApi.setMediaDisplay(id, { batch_id: batchId })
      if (res.code === 20000) {
        message.success('展示批次已更新')
        fetchInstrument()
        instrumentsApi.getMedia(id).then(r => { if (r.code === 20000) setMediaDetail(r.data) })
      }
    } catch (e) {
      message.error('更新失败: ' + e.message)
    }
  }

  const handleDeleteBatch = async (batchId) => {
    try {
      const res = await instrumentsApi.deleteMediaBatch(id, batchId)
      if (res.code === 20000) {
        message.success('批次已删除')
        fetchInstrument()
        instrumentsApi.getMedia(id).then(r => { if (r.code === 20000) setMediaDetail(r.data) })
      }
    } catch (e) {
      message.error('删除失败: ' + e.message)
    }
  }

  const handleDeleteVideo = async (batchId) => {
    handleDeleteBatch(batchId)
  }

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
  const overdueDailyFee = pricing?.overdue_daily_fee || 0

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
                  {mediaLoading ? <Spin /> : (() => {
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
                        <div className="mb-4">
                          <h4 className="text-sm font-medium text-gray-600 mb-2">当前展示</h4>
                          {images.length > 0 ? (
                            <Image.PreviewGroup>
                              {images.map((item, index) => (
                                <Image
                                  key={index}
                                  src={item.url || item}
                                  alt={`${instrument.sn}-${index}`}
                                  width={80}
                                  height={80}
                                  className="mr-2 mb-2 object-cover rounded"
                                />
                              ))}
                            </Image.PreviewGroup>
                          ) : (
                            <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无展示图片" />
                          )}
                        </div>

                        {video && (
                          <div className="mb-4">
                            <h4 className="text-sm font-medium text-gray-600 mb-2">当前视频</h4>
                            <div className="flex items-start gap-4">
                              <video src={video.url} controls width="240" className="rounded" />
                              <Popconfirm title="确定删除此视频？" onConfirm={() => handleDeleteVideo(video.batch_id)}>
                                <Button danger size="small">删除视频</Button>
                              </Popconfirm>
                            </div>
                          </div>
                        )}

                        {mediaDetail?.groups?.length > 0 && (
                          <div>
                            <h4 className="text-sm font-medium text-gray-600 mb-2">历史批次</h4>
                            {mediaDetail.groups.map(group => (
                              <Card key={group.batch_id} size="small" className="mb-2"
                                title={
                                  <Space>
                                    <Tag>{group.batch_type}</Tag>
                                    <span className="text-xs text-gray-400">{new Date(group.created_at).toLocaleString()}</span>
                                    {group.batch_id === (mediaDetail.display?.[0]?.batch_id) && <Tag color="green">当前展示</Tag>}
                                  </Space>
                                }
                                extra={
                                  <Space>
                                    <Button size="small" onClick={() => handleSetDisplay(group.batch_id)}>设为展示</Button>
                                    <Popconfirm title="删除此批次将同时删除批次内所有文件，确定？" onConfirm={() => handleDeleteBatch(group.batch_id)}>
                                      <Button danger size="small">删除</Button>
                                    </Popconfirm>
                                  </Space>
                                }
                              >
                                <Image.PreviewGroup>
                                  {group.items.filter(i => i.file_type === 'image').map((item, idx) => (
                                    <Image key={idx} src={item.url} width={60} height={60} className="mr-1 mb-1 object-cover rounded" />
                                  ))}
                                </Image.PreviewGroup>
                              </Card>
                            ))}
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
            <Card title="分段租金策略">
              {pricingV2Loading ? (
                <Spin />
              ) : pricingV2?.tiers?.length > 0 ? (
                <>
                  <Table
                    dataSource={pricingV2.tiers.map((t, i) => ({ ...t, _key: i }))}
                    rowKey="_key"
                    pagination={false}
                    columns={[
                      {
                        title: '阶段',
                        key: 'name',
                        width: 80,
                        render: (_, __, i) => `第${i + 1}阶`,
                      },
                      {
                        title: '天数范围',
                        key: 'range',
                        width: 150,
                        render: (_, r, i) => {
                          const prevMax = i > 0 ? pricingV2.tiers[i - 1].days_max : 0
                          const daysMax = r.days_max > 0 ? r.days_max : '以上'
                          return `${prevMax + 1}-${daysMax}天`
                        },
                      },
                      {
                        title: '日租金',
                        dataIndex: 'daily_rate',
                        key: 'daily',
                        width: 100,
                        render: (v) => `¥${(v || 0).toFixed(2)}`,
                      },
                    ]}
                  />
                  <Divider />
                  <Descriptions column={2} size="small" bordered>
                    <Descriptions.Item label="日均底价">
                      ¥{(pricingV2?.base_daily_rate || 0).toFixed(2)}/天
                    </Descriptions.Item>
                    <Descriptions.Item label="押金">
                      ¥{(pricingV2?.deposit || 0).toFixed(2)}
                    </Descriptions.Item>
                    <Descriptions.Item label="物流费">
                      ¥{(pricingV2?.shipping_fee ?? 0).toFixed(2)}
                    </Descriptions.Item>
                    <Descriptions.Item label="逾期日费">
                      ¥{(overdueDailyFee || 0).toFixed(2)}/天
                    </Descriptions.Item>
                  </Descriptions>
                </>
              ) : (
                <Empty description="暂未配置分阶段定价" />
              )}
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
