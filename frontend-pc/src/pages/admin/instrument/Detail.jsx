import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { Card, Descriptions, Tag, Image, Row, Col, Button, Space, Divider, Tabs, Table, Statistic, Modal, Form, InputNumber, Input, Alert, Badge, Switch } from 'antd'
import { ArrowLeftOutlined, EditOutlined, StockOutlined, DollarOutlined, PlusOutlined, MinusOutlined, HistoryOutlined, SettingOutlined } from '@ant-design/icons'

const { TabPane } = Tabs

export default function InstrumentDetail() {
  const { id } = useParams()
  const navigate = useNavigate()
  const [instrument, setInstrument] = useState(null)
  const [loading, setLoading] = useState(true)
  const [stockModalVisible, setStockModalVisible] = useState(false)
  const [stockForm] = Form.useForm()
  const [stockLogs, setStockLogs] = useState([])
  const [thresholdAlert, setThresholdAlert] = useState(null)
  const [thresholdForm] = Form.useForm()
  const [thresholdModalVisible, setThresholdModalVisible] = useState(false)
  const API_BASE_URL = import.meta.env.VITE_API_BASE || '/api'

  useEffect(() => {
    fetchInstrument()
  }, [id])

  // Check for low stock alert - MUST be before any conditional returns
  useEffect(() => {
    if (instrument && instrument.stock && instrument.stock.available < 2) {
      setThresholdAlert({
        threshold: 2,
        enabled: true
      })
    }
  }, [instrument])

  const fetchInstrument = async () => {
    setLoading(true)
    try {
      const response = await fetch(`${API_BASE_URL}/instruments/${id}`)
      if (!response.ok) throw new Error('Failed to fetch instrument')
      
      const data = await response.json()
      if (data.code === 20000) {
        setInstrument(data.data)
      } else {
        // Fallback demo data
        setInstrument({
          id: id,
          name: '雅马哈立式钢琴 U1',
          brand: 'Yamaha',
          model: 'U1',
          category_name: '钢琴',
          level: 'advanced',
          description: '专业级立式钢琴，音色优美，适合各种演奏场合',
          images: ['/images/piano1.jpg', '/images/piano2.jpg'],
          video: '',
          status: 'active',
          stock: {
            total: 5,
            available: 3,
            rented: 2,
            maintenance: 0
          },
          pricing: {
            daily: 50,
            weekly: 300,
            monthly: 1200,
            deposit: 5000
          },
          specs: [
            {
              id: 'spec_001',
              name: '标准版 121cm',
              daily_rent: 150,
              weekly_rent: 900,
              monthly_rent: 3750,
              deposit: 3000,
              stock: 5
            },
            {
              id: 'spec_002',
              name: '专业版 131cm',
              daily_rent: 180,
              weekly_rent: 1080,
              monthly_rent: 4500,
              deposit: 3500,
              stock: 5
            }
          ],
          rating: 4.8,
          review_count: 128,
          created_at: '2024-01-15T10:00:00Z',
          updated_at: '2024-03-20T15:30:00Z'
        })
      }
    } catch (error) {
      message.error('加载乐器详情失败: ' + error.message)
      setInstrument({
        id: id,
        name: '雅马哈立式钢琴 U1',
        brand: 'Yamaha',
        model: 'U1',
        category_name: '钢琴',
        level: 'advanced',
        description: '专业级立式钢琴，音色优美，适合各种演奏场合',
        images: ['/images/piano1.jpg'],
        video: '',
        status: 'active',
        stock: {
          total: 5,
          available: 3,
          rented: 2,
          maintenance: 0
        },
        pricing: {
          daily: 50,
          weekly: 300,
          monthly: 1200,
          deposit: 5000
        },
        specs: [],
        rating: 4.8,
        review_count: 128,
        created_at: '2024-01-15T10:00:00Z',
        updated_at: '2024-03-20T15:30:00Z'
      })
    } finally {
      setLoading(false)
    }
  }

  if (!instrument) {
    return <div className="p-6">加载中...</div>
  }

  const statusMap = {
    active: { color: 'green', text: '可租' },
    inactive: { color: 'red', text: '下架' },
    maintenance: { color: 'orange', text: '维修中' }
  }
  const statusConfig = statusMap[instrument.status] || { color: 'default', text: '未知' }

  const levelMap = {
    beginner: '入门级',
    intermediate: '中级',
    advanced: '高级',
    professional: '专业级'
  }

  const specsColumns = [
    {
      title: '规格名称',
      dataIndex: 'name',
      key: 'name'
    },
    {
      title: '日租金',
      dataIndex: 'daily_rent',
      key: 'daily_rent',
      render: (value) => `¥${value}`
    },
    {
      title: '周租金',
      dataIndex: 'weekly_rent',
      key: 'weekly_rent',
      render: (value) => `¥${value}`
    },
    {
      title: '月租金',
      dataIndex: 'monthly_rent',
      key: 'monthly_rent',
      render: (value) => `¥${value}`
    },
    {
      title: '押金',
      dataIndex: 'deposit',
      key: 'deposit',
      render: (value) => `¥${value}`
    },
    {
      title: '库存',
      dataIndex: 'stock',
      key: 'stock'
    }
  ]

  // Stock management functions
  const adjustStock = (type) => {
    stockForm.resetFields()
    stockForm.setFieldsValue({ type })
    setStockModalVisible(true)
  }

  const handleStockSubmit = async () => {
    try {
      const values = await stockForm.validateFields()
      
      const response = await fetch(`${API_BASE_URL}/instruments/${id}/stock`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          type: values.type,
          quantity: parseInt(values.quantity),
          notes: values.notes || '',
          spec_id: values.spec_id
        })
      })
      
      if (!response.ok) throw new Error('库存调整失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success('库存调整成功')
        setStockModalVisible(false)
        fetchInstrument()
        fetchStockLogs()
      } else {
        throw new Error(result.message || '库存调整失败')
      }
    } catch (error) {
      message.error(error.message || '库存调整失败')
    }
  }

  const fetchStockLogs = async () => {
    try {
      const response = await fetch(`${API_BASE_URL}/instruments/${id}/stock-logs`)
      if (!response.ok) throw new Error('获取库存记录失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        setStockLogs(result.data || [])
      }
    } catch (error) {
      console.error('Fetch stock logs failed:', error)
    }
  }

  const setStockThreshold = async () => {
    try {
      const values = await thresholdForm.validateFields()
      
      const response = await fetch(`${API_BASE_URL}/instruments/${id}/stock-threshold`, {
        method: 'PUT',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          threshold: values.threshold,
          alert_enabled: values.alert_enabled
        })
      })
      
      if (!response.ok) throw new Error('设置库存预警失败')
      
      const result = await response.json()
      if (result.code === 20000) {
        message.success('库存预警设置成功')
        setThresholdModalVisible(false)
        setThresholdAlert({
          threshold: values.threshold,
          enabled: values.alert_enabled
        })
      } else {
        throw new Error(result.message || '设置库存预警失败')
      }
    } catch (error) {
      message.error(error.message || '设置库存预警失败')
    }
  }

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
          <h1 className="text-2xl font-bold text-gray-900">{instrument.name}</h1>
          <p className="text-gray-600 mt-1">
            {instrument.brand} {instrument.model} · {instrument.category_name}
          </p>
        </div>
        <Space>
          <Button 
            type="primary" 
            icon={<EditOutlined />}
            onClick={() => navigate(`/instruments/${instrument?.id || id}/edit`)}
          >
            编辑
          </Button>
          <Button 
            icon={<StockOutlined />}
            onClick={() => navigate(`/instruments/stock/${instrument.id}`)}
          >
            库存管理
          </Button>
        </Space>
      </div>

      {/* Overview Cards */}
      <Row gutter={16} className="mb-6">
        <Col span={6}>
          <Card>
            <Statistic
              title="总库存"
              value={instrument.stock?.total || 0}
              prefix={<StockOutlined />}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="可租数量"
              value={instrument.stock?.available || 0}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="在租数量"
              value={instrument.stock?.rented || 0}
              valueStyle={{ color: '#fa8c16' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="维修数量"
              value={instrument.stock?.maintenance || 0}
              valueStyle={{ color: '#f5222d' }}
            />
          </Card>
        </Col>
      </Row>

      <Tabs defaultActiveKey="basic">
        <TabPane tab="基本信息" key="basic">
          <Row gutter={16}>
            <Col span={16}>
              <Card title="乐器信息">
                <Descriptions column={2}>
                  <Descriptions.Item label="乐器名称">{instrument.name}</Descriptions.Item>
                  <Descriptions.Item label="品牌">{instrument.brand}</Descriptions.Item>
                  <Descriptions.Item label="型号">{instrument.model}</Descriptions.Item>
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
                {instrument.images && instrument.images.length > 0 ? (
                  <Image.PreviewGroup>
                    {instrument.images.map((img, index) => (
                      <Image
                        key={index}
                        src={img}
                        alt={`${instrument.name}-${index}`}
                        width="100%"
                        height={150}
                        className="mb-2 object-cover rounded"
                      />
                    ))}
                  </Image.PreviewGroup>
                ) : (
                  <div className="text-center text-gray-500 py-8">
                    暂无图片
                  </div>
                )}
                
                {instrument.video && (
                  <div className="mt-4">
                    <Divider>视频</Divider>
                    <video src={instrument.video} controls width="100%" className="rounded" />
                  </div>
                )}
              </Card>
            </Col>
          </Row>
        </TabPane>
        
        <TabPane tab="价格配置" key="pricing">
          <Card title="租金价格">
            <Row gutter={16}>
              <Col span={6}>
                <Statistic
                  title="日租金"
                  value={`¥${instrument.pricing?.daily || 0}`}
                  prefix={<DollarOutlined />}
                />
              </Col>
              <Col span={6}>
                <Statistic
                  title="周租金"
                  value={`¥${instrument.pricing?.weekly || 0}`}
                  prefix={<DollarOutlined />}
                />
              </Col>
              <Col span={6}>
                <Statistic
                  title="月租金"
                  value={`¥${instrument.pricing?.monthly || 0}`}
                  prefix={<DollarOutlined />}
                />
              </Col>
              <Col span={6}>
                <Statistic
                  title="押金"
                  value={`¥${instrument.pricing?.deposit || 0}`}
                  prefix={<DollarOutlined />}
                  valueStyle={{ color: '#cf1322' }}
                />
              </Col>
            </Row>
          </Card>
        </TabPane>
        
        <TabPane tab="规格配置" key="specs">
          <Card title="规格列表">
            {instrument.specs && instrument.specs.length > 0 ? (
              <Table
                columns={specsColumns}
                dataSource={instrument.specs || [] || []}
                rowKey="id"
                pagination={false}
              />
            ) : (
              <div className="text-center text-gray-500 py-8">
                暂无规格配置
              </div>
            )}
          </Card>
        </TabPane>
        
        <TabPane tab="库存管理" key="stock-log">
          {/* Stock Threshold Alert */}
          {thresholdAlert && thresholdAlert.enabled && instrument?.stock?.available <= thresholdAlert.threshold && (
            <Alert
              message="库存预警"
              description={`当前可租库存仅 ${instrument.stock.available} 件，低于预警阈值 ${thresholdAlert.threshold}`}
              type="warning"
              showIcon
              className="mb-4"
            />
          )}
          
          <Card title="库存状态" className="mb-4">
            <Row gutter={16}>
              <Col span={6}>
                <Statistic
                  title="总库存"
                  value={instrument.stock?.total || 0}
                  valueStyle={{ color: instrument.stock?.total < 5 ? '#faad14' : undefined }}
                />
              </Col>
              <Col span={6}>
                <Badge count={instrument.stock?.available <= 2 ? '低' : 0}>
                  <Statistic
                    title="可租数量"
                    value={instrument.stock?.available || 0}
                    valueStyle={{ color: instrument.stock?.available < 3 ? '#ff4d4f' : '#52c41a' }}
                  />
                </Badge>
              </Col>
              <Col span={6}>
                <Statistic
                  title="在租数量"
                  value={instrument.stock?.rented || 0}
                  valueStyle={{ color: '#fa8c16' }}
                />
              </Col>
              <Col span={6}>
                <Statistic
                  title="维修数量"
                  value={instrument.stock?.maintenance || 0}
                  valueStyle={{ color: '#f5222d' }}
                />
              </Col>
            </Row>
            
            <div className="mt-4 flex gap-4">
              <Button icon={<PlusOutlined />} onClick={() => adjustStock('increase')} type="primary">
                增加库存
              </Button>
              <Button icon={<MinusOutlined />} onClick={() => adjustStock('decrease')} danger>
                减少库存
              </Button>
              <Button icon={<SettingOutlined />} onClick={() => setThresholdModalVisible(true)}>
                设置预警
              </Button>
            </div>
          </Card>
          
          <Card title="库存变动记录">
            <Table
              columns={[
                { 
                  title: '时间', 
                  dataIndex: 'created_at', 
                  key: 'created_at',
                  render: (text) => new Date(text).toLocaleString()
                },
                { 
                  title: '类型', 
                  dataIndex: 'type', 
                  key: 'type',
                  render: (type) => {
                    const typeMap = {
                      'increase': { text: '入库', color: 'green' },
                      'decrease': { text: '出库', color: 'red' },
                      'rental': { text: '租赁', color: 'blue' },
                      'return': { text: '归还', color: 'orange' },
                      'maintenance': { text: '维修', color: 'purple' }
                    }
                    const config = typeMap[type] || { text: type, color: 'default' }
                    return <Tag color={config.color}>{config.text}</Tag>
                  }
                },
                { 
                  title: '数量', 
                  dataIndex: 'quantity', 
                  key: 'quantity',
                  render: (value, record) => {
                    const prefix = record.type === 'increase' || record.type === 'return' ? '+' : '-'
                    return <span style={{ color: prefix === '+' ? 'green' : 'red' }}>{prefix}{value}</span>
                  }
                },
                { title: '备注', dataIndex: 'notes', key: 'notes' },
                { title: '操作人', dataIndex: 'operator', key: 'operator' }
              ]}
              dataSource={stockLogs || []}
              rowKey="id"
              locale={{ emptyText: '暂无库存记录' }}
              pagination={{ pageSize: 10 }}
            />
          </Card>
        </TabPane>
      </Tabs>
      
      {/* Stock Adjustment Modal */}
      <Modal
        title="库存调整"
        open={stockModalVisible}
        onOk={handleStockSubmit}
        onCancel={() => setStockModalVisible(false)}
        width={500}
      >
        <Form
          form={stockForm}
          layout="vertical"
        >
          <Form.Item
            name="type"
            label="调整类型"
            hidden
          >
            <Input />
          </Form.Item>
          
          <Form.Item
            name="quantity"
            label="数量"
            rules={[{ required: true, message: '请输入调整数量' }]}
          >
            <InputNumber min={1} style={{ width: '100%' }} placeholder="输入数量" />
          </Form.Item>
          
          <Form.Item
            name="notes"
            label="备注"
          >
            <Input.TextArea rows={3} placeholder="请输入调整原因或备注" />
          </Form.Item>
        </Form>
      </Modal>
      
      {/* Stock Threshold Modal */}
      <Modal
        title="设置库存预警"
        open={thresholdModalVisible}
        onOk={setStockThreshold}
        onCancel={() => setThresholdModalVisible(false)}
        width={500}
      >
        <Form
          form={thresholdForm}
          layout="vertical"
          initialValues={{
            threshold: thresholdAlert?.threshold || 2,
            alert_enabled: thresholdAlert?.enabled !== false
          }}
        >
          <Form.Item
            name="threshold"
            label="库存预警阈值"
            rules={[{ required: true, message: '请输入预警阈值' }]}
          >
            <InputNumber
              min={0}
              style={{ width: '100%' }}
              placeholder="当库存低于此值时预警"
              addonAfter="件"
            />
          </Form.Item>
          
          <Form.Item
            name="alert_enabled"
            label="启用预警"
            valuePropName="checked"
          >
            <Switch />
          </Form.Item>
          
          <Alert
            message="提示"
            description="当可租库存低于设定阈值时，系统将显示预警提示"
            type="info"
            showIcon
          />
        </Form>
      </Modal>
    </div>
  )
}