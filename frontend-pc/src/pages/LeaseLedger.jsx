import { useState, useEffect } from 'react'
import { useSearchParams } from 'react-router-dom'
import { formatCents } from '../utils/money'
import { Table, Button, Input, Select, Space, Tag, Card, Modal, Form, InputNumber, message, Spin, Empty } from 'antd'
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import { leaseApi } from '../services/api'

// #2005 S4: 业务日期（北京时间，与 Dashboard 卡片口径一致）
const bjDateStr = (offsetDays = 0) =>
  new Date(Date.now() + 8 * 3600000 + offsetDays * 86400000).toISOString().split('T')[0]

// 仪表盘联动筛选：?filter=expiring|active|overdue
const FILTER_LABEL = { expiring: '今日到期', overdue: '逾期未归还', active: '生效中' }

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '生效中', value: 'active' },
  { label: '已到期', value: 'expired' },
  { label: '已终止', value: 'terminated' },
]

export default function LeaseLedger() {
  const [searchParams, setSearchParams] = useSearchParams()
  const filterParam = searchParams.get('filter') || '' // #2005 S4: expiring|active|overdue
  const [leases, setLeases] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [pagination, setPagination] = useState({ current: 1, pageSize: 10, total: 0 })
  const [filters, setFilters] = useState({
    status: ['expiring', 'active', 'overdue'].includes(filterParam) ? 'active' : '',
    keyword: '',
  })
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    loadLeases()
  }, [pagination.current, pagination.pageSize, filters.status, filterParam])

  const loadLeases = async () => {
    try {
      setLoading(true)
      setError(null)
      const params = {
        page: pagination.current,
        pageSize: pagination.pageSize,
      }
      if (filterParam) {
        // #2005 S4: 仪表盘卡片联动——生效中 + 日期口径（expiring=今日到期；overdue=严格早于今天）
        params.status = 'active'
        if (filterParam === 'expiring') params.end_date_eq = bjDateStr(0)
        if (filterParam === 'overdue') params.end_date = bjDateStr(-1)
      } else if (filters.status) {
        params.status = filters.status
      }
      
      const response = await leaseApi.list(params)
      if (response.code === 20000 && response.data) {
        setLeases(response.data.list || [])
        setPagination(prev => ({
          ...prev,
          total: response.data.total || 0,
        }))
      }
    } catch (err) {
      setError(err.message)
      message.error('加载失败: ' + err.message)
    } finally {
      setLoading(false)
    }
  }

  const handleSearch = () => {
    setPagination(prev => ({ ...prev, current: 1 }))
    loadLeases()
  }

  const handleTableChange = (pag) => {
    setPagination({
      current: pag.current,
      pageSize: pag.pageSize,
      total: pagination.total,
    })
  }

  const handleCreateLease = async (values) => {
    try {
      setSaving(true)
      await leaseApi.create(values)
      message.success('租约创建成功')
      setCreateModalOpen(false)
      form.resetFields()
      loadLeases()
    } catch (err) {
      message.error('创建失败: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const handleTerminate = async (lease) => {
    try {
      await leaseApi.terminate(lease.id)
      message.success('租约已终止')
      loadLeases()
    } catch (err) {
      message.error('操作失败: ' + err.message)
    }
  }

  const columns = [
    {
      title: '租约ID',
      dataIndex: 'id',
      key: 'id',
      width: 220,
      ellipsis: true,
    },
    {
      title: '乐器ID',
      dataIndex: 'instrument_id',
      key: 'instrument_id',
      width: 220,
      ellipsis: true,
    },
    {
      title: '用户ID',
      dataIndex: 'user_id',
      key: 'user_id',
      width: 220,
      ellipsis: true,
    },
    {
      title: '开始日期',
      dataIndex: 'start_date',
      key: 'start_date',
      width: 120,
    },
    {
      title: '结束日期',
      dataIndex: 'end_date',
      key: 'end_date',
      width: 120,
    },
    {
      title: '月租金',
      dataIndex: 'monthly_rent',
      key: 'monthly_rent',
      width: 100,
      align: 'right',
      render: (value) => `¥${formatCents(value)}`,
    },
    {
      title: '押金',
      dataIndex: 'deposit_amount',
      key: 'deposit_amount',
      width: 100,
      align: 'right',
      render: (value) => `¥${formatCents(value)}`,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => {
        const statusMap = {
          active: { text: '生效中', color: 'green' },
          expired: { text: '已到期', color: 'orange' },
          terminated: { text: '已终止', color: 'red' },
        }
        const info = statusMap[status] || { text: status, color: 'default' }
        return <Tag color={info.color}>{info.text}</Tag>
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 120,
      render: (_, record) => (
        <Space>
          {record.status === 'active' && (
            <Button type="link" danger size="small" onClick={() => handleTerminate(record)}>
              终止
            </Button>
          )}
        </Space>
      ),
    },
  ]

  if (loading && leases.length === 0) {
    return (
      <div className="p-6">
        <div className="text-center py-16">
          <Spin size="large" />
          <div className="mt-4 text-gray-500">数据正在同步中...</div>
        </div>
      </div>
    )
  }

  if (error && leases.length === 0) {
    return (
      <div className="p-6">
        <Empty description="数据加载失败" />
        <Button type="primary" onClick={loadLeases} className="mt-4">
          重试
        </Button>
      </div>
    )
  }

  return (
    <div className="p-6">
      <Card
        title="租约台账"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
            新建租约
          </Button>
        }
      >
        <div className="mb-4 flex gap-4 items-center flex-wrap">
          <Input
            placeholder="搜索租约ID/乐器ID/用户ID"
            prefix={<SearchOutlined />}
            style={{ width: 250 }}
            value={filters.keyword}
            onChange={(e) => setFilters(prev => ({ ...prev, keyword: e.target.value }))}
            onPressEnter={handleSearch}
          />
          <Select
            options={statusOptions}
            value={filters.status}
            onChange={(value) => {
              setFilters(prev => ({ ...prev, status: value }))
              setPagination(prev => ({ ...prev, current: 1 }))
            }}
            style={{ width: 120 }}
          />
          <Button icon={<ReloadOutlined />} onClick={loadLeases}>
            刷新
          </Button>
        </div>

        {filterParam && (
          <div className="mb-4">
            <Space>
              <Tag color="blue">当前筛选: {FILTER_LABEL[filterParam] || filterParam}</Tag>
              <Button
                size="small"
                onClick={() => {
                  setSearchParams({})
                  setFilters(prev => ({ ...prev, status: '' }))
                  setPagination(prev => ({ ...prev, current: 1 }))
                }}
              >
                清除筛选
              </Button>
            </Space>
          </div>
        )}

        <Table
          columns={columns}
          dataSource={leases || [] || []}
          rowKey="id"
          loading={loading}
          pagination={{
            current: pagination.current,
            pageSize: pagination.pageSize,
            total: pagination.total,
            showSizeChanger: true,
            showQuickJumper: true,
            pageSizeOptions: ['10', '20', '50'],
            showTotal: (total) => `共 ${total} 条`,
          }}
          onChange={handleTableChange}
          scroll={{ x: 1200 }}
        />
      </Card>

      <Modal
        title="新建租约"
        open={createModalOpen}
        onCancel={() => {
          setCreateModalOpen(false)
          form.resetFields()
        }}
        onOk={() => form.submit()}
        confirmLoading={saving}
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleCreateLease}
        >
          <Form.Item
            name="user_id"
            label="用户ID"
            rules={[{ required: true, message: '请输入用户ID' }]}
          >
            <Input placeholder="请输入用户ID" />
          </Form.Item>

          <Form.Item
            name="instrument_id"
            label="乐器ID"
            rules={[{ required: true, message: '请输入乐器ID' }]}
          >
            <Input placeholder="请输入乐器ID" />
          </Form.Item>

          <Form.Item
            name="start_date"
            label="开始日期"
            rules={[{ required: true, message: '请选择开始日期' }]}
          >
            <Input type="date" />
          </Form.Item>

          <Form.Item
            name="end_date"
            label="结束日期"
            rules={[{ required: true, message: '请选择结束日期' }]}
          >
            <Input type="date" />
          </Form.Item>

          <Form.Item
            name="monthly_rent"
            label="月租金"
            rules={[{ required: true, message: '请输入月租金' }]}
          >
            <InputNumber min={0} style={{ width: '100%' }} placeholder="请输入月租金" />
          </Form.Item>

          <Form.Item
            name="deposit_amount"
            label="押金"
            rules={[{ required: true, message: '请输入押金' }]}
          >
            <InputNumber min={0} style={{ width: '100%' }} placeholder="请输入押金" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
