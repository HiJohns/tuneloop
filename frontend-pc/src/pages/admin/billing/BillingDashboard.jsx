import { useState, useEffect } from 'react'
import { Card, Table, Tag, Row, Col, DatePicker, Button, message, Space, Statistic } from 'antd'
import { DownloadOutlined, ReloadOutlined } from '@ant-design/icons'
import api from '../../../services/api'

const { RangePicker } = DatePicker

const statusLabels = {
  reserved: '未支付', paid: '待发货', pending_shipment: '待发货',
  in_transit: '运输中', shipped: '已发货', in_lease: '租赁中',
  returning: '归还中', returned: '已归还', completed: '已完成',
  cancelled: '已取消', expired: '已超期',
}

const columns = [
  { title: '订单号', dataIndex: 'order_id', key: 'order_id', width: 100, render: v => v?.slice(0, 8) },
  { title: '时间', dataIndex: 'created_at', key: 'created_at', width: 100, render: v => v?.slice(0, 10) },
  { title: '用户', dataIndex: 'user_name', key: 'user_name' },
  { title: '乐器', dataIndex: 'instrument_name', key: 'instrument_name' },
  { title: '实付', dataIndex: 'cash_paid', key: 'cash_paid', width: 80, render: v => `¥${Number(v).toFixed(2)}` },
  { title: '预付点', dataIndex: 'prepaid_used', key: 'prepaid_used', width: 80, render: v => `¥${Number(v).toFixed(2)}` },
  { title: '赠点', dataIndex: 'gift_used', key: 'gift_used', width: 80, render: v => `¥${Number(v).toFixed(2)}` },
  { title: '押金', dataIndex: 'deposit', key: 'deposit', width: 80, render: v => `¥${Number(v).toFixed(2)}` },
  { title: '状态', dataIndex: 'status', key: 'status', width: 80, render: v => <Tag>{statusLabels[v] || v}</Tag> },
]

export default function BillingDashboard() {
  const [data, setData] = useState([])
  const [loading, setLoading] = useState(false)
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [pageSize, setPageSize] = useState(20)
  const [summary, setSummary] = useState(null)
  const [dateRange, setDateRange] = useState([])

  const fetchReport = async () => {
    setLoading(true)
    try {
      const params = { page, page_size: pageSize }
      if (dateRange[0]) params.start = dateRange[0].format('YYYY-MM-DD')
      if (dateRange[1]) params.end = dateRange[1].format('YYYY-MM-DD')
      const resp = await api.get('/admin/billing/report', { params })
      if (resp.code === 20000) {
        setData(resp.data.list || [])
        setTotal(resp.data.total || 0)
        setSummary(resp.data.summary)
      }
    } catch (err) {
      message.error('获取账单失败')
    }
    setLoading(false)
  }

  useEffect(() => { fetchReport() }, [page, pageSize])

  const handleExport = () => {
    const params = new URLSearchParams({ format: 'csv' })
    if (dateRange[0]) params.set('start', dateRange[0].format('YYYY-MM-DD'))
    if (dateRange[1]) params.set('end', dateRange[1].format('YYYY-MM-DD'))
    window.open(`/api/admin/billing/report?${params.toString()}`)
  }

  return (
    <div style={{ padding: 24 }}>
      <Card title="账单报表" extra={
        <Space>
          <RangePicker value={dateRange} onChange={setDateRange} />
          <Button icon={<ReloadOutlined />} onClick={fetchReport}>查询</Button>
          <Button icon={<DownloadOutlined />} onClick={handleExport}>导出 CSV</Button>
        </Space>
      }>
        {summary && (
          <Row gutter={16} style={{ marginBottom: 16 }}>
            <Col span={4}><Card><Statistic title="总订单" value={summary.total_orders} /></Card></Col>
            <Col span={5}><Card><Statistic title="实付总额" value={summary.total_cash_paid} precision={2} prefix="¥" /></Card></Col>
            <Col span={5}><Card><Statistic title="预付点抵扣" value={summary.total_prepaid_used} precision={2} prefix="¥" /></Card></Col>
            <Col span={5}><Card><Statistic title="赠点抵扣" value={summary.total_gift_used} precision={2} prefix="¥" /></Card></Col>
            <Col span={5}><Card><Statistic title="总退款" value={summary.total_refund} precision={2} prefix="¥" valueStyle={{ color: '#cf1322' }} /></Card></Col>
          </Row>
        )}
        <Table
          dataSource={data}
          columns={columns}
          rowKey="order_id"
          loading={loading}
          pagination={{
            current: page, pageSize, total,
            onChange: (p, ps) => { setPage(p); setPageSize(ps) },
            showSizeChanger: true, showTotal: t => `共 ${t} 条`,
          }}
          size="small"
          scroll={{ x: 800 }}
        />
      </Card>
    </div>
  )
}
