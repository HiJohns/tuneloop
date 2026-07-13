import { useState, useEffect } from 'react'
import { Card, Table, Tag, Select, Button, message, DatePicker, Input, Space } from 'antd'
import { DownloadOutlined, SearchOutlined, ReloadOutlined } from '@ant-design/icons'
import { api } from '../../../services/api'

const { RangePicker } = DatePicker

const typeOptions = [
  { value: '', label: '全部类别' },
  { value: 'rent', label: '租赁支付' },
  { value: 'repair', label: '报修支付' },
  { value: 'points', label: '点数购买' },
  { value: 'damage', label: '定损赔偿' },
]

const methodOptions = [
  { value: '', label: '全部方式' },
  { value: 'jsapi', label: 'JSAPI' },
  { value: 'native', label: 'Native' },
  { value: 'h5', label: 'H5' },
  { value: 'mock', label: '模拟（测试）' },
]

const statusConfig = {
  paid: { text: '已支付', color: 'green' },
  pending: { text: '待支付', color: 'orange' },
  failed: { text: '失败', color: 'red' },
  closed: { text: '已关闭', color: 'gray' },
  refunding: { text: '退款中', color: 'blue' },
  refunded: { text: '已退款', color: 'purple' },
}

export default function PaymentList() {
  const [data, setData] = useState([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [filters, setFilters] = useState({ order_type: '', method: '', status: '', search: '', start_date: '', end_date: '' })

  useEffect(() => { fetchData() }, [page, pageSize, filters])

  const fetchData = async () => {
    setLoading(true)
    try {
      const params = { page, page_size: pageSize, ...filters }
      const resp = await api.get('/admin/payments', { params })
      setData(resp?.data?.list || [])
      setTotal(resp?.data?.total || 0)
    } catch { message.error('获取失败') }
    setLoading(false)
  }

  const handleExport = async () => {
    try {
      const query = new URLSearchParams(filters).toString()
      const link = document.createElement('a')
      link.href = `/api/admin/payments/export?${query}`
      link.click()
    } catch { message.error('导出失败') }
  }

  const handleQuery = async (outTradeNo) => {
    if (!outTradeNo) { message.warning('无商户订单号'); return }
    try {
      const resp = await api.post(`/admin/payments/${outTradeNo}/query`)
      if (resp.code === 20000) {
        message.success(`微信状态: ${resp.data.wechat_state}, 交易号: ${resp.data.transaction_id || '-'}`)
      } else {
        message.error(resp.message || '查询失败')
      }
    } catch { message.error('查询接口异常') }
  }

  const columns = [
    { title: '时间', dataIndex: 'created_at', width: 160, render: (v) => v ? new Date(v).toLocaleString() : '-' },
    { title: '商户订单号', dataIndex: 'out_trade_no', width: 180, ellipsis: true },
    { title: '微信交易号', dataIndex: 'transaction_id', width: 180, ellipsis: true },
    { title: '类别', dataIndex: 'order_type', width: 100, render: (v) => typeOptions.find(o => o.value === v)?.label || v },
    { title: '金额', dataIndex: 'amount', width: 100, render: (v) => v != null ? `¥${Number(v).toFixed(2)}` : '-' },
    { title: '方式', dataIndex: 'method', width: 100, render: (v) => methodOptions.find(o => o.value === v)?.label || v || '-' },
    { title: '状态', dataIndex: 'status', width: 90, render: (v) => {
      const cfg = statusConfig[v] || { text: v, color: 'default' }
      return <Tag color={cfg.color}>{cfg.text}</Tag>
    }},
    { title: '退款', key: 'refunds', width: 160, render: (_, r) => {
      if (!r.refunds?.length) return <span style={{ color: '#999' }}>无</span>
      return r.refunds.map((ref, i) => (
        <div key={i} style={{ fontSize: 12, marginBottom: 2 }}>
          退款 ¥{Number(ref.amount).toFixed(2)} <Tag color={ref.status === 'refunded' ? 'green' : ref.status === 'failed' ? 'red' : 'blue'}>{ref.status}</Tag>
        </div>
      ))
    }},
    { title: '操作', width: 120, render: (_, r) => (
      <Button size="small" onClick={() => handleQuery(r.out_trade_no)} disabled={!r.out_trade_no}>查单</Button>
    )},
  ]

  return (
    <div style={{ padding: '24px' }}>
      <Card>
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
          <h2 style={{ margin: 0 }}>支付明细</h2>
          <Space>
            <RangePicker onChange={(_, dateStrings) => setFilters(f => ({ ...f, start_date: dateStrings[0], end_date: dateStrings[1] }))} />
            <Select value={filters.order_type} onChange={v => { setFilters(f => ({ ...f, order_type: v })); setPage(1) }} options={typeOptions} style={{ width: 120 }} />
            <Select value={filters.method} onChange={v => { setFilters(f => ({ ...f, method: v })); setPage(1) }} options={methodOptions} style={{ width: 120 }} />
            <Select value={filters.status} onChange={v => { setFilters(f => ({ ...f, status: v })); setPage(1) }} options={[
              { value: '', label: '全部状态' },
              ...Object.entries(statusConfig).map(([k, v]) => ({ value: k, label: v.text })),
            ]} style={{ width: 120 }} />
            <Input
              placeholder="搜索订单号/交易号"
              prefix={<SearchOutlined />}
              value={filters.search}
              onChange={e => setFilters(f => ({ ...f, search: e.target.value }))}
              style={{ width: 180 }}
              allowClear
            />
            <Button icon={<ReloadOutlined />} onClick={fetchData}>刷新</Button>
            <Button icon={<DownloadOutlined />} onClick={handleExport}>导出CSV</Button>
          </Space>
        </div>
        <Table
          columns={columns}
          dataSource={data}
          loading={loading}
          rowKey="id"
          pagination={{ current: page, pageSize, total, showSizeChanger: true, showTotal: t => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps) },
          }}
          scroll={{ x: 1400 }}
        />
      </Card>
    </div>
  )
}
