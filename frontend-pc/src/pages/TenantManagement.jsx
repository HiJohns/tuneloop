import { useState, useEffect } from 'react'
import { Table, Button, Modal, Form, Input, message, Tag, Space, Select } from 'antd'
import { PlusOutlined } from '@ant-design/icons'
import { api } from '../services/api'

const { Option } = Select

export default function TenantManagement() {
  const [tenants, setTenants] = useState([])
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchTenants()
  }, [])

  const fetchTenants = async () => {
    try {
      setLoading(true)
      const response = await api.get('/system/tenants')
      setTenants(response.data || [])
      setLoading(false)
    } catch (error) {
      console.error('Failed to fetch tenants:', error)
      message.error('加载租户列表失败')
      setLoading(false)
    }
  }

  const handleCreateTenant = () => {
    form.resetFields()
    setModalVisible(true)
  }

  const handleSubmit = async (values) => {
    try {
      await api.post('/system/tenants', values)
      message.success('租户创建成功')
      setModalVisible(false)
      fetchTenants()
    } catch (error) {
      console.error('Failed to create tenant:', error)
      message.error('创建租户失败')
    }
  }

  const columns = [
    {
      title: '租户名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '租户ID',
      dataIndex: 'tenant_id',
      key: 'tenant_id',
    },
    {
      title: '状态',
      key: 'status',
      render: (_, record) => (
        <Tag color="green">启用</Tag>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (text) => new Date(text).toLocaleDateString(),
    },
  ]

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">租户管理</h1>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleCreateTenant}>
          创建租户
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={tenants}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />

      <Modal
        title="创建租户"
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="name"
            label="租户名称"
            rules={[{ required: true, message: '请输入租户名称' }]}
          >
            <Input placeholder="请输入租户名称" />
          </Form.Item>

          <Form.Item
            name="owner_email"
            label="Owner邮箱"
            rules={[
              { required: true, message: '请输入Owner邮箱' },
              { type: 'email', message: '请输入有效的邮箱地址' }
            ]}
          >
            <Input placeholder="owner@example.com" />
          </Form.Item>

          <Form.Item
            name="owner_name"
            label="Owner姓名"
            rules={[{ required: true, message: '请输入Owner姓名' }]}
          >
            <Input placeholder="请输入Owner姓名" />
          </Form.Item>

          <Form.Item
            name="owner_password"
            label="Owner密码"
            rules={[{ required: true, message: '请输入Owner密码' }]}
          >
            <Input.Password placeholder="请输入密码" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}