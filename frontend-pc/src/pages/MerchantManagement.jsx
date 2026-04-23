import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Table, Button, Modal, Form, Input, message, Card, Space, Popconfirm } from 'antd';
import api from '../services/api';

const MerchantManagement = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingMerchant, setEditingMerchant] = useState(null);

  useEffect(() => {
    fetchMerchants();
  }, []);

  const fetchMerchants = async () => {
    setLoading(true);
    try {
      const response = await api.get('/api/merchants');
      setMerchants(response.data.list || []);
    } catch (error) {
      message.error('Failed to fetch merchants');
      console.error('Fetch error:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = () => {
    setEditingMerchant(null);
    form.resetFields();
    setModalOpen(true);
  };

  const handleEdit = (record) => {
    setEditingMerchant(record);
    form.setFieldsValue(record);
    setModalOpen(true);
  };

  const handleDelete = async (id) => {
    try {
      await api.delete(`/api/merchants/${id}`);
      message.success('Merchant deleted successfully');
      fetchMerchants();
    } catch (error) {
      message.error(error.response?.data?.message || 'Failed to delete merchant');
    }
  };

  const handleSubmit = async (values) => {
    try {
      if (editingMerchant) {
        await api.put(`/api/merchants/${editingMerchant.id}`, values);
        message.success('Merchant updated successfully');
      } else {
        await api.post('/api/merchants', values);
        message.success('Merchant created successfully');
      }
      setModalOpen(false);
      fetchMerchants();
    } catch (error) {
      message.error(error.response?.data?.message || 'Operation failed');
    }
  };

  const columns = [
    {
      title: 'Name',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: 'Code',
      dataIndex: 'code',
      key: 'code',
    },
    {
      title: 'Contact',
      dataIndex: 'contact_name',
      key: 'contact_name',
    },
    {
      title: 'Contact Email',
      dataIndex: 'contact_email',
      key: 'contact_email',
    },
    {
      title: 'Status',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <span style={{ color: status === 'active' ? 'green' : 'red' }}>
          {status?.toUpperCase()}
        </span>
      ),
    },
    {
      title: 'Created At',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => new Date(date).toLocaleDateString(),
    },
    {
      title: 'Actions',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button type="link" onClick={() => handleEdit(record)}>
            Edit
          </Button>
          <Popconfirm
            title="Are you sure to delete this merchant?"
            onConfirm={() => handleDelete(record.id)}
            okText="Yes"
            cancelText="No"
          >
            <Button type="link" danger>
              Delete
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="Merchant Management"
        extra={
          <Button type="primary" onClick={handleCreate}>
            Create Merchant
          </Button>
        }
      >
        <Table
          columns={columns}
          dataSource={merchants}
          loading={loading}
          rowKey="id"
          pagination={{ defaultPageSize: 20 }}
        />
      </Card>

      <Modal
        title={editingMerchant ? 'Edit Merchant' : 'Create Merchant'}
        open={modalOpen}
        onOk={() => form.submit()}
        onCancel={() => setModalOpen(false)}
        width={600}
      >
        <Form form={form} onFinish={handleSubmit} layout="vertical">
          <Form.Item
            name="name"
            label="Merchant Name"
            rules={[{ required: true, message: 'Please enter merchant name' }]}
          >
            <Input placeholder="Enter merchant name" />
          </Form.Item>

          <Form.Item
            name="code"
            label="Merchant Code"
            rules={[
              { required: true, message: 'Please enter merchant code' },
              { pattern: /^[a-z0-9-]+$/, message: 'Only lowercase letters, numbers, and hyphens allowed' },
            ]}
          >
            <Input placeholder="enter-code-here" disabled={!!editingMerchant} />
          </Form.Item>

          <Form.Item name="contact_name" label="Contact Name">
            <Input placeholder="Enter contact name" />
          </Form.Item>

          <Form.Item
            name="contact_email"
            label="Contact Email"
            rules={[{ type: 'email', message: 'Please enter valid email' }]}
          >
            <Input placeholder="contact@example.com" />
          </Form.Item>

          <Form.Item name="contact_phone" label="Contact Phone">
            <Input placeholder="13800000000" />
          </Form.Item>

          <Form.Item
            name="admin_uid"
            label="Admin User ID"
            rules={[{ required: true, message: 'Please select admin user' }]}
          >
            <Input placeholder="User UUID" disabled={!!editingMerchant} />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default MerchantManagement;
