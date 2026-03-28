import { useState, useEffect } from 'react'
import { Table, Button, Input, Select, Space, Tag, Card, Modal, Form, InputNumber, message, Spin, Empty } from 'antd'
import { PlusOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import { depositApi } from '../services/api'

const typeOptions = [
  { label: '全部类型', value: '' },
  { label: '支付', value: 'payment' },
  { label: '退款', value: 'refund' },
]

const statusOptions = [
  { label: '全部状态', value: '' },
  { label: '待处理', value: 'pending' },
  { label: '已完成', value: 'completed' },
  { label: '已取消', value: 'cancelled' },
]

export default function DepositFlow() {
  const [deposits, setDeposits] = useState([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState(null)
  const [pagination, setPagination] = useState({ current: 1, pageSize: 10, total: 0 })
  const [filters, setFilters] = useState({ type: '', status: '' })
  const [createModalOpen, setCreateModalOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    loadDeposits()
  }, [pagination.current, pagination.pageSize, filters.type, filters.status])

  const loadDeposits = async () => {
    try {
      setLoading(true)
      setError(null)
      const params = {
        page: pagination.current,
        pageSize: pagination.pageSize,
      }
      if (filters.type) {
        params.type = filters.type
      }
      if (filters.status) {
        params.status = filters.status
      }
      
      const response = await depositApi.list(params)
      if (response.code === 20000 && response.data) {
        setDeposits(response.data.list || [])
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

  const handleTableChange = (pag) => {
    setPagination({
      current: pag.current,
      pageSize: pag.pageSize,
      total: pagination.total,
    })
  }

  const handleCreateDeposit = async (values) => {
    try {
      setSaving(true)
      await depositApi.create(values)
      message.success('押金记录创建成功')
      setCreateModalOpen(false)
      form.resetFields()
      loadDeposits()
    } catch (err) {
      message.error('创建失败: ' + err.message)
    } finally {
      setSaving(false)
    }
  }

  const columns = [
    {
      title: '交易ID',
      dataIndex: 'id',
      key: 'id',
      width: 220,
      ellipsis: true,
    },
    {
      title: '租约ID',
      dataIndex: 'lease_id',
      key: 'lease_id',
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
      title: '金额',
      dataIndex: 'amount',
      key: 'amount',
      width: 120,
      align: 'right',
      render: (value) => `¥${(value || 0).toLocaleString()}`,
    },
    {
      title: '类型',
      dataIndex: 'type',
      key: 'type',
      width: 100,
      render: (type) => {
        const typeMap = {
          payment: { text: '支付', color: 'blue' },
          refund: { text: '退款', color: 'green' },
        }
        const info = typeMap[type] || { text: type, color: 'default' }
        return <Tag color={info.color}>{info.text}</Tag>
      },
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => {
        const statusMap = {
          pending: { text: '待处理', color: 'orange' },
          completed: { text: '已完成', color: 'green' },
          cancelled: { text: '已取消', color: 'red' },
        }
        const info = statusMap[status] || { text: status, color: 'default' }
        return <Tag color={info.color}>{info.text}</Tag>
      },
    },
    {
      title: '交易日期',
      dataIndex: 'transaction_date',
      key: 'transaction_date',
      width: 120,
    },
    {
      title: '备注',
      dataIndex: 'notes',
      key: 'notes',
      ellipsis: true,
    },
  ]

  if (loading && deposits.length === 0) {
    return (
      <div className="p-6">
        <div className="text-center py-16">
          <Spin size="large" />
          <div className="mt-4 text-gray-500">数据正在同步中...</div>
        </div>
      </div>
    )
  }

  if (error && deposits.length === 0) {
    return (
      <div className="p-6">
        <Empty description="数据加载失败" />
        <Button type="primary" onClick={loadDeposits} className="mt-4">
          重试
        </Button>
      </div>
    )
  }

  return (
    <div className="p-6">
      <Card
        title="押金流水"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setCreateModalOpen(true)}>
            新建记录
          </Button>
        }
      >
        <div className="mb-4 flex gap-4 items-center flex-wrap">
          <Select
            options={typeOptions}
            value={filters.type}
            onChange={(value) => {
              setFilters(prev => ({ ...prev, type: value }))
              setPagination(prev => ({ ...prev, current: 1 }))
            }}
            style={{ width: 120 }}
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
          <Button icon={<ReloadOutlined />} onClick={loadDeposits}>
            刷新
          </Button>
        </div>

        <Table
          columns={columns}
          dataSource={deposits || [] || []}
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
        title="新建押金记录"
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
          onFinish={handleCreateDeposit}
        >
          <Form.Item
            name="lease_id"
            label="租约ID"
            rules={[{ required: true, message: '请输入租约ID' }]}
          >
            <Input placeholder="请输入租约ID" />
          </Form.Item>

          <Form.Item
            name="user_id"
            label="用户ID"
            rules={[{ required: true, message: '请输入用户ID' }]}
          >
            <Input placeholder="请输入用户ID" />
          </Form.Item>

          <Form.Item
            name="amount"
            label="金额"
            rules={[{ required: true, message: '请输入金额' }]}
          >
            <InputNumber min={0} style={{ width: '100%' }} placeholder="请输入金额" />
          </Form.Item>

          <Form.Item
            name="type"
            label="类型"
            rules={[{ required: true, message: '请选择类型' }]}
          >
            <Select placeholder="请选择类型">
              <Select.Option value="payment">支付</Select.Option>
              <Select.Option value="refund">退款</Select.Option>
            </Select>
          </Form.Item>

          <Form.Item
            name="transaction_date"
            label="交易日期"
            rules={[{ required: true, message: '请选择交易日期' }]}
          >
            <Input type="date" />
          </Form.Item>

          <Form.Item name="notes" label="备注">
            <Input.TextArea rows={3} placeholder="请输入备注" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
