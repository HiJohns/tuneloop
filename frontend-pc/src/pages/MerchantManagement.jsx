import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Table, Button, Modal, Form, Input, message, Card, Space, Popconfirm } from 'antd';
import { UserOutlined } from '@ant-design/icons';
import api from '../services/api';
import InlineUserSelector from '../components/InlineUserSelector';

const MerchantManagement = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingMerchant, setEditingMerchant] = useState(null);
  const [adminInfo, setAdminInfo] = useState({ name: '', id: null, email: '' });

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
    setAdminInfo({ name: '', id: null, email: '' });
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
      const isNewUser = adminInfo.isNew || false
      const submitData = { ...values }
      
      // If new user, pass admin info instead of admin_uid
      if (isNewUser && adminInfo.name && adminInfo.email) {
        submitData.admin_name = adminInfo.name
        submitData.admin_email = adminInfo.email
        submitData.admin_phone = adminInfo.phone || ''
      } else {
        submitData.admin_uid = adminInfo.id || values.admin_uid
      }

      if (editingMerchant) {
        await api.put(`/api/merchants/${editingMerchant.id}`, submitData);
        message.success('Merchant updated successfully');
      } else {
        await api.post('/api/merchants', submitData);
        message.success('Merchant created successfully');
      }
      setModalOpen(false);
      fetchMerchants();
    } catch (error) {
      message.error(error.response?.data?.message || 'Operation failed');
    }
  };

  const handleAdminChange = (users) => {
    if (!users || users.length === 0) {
      setAdminInfo({ name: '', id: null, email: '' });
      form.setFieldsValue({ admin_uid: null });
      return;
    }
    const user = users[0];
    setAdminInfo({
      name: user.name || user.user_name,
      id: user.user_id || user.id,
      email: user.email || '',
      isNew: user.isNew || false,
    });
    form.setFieldsValue({ admin_uid: user.user_id || user.id });
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
            label="管理员"
            rules={[{ required: !editingMerchant, message: '请选择管理员' }]}
          >
            {adminInfo.id ? (
              <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                <UserOutlined style={{ fontSize: 18, color: '#52c41a' }} />
                <span style={{ fontWeight: 500 }}>{adminInfo.name}</span>
                {adminInfo.email && <span style={{ color: '#999' }}>({adminInfo.email})</span>}
                {!editingMerchant && (
                  <Button
                    type="link"
                    onClick={() => {
                      setAdminInfo({ name: '', id: null, email: '' });
                      form.setFieldsValue({ admin_uid: null });
                    }}
                  >
                    更换
                  </Button>
                )}
              </div>
            ) : (
              <InlineUserSelector
                mode="single"
                merchantId="current-merchant-id"
                value={[]}
                onChange={handleAdminChange}
              />
            )}
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default MerchantManagement;
