import { useCallback, useEffect, useState } from 'react'
import { Card, Table, Button, Space, Tag, message, Modal, Input, Select, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { invitesApi } from '../services/api'
import { formatBeijingDate } from '../utils/date'

// #2114 邀请管理（子页）：网点管理员的加入申请 → 商户管理员审批。
const STATUS_META = {
  pending_approval: { color: 'orange', text: '待审批' },
  pending: { color: 'blue', text: '等待中' },
  accepted: { color: 'green', text: '已接受' },
  rejected: { color: 'red', text: '已拒绝' },
  expired: { color: 'default', text: '已过期' },
  cancelled: { color: 'default', text: '已取消' },
}

const KIND_LABEL = {
  site_member: '网点成员',
  merchant_member: '商户成员',
  merchant_staff: '商户直属员工',
  platform_staff: '平台员工',
}

const ROLE_LABEL = {
  site_admin: '网点管理员',
  site_member: '网点员工',
  member: '商户成员',
  merchant_staff: '商户直属员工',
  repair_technician: '维修师傅',
}

export default function InviteManagement() {
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  const [status, setStatus] = useState(undefined)
  const [rejectTarget, setRejectTarget] = useState(null)
  const [rejectReason, setRejectReason] = useState('')
  const [submitting, setSubmitting] = useState(false)

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await invitesApi.list(status ? { status } : {})
      if (resp.code === 20000) {
        setList(resp.data?.invites || [])
      } else {
        message.error(resp.message || '加载邀请列表失败')
      }
    } catch (err) {
      message.error('加载邀请列表失败: ' + (err.message || ''))
    } finally {
      setLoading(false)
    }
  }, [status])

  useEffect(() => {
    fetchList()
  }, [fetchList])

  const handleApprove = (record) => {
    Modal.confirm({
      title: '同意该加入申请？',
      content: `将向 ${record.invitee_name || record.invitee_identifier} 发出加入邀请，待其本人接受后生效。`,
      okText: '同意',
      cancelText: '取消',
      onOk: async () => {
        try {
          const resp = await invitesApi.approve(record.id)
          if (resp.code === 20000) {
            message.success('已同意，邀请已发出')
            fetchList()
          } else {
            message.error(resp.message || '操作失败')
          }
        } catch (err) {
          message.error('操作失败: ' + (err.message || ''))
        }
      },
    })
  }

  const openReject = (record) => {
    setRejectTarget(record)
    setRejectReason('')
  }

  const handleReject = async () => {
    setSubmitting(true)
    try {
      const resp = await invitesApi.reject(rejectTarget.id, rejectReason)
      if (resp.code === 20000) {
        message.success('已拒绝该申请')
        setRejectTarget(null)
        fetchList()
      } else {
        message.error(resp.message || '操作失败')
      }
    } catch (err) {
      message.error('操作失败: ' + (err.message || ''))
    } finally {
      setSubmitting(false)
    }
  }

  const columns = [
    {
      title: '提交网点',
      dataIndex: 'site_name',
      key: 'site_name',
      width: 160,
      render: (v, record) => v || (record.kind === 'platform_staff' ? '平台' : '商户直属'),
    },
    {
      title: '对象',
      key: 'invitee',
      render: (_, record) => (
        <div>
          <div>{record.invitee_name || '-'}</div>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>{record.invitee_identifier || '-'}</Typography.Text>
        </div>
      ),
    },
    {
      title: '类型 / 职位',
      key: 'kind',
      width: 160,
      render: (_, record) => (
        <Space direction="vertical" size={0}>
          <span>{KIND_LABEL[record.kind] || record.kind}</span>
          <Typography.Text type="secondary" style={{ fontSize: 12 }}>
            {ROLE_LABEL[record.role] || record.role}
          </Typography.Text>
        </Space>
      ),
    },
    {
      title: '时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (v) => formatBeijingDate(v),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (v) => {
        const meta = STATUS_META[v] || { color: 'default', text: v }
        return <Tag color={meta.color}>{meta.text}</Tag>
      },
    },
    {
      title: '操作',
      key: 'actions',
      width: 160,
      render: (_, record) => {
        if (record.status !== 'pending_approval') {
          return record.status === 'rejected' && record.reject_reason
            ? <Typography.Text type="secondary" style={{ fontSize: 12 }}>{record.reject_reason}</Typography.Text>
            : <span style={{ color: '#bbb' }}>—</span>
        }
        return (
          <Space>
            <Button type="link" size="small" onClick={() => handleApprove(record)}>同意</Button>
            <Button type="link" size="small" danger onClick={() => openReject(record)}>拒绝</Button>
          </Space>
        )
      },
    },
  ]

  return (
    <div className="p-6">
      <Card
        title="邀请管理"
        extra={
          <Space>
            <Select
              placeholder="全部状态"
              allowClear
              style={{ width: 140 }}
              value={status}
              onChange={setStatus}
              options={[
                { value: 'pending_approval', label: '待审批' },
                { value: 'pending', label: '等待中' },
                { value: 'accepted', label: '已接受' },
                { value: 'rejected', label: '已拒绝' },
              ]}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchList}>刷新</Button>
          </Space>
        }
      >
        <Table
          columns={columns}
          dataSource={list}
          loading={loading}
          rowKey="id"
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
          locale={{ emptyText: '暂无邀请记录' }}
        />
      </Card>

      <Modal
        title="拒绝加入申请"
        open={!!rejectTarget}
        onCancel={() => setRejectTarget(null)}
        onOk={handleReject}
        okText="确认拒绝"
        okButtonProps={{ danger: true, loading: submitting }}
        cancelText="取消"
      >
        <p style={{ color: '#666' }}>
          拒绝 {rejectTarget?.invitee_name || rejectTarget?.invitee_identifier} 的加入申请。
        </p>
        <Input.TextArea
          rows={3}
          placeholder="拒绝原因（选填，将通知申请人）"
          value={rejectReason}
          onChange={(e) => setRejectReason(e.target.value)}
        />
      </Modal>
    </div>
  )
}
