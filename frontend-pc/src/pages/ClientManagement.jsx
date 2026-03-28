import { useState, useEffect } from 'react'
import { Table, Button, Modal, Form, Input, message, Tag, Space } from 'antd'
import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons'
import { api } from '../services/api'

const IAM_API_URL = import.meta.env.VITE_BEACONIAM_INTERNAL_URL || 'http://localhost:5552'

export default function ClientManagement() {
  const [clients, setClients] = useState([])
  const [loading, setLoading] = useState(false)
  const [modalVisible, setModalVisible] = useState(false)
  const [editingClient, setEditingClient] = useState(null)
  const [form] = Form.useForm()

  useEffect(() => {
    fetchClients()
  }, [])

  const fetchClients = async () => {
    try {
      setLoading(true)
      const response = await api.get('/system/clients')
      setClients(response.data || [])
      setLoading(false)
    } catch (error) {
      console.error('Failed to fetch clients:', error)
      message.error('加载客户端列表失败')
      setLoading(false)
    }
  }

  const handleAddClient = () => {
    setEditingClient(null)
    form.resetFields()
    setModalVisible(true)
  }

  const handleEditClient = (client) => {
    setEditingClient(client)
    form.setFieldsValue({
      name: client.name,
      client_id: client.client_id,
      redirect_uris: client.redirect_uris?.join(', ') || ''
    })
    setModalVisible(true)
  }

  const handleDeleteClient = async (clientId) => {
    try {
      await api.delete(`/system/clients/${clientId}`)
      message.success('删除成功')
      fetchClients()
    } catch (error) {
      console.error('Failed to delete client:', error)
      message.error('删除失败')
    }
  }

  const handleSubmit = async (values) => {
    try {
      if (editingClient) {
        await api.put(`/system/clients/${editingClient.id}`, values)
        message.success('更新成功')
      } else {
        await api.post('/system/clients', values)
        message.success('创建成功')
      }
      setModalVisible(false)
      fetchClients()
    } catch (error) {
      console.error('Failed to save client:', error)
      message.error(editingClient ? '更新失败' : '创建失败')
    }
  }

  const columns = [
    {
      title: '客户端ID',
      dataIndex: 'client_id',
      key: 'client_id',
    },
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '状态',
      key: 'status',
      render: (_, record) => (
        <Tag color="green">启用</Tag>
      ),
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space size="middle">
          <Button type="link" icon={<EditOutlined />} onClick={() => handleEditClient(record)}>
            编辑
          </Button>
          <Button type="link" danger icon={<DeleteOutlined />} onClick={() => handleDeleteClient(record.id)}>
            删除
          </Button>
        </Space>
      ),
    },
  ]

  return (
    <div className="p-6">
      <div className="flex justify-between items-center mb-6">
        <h1 className="text-2xl font-bold">客户端管理</h1>
        <Button type="primary" icon={<PlusOutlined />} onClick={handleAddClient}>
          添加客户端
        </Button>
      </div>

      <Table
        columns={columns}
        dataSource={clients || [] || []}
        rowKey="id"
        loading={loading}
        pagination={{ pageSize: 10 }}
      />

      <Modal
        title={editingClient ? '编辑客户端' : '添加客户端'}
        open={modalVisible}
        onCancel={() => setModalVisible(false)}
        onOk={() => form.submit()}
        width={600}
      >
        <Form form={form} layout="vertical" onFinish={handleSubmit}>
          <Form.Item
            name="name"
            label="客户端名称"
            rules={[{ required: true, message: '请输入客户端名称' }]}
          >
            <Input placeholder="请输入客户端名称" />
          </Form.Item>

          <Form.Item
            name="client_id"
            label="客户端ID"
            rules={[{ required: true, message: '请输入客户端ID' }]}
          >
            <Input placeholder="请输入客户端ID" disabled={!!editingClient} />
          </Form.Item>

          <Form.Item
            name="redirect_uris"
            label="重定向URI (用逗号分隔)"
            rules={[{ required: true, message: '请输入重定向URI' }]}
          >
            <Input.TextArea rows={3} placeholder="http://localhost:5554/callback,http://localhost:5553/callback" />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  )
}