import { useCallback, useEffect, useState } from 'react'
import { Card, Table, Button, Space, message, Modal, Input, Form, Alert, Typography } from 'antd'
import { PlusOutlined, UserAddOutlined, ReloadOutlined } from '@ant-design/icons'
import { useNavigate } from 'react-router-dom'
import { invitesApi, personnelApi } from '../services/api'
import { formatBeijingDate } from '../utils/date'

// #2114 直属成员管理（子页）：
// - 商户管理员：管理/邀请本商户直属员工（merchant_staff）
// - 平台管理员：邀请平台直属员工（platform_staff）
const ROLE_LABEL = {
  merchant_admin: '商户管理员',
  site_admin: '网点管理员',
  site_member: '商户直属员工',
  repair_technician: '维修师傅',
  platform_staff: '平台员工',
}

export default function DirectMemberManagement() {
  const navigate = useNavigate()
  const businessRole = localStorage.getItem('user_business_role') || ''
  const isMerchantAdmin = businessRole === 'merchant_admin'
  const isSystemAdmin = businessRole === 'system_admin'

  const [merchant, setMerchant] = useState(null)
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  const [inviteOpen, setInviteOpen] = useState(false)
  const [inviteKind, setInviteKind] = useState('merchant_staff')
  const [inviteForm] = Form.useForm()
  const [inviting, setInviting] = useState(false)

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await personnelApi.list({ direct: 'true', page: 1, page_size: 100 })
      if (resp.code === 20000) setList(resp.data?.list || [])
    } catch (err) {
      message.error('加载直属员工失败: ' + (err.message || ''))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    fetchList()
    if (isMerchantAdmin) {
      invitesApi.myMerchant()
        .then((resp) => { if (resp.code === 20000) setMerchant(resp.data) })
        .catch(() => {})
    }
  }, [fetchList, isMerchantAdmin])

  const openInvite = (kind) => {
    setInviteKind(kind)
    inviteForm.resetFields()
    setInviteOpen(true)
  }

  const handleInvite = async () => {
    try {
      const values = await inviteForm.validateFields()
      setInviting(true)
      let resp
      if (inviteKind === 'platform_staff') {
        resp = await invitesApi.invitePlatformStaff({ identifier: values.identifier.trim() })
      } else {
        if (!merchant?.id) {
          message.error('未获取到商户上下文，无法邀请')
          setInviting(false)
          return
        }
        resp = await invitesApi.inviteToMerchant(merchant.id, {
          identifier: values.identifier.trim(),
          kind: 'merchant_staff',
        })
      }
      if (resp.code === 20100) {
        message.success('邀请已发出，待对方接受')
        setInviteOpen(false)
      } else {
        message.error(resp.message || '邀请失败')
      }
    } catch (err) {
      if (err?.errorFields) return
      message.error('邀请失败: ' + (err.message || ''))
    } finally {
      setInviting(false)
    }
  }

  const columns = [
    { title: '姓名', dataIndex: 'name', key: 'name', width: 140 },
    { title: '邮箱', dataIndex: 'email', key: 'email', ellipsis: true },
    { title: '手机号', dataIndex: 'phone', key: 'phone', width: 140 },
    {
      title: '职位',
      dataIndex: 'position',
      key: 'position',
      width: 160,
      render: (v, record) => v || ROLE_LABEL[record.role] || record.role || '-',
    },
    {
      title: '加入时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 160,
      render: (v) => (v ? formatBeijingDate(v) : '-'),
    },
  ]

  return (
    <div className="p-6">
      <Card
        title={isSystemAdmin ? '直属成员管理 · 平台' : '直属成员管理 · 商户直属'}
        extra={
          <Space>
            <Button icon={<ReloadOutlined />} onClick={fetchList}>刷新</Button>
            {isMerchantAdmin && (
              <Button
                icon={<UserAddOutlined />}
                disabled={!merchant?.id}
                onClick={() => openInvite('merchant_staff')}
              >
                邀请直属员工
              </Button>
            )}
            {isSystemAdmin && (
              <Button icon={<UserAddOutlined />} onClick={() => openInvite('platform_staff')}>
                邀请平台员工
              </Button>
            )}
            <Button type="primary" icon={<PlusOutlined />} onClick={() => navigate('/staff')}>
              创建员工
            </Button>
          </Space>
        }
      >
        {isMerchantAdmin && (
          <Alert
            type="info"
            showIcon
            style={{ marginBottom: 16 }}
            message={`商户：${merchant?.name || '（加载中）'}`}
            description="直属员工直接归属商户（不属于任何网点）。邀请需对方在「我的 → 系统消息」接受后生效。"
          />
        )}
        <Table
          columns={columns}
          dataSource={list}
          loading={loading}
          rowKey="id"
          pagination={{ pageSize: 10, showTotal: (t) => `共 ${t} 条` }}
          locale={{ emptyText: '暂无直属员工' }}
        />
      </Card>

      <Modal
        title={inviteKind === 'platform_staff' ? '邀请平台员工' : '邀请商户直属员工'}
        open={inviteOpen}
        onCancel={() => setInviteOpen(false)}
        onOk={handleInvite}
        okText="发出邀请"
        okButtonProps={{ loading: inviting }}
        cancelText="取消"
        destroyOnClose
      >
        <Alert
          type="info"
          showIcon
          style={{ marginBottom: 16 }}
          message="被邀请人需已在本平台注册"
          description={
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              输入对方的手机号或邮箱；若尚未注册，请先让本人注册（或使用「创建员工」）。
            </Typography.Text>
          }
        />
        <Form form={inviteForm} layout="vertical">
          <Form.Item
            name="identifier"
            label="手机号 / 邮箱"
            rules={[{ required: true, message: '请输入被邀请人的手机号或邮箱' }]}
          >
            <Input placeholder="手机号或邮箱" autoComplete="off" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}
