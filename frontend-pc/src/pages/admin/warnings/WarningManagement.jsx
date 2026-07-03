import { useState, useEffect } from 'react'
import { Card, Table, Tag, Select, Button, message, Modal, Input } from 'antd'
import { api } from '../../../services/api'

const severityColors = { low: 'green', medium: 'orange', high: 'red' }
const statusColors = { open: 'red', acknowledged: 'blue', resolved: 'gray' }

export default function WarningManagement() {
  const [warnings, setWarnings] = useState([])
  const [loading, setLoading] = useState(true)
  const [filter, setFilter] = useState({ status: '', level: '' })
  const [resolveModal, setResolveModal] = useState(null)
  const [resolveNote, setResolveNote] = useState('')

  const fetchWarnings = () => {
    setLoading(true)
    const params = new URLSearchParams()
    if (filter.status) params.set('status', filter.status)
    if (filter.level) params.set('level', filter.level)
    api.get(`/warnings?${params}`).then(r => {
      if (r.code === 20000) setWarnings(r.data?.list || [])
    }).catch(() => {}).finally(() => setLoading(false))
  }

  useEffect(() => { fetchWarnings() }, [filter])

  const handleAcknowledge = async (id) => {
    const r = await api.put(`/warnings/${id}/status`, { status: 'acknowledged' })
    if (r.code === 20000) { message.success('已标记已读'); fetchWarnings() }
  }

  const handleResolve = async () => {
    if (!resolveModal) return
    const r = await api.put(`/warnings/${resolveModal}/status`, { status: 'resolved' })
    if (r.code === 20000) { message.success('已解除'); setResolveModal(null); fetchWarnings() }
  }

  const columns = [
    { title: '级别', dataIndex: 'level', width: 80, render: l => <Tag color={severityColors[l]}>{l}</Tag> },
    { title: '事由', dataIndex: 'reason', width: 120 },
    { title: '描述', dataIndex: 'description', ellipsis: true },
    { title: '状态', dataIndex: 'status', width: 100, render: s => <Tag color={statusColors[s]}>{s}</Tag> },
    { title: '时间', dataIndex: 'created_at', width: 160, render: v => v ? new Date(v).toLocaleString() : '-' },
    { title: '操作', width: 200, render: (_, r) => (
      <span>
        {r.status === 'open' && <Button size="small" onClick={() => handleAcknowledge(r.id)} style={{ marginRight: 8 }}>标记已读</Button>}
        {r.status !== 'resolved' && <Button size="small" onClick={() => setResolveModal(r.id)}>解除</Button>}
      </span>
    )},
  ]

  return (
    <Card title="警告管理" extra={
      <span>
        <Select value={filter.status} onChange={v => setFilter(p => ({ ...p, status: v }))} allowClear placeholder="全部状态" style={{ width: 120, marginRight: 8 }}>
          <Select.Option value="open">未处理</Select.Option>
          <Select.Option value="acknowledged">已读</Select.Option>
          <Select.Option value="resolved">已解除</Select.Option>
        </Select>
        <Select value={filter.level} onChange={v => setFilter(p => ({ ...p, level: v }))} allowClear placeholder="全部级别" style={{ width: 120 }}>
          <Select.Option value="low">低</Select.Option>
          <Select.Option value="medium">中</Select.Option>
          <Select.Option value="high">高</Select.Option>
        </Select>
      </span>
    }>
      <Table rowKey="id" dataSource={warnings} columns={columns} loading={loading} />
      <Modal title="解除警告" open={!!resolveModal} onOk={handleResolve} onCancel={() => setResolveModal(null)}>
        <Input.TextArea value={resolveNote} onChange={e => setResolveNote(e.target.value)} placeholder="备注（可选）" rows={3} />
      </Modal>
    </Card>
  )
}
