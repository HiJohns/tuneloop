import { useState } from 'react'
import { Table, Tag, Space, Form, Select, Statistic, Row, Col, Drawer, Timeline, Button, Badge } from 'antd'
import { EyeOutlined, EditOutlined } from '@ant-design/icons'
import { assets } from '../data/mockData'

const statusColors = {
  "在租": "green",
  "待租": "blue",
  "维修中": "orange"
}

const levelColors = {
  "入门级": "default",
  "专业级": "blue",
  "大师级": "gold"
}

const sites = [
  { value: "Site-001", label: "北京总店" },
  { value: "Site-002", label: "上海分店" },
  { value: "Site-003", label: "维修供应商" }
]

export default function Dashboard() {
  const [form] = Form.useForm()
  const [selectedSite, setSelectedSite] = useState(null)
  const [drawerOpen, setDrawerOpen] = useState(false)
  const [selectedAsset, setSelectedAsset] = useState(null)
  const [statusFilter, setStatusFilter] = useState(null)

  const today = new Date().toISOString().split('T')[0]

  const filteredAssets = selectedSite
    ? assets.filter(a => a.siteId === selectedSite)
    : assets

  const displayedAssets = statusFilter
    ? filteredAssets.filter(a => a.status === statusFilter)
    : filteredAssets

  const totalValue = filteredAssets
    .filter(a => a.status === "在租")
    .reduce((sum, a) => sum + a.value, 0)

  const expiringToday = filteredAssets.filter(a => 
    a.leaseEnd && a.leaseEnd <= today && a.status === "在租"
  ).length

  const overdueAssets = filteredAssets.filter(a => 
    a.leaseEnd && a.leaseEnd < today && a.status === "在租"
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
          "待租": { color: 'blue', text: '在线' },
          "维修中": { color: 'blue', text: '维修中' }
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
      render: (value) => `¥${value.toLocaleString()}`
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

  return (
    <div>
      <Row gutter={16} className="mb-6">
        <Col span={8}>
          <Card>
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
              cursor: 'default',
              borderColor: '#ff4d4f',
              background: '#fff1f0',
              transition: 'all 0.3s'
            }}
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
        dataSource={displayedAssets} 
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
            <p><strong>估值:</strong> ¥{selectedAsset.value.toLocaleString()}</p>
            <p><strong>网点:</strong> {selectedAsset.site}</p>
            {selectedAsset.leaseEnd && (
              <p><strong>到期日:</strong> {selectedAsset.leaseEnd}</p>
            )}

            <div className="mt-4">
              <p><strong>流转轨迹:</strong></p>
              <Timeline
                items={selectedAsset.history.map((h) => ({
                  color: h.action === '维修' ? 'orange' : 'green',
                  children: (
                    <div>
                      <p>{h.date} - {h.action}</p>
                      {h.renter && <p>租户: {h.renter}</p>}
                      {h.note && <p>备注: {h.note}</p>}
                    </div>
                  )
                }))}
              />
              <p><strong>维修次数:</strong> {selectedAsset.repairCount} 次</p>
            </div>
          </div>
        )}
      </Drawer>


    </div>
  )
}

function Card({ children, className }) {
  return (
    <div className={`bg-white p-4 rounded shadow ${className}`}>
      {children}
    </div>
  )
}
