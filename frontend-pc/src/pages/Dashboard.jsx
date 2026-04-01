import { useState, useEffect } from 'react'
import { Table, Tag, Space, Form, Select, Statistic, Row, Col, Drawer, Timeline, Button, Badge, Spin } from 'antd'
import { EyeOutlined, EditOutlined, DollarOutlined, ShoppingOutlined, ToolOutlined, BarChartOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { inventoryApi, sitesApi, ordersApi, maintenanceApi, leaseApi } from '../services/api'
import { LineChart, Line, PieChart, Pie, Cell, ResponsiveContainer, XAxis, YAxis, Legend, Tooltip } from 'recharts'

const statusColors = {
  "在租": "green",
  "待租": "blue",
  "维修中": "orange",
  "available": "blue",
  "rented": "green",
  "maintenance": "orange",
}

const levelColors = {
  "入门级": "default",
  "专业级": "blue",
  "大师级": "gold",
}

export default function Dashboard() {
  const [form] = Form.useForm()
  const [selectedSite, setSelectedSite] = useState(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [selectedAsset, setSelectedAsset] = useState(null)
  const [statusFilter, setStatusFilter] = useState(null)
  const [sites, setSites] = useState([])
  const [loading, setLoading] = useState(true)
  const [assets, setAssets] = useState([])
  const [totalAssets, setTotalAssets] = useState(0)
  const [activeRentals, setActiveRentals] = useState(0)
  const [todaysNewOrders, setTodaysNewOrders] = useState(0)
  const [maintenanceDue, setMaintenanceDue] = useState(0)
  const navigate = useNavigate()

  useEffect(() => {
    loadData()
  }, [])

  const loadData = async () => {
    setLoading(true)
    try {
      const today = new Date().toISOString().split('T')[0]
      const [inventoryData, sitesData, leasesData, ordersResponse, maintenanceData] = await Promise.all([
        inventoryApi.list(),
        sitesApi.list(),
        leaseApi.list(),
        api.get(`/orders?start_date=${today}&end_date=${today}`),
        maintenanceApi.listMerchant(),
      ])
      setAssets(inventoryData || [])
      setSites((sitesData || []).map(s => ({
        value: s.id,
        label: s.name,
      })))
      
      // Calculate new KPI values
      if (leasesData) {
        const activeLeases = leasesData.filter(l => l.status === 'active')
        setActiveRentals(activeLeases.length)
        
        const totalValue = activeLeases.reduce((sum, lease) => {
          return sum + (lease.monthly_rent || 0) + (lease.deposit_amount || 0)
        }, 0)
        setTotalAssets(totalValue)
      }
      
      setTodaysNewOrders(ordersResponse?.data?.length || 0)
      
      if (maintenanceData) {
        const pendingMaintenance = maintenanceData.filter(m => 
          m.status === 'PENDING' || m.status === 'PROCESSING'
        )
        setMaintenanceDue(pendingMaintenance.length)
      }
    } catch (error) {
      console.error('Failed to load data:', error)
    } finally {
      setLoading(false)
    }
  }

  const handleCardClick = (filterType) => {
    if (filterType === '在租') {
      navigate('/site/stock?status=rented')
    } else if (filterType === '维修中') {
      navigate('/site/stock?status=maintenance')
    } else if (filterType === '逾期') {
      navigate('/site/stock?overdue=true')
    }
  }

  const today = new Date().toISOString().split('T')[0]

  const filteredAssets = selectedSite
    ? assets.filter(a => a.siteId === selectedSite)
    : assets

  const displayedAssets = statusFilter
    ? filteredAssets.filter(a => a.status === statusFilter)
    : filteredAssets

  const totalValue = filteredAssets
    .filter(a => a.status === "在租" || a.status === "rented")
    .reduce((sum, a) => sum + (a.value || 0), 0)

  const expiringToday = filteredAssets.filter(a => 
    a.leaseEnd && a.leaseEnd <= today && (a.status === "在租" || a.status === "rented")
  ).length

  const overdueAssets = filteredAssets.filter(a => 
    a.leaseEnd && a.leaseEnd < today && (a.status === "在租" || a.status === "rented")
  ).length

  const handleRowClick = (record) => {
    setSelectedAsset(record)
    setDrawerOpen(true)
  }

  const columns = [
    {
      title: '资产ID',
      dataIndex: 'id',
      key: 'id',
      fixed: 'left',
      width: 120,
    },
    {
      title: '乐器名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const statusMap = {
          "在租": { color: 'green', text: '在线' },
          "rented": { color: 'green', text: '在租' },
          "待租": { color: 'blue', text: '在线' },
          "available": { color: 'blue', text: '待租' },
          "维修中": { color: 'orange', text: '维修中' },
          "maintenance": { color: 'orange', text: '维修中' }
        }
        const info = statusMap[status] || { color: 'default', text: status }
        return <Tag color={info.color}>{info.text}</Tag>
      }
    },
    {
      title: '类别',
      dataIndex: 'category',
      key: 'category',
    },
    {
      title: '级别',
      dataIndex: 'level',
      key: 'level',
      render: (level) => (
        <Tag color={levelColors[level]}>{level}</Tag>
      )
    },
    {
      title: '所属网点',
      dataIndex: 'site',
      key: 'site',
    },
    {
      title: '估值',
      dataIndex: 'value',
      key: 'value',
      align: 'right',
      render: (value) => `¥${(value || 0).toLocaleString()}`
    },
    {
      title: '操作',
      key: 'action',
      fixed: 'right',
      width: 120,
      render: (_, record) => (
        <Space>
          <Button type="link" icon={<EyeOutlined />} onClick={() => handleRowClick(record)}>详情</Button>
          <Button type="link" icon={<EditOutlined />}>编辑</Button>
        </Space>
      )
    }
  ]

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  return (
    <div>
      <Row gutter={16} className="mb-6">
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => handleCardClick('在租')}>
            <Statistic
              title="在租资产总额"
              value={totalValue}
              precision={0}
              suffix="元"
              valueStyle={{ color: '#3f8600' }}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card>
            <Statistic
              title="今日到期租约"
              value={expiringToday}
              valueStyle={{ color: '#cf1322' }}
              prefix={<Badge status="error" />}
            />
          </Card>
        </Col>
        <Col span={8}>
          <Card
            style={{ 
              cursor: 'pointer',
              borderColor: '#ff4d4f',
              background: '#fff1f0',
              transition: 'all 0.3s'
            }}
            onClick={() => handleCardClick('逾期')}
          >
            <Statistic
              title="逾期未归还"
              value={overdueAssets}
              valueStyle={{ 
                color: '#faad14',
                fontWeight: 'bold'
              }}
              prefix={<Badge status="warning" />}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} className="mb-6">
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => handleCardClick('total-assets')}>
            <Statistic
              title="Total Assets"
              value={totalAssets}
              precision={0}
              prefix={<DollarOutlined />}
              valueStyle={{ color: '#1890ff' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => handleCardClick('active-rentals')}>
            <Statistic
              title="Active Rentals"
              value={activeRentals}
              prefix={<ShoppingOutlined />}
              valueStyle={{ color: '#52c41a' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => handleCardClick('new-orders')}>
            <Statistic
              title="Today's New Orders"
              value={todaysNewOrders}
              prefix={<BarChartOutlined />}
              valueStyle={{ color: '#722ed1' }}
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => handleCardClick('maintenance')}>
            <Statistic
              title="Maintenance Due"
              value={maintenanceDue}
              prefix={<ToolOutlined />}
              valueStyle={{ color: '#faad14' }}
            />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} className="mb-6">
        <Col span={12}>
          <Card title="Revenue Trend" style={{ height: '300px' }}>
            <ResponsiveContainer width="100%" height="100%">
              <LineChart data={[
                { month: 'Jan', revenue: 4000 },
                { month: 'Feb', revenue: 3000 },
                { month: 'Mar', revenue: 5000 },
                { month: 'Apr', revenue: 4500 },
                { month: 'May', revenue: 6000 },
                { month: 'Jun', revenue: 5500 },
              ]}>
                <XAxis dataKey="month" />
                <YAxis />
                <Tooltip />
                <Line type="monotone" dataKey="revenue" stroke="#1890ff" strokeWidth={2} />
              </LineChart>
            </ResponsiveContainer>
          </Card>
        </Col>
        <Col span={12}>
          <Card title="Asset Status Distribution" style={{ height: '300px' }}>
            <ResponsiveContainer width="100%" height="100%">
              <PieChart>
                <Pie
                  data={[
                    { name: 'Available', value: assets.filter(a => a.status === 'available' || a.status === '待租').length },
                    { name: 'Rented', value: assets.filter(a => a.status === 'rented' || a.status === '在租').length },
                    { name: 'Repairing', value: assets.filter(a => a.status === 'maintenance' || a.status === '维修中').length },
                  ]}
                  cx="50%"
                  cy="50%"
                  innerRadius={40}
                  outerRadius={80}
                  paddingAngle={5}
                  dataKey="value"
                >
                  <Cell key="available" fill="#1890ff" />
                  <Cell key="rented" fill="#52c41a" />
                  <Cell key="repairing" fill="#faad14" />
                </Pie>
                <Tooltip />
                <Legend />
              </PieChart>
            </ResponsiveContainer>
          </Card>
        </Col>
      </Row>

      {statusFilter && (
        <div className="mb-4">
          <Space>
            <span>当前过滤: 状态 = {statusFilter}</span>
            <Button onClick={() => setStatusFilter(null)} size="small">清除过滤</Button>
          </Space>
        </div>
      )}

      <Form form={form} layout="inline" className="mb-4">
        <Form.Item label="网点筛选" name="site">
          <Select
            placeholder="请选择网点"
            allowClear
            style={{ width: 200 }}
            options={sites}
            onChange={(value) => setSelectedSite(value)}
          />
        </Form.Item>
      </Form>

      <Table 
        columns={columns} 
        dataSource={displayedAssets || []} 
        rowKey="id"
        pagination={{ total: displayedAssets.length, pageSize: 10, showSizeChanger: true, showTotal: (total) => `共 ${total} 条` }}
        scroll={{ x: 1000 }}
        onRow={(record) => ({
          onClick: () => handleRowClick(record),
          style: { cursor: 'pointer' }
        })}
      />

      <Drawer
        title="资产详情"
        placement="right"
        width={500}
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
      >
        {selectedAsset && (
          <div>
            <p><strong>资产ID:</strong> {selectedAsset.id}</p>
            <p><strong>名称:</strong> {selectedAsset.name}</p>
            <p><strong>级别:</strong> <Tag color={levelColors[selectedAsset.level]}>{selectedAsset.level}</Tag></p>
            <p><strong>状态:</strong> <Tag color={statusColors[selectedAsset.status]}>{selectedAsset.status}</Tag></p>
            <p><strong>估值:</strong> ¥{(selectedAsset.value || 0).toLocaleString()}</p>
            <p><strong>网点:</strong> {selectedAsset.site}</p>
            {selectedAsset.leaseEnd && (
              <p><strong>到期日:</strong> {selectedAsset.leaseEnd}</p>
            )}
          </div>
        )}
      </Drawer>
    </div>
  )
}

function Card({ children, className, ...props }) {
  return (
    <div className={`bg-white p-4 rounded shadow ${className}`} {...props}>
      {children}
    </div>
  )
}
