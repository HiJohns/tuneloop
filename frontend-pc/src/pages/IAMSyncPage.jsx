import { useState } from 'react'
import { Card, Button, Table, Tag, Spin, Space, message, Typography } from 'antd'
import { SyncOutlined } from '@ant-design/icons'
import { iamApi } from '../services/api'

const resultColors = {
  added: 'green',
  existing: 'default',
  updated: 'blue',
  skipped: 'orange',
  error: 'red',
}

const resultLabels = {
  added: '新加',
  existing: '已存在',
  updated: '更新',
  skipped: '跳过',
  error: '错误',
}

export default function IAMSyncPage() {
  const [loading, setLoading] = useState(false)
  const [orgDetails, setOrgDetails] = useState([])
  const [orgSummary, setOrgSummary] = useState(null)
  const [userDetails, setUserDetails] = useState([])
  const [userSummary, setUserSummary] = useState(null)

  const handleSync = async () => {
    setLoading(true)
    setOrgDetails([])
    setOrgSummary(null)
    setUserDetails([])
    setUserSummary(null)

    try {
      const orgRes = await iamApi.syncOrganizations()
      if (orgRes.code === 20000) {
        setOrgDetails(orgRes.data?.details || [])
        setOrgSummary({ synced: orgRes.data?.synced || 0, skipped: orgRes.data?.skipped || 0, conflicts: orgRes.data?.conflicts || 0 })
      } else {
        message.error('组织同步失败: ' + (orgRes.message || '未知错误'))
      }

      const userRes = await iamApi.syncUsers()
      if (userRes.code === 20000) {
        setUserDetails(userRes.data?.details || [])
        setUserSummary({ synced: userRes.data?.synced || 0, skipped: userRes.data?.skipped || 0, conflicts: userRes.data?.conflicts || 0 })
      } else {
        message.error('用户同步失败: ' + (userRes.message || '未知错误'))
      }
    } catch (err) {
      message.error('同步失败: ' + (err.message || ''))
    }
    setLoading(false)
  }

  const orgColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 260 },
    { title: '名称', dataIndex: 'name', key: 'name' },
    { title: '上级 ID', dataIndex: 'parent_id', key: 'parent_id', ellipsis: true, width: 260 },
    { title: '类型', dataIndex: 'kind', key: 'kind', width: 100 },
    {
      title: '结果', dataIndex: 'result', key: 'result', width: 100,
      render: (r) => <Tag color={resultColors[r] || 'default'}>{resultLabels[r] || r}</Tag>,
    },
  ]

  const userColumns = [
    { title: 'ID', dataIndex: 'id', key: 'id', ellipsis: true, width: 260 },
    { title: '姓名', dataIndex: 'name', key: 'name' },
    { title: '邮箱', dataIndex: 'email', key: 'email' },
    { title: '组织', dataIndex: 'org_id', key: 'org_id', ellipsis: true, width: 260 },
    {
      title: '结果', dataIndex: 'result', key: 'result', width: 100,
      render: (r) => <Tag color={resultColors[r] || 'default'}>{resultLabels[r] || r}</Tag>,
    },
  ]

  return (
    <div style={{ padding: 24 }}>
      <Card title="与 IAM 同步" style={{ marginBottom: 24 }}>
        <Button type="primary" icon={<SyncOutlined />} onClick={handleSync} loading={loading} size="large">
          开始同步
        </Button>

        {orgSummary && userSummary && (
          <div style={{ marginTop: 16 }}>
            <Typography.Text>
              组织 — 共 {orgSummary.synced + orgSummary.skipped + orgSummary.conflicts} 条，
              新增/更新 {orgSummary.synced}，已存在 {orgSummary.skipped}，冲突 {orgSummary.conflicts}
            </Typography.Text>
            <br />
            <Typography.Text>
              用户 — 共 {userSummary.synced + userSummary.skipped + userSummary.conflicts} 条，
              新增/更新 {userSummary.synced}，已存在 {userSummary.skipped}，冲突 {userSummary.conflicts}
            </Typography.Text>
          </div>
        )}
      </Card>

      {loading && <Spin size="large" style={{ display: 'block', margin: '40px auto' }} />}

      {orgDetails.length > 0 && (
        <Card title={`组织同步结果（${orgDetails.length} 条）`} style={{ marginBottom: 24 }}>
          <Table dataSource={orgDetails} columns={orgColumns} rowKey="id" size="small" pagination={false} />
        </Card>
      )}

      {userDetails.length > 0 && (
        <Card title={`用户同步结果（${userDetails.length} 条）`}>
          <Table dataSource={userDetails} columns={userColumns} rowKey="id" size="small" pagination={false} />
        </Card>
      )}
    </div>
  )
}
