// 平台员工管理（#1795 T6 / #1897）
// 仅系统管理员可用；平台员工 = 顶级组织（当前管理员 oid）中 role=staff 的成员。
// 密码设置与其他成员管理一致：默认自动生成初始密码（仅展示一次），可手动设置。
import { useState, useCallback, useEffect } from 'react'
import { Table, Card, Button, Space, Tag, Modal, Form, Input, Radio, Checkbox, message, Popconfirm } from 'antd'
import { PlusOutlined, StopOutlined } from '@ant-design/icons'
import { platformStaffApi } from '../../../services/api'

export default function PlatformStaffPage() {
  const [list, setList] = useState([])
  const [loading, setLoading] = useState(false)
  const [createVisible, setCreateVisible] = useState(false)
  const [creating, setCreating] = useState(false)
  const [autoGenerate, setAutoGenerate] = useState(true)
  const [form] = Form.useForm()

  const fetchList = useCallback(async () => {
    setLoading(true)
    try {
      const resp = await platformStaffApi.list()
      if (resp.code === 20000) {
        setList(resp.data?.list || [])
      } else {
        message.error(resp.message || '加载平台员工失败')
      }
    } catch (error) {
      message.error(error.message || '加载平台员工失败')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => { fetchList() }, [fetchList])

  const handleCreate = async () => {
    const values = await form.validateFields()
    setCreating(true)
    try {
      const resp = await platformStaffApi.create(values)
      if (resp.code === 20000) {
        setCreateVisible(false)
        form.resetFields()
        setAutoGenerate(true)
        fetchList()
        const initialPwd = resp.data?.initial_password
        if (initialPwd) {
          Modal.info({
            title: '平台员工已创建',
            width: 480,
            content: (
              <div>
                <p>以下为该员工的初始密码，仅展示一次，请妥善保存并告知本人：</p>
                <div style={{
                  padding: '12px 16px', background: '#f5f5f5', borderRadius: 4,
                  fontFamily: 'monospace', fontSize: 18, textAlign: 'center',
                  margin: '12px 0', userSelect: 'all',
                }}>{initialPwd}</div>
              </div>
            ),
          })
        } else {
          message.success('平台员工已创建')
        }
      } else {
        message.error(resp.message || '创建失败')
      }
    } catch (error) {
      message.error(error.message || '创建失败')
    } finally {
      setCreating(false)
    }
  }

  const handleDisable = async (record) => {
    try {
      const resp = await platformStaffApi.disable(record.id)
      if (resp.code === 20000) {
        message.success('已禁用')
        fetchList()
      } else {
        message.error(resp.message || '禁用失败')
      }
    } catch (error) {
      message.error(error.message || '禁用失败')
    }
  }

  const columns = [
    { title: '姓名', dataIndex: 'name', key: 'name', render: (v) => v || '-' },
    { title: '用户名', dataIndex: 'username', key: 'username' },
    { title: '手机号', dataIndex: 'phone', key: 'phone', render: (v) => v || '-' },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (v) => <Tag color={v === 'disabled' ? 'red' : 'green'}>{v === 'disabled' ? '已禁用' : '启用'}</Tag>,
    },
    {
      title: '待审核数',
      dataIndex: 'pending_review_count',
      key: 'pending_count',
      render: (v) => (v > 0 ? <Tag color="orange">{v}</Tag> : <span style={{ color: '#9ca3af' }}>0</span>),
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space>
          {record.status !== 'disabled' && (
            <Popconfirm
              title="确定禁用该平台员工？"
              description="禁用后其用户/审核队列访问将失效（保留记录可追溯）"
              onConfirm={() => handleDisable(record)}
              okText="确定禁用"
              cancelText="取消"
            >
              <Button size="small" danger icon={<StopOutlined />}>禁用</Button>
            </Popconfirm>
          )}
        </Space>
      ),
    },
  ]

  return (
    <Card
      title="平台员工管理"
      extra={
        <Button type="primary" size="small" icon={<PlusOutlined />} onClick={() => setCreateVisible(true)}>
          新建平台员工
        </Button>
      }
    >
      <Table
        rowKey="id"
        columns={columns}
        dataSource={list}
        loading={loading}
        pagination={{ pageSize: 10 }}
        locale={{ emptyText: '暂无平台员工（顶级组织下尚无 staff 成员）' }}
      />
      <Modal
        title="新建平台员工"
        open={createVisible}
        onOk={handleCreate}
        onCancel={() => { setCreateVisible(false); setAutoGenerate(true) }}
        okText="创建"
        cancelText="取消"
        confirmLoading={creating}
      >
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="姓名" rules={[{ required: true, message: '请填写姓名' }]}>
            <Input placeholder="请输入姓名" />
          </Form.Item>
          <Form.Item name="username" label="用户名" rules={[{ required: true, message: '请填写用户名' }]}>
            <Input placeholder="登录用户名（唯一）" />
          </Form.Item>
          <Form.Item name="phone" label="手机号">
            <Input placeholder="选填" />
          </Form.Item>
          <Form.Item name="email" label="邮箱">
            <Input placeholder="选填" />
          </Form.Item>
          <Form.Item name="auto_generate" label="密码设置" initialValue={true}>
            <Radio.Group onChange={e => setAutoGenerate(e.target.value)}>
              <Radio value={true}>自动生成</Radio>
              <Radio value={false}>手动设置</Radio>
            </Radio.Group>
          </Form.Item>
          {!autoGenerate && (
            <Form.Item name="password" label="初始密码" rules={[{ required: true, min: 8, message: '至少 8 位' }]}>
              <Input.Password placeholder="8位+大写+小写+数字" />
            </Form.Item>
          )}
          <Form.Item name="force_password_change" valuePropName="checked" initialValue={true}>
            <Checkbox>首次登录时强制修改密码</Checkbox>
          </Form.Item>
        </Form>
      </Modal>
    </Card>
  )
}
