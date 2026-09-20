import { useState, useEffect } from 'react'
import { Card, Row, Col, Statistic, Table, Spin, Empty, Button, Space } from 'antd'
import {
  ShopOutlined, TeamOutlined, UserOutlined, IdcardOutlined, AlertOutlined,
  ShoppingOutlined, ToolOutlined, ReloadOutlined,
  AuditOutlined, InboxOutlined, ContainerOutlined,
} from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { LineChart, Line, PieChart, Pie, Cell, ResponsiveContainer, XAxis, YAxis, Legend, Tooltip } from 'recharts'
import { api } from '../services/api'
import { formatBeijingDateTimeShort } from '../utils/date'

// #2005 S2: 仪表盘按角色分层——
//   system_admin / platform_staff → 平台治理视图（商户/用户/审核，无经营 KPI）
//   merchant_admin / site_admin / site_member → 经营仪表盘（数据源 GET /admin/dashboard/stats，后端按作用域）
const STATUS_LABEL = { available: '可租', rented: '在租', maintenance: '维修中' }
const STATUS_COLOR = { available: '#1890ff', rented: '#52c41a', maintenance: '#faad14' }

const SCOPE_LABEL = {
  merchant_admin: '商户资产总数',
  site_admin: '本网点资产总数',
  site_member: '本网点资产总数',
}

export default function Dashboard() {
  const [data, setData] = useState(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(false)
  const navigate = useNavigate()

  const load = async () => {
    setLoading(true)
    setError(false)
    try {
      const resp = await api.get('/admin/dashboard/stats')
      if (resp.code === 20000 && resp.data) {
        setData(resp.data)
      } else {
        setError(true)
      }
    } catch (e) {
      setError(true)
    } finally {
      setLoading(false)
    }
  }

  useEffect(() => { load() }, [])

  if (loading) {
    return (
      <div style={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: 400 }}>
        <Spin size="large" />
      </div>
    )
  }

  if (error || !data) {
    return (
      <div style={{ padding: 40, textAlign: 'center' }}>
        <Empty description="仪表盘数据加载失败" />
        <Button type="primary" icon={<ReloadOutlined />} onClick={load} style={{ marginTop: 16 }}>
          重试
        </Button>
      </div>
    )
  }

  const role = data.role
  if (role === 'system_admin' || role === 'platform_staff') {
    return <GovernanceView data={data} navigate={navigate} />
  }
  return <OpsDashboard data={data} role={role} navigate={navigate} />
}

// ---------------- 平台治理视图（system_admin / platform_staff） ----------------
function GovernanceView({ data, navigate }) {
  const shortcuts = [
    { label: '商户管理', icon: <ShopOutlined />, to: '/merchants' },
    { label: '用户管理', icon: <UserOutlined />, to: '/system/user-management' },
    { label: '实名审核队列', icon: <IdcardOutlined />, to: '/face-review' },
    { label: '申诉处理', icon: <InboxOutlined />, to: '/appeals' },
    { label: '操作日志', icon: <AuditOutlined />, to: '/system/audit-logs' },
  ]
  const columns = [
    { title: '商户', dataIndex: 'name', key: 'name', render: v => v || '-' },
    { title: '租户 ID', dataIndex: 'tenant_id', key: 'tenant_id', render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v ? `${v.slice(0, 8)}…` : '-'}</span> },
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: v => (v ? formatBeijingDateTimeShort(v) : '-') },
  ]

  return (
    <div>
      <Row gutter={16} className="mb-6">
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/merchants')}>
            <Statistic title="商户总数" value={data.merchants_count || 0} prefix={<ShopOutlined />} valueStyle={{ color: '#1890ff' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic title="租户总数" value={data.tenants_count || 0} prefix={<ContainerOutlined />} />
          </Card>
        </Col>
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/system/user-management')}>
            <Statistic title="用户总数" value={data.users_count || 0} prefix={<TeamOutlined />} valueStyle={{ color: '#722ed1' }} />
          </Card>
        </Col>
        <Col span={6}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/face-review')}>
            <Statistic title="待实名审核" value={data.pending_face_review || 0} prefix={<IdcardOutlined />} valueStyle={{ color: '#fa8c16' }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} className="mb-6">
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/appeals')}>
            <Statistic title="待处理申诉" value={data.pending_appeals || 0} prefix={<AlertOutlined />} valueStyle={{ color: '#cf1322' }} />
          </Card>
        </Col>
        <Col span={16}>
          <Card title="快捷入口">
            <Space wrap>
              {shortcuts.map(s => (
                <Button key={s.to} icon={s.icon} onClick={() => navigate(s.to)}>{s.label}</Button>
              ))}
            </Space>
          </Card>
        </Col>
      </Row>

      <Card title="最近商户">
        <Table
          columns={columns}
          dataSource={data.recent_merchants || []}
          rowKey="id"
          pagination={false}
          size="small"
          locale={{ emptyText: '暂无商户' }}
        />
      </Card>
    </div>
  )
}

// ---------------- 经营仪表盘（merchant_admin / site_admin / site_member） ----------------
function OpsDashboard({ data, role, navigate }) {
  const dist = data.status_distribution || []
  const trend = (data.revenue_trend || []).map(r => ({ month: r.month, revenue: r.revenue }))
  const assetTitle = SCOPE_LABEL[role] || '资产总数'

  return (
    <div>
      <Row gutter={16} className="mb-6">
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/site/stock?status=rented')}>
            <Statistic title="在租资产数量" value={data.rented_assets || 0} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate(`/orders?status=in_lease&lease_end_date_eq=${new Date(Date.now() + 8 * 3600000).toISOString().split('T')[0]}`)}>
            <Statistic title="今日到期租约" value={data.expiring_today || 0} prefix={<ToolOutlined />} valueStyle={{ color: '#cf1322' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card
            style={{ cursor: 'pointer', borderColor: '#ff4d4f', background: '#fff1f0' }}
            onClick={() => navigate('/overdue-alerts')}
          >
            <Statistic title="逾期未归还" value={data.overdue || 0} prefix={<AlertOutlined />} valueStyle={{ color: '#faad14', fontWeight: 'bold' }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} className="mb-6">
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/instruments/list')}>
            <Statistic title={assetTitle} value={data.total_assets || 0} prefix={<ShoppingOutlined />} valueStyle={{ color: '#1890ff' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/orders?status=in_lease')}>
            <Statistic title="生效租约" value={data.active_leases || 0} prefix={<ShoppingOutlined />} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate(`/orders?start_date=${new Date(Date.now() + 8 * 3600000).toISOString().split('T')[0]}&end_date=${new Date(Date.now() + 8 * 3600000).toISOString().split('T')[0]}`)}>
            <Statistic title="今日新订单" value={data.new_orders_today || 0} valueStyle={{ color: '#722ed1' }} />
          </Card>
        </Col>
      </Row>

      <Row gutter={16} className="mb-6">
        <Col span={8}>
          <Card style={{ cursor: 'pointer' }} onClick={() => navigate('/site/stock?status=available')}>
            <Statistic title="可租资产" value={data.available_assets || 0} valueStyle={{ color: '#52c41a' }} />
          </Card>
        </Col>
        <Col span={16}>
          <Card title="收入趋势（近 6 个月）" style={{ cursor: 'pointer' }} onClick={() => navigate('/admin/billing')}>
            {trend.length > 0 ? (
              <ResponsiveContainer width="100%" height={220}>
                <LineChart data={trend}>
                  <XAxis dataKey="month" />
                  <YAxis />
                  <Tooltip formatter={(v) => `¥${Number(v).toFixed(2)}`} />
                  <Line type="monotone" dataKey="revenue" stroke="#1890ff" strokeWidth={2} />
                </LineChart>
              </ResponsiveContainer>
            ) : (
              <Empty description="暂无收入数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>
      </Row>

      <Row gutter={16} className="mb-6">
        <Col span={24}>
          <Card title="资产状态分布">
            {dist.some(d => d.value > 0) ? (
              <ResponsiveContainer width="100%" height={260}>
                <PieChart>
                  <Pie
                    data={dist.map(d => ({ ...d, name: STATUS_LABEL[d.name] || d.name }))}
                    cx="50%"
                    cy="50%"
                    innerRadius={50}
                    outerRadius={90}
                    paddingAngle={5}
                    dataKey="value"
                    onClick={(entry) => {
                      const raw = Object.keys(STATUS_LABEL).find(k => STATUS_LABEL[k] === entry.name) || entry.name
                      navigate(`/site/stock?status=${raw}`)
                    }}
                  >
                    {dist.map(d => (
                      <Cell key={d.name} fill={STATUS_COLOR[d.name] || '#d9d9d9'} style={{ cursor: 'pointer' }} />
                    ))}
                  </Pie>
                  <Tooltip formatter={(v, n) => [`${v} 件`, n]} />
                  <Legend />
                </PieChart>
              </ResponsiveContainer>
            ) : (
              <Empty description="暂无资产数据" image={Empty.PRESENTED_IMAGE_SIMPLE} />
            )}
          </Card>
        </Col>
      </Row>

      <div style={{ color: '#8c8c8c', fontSize: 12 }}>
        单位：元（金额）；数据作用域由当前账号角色决定。
      </div>
    </div>
  )
}
