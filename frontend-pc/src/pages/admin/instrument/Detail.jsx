import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Descriptions, Tag, Image, Row, Col, Button, Space, Divider, Tabs, Table, Spin, Empty, message, Popconfirm, Input, InputNumber, Form, Select, TreeSelect, Switch } from 'antd'
import { ArrowLeftOutlined, DeleteOutlined, EditOutlined, DollarOutlined, UserOutlined, EnvironmentOutlined, CalendarOutlined, TruckOutlined } from '@ant-design/icons'
import { api, pricingApi, instrumentsApi, request } from '../../../services/api'

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

  useEffect(() => {
    fetchInstrument()
  }, [id])

  useEffect(() => {
    if (instrument?.sn) fetchLeaseData()
  }, [instrument?.sn])

  useEffect(() => {
    instrumentsApi.getLevels().then(res => {
      if (res.code === 20000) setLevels(res.data || [])
    }).catch(() => {})
    api.get('/categories').then(res => {
      if (res.code === 20000) {
        const mapTree = (nodes) => (nodes || []).map(n => ({
          value: n.id,
          title: n.name,
          children: n.sub_categories?.length > 0 ? mapTree(n.sub_categories) : undefined
        }))
        setCategoryTree(mapTree(res.data?.list || []))
      }
    }).catch(() => {})
  }, [])

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
  const [editingCard, setEditingCard] = useState(null)
  const [savingCard, setSavingCard] = useState(false)
  const [editValues, setEditValues] = useState({})
  const [levels, setLevels] = useState([])
  const [categoryTree, setCategoryTree] = useState([])
  const [activeTab, setActiveTab] = useState('basic')

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

  const handleDelete = async () => {
    try {
      const result = await api.delete(`/instruments/${id}`)
      if (result.code === 20000) {
        message.success('删除成功')
        navigate(-1)
      }
    } catch (e) {
      message.error('删除失败: ' + (e.message || ''))
    }
  }

  const handleSaveCard = async (card, fields) => {
    setSavingCard(true)
    try {
      const result = await api.put(`/instruments/${id}`, fields)
      if (result.code === 20000) {
        message.success('保存成功')
        setEditingCard(null)
        fetchInstrument()
      } else {
        message.error(result.message || '保存失败')
      }
    } catch (e) {
      message.error('保存失败: ' + (e.message || ''))
    } finally {
      setSavingCard(false)
    }
  }

  const fetchInstrument = async () => {
    setLoading(true)
    try {
      const data = await instrumentsApi.get(id)
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
      const orderResult = await api.get(`/orders/by-instrument-sn?sn=${encodeURIComponent(instrument.sn)}`)
      if (orderResult.code !== 20000 || !orderResult.data?.order_id) {
        setLeaseData(null)
        setLeaseLoading(false)
        return
      }

      const detailResult = await api.get(`/orders/${orderResult.data.order_id}`)
      if (detailResult.code !== 20000) {
        setLeaseData(null)
        setLeaseLoading(false)
        return
      }

      const order = detailResult.data
      let userData = null
      if (order.user_id) {
        const userResult = await api.get(`/users/${order.user_id}`)
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
    rented: { color: 'orange', text: '租赁中' },
    maintenance: { color: 'red', text: '维修中' },
    archived: { color: 'default', text: '已下架' },
    lost: { color: 'default', text: '已丢失' }
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

  const activeStatuses = ['rented']

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
        <Popconfirm title="确定要删除这个乐器吗？" onConfirm={handleDelete}>
          <Button danger icon={<DeleteOutlined />}>
            删除
          </Button>
        </Popconfirm>
      </div>

      <Tabs activeKey={activeTab} onChange={setActiveTab} items={[
        {
          label: '基本信息',
          key: 'basic',
          children: (
            <Row gutter={16}>
              <Col span={16}>
                <Card title="乐器信息"
                  extra={editingCard === 'basic' ? null : <EditOutlined className="cursor-pointer" onClick={() => { setEditingCard('basic'); setEditValues({}) }} />}
                >
                  {editingCard === 'basic' ? (
                    <div className="space-y-3">
                      <div>
                        <label className="text-sm text-gray-500">识别码</label>
                        <div className="text-sm font-medium py-1 px-2 bg-gray-50 rounded">{instrument.sn}</div>
                      </div>
                      <div>
                        <label className="text-sm text-gray-500">分类</label>
                        <TreeSelect
                          treeData={categoryTree}
                          value={editValues.category_id !== undefined ? editValues.category_id : instrument.category_id}
                          onChange={(val) => setEditValues(prev => ({ ...prev, category_id: val }))}
                          style={{ width: '100%' }}
                          placeholder="选择分类"
                        />
                      </div>
                      <div>
                        <label className="text-sm text-gray-500">级别</label>
                        <Select
                          value={editValues.level_id !== undefined ? editValues.level_id : instrument.level_id}
                          onChange={(val) => setEditValues(prev => ({ ...prev, level_id: val }))}
                          style={{ width: '100%' }}
                          placeholder="选择级别"
                          options={levels.map(l => ({ value: l.id, label: l.caption }))}
                        />
                      </div>
                      <div className="flex gap-2">
                        <Button type="primary" loading={savingCard} onClick={() => {
                          handleSaveCard('basic', {
                            category_id: editValues.category_id !== undefined ? editValues.category_id : instrument.category_id,
                            level_id: editValues.level_id !== undefined ? editValues.level_id : instrument.level_id,
                          })
                        }}>保存</Button>
                        <Button onClick={() => setEditingCard(null)}>取消</Button>
                      </div>
                    </div>
                  ) : (
                  <Descriptions column={2}>
                    <Descriptions.Item label="识别码">{instrument.sn}</Descriptions.Item>
                    <Descriptions.Item label="分类">{instrument.category_name}</Descriptions.Item>
                    <Descriptions.Item label="级别">
                      <Tag color="blue">{instrument.level_name || levelMap[instrument.level] || '未设置'}</Tag>
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
                    {instrument.properties && Object.entries(instrument.properties).map(([key, vals]) => (
                      <Descriptions.Item label={key} key={key}>
                        {Array.isArray(vals) ? vals.join(', ') : vals}
                      </Descriptions.Item>
                    ))}
                  </Descriptions>
                  )}
                </Card>
                
                <Card title="描述" className="mt-4"
                  extra={editingCard === 'desc' ? null : <EditOutlined className="cursor-pointer" onClick={() => setEditingCard('desc')} />}
                >
                  {editingCard === 'desc' ? (
                    <div className="space-y-3">
                      <Input.TextArea defaultValue={instrument.description} id="edit-desc" rows={4} />
                      <div className="flex gap-2">
                        <Button type="primary" loading={savingCard} onClick={() => {
                          handleSaveCard('desc', {
                            description: document.getElementById('edit-desc')?.value || ''
                          })
                        }}>保存</Button>
                        <Button onClick={() => setEditingCard(null)}>取消</Button>
                      </div>
                    </div>
                  ) : (
                  <p className="text-gray-700">{instrument.description || '暂无描述'}</p>
                  )}
                </Card>
              </Col>
              
              <Col span={8}>
                <Card title="价格配置"
                  extra={editingCard === 'pricing' ? null : <EditOutlined className="cursor-pointer" onClick={() => setEditingCard('pricing')} />}
                >
                  {editingCard === 'pricing' ? (
                    <div className="space-y-3">
                      <div>
                        <label className="text-sm text-gray-500">基础日均价 (¥)</label>
                        <InputNumber
                          defaultValue={pricingV2?.base_daily_rate ?? instrument.base_daily_rate ?? 0}
                          id="edit-daily-rate" min={0} className="w-full"
                        />
                      </div>
                      <div>
                        <label className="text-sm text-gray-500">押金 (¥)</label>
                        <InputNumber
                          defaultValue={instrument.deposit ?? pricingV2?.deposit ?? parsePricing(instrument.pricing)?.deposit ?? 0}
                          id="edit-deposit" min={0} className="w-full"
                        />
                      </div>
                      <div>
                        <label className="text-sm text-gray-500">物流费 (¥)</label>
                        <InputNumber
                          defaultValue={pricingV2?.shipping_fee ?? parsePricing(instrument.pricing)?.shipping_fee ?? 0}
                          id="edit-shipping" min={0} className="w-full"
                        />
                      </div>
                      <div>
                        <label className="text-sm text-gray-500">逾期日费 (¥/天)</label>
                        <InputNumber
                          defaultValue={overdueDailyFee || 0}
                          id="edit-overdue" min={0} className="w-full"
                        />
                      </div>
                      <div className="flex gap-2">
                        <Button type="primary" loading={savingCard} onClick={() => {
                          handleSaveCard('pricing', {
                            deposit: parseFloat(document.getElementById('edit-deposit')?.value || '0'),
                            shipping_fee: parseFloat(document.getElementById('edit-shipping')?.value || '0'),
                            base_daily_rate: parseFloat(document.getElementById('edit-daily-rate')?.value || '0'),
                            overdue_daily_fee: parseFloat(document.getElementById('edit-overdue')?.value || '0'),
                          })
                        }}>保存</Button>
                        <Button onClick={() => setEditingCard(null)}>取消</Button>
                      </div>
                    </div>
                  ) : pricingV2Loading ? (
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
                        <Descriptions.Item label="押金">
                          ¥{(instrument.deposit ?? pricingV2?.deposit ?? parsePricing(instrument.pricing)?.deposit ?? 0).toFixed(2)}
                        </Descriptions.Item>
                        <Descriptions.Item label="物流费">
                          ¥{(pricingV2?.shipping_fee ?? parsePricing(instrument.pricing)?.shipping_fee ?? 0).toFixed(2)}
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
              </Col>
            </Row>
          )
        },
        {
          label: '多媒体',
          key: 'media',
          children: (
            <Row gutter={16}>
              <Col span={12}>
                <Card title="展示图像"
                  extra={
                    <label className="text-xs text-brand-primary cursor-pointer hover:underline">
                      + 新增
                      <input
                        type="file"
                        accept="image/jpeg,image/png,image/webp"
                        className="hidden"
                        onChange={async (e) => {
                          const file = e.target.files?.[0]
                          if (!file) return
                          try {
                            const res = await instrumentsApi.displayImageUpload(id, file)
                            if (res.code === 20000) {
                              message.success('展示图上传成功')
                              fetchInstrument()
                            } else {
                              message.error(res.message || '上传失败')
                            }
                          } catch (err) {
                            message.error('上传失败: ' + (err.message || ''))
                          }
                          e.target.value = ''
                        }}
                      />
                    </label>
                  }
                >
                  {(() => {
                    const media = instrument.media
                    const displayImages = media?.display?.filter(m => m.file_type === 'image') || []
                    const images = displayImages.length > 0
                      ? displayImages
                      : (instrument.images?.length ? instrument.images.map(u => ({ url: u, file_type: 'image' })) : [])
                    
                    return images.length > 0 ? (
                      <Image.PreviewGroup>
                        {images.map((item, index) => (
                          <div key={index} className="inline-block relative mr-2 mb-2 group">
                            <Image
                              src={item.url || item}
                              alt={`${instrument.sn}-${index}`}
                              width={120}
                              height={120}
                              className="object-cover rounded"
                            />
                            {item.batch_id ? (
                              <Popconfirm title="删除此图片？" onConfirm={() => handleDeleteBatch(item.batch_id)}>
                                <Button size="small" danger className="absolute top-0 right-0 opacity-0 group-hover:opacity-100" style={{ borderRadius: '0 4px 0 4px' }}>×</Button>
                              </Popconfirm>
                            ) : (
                              <Popconfirm title="删除此图片？" onConfirm={async () => {
                                try {
                                  const updatedImages = (instrument.images || []).filter(u => u !== item.url)
                                  await api.put(`/instruments/${id}`, { images: updatedImages })
                                  message.success('图片已删除')
                                  fetchInstrument()
                                } catch (e) {
                                  message.error('删除失败: ' + (e.message || ''))
                                }
                              }}>
                                <Button size="small" danger className="absolute top-0 right-0 opacity-0 group-hover:opacity-100" style={{ borderRadius: '0 4px 0 4px' }}>×</Button>
                              </Popconfirm>
                            )}
                          </div>
                        ))}
                      </Image.PreviewGroup>
                    ) : (
                      <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无展示图片" />
                    )
                  })()}
                </Card>

                <Card title="视频" className="mt-4"
                  extra={
                    <label className="text-xs text-brand-primary cursor-pointer hover:underline">
                      {instrument.video || mediaDetail?.video ? '替换' : '上传'}
                      <input
                        type="file"
                        accept="video/*"
                        className="hidden"
                        onChange={async (e) => {
                          const file = e.target.files?.[0]
                          if (!file) return
                          try {
                            const formData = new FormData()
                            formData.append('file', file)
                            const res = await request('/upload', { method: 'POST', body: formData })
                            const url = res?.data?.url || res?.url || ''
                            if (url) {
                              await api.put(`/instruments/${id}`, { video: url })
                              message.success('视频更新成功')
                              fetchInstrument()
                            } else {
                              message.error(res?.message || '上传失败')
                            }
                          } catch (err) {
                            message.error('上传失败: ' + (err.message || ''))
                          }
                          e.target.value = ''
                        }}
                      />
                    </label>
                  }
                >
                  {(instrument.video || mediaDetail?.video) ? (
                    <div className="flex items-start gap-4">
                      <video
                        src={instrument.video || mediaDetail?.video?.url}
                        controls
                        width="240"
                        className="rounded"
                      />
                      <Popconfirm title="确定删除此视频？" onConfirm={async () => {
                        const batchId = instrument.media?.video?.batch_id || mediaDetail?.video?.batch_id
                        if (batchId) {
                          handleDeleteVideo(batchId)
                        } else {
                          try {
                            await api.put(`/instruments/${id}`, { video: '' })
                            message.success('视频已删除')
                            fetchInstrument()
                          } catch (e) {
                            message.error('删除失败: ' + (e.message || ''))
                          }
                        }
                      }}>
                        <Button danger size="small">删除</Button>
                      </Popconfirm>
                    </div>
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无视频" />
                  )}
                </Card>
              </Col>

              <Col span={12}>
                <Card title="海报"
                  extra={
                    <label className="text-xs text-brand-primary cursor-pointer hover:underline">
                      {instrument.poster ? '替换' : '上传'}
                      <input
                        type="file"
                        accept="image/*"
                        className="hidden"
                        onChange={async (e) => {
                          const file = e.target.files?.[0]
                          if (!file) return
                          try {
                            const formData = new FormData()
                            formData.append('file', file)
                            const res = await request('/upload', { method: 'POST', body: formData })
                            const url = res?.data?.url || res?.url || ''
                            if (url) {
                              await api.put(`/instruments/${id}`, { poster: url })
                              message.success('海报更新成功')
                              fetchInstrument()
                            } else {
                              message.error(res?.message || '上传失败')
                            }
                          } catch (err) {
                            message.error('上传失败: ' + (err.message || ''))
                          }
                          e.target.value = ''
                        }}
                      />
                    </label>
                  }
                >
                  {instrument.poster ? (
                    <img src={instrument.poster} alt="海报" style={{ maxWidth: '100%', maxHeight: 400, objectFit: 'contain', borderRadius: 8 }} />
                  ) : (
                    <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="暂无海报" />
                  )}
                </Card>
              </Col>
            </Row>
          )
        },
        {
          label: '当前租赁',
          key: 'lease',
          children: (
            <>
              {leaseLoading ? (
                <Spin />
              ) : !activeStatuses.includes(instrument.stock_status) ? (
                <Empty description="当前无租赁信息" />
              ) : !leaseData ? (
                <Empty description="未找到关联订单" />
              ) : (
                <div className="space-y-4">
                  <Descriptions column={1} bordered size="small">
                    <Descriptions.Item label="租赁人">
                      <Space><UserOutlined />{leaseData.user?.name || '-'}</Space>
                    </Descriptions.Item>
                    <Descriptions.Item label="电话">{leaseData.user?.phone || '-'}</Descriptions.Item>
                    <Descriptions.Item label="租期">
                      <Space><CalendarOutlined />{leaseData.order?.start_date || '-'} 至 {leaseData.order?.end_date || '-'}</Space>
                    </Descriptions.Item>
                  </Descriptions>
                  <div className="text-sm text-gray-500 space-y-1">
                    <p>月租金: ¥{leaseData.order?.monthly_rent || 0}</p>
                    <p>押金: ¥{leaseData.order?.deposit || 0}</p>
                  </div>
                </div>
              )}
            </>
          )
        },
        {
          label: '日志',
          key: 'log',
          children: (
            <ActivityLogTab instrumentId={id} />
          )
        },
        {
          label: '促销覆盖',
          key: 'promo',
          children: (
            <PromoOverrideTab instrumentId={id} />
          )
        }
      ]} />
    </div>
  )
}

function ActivityLogTab({ instrumentId }) {
  const [sessions, setSessions] = useState([])
  const [auditLogs, setAuditLogs] = useState([])
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!instrumentId) return
    setLoading(true)
    Promise.all([
      instrumentsApi.getActivityLog(instrumentId).then(res => {
        if (res.code === 20000) setSessions(res.data?.sessions || [])
      }).catch(() => {}),
      api.get('/admin/audit-logs', { params: { resource_id: instrumentId, pageSize: 50 } }).then(res => {
        if (res.code === 20000) setAuditLogs(res.data?.list || [])
      }).catch(() => {}),
    ]).finally(() => setLoading(false))
  }, [instrumentId])

  if (loading) return <Spin />

  const actionMap = { CREATE: '创建', UPDATE: '编辑', DELETE: '删除' }

  return (
    <div className="space-y-4">
      {auditLogs.length > 0 && (
        <Card title="操作记录" size="small">
          <div className="max-h-60 overflow-y-auto space-y-1">
            {auditLogs.map((log, i) => (
              <div key={log.id || i} className="flex gap-3 items-center py-1.5 px-2 bg-gray-50 rounded text-xs">
                <span className="text-gray-400 w-32 flex-shrink-0">{new Date(log.created_at).toLocaleString()}</span>
                <Tag color={log.action === 'CREATE' ? 'green' : log.action === 'DELETE' ? 'red' : 'blue'}>
                  {actionMap[log.action] || log.action}
                </Tag>
                <span className="text-gray-600">{log.ip_address}</span>
              </div>
            ))}
          </div>
        </Card>
      )}

      {sessions.length > 0 && sessions.map(session => (
        <Card key={session.order_id} size="small"
          title={
            <Space>
              <span className="font-mono text-xs">#{session.order_id.slice(0, 8)}</span>
              <Tag>{session.status}</Tag>
              <span className="text-xs text-gray-400">{session.start_date} ~ {session.end_date || '进行中'}</span>
            </Space>
          }
        >
          {session.events.length === 0 ? (
            <Empty description="暂无操作记录" />
          ) : (
            <div className="space-y-2">
              {session.events.map((event, idx) => (
                <div key={idx} className="flex gap-3 p-2 bg-gray-50 rounded">
                  <div className="w-20 flex-shrink-0 text-xs text-gray-400">{new Date(event.time).toLocaleString()}</div>
                  <div className="flex-1">
                    <p className="text-sm font-medium">{event.event}</p>
                    {event.operator && <p className="text-xs text-gray-400">操作人: {event.operator}</p>}
                    {event.media?.length > 0 && (
                      <div className="flex gap-1 mt-1 flex-wrap">
                        {event.media.filter(m => m.url).map((m, mi) => (
                          <div key={mi} className="relative w-[60px] h-[60px]">
                            <Image src={m.url} width={60} height={60} className="object-cover rounded" preview={{ mask: null }} />
                            {m.file_type === 'video_thumb' && (
                              <div className="absolute inset-0 flex items-center justify-center bg-black/30 rounded">
                                <div className="w-0 h-0 border-t-[8px] border-b-[8px] border-l-[14px] border-t-transparent border-b-transparent border-l-white ml-1" />
                              </div>
                            )}
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )}
        </Card>
      ))}
    </div>
  )
}

function PromoOverrideTab({ instrumentId }) {
  const [discountEnabled, setDiscountEnabled] = useState(true)
  const [rebateEnabled, setRebateEnabled] = useState(true)
  const [loading, setLoading] = useState(true)

  useEffect(() => {
    if (!instrumentId) return
    setLoading(true)
    api.get(`/instruments/${instrumentId}/promo-overrides`).then(res => {
      if (res.code === 20000 && res.data) {
        res.data.forEach(o => {
          if (o.override_type === 'discount') setDiscountEnabled(o.enabled)
          if (o.override_type === 'rebate') setRebateEnabled(o.enabled)
        })
      }
    }).catch(() => {}).finally(() => setLoading(false))
  }, [instrumentId])

  const handleToggle = async (type, value) => {
    const res = await api.put(`/instruments/${instrumentId}/promo-overrides`, { override_type: type, enabled: value })
    if (res.code === 20000) {
      if (type === 'discount') setDiscountEnabled(value)
      else setRebateEnabled(value)
      message.success('已更新')
    } else {
      message.error(res.message)
    }
  }

  if (loading) return <Spin />

  return (
    <Card title="乐器促销覆盖" size="small">
      <div className="space-y-4">
        <div className="flex items-center justify-between p-3 bg-gray-50 rounded">
          <div>
            <div className="font-medium">会员折扣政策</div>
            <div className="text-xs text-gray-400">开启后该乐器适用会员折扣</div>
          </div>
          <Switch checked={discountEnabled} onChange={v => handleToggle('discount', v)} />
        </div>
        <div className="flex items-center justify-between p-3 bg-gray-50 rounded">
          <div>
            <div className="font-medium">返点政策</div>
            <div className="text-xs text-gray-400">开启后该乐器适用返点政策</div>
          </div>
          <Switch checked={rebateEnabled} onChange={v => handleToggle('rebate', v)} />
        </div>
      </div>
    </Card>
  )
}
