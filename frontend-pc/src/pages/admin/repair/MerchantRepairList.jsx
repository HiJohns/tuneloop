import { useState, useEffect } from 'react'
import { Card, Table, Tag, Spin, Select } from 'antd'
import { adminApi } from '../../../services/api'

const statusLabels = {
  pending_ship: '待发送', shipping: '发送中', inspecting: '质检中',
  quoted: '待回复', pending_payment: '待付款', pending_cancel: '待取消',
  repairing: '维修中', return_pending: '待发回', returned: '已发回',
  closed: '已关闭', appealing: '申诉中',
}
const statusColors = {
  pending_ship: 'orange', shipping: 'blue', inspecting: 'purple',
  quoted: 'gold', pending_payment: 'cyan', pending_cancel: 'red',
  repairing: 'geekblue', return_pending: 'lime', returned: 'green',
}

export default function MerchantRepairList() {
  const [requests, setRequests] = useState([])
  const [loading, setLoading] = useState(true)
  const [statusFilter, setStatusFilter] = useState('')

  useEffect(() => {
    const params = statusFilter ? `?status=${statusFilter}` : ''
    adminApi.get(`/merchant/repair-requests${params}`).then(r => {
      if (r.code === 20000) setRequests(r.data?.list || [])
    }).finally(() => setLoading(false))
  }, [statusFilter])

  const columns = [
    { title: '创建时间', dataIndex: 'created_at', key: 'created_at', render: v => v ? new Date(v).toLocaleDateString() : '-' },
    { title: '乐器', dataIndex: 'user_instrument_id', key: 'instrument', render: (_, r) => r.sn || '-' },
    { title: '状态', dataIndex: 'status', key: 'status', render: s => <Tag color={statusColors[s]}>{statusLabels[s] || s}</Tag> },
    { title: '网点', dataIndex: 'site_id', key: 'site', render: v => v?.slice(0, 8) || '-' },
    { title: '报价', dataIndex: 'quote_amount', key: 'quote', render: v => v ? `¥${Number(v).toFixed(2)}` : '-' },
  ]

  return (
    <Card title="报修列表" extra={
      <Select value={statusFilter} onChange={setStatusFilter} allowClear placeholder="全部状态" style={{ width: 140 }}>
        {Object.entries(statusLabels).map(([k, v]) => <Select.Option key={k} value={k}>{v}</Select.Option>)}
      </Select>
    }>
      {loading ? <Spin /> : <Table rowKey="id" dataSource={requests} columns={columns} />}
    </Card>
  )
}
