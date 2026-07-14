import { useState, useEffect } from 'react'
import { Table, Card, Tag, Input, Select, Space, Spin, Button, Modal, message, DatePicker } from 'antd'
import { useNavigate } from 'react-router-dom'
import dayjs from 'dayjs'
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

const NOT_STARTED = ['reserved', 'paid', 'in_transit', 'shipped']

const formatMD = (s) => {
  if (!s) return '-'
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

const calcDays = (r) => {
  if (NOT_STARTED.includes(r.status)) return '-'
  if (!r.start_date) return '-'
  const start = new Date(r.start_date)
  if (isNaN(start.getTime())) return '-'
  const end = r.returned_at ? new Date(r.returned_at) : new Date()
  if (isNaN(end.getTime())) return '-'
  const diff = Math.round((end - start) / (1000 * 60 * 60 * 24)) + 1
  return `${diff}天`
}

export default function OrderManagement() {
  const [orders, setOrders] = useState([])
  const [loading, setLoading] = useState(true)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [statusFilter, setStatusFilter] = useState('')
  const [snSearch, setSnSearch] = useState('')
  const [debugMode, setDebugMode] = useState(false)
  const [editModal, setEditModal] = useState({ open: false, order: null, status: '', startDate: null, endDate: null })
  const [editSaving, setEditSaving] = useState(false)

  useEffect(() => { api.get('/config').then(r => { if (r.code === 20000 && r.data?.debug_mode) setDebugMode(true) }).catch(() => {}) }, [])

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
    { title: '租期起始', key: 'start_date', render: (_, r) => formatMD(r.start_date) },
    { title: '租期结束', key: 'end_date', render: (_, r) => NOT_STARTED.includes(r.status) ? '-' : formatMD(r.returned_at || r.end_date) },
    { title: '租赁天数', key: 'days', render: (_, r) => calcDays(r) },
    { title: '租赁人', dataIndex: 'user_name', key: 'user_name', render: (v) => v || '-' },
    { title: '下单时间', dataIndex: 'created_at', key: 'created_at', render: (v) => v ? formatMD(v) : '-' },
    ...(debugMode ? [{
      title: '操作', key: 'action', render: (_, r) => (
        <Button size="small" onClick={() => setEditModal({ open: true, order: r, status: r.status, startDate: r.start_date ? dayjs(r.start_date) : null, endDate: r.end_date ? dayjs(r.end_date) : null })}>编辑</Button>
      )
    }] : []),
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
      <Modal title="编辑订单" open={editModal.open} onCancel={() => setEditModal(p => ({ ...p, open: false }))}
        onOk={async () => {
          setEditSaving(true)
          try {
            const startDateStr = editModal.startDate ? editModal.startDate.format('YYYY-MM-DD') : ''
            const endDateStr = editModal.endDate ? editModal.endDate.format('YYYY-MM-DD') : ''
            const res = await api.put(`/orders/${editModal.order.id}/admin-update`, {
              start_date: startDateStr,
              end_date: endDateStr,
              status: editModal.status,
            })
            if (res.code === 20000) {
              message.success('订单已更新')
              setEditModal(p => ({ ...p, open: false }))
              fetchOrders()
            } else {
              message.error(res.message || '更新失败')
            }
          } catch { message.error('更新失败') }
          setEditSaving(false)
        }}
        confirmLoading={editSaving}>
        <div style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>订单状态</div>
          <Select value={editModal.status} onChange={v => setEditModal(p => ({ ...p, status: v }))} style={{ width: '100%' }}>
            {Object.entries(STATUS_MAP).map(([k, v]) => (
              <Select.Option key={k} value={k}>{v.text}</Select.Option>
            ))}
          </Select>
        </div>
        <div style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>起始日期</div>
          <DatePicker value={editModal.startDate} onChange={d => setEditModal(p => ({ ...p, startDate: d }))} style={{ width: '100%' }} placeholder="选择日期" />
        </div>
        <div style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>结束日期</div>
          <DatePicker value={editModal.endDate} onChange={d => setEditModal(p => ({ ...p, endDate: d }))} style={{ width: '100%' }} placeholder="选择日期" />
        </div>
      </Modal>
    </Card>
  )
}
