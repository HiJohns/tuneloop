import { useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Table, Button, Modal, Form, Input, message, Card, Space, Popconfirm } from 'antd';
import api from '../services/api';
import ManagerSelector from '../components/ManagerSelector';

const MerchantManagement = () => {
  const navigate = useNavigate();
  const [form] = Form.useForm();
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalOpen, setModalOpen] = useState(false);
  const [editingMerchant, setEditingMerchant] = useState(null);
  const [adminInfo, setAdminInfo] = useState({ name: '', id: null, email: '', username: '' });
  const [conflictOptions, setConflictOptions] = useState(null);

  useEffect(() => {
    fetchMerchants();
  }, []);

  const fetchMerchants = async () => {
    setLoading(true);
    try {
      const response = await api.get('/merchants');
      setMerchants(response.data.list || []);
    } catch (error) {
      message.error('获取商户列表失败');
      console.error('Fetch error:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleCreate = () => {
    setEditingMerchant(null);
    form.resetFields();
    setAdminInfo({ name: '', id: null, email: '', username: '' });
    setConflictOptions(null);
    setModalOpen(true);
  };

  const handleEdit = (record) => {
    setEditingMerchant(record);
    form.setFieldsValue(record);
    setModalOpen(true);
  };

  const handleDelete = async (id) => {
    try {
      await api.delete(`/merchants/${id}`);
      message.success('商户删除成功');
      fetchMerchants();
    } catch (error) {
      message.error(error.response?.data?.message || '删除商户失败');
    }
  };

  const handleSubmit = async (values) => {
    try {
      const isNewUser = adminInfo.isNew || false
      if (!editingMerchant && !adminInfo.id && !isNewUser) {
        message.warning('请选择管理员或填写管理员信息')
        return
      }
      const submitData = { ...values }
      
      if (isNewUser && adminInfo.name && adminInfo.email) {
        // Scenario 2: new user — send admin_uid=null + admin fields
        submitData.admin_uid = null
        submitData.admin_name = adminInfo.name
        submitData.admin_username = adminInfo.username
        submitData.admin_email = adminInfo.email
        submitData.admin_phone = adminInfo.phone || ''
      } else {
        // Scenario 1: existing user — send admin_uid
        submitData.admin_uid = adminInfo.id || values.admin_uid
      }

      if (editingMerchant) {
        await api.put(`/merchants/${editingMerchant.id}`, submitData);
        message.success('商户更新成功');
      } else {
        await api.post('/merchants', submitData);
        message.success('商户创建成功');
      }
      setModalOpen(false);
      fetchMerchants();
    } catch (error) {
      const resp = error.response
      // 409 with conflict list → show all in search dropdown
      if (resp?.status === 409 && resp?.data?.data?.conflicts?.length > 0) {
        const conflicts = resp.data.data.conflicts
        setConflictOptions(conflicts)
        message.warning(`发现 ${conflicts.length} 个冲突账户，请在搜索中选择`)
        return
      }
      message.error(error.response?.data?.message || '操作失败');
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
    },
    {
      title: '联系电话',
      dataIndex: 'phone',
      key: 'phone',
    },
    {
      title: '地址',
      dataIndex: 'address',
      key: 'address',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <span style={{ color: status === 'active' ? 'green' : 'red' }}>
          {status === 'active' ? '启用' : '停用'}
        </span>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => new Date(date).toLocaleDateString(),
    },
    {
      title: '操作',
      key: 'actions',
      render: (_, record) => (
        <Space>
          <Button type="link" onClick={() => handleEdit(record)}>
            编辑
          </Button>
          <Popconfirm
            title="确定要删除此商户吗？"
            onConfirm={() => handleDelete(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="link" danger>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="商户管理"
        extra={
          <Button type="primary" onClick={handleCreate}>
            创建商户
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
        title={editingMerchant ? '编辑商户' : '创建商户'}
        open={modalOpen}
        onOk={() => form.submit()}
        onCancel={() => setModalOpen(false)}
        width={600}
      >
        <Form form={form} onFinish={handleSubmit} layout="vertical">
          <Form.Item
            name="name"
            label="商户名"
            rules={[{ required: true, message: '请输入商户名' }]}
          >
            <Input placeholder="输入商户名称" />
          </Form.Item>

          <Form.Item name="phone" label="联系电话">
            <Input placeholder="输入联系电话" />
          </Form.Item>

          <Form.Item name="address" label="地址">
            <Input placeholder="输入地址" />
          </Form.Item>

          <Form.Item label="管理员">
            <ManagerSelector
              value={adminInfo}
              onChange={(info) => {
                if (info.id || info.isNew) {
                  setAdminInfo({ name: info.name, id: info.id, email: info.email || '', username: info.username || '', isNew: info.isNew || false })
                  form.setFieldsValue({ admin_uid: info.id || undefined })
                } else {
                  setAdminInfo({ name: '', id: null, email: '', username: '' })
                  form.setFieldsValue({ admin_uid: null })
                }
                setConflictOptions(null)
              }}
              conflictOptions={conflictOptions}
              conflictMessage={conflictOptions ? `发现 ${conflictOptions.length} 个冲突账户，请在搜索中选择` : ''}
            />
          </Form.Item>
        </Form>
      </Modal>
    </div>
  );
};

export default MerchantManagement;
