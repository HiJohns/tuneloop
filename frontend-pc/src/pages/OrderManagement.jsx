import { useState, useEffect } from 'react'
import { Table, Card, Tag, Input, Select, Space, Spin } from 'antd'
import { useNavigate } from 'react-router-dom'
import api from '../services/api'

const STATUS_MAP = {
  reserved: { color: 'orange', text: '未支付' },
  paid: { color: 'blue', text: '待发货' },
  in_transit: { color: 'cyan', text: '运输中' },
  shipped: { color: 'blue', text: '已发货' },
  in_lease: { color: 'green', text: '租赁中' },
  returning: { color: 'yellow', text: '归还中' },
  returned: { color: 'default', text: '已归还' },
  completed: { color: 'default', text: '已完成' },
  cancelled: { color: 'red', text: '已取消' },
}

export default function OrderManagement() {
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('')
  const [snSearch, setSnSearch] = useState('')

  useEffect(() => { fetchOrders() }, [page, statusFilter])

  const fetchOrders = async () => {
    setLoading(true)
    try {
      const params = { page, pageSize: 20 }
      if (statusFilter) params.status = statusFilter
      if (snSearch) params.sn = snSearch
      const res = await api.get('/merchant/orders', { params })
      if (res.code === 20000) {
        setOrders(res.data.list)
        setTotal(res.data.total)
      }
    } catch { /* ignore */ }
    setLoading(false)
  }

  const columns = [
    { title: '订单号', dataIndex: 'id', key: 'id', render: (id) => id.slice(0, 8) + '...' },
    { title: '乐器', dataIndex: 'instrument_name', key: 'instrument_name', render: (_, r) => r.instrument_name || r.instrument_sn || '-' },
    { title: 'SN', dataIndex: 'instrument_sn', key: 'instrument_sn' },
    { title: '状态', dataIndex: 'status', key: 'status', render: (s) => {
      const m = STATUS_MAP[s] || { color: 'default', text: s }
      return <Tag color={m.color}>{m.text}</Tag>
    }},
    { title: '租期', dataIndex: 'start_date', key: 'start_date', render: (_, r) => `${r.start_date || '?'} ~ ${r.end_date || '?'}` },
    { title: '下单时间', dataIndex: 'created_at', key: 'created_at' },
  ]

  return (
    <Card title="订单管理" style={{ margin: 24 }}>
      <Space style={{ marginBottom: 16 }}>
        <Select
          placeholder="筛选状态" allowClear style={{ width: 140 }}
          onChange={v => { setStatusFilter(v || ''); setPage(1) }}
          options={Object.entries(STATUS_MAP).map(([k, v]) => ({ value: k, label: v.text }))}
        />
        <Input.Search
          placeholder="搜索 SN" style={{ width: 200 }}
          onSearch={v => { setSnSearch(v); setPage(1) }}
        />
      </Space>
      <Spin spinning={loading}>
        <Table
          dataSource={orders} columns={columns} rowKey="id"
          pagination={{ current: page, total, pageSize: 20, onChange: setPage }}
        />
      </Spin>
    </Card>
  )
}
