import { useState, useEffect } from 'react'
import { Card, Table, Tag, Button, message, Modal, Input, Upload, Space, Descriptions } from 'antd'
import { EyeOutlined, UploadOutlined } from '@ant-design/icons'
import { api } from '../../../services/api'

const statusConfig = {
  pending: { text: '待开票', color: 'orange' },
  replied: { text: '已开票', color: 'green' },
}

function formatCents(cents) {
  return cents != null ? `¥${(cents / 100).toFixed(2)}` : '-'
}

export default function InvoiceList() {
  const [data, setData] = useState([])
  const [loading, setLoading] = useState(false)
  const [detailVisible, setDetailVisible] = useState(false)
  const [replyVisible, setReplyVisible] = useState(false)
  const [current, setCurrent] = useState(null)
  const [replyText, setReplyText] = useState('')
  const [replyFileUrl, setReplyFileUrl] = useState('')
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => { fetchData() }, [])

  const fetchData = async () => {
    setLoading(true)
    try {
      const resp = await api.get('/merchant/invoices')
      setData(resp?.data || [])
    } catch { message.error('获取失败') }
    setLoading(false)
  }

  const showDetail = (record) => {
    setCurrent(record)
    setDetailVisible(true)
  }

  const showReply = (record) => {
    setCurrent(record)
    setReplyText('')
    setReplyFileUrl('')
    setReplyVisible(true)
  }

  const handleReply = async () => {
    if (!replyText.trim() && !replyFileUrl.trim()) {
      message.warning('请输入回复内容或上传发票文件')
      return
    }
    setSubmitting(true)
    try {
      const resp = await api.post(`/merchant/invoices/${current.id}/reply`, {
        reply: replyText,
        invoice_file: replyFileUrl,
      })
      if (resp.code === 20000) {
        message.success('回复成功')
        setReplyVisible(false)
        fetchData()
      } else {
        message.error(resp.message || '回复失败')
      }
    } catch { message.error('回复失败') }
    setSubmitting(false)
  }

  const handleUpload = async (file) => {
    const formData = new FormData()
    formData.append('file', file)
    try {
      const resp = await api.post('/upload', formData, { headers: {} })
      if (resp.code === 20000 && resp.data?.url) {
        setReplyFileUrl(resp.data.url)
        message.success('上传成功')
      } else {
        message.error('上传失败')
      }
    } catch { message.error('上传失败') }
    return false // prevent antd default upload
  }

  const columns = [
    { title: '申请时间', dataIndex: 'created_at', width: 170, render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '顾客', dataIndex: 'customer_name', width: 120, ellipsis: true },
    { title: '订单数', dataIndex: 'order_count', width: 80, align: 'center' },
    {
      title: '开票金额', dataIndex: 'total_amount', width: 120,
      render: v => <span style={{ fontWeight: 600, color: '#D97706' }}>{formatCents(v)}</span>,
    },
    {
      title: '状态', dataIndex: 'status', width: 100,
      render: v => <Tag color={statusConfig[v]?.color || 'default'}>{statusConfig[v]?.text || v}</Tag>,
    },
    { title: '回复时间', dataIndex: 'replied_at', width: 170, render: v => v ? new Date(v).toLocaleString() : '-' },
    {
      title: '操作', width: 140,
      render: (_, record) => (
        <Space size="small">
          <Button size="small" icon={<EyeOutlined />} onClick={() => showDetail(record)}>详情</Button>
          {record.status === 'pending' && (
            <Button size="small" type="primary" onClick={() => showReply(record)}>开票回复</Button>
          )}
        </Space>
      ),
    },
  ]

  const orderColumns = [
    { title: '订单号', dataIndex: 'order_id', width: 280, ellipsis: true },
    { title: 'SN', dataIndex: 'sn', width: 140, ellipsis: true },
    { title: '下单日', dataIndex: 'created_at', width: 120, render: v => v ? new Date(v).toLocaleDateString() : '-' },
    { title: '实际租金', dataIndex: 'actual_rent_cents', width: 100, render: v => formatCents(v) },
    { title: '逾期费用', dataIndex: 'overdue_cents', width: 100, render: v => formatCents(v) },
    { title: '合计', dataIndex: 'total_cents', width: 100, render: v => <span style={{ fontWeight: 600 }}>{formatCents(v)}</span> },
  ]

  return (
    <div>
      <Card title="发票申请管理">
        <Table
          columns={columns}
          dataSource={data}
          rowKey="id"
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      </Card>

      {/* Detail Modal */}
      <Modal
        title="发票申请详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={800}
      >
        {current && (
          <>
            <Descriptions column={2} bordered size="small" style={{ marginBottom: 16 }}>
              <Descriptions.Item label="申请时间">{current.created_at ? new Date(current.created_at).toLocaleString() : '-'}</Descriptions.Item>
              <Descriptions.Item label="顾客">{current.customer_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="订单数">{current.order_count}</Descriptions.Item>
              <Descriptions.Item label="开票金额"><span style={{ fontWeight: 700, color: '#D97706' }}>{formatCents(current.total_amount)}</span></Descriptions.Item>
              <Descriptions.Item label="状态"><Tag color={statusConfig[current.status]?.color}>{statusConfig[current.status]?.text}</Tag></Descriptions.Item>
              <Descriptions.Item label="回复时间">{current.replied_at ? new Date(current.replied_at).toLocaleString() : '-'}</Descriptions.Item>
              {current.reply && (
                <Descriptions.Item label="回复内容" span={2}>{current.reply}</Descriptions.Item>
              )}
              {current.invoice_file && (
                <Descriptions.Item label="发票文件" span={2}>
                  <a href={current.invoice_file} target="_blank" rel="noopener noreferrer">{current.invoice_file}</a>
                </Descriptions.Item>
              )}
            </Descriptions>
            <Table
              columns={orderColumns}
              dataSource={current.orders || []}
              rowKey="order_id"
              size="small"
              pagination={false}
            />
          </>
        )}
      </Modal>

      {/* Reply Modal */}
      <Modal
        title="开票回复"
        open={replyVisible}
        onCancel={() => setReplyVisible(false)}
        onOk={handleReply}
        confirmLoading={submitting}
        okText="提交回复"
      >
        {current && (
          <div>
            <p>顾客：{current.customer_name} · 订单数：{current.order_count} · 金额：{formatCents(current.total_amount)}</p>
            <div style={{ marginTop: 16 }}>
              <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>回复内容</label>
              <Input.TextArea
                value={replyText}
                onChange={e => setReplyText(e.target.value)}
                rows={3}
                placeholder="请输入回复内容"
              />
            </div>
            <div style={{ marginTop: 16 }}>
              <label style={{ display: 'block', marginBottom: 8, fontWeight: 500 }}>上传发票文件</label>
              <Upload beforeUpload={handleUpload} showUploadList={false}>
                <Button icon={<UploadOutlined />}>选择文件</Button>
              </Upload>
              {replyFileUrl && (
                <p style={{ marginTop: 8, color: '#16a34a', fontSize: 13 }}>✓ 文件已上传</p>
              )}
            </div>
          </div>
        )}
      </Modal>
    </div>
  )
}
