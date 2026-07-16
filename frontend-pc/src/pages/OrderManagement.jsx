import { useState, useEffect } from 'react'
import { Table, Card, Tag, Input, Select, Space, Spin, Button, Modal, message, DatePicker } from 'antd'
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
const NOT_RETURNED = ['reserved', 'paid', 'in_transit', 'shipped', 'in_lease']

const formatMD = (s) => {
  if (!s) return '-'
  // Parse YYYY-MM-DD or YYYY-MM-DDTHH:MM... as local date
  const m = s.length >= 10 ? s.slice(0, 10) : s
  const parts = m.split('-')
  if (parts.length === 3) {
    const year = parseInt(parts[0]), month = parseInt(parts[1]), day = parseInt(parts[2])
    if (!isNaN(year) && !isNaN(month) && !isNaN(day)) return `${month}月${day}日`
  }
  const d = new Date(s)
  if (isNaN(d.getTime())) return s
  return `${d.getMonth() + 1}月${d.getDate()}日`
}

const calcDays = (r) => {
  if (!r.delivered_at) return '-'
  const start = new Date(r.delivered_at)
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
  const [debugModal, setDebugModal] = useState({ open: false, order: null, status: '', deliveredAt: null, returnedAt: null })
  const [debugSaving, setDebugSaving] = useState(false)

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
    { title: '订单号', dataIndex: 'id', key: 'id', width: 100, render: (id) => id.slice(0, 8) + '...' },
    { title: 'SN', dataIndex: 'instrument_sn', key: 'instrument_sn', width: 100 },
    { title: '状态', dataIndex: 'status', key: 'status', width: 80, render: (s) => {
      const m = STATUS_MAP[s] || { color: 'default', text: s }
      return <Tag color={m.color}>{m.text}</Tag>
    }},
    { title: '下单时间', dataIndex: 'created_at', key: 'created_at', width: 80, render: (v) => v ? formatMD(v) : '-' },
    { title: '租期起始', key: 'delivered_at', width: 80, render: (_, r) => r.delivered_at ? formatMD(r.delivered_at) : '-' },
    { title: '预计归还', key: 'end_date', width: 80, render: (_, r) => formatMD(r.end_date) },
    { title: '租期结束', key: 'returned_at', width: 80, render: (_, r) => r.returned_at ? formatMD(r.returned_at) : '-' },
    { title: '租赁天数', key: 'days', width: 70, render: (_, r) => calcDays(r) },
    { title: '租赁人', dataIndex: 'user_name', key: 'user_name', width: 100, render: (v) => v || '-' },
    ...(debugMode ? [{
      title: '操作', key: 'action', width: 80, render: (_, r) => (
        <Button size="small" onClick={() => setDebugModal({
          open: true, order: r, status: r.status,
          deliveredAt: r.delivered_at ? dayjs(r.delivered_at) : null,
          returnedAt: r.returned_at ? dayjs(r.returned_at) : null,
        })}>调试</Button>
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
          scroll={{ x: 800 }}
        />
      </Spin>
      <Modal title="调试订单" open={debugModal.open} onCancel={() => setDebugModal(p => ({ ...p, open: false }))}
        footer={[
          <Button key="clear-start" danger onClick={async () => {
            setDebugSaving(true)
            try {
              const res = await api.put(`/orders/${debugModal.order.id}/admin-update?clear_delivered=1&clear_returned=1`, {
                status: shippedStatus(debugModal.order.status),
              })
              if (res.code === 20000) { message.success('已清除起始日'); setDebugModal(p => ({ ...p, deliveredAt: null, returnedAt: null })); fetchOrders() }
              else message.error(res.message || '清除失败')
            } catch { message.error('清除失败') }
            setDebugSaving(false)
          }}>清除起始日</Button>,
          <Button key="clear-end" onClick={async () => {
            setDebugSaving(true)
            try {
              const res = await api.put(`/orders/${debugModal.order.id}/admin-update?clear_returned=1`, {
                status: 'in_lease',
              })
              if (res.code === 20000) { message.success('已清除归还日'); setDebugModal(p => ({ ...p, returnedAt: null })); fetchOrders() }
              else message.error(res.message || '清除失败')
            } catch { message.error('清除失败') }
            setDebugSaving(false)
          }}>清除归还日</Button>,
          <Button key="save" type="primary" loading={debugSaving} onClick={async () => {
            setDebugSaving(true)
            try {
              const body = { status: debugModal.status }
              const params = new URLSearchParams()
              if (debugModal.deliveredAt) {
                body.delivered_at = debugModal.deliveredAt.format('YYYY-MM-DD')
              } else {
                params.set('clear_delivered', '1')
              }
              if (debugModal.returnedAt) {
                body.returned_at = debugModal.returnedAt.format('YYYY-MM-DD')
              } else {
                params.set('clear_returned', '1')
              }
              const qs = params.toString()
              const url = `/orders/${debugModal.order.id}/admin-update${qs ? '?' + qs : ''}`
              const res = await api.put(url, body)
              if (res.code === 20000) { message.success('已保存'); setDebugModal(p => ({ ...p, open: false })); fetchOrders() }
              else message.error(res.message || '保存失败')
            } catch (err) { message.error('保存失败') }
            setDebugSaving(false)
          }}>保存</Button>,
        ]}>
        <div style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>订单状态</div>
          <Select value={debugModal.status} onChange={v => setDebugModal(p => ({ ...p, status: v }))} style={{ width: '100%' }}>
            {Object.entries(STATUS_MAP).map(([k, v]) => (
              <Select.Option key={k} value={k}>{v.text}</Select.Option>
            ))}
          </Select>
        </div>
        <div style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>到货日（起始）</div>
          <DatePicker value={debugModal.deliveredAt} onChange={d => setDebugModal(p => ({ ...p, deliveredAt: d }))} style={{ width: '100%' }} placeholder="选择日期/留空清除" />
        </div>
        <div style={{ marginBottom: 12 }}>
          <div style={{ marginBottom: 4, fontSize: 12, color: '#888' }}>归还日（结束）</div>
          <DatePicker value={debugModal.returnedAt} onChange={d => setDebugModal(p => ({ ...p, returnedAt: d }))} style={{ width: '100%' }} placeholder="选择日期/留空清除" />
        </div>
      </Modal>
    </Card>
  )
}

function shippedStatus(status) {
  const idx = ['reserved', 'paid', 'pending_shipment', 'in_transit', 'shipped'].indexOf(status)
  return idx >= 0 ? 'shipped' : status
}
