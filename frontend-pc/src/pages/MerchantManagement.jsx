import { useEffect, useState } from 'react';
import { useNavigate, useParams, useLocation } from 'react-router-dom';
import { Card, Table, Button, Form, Input, Select, Switch, message, Space, Popconfirm, Tag, InputNumber, Tabs, Descriptions, Empty } from 'antd';
import { PlusOutlined, DeleteOutlined } from '@ant-design/icons';
import api from '../services/api';
import MerchantMemberManagement from '../components/MerchantMemberManagement';

const MerchantManagement = () => {
  const navigate = useNavigate();
  const { id } = useParams();
  const location = useLocation();
  const [merchants, setMerchants] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editingMerchant, setEditingMerchant] = useState(null);
  const [merchantType, setMerchantType] = useState('full');
  const [settlementMerchant, setSettlementMerchant] = useState(null);
  const [settlementForm] = Form.useForm();
  const [settlementLoading, setSettlementLoading] = useState(false);
  const [selectedMerchant, setSelectedMerchant] = useState(null);
  const [viewMode, setViewMode] = useState('detail'); // 'detail' | 'form'
  const [formMode, setFormMode] = useState('create'); // 'create' | 'edit'
  const [form] = Form.useForm();
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    fetchMerchants();
  }, []);

  // Sync URL → state (detail or create form)
  useEffect(() => {
    const params = new URLSearchParams(location.search);
    if (location.pathname === '/merchants/new') {
      setViewMode('form');
      setFormMode('create');
      setEditingMerchant(null);
      form.resetFields();
      setMerchantType('full');
    } else if (params.get('mode') === 'edit' && id) {
      setViewMode('form');
      setFormMode('edit');
      const found = merchants.find(m => m.id === id);
      if (found) {
        setEditingMerchant(found);
        form.setFieldsValue(found);
        setMerchantType(found.merchant_type || 'full');
      }
    } else if (id) {
      setViewMode('detail');
      setFormMode('create');
      const found = merchants.find(m => m.id === id);
      if (found) setSelectedMerchant(found);
    } else {
      setViewMode('detail');
      setSelectedMerchant(null);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [id, location.pathname, location.search]);

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
    navigate('/merchants/new');
  };

  const handleEdit = (record) => {
    setEditingMerchant(record);
    setViewMode('form');
    setFormMode('edit');
    form.setFieldsValue(record);
    setMerchantType(record.merchant_type || 'full');
    navigate('/merchants/' + record.id + '?mode=edit');
  };

  const handleBackToList = () => {
    setSelectedMerchant(null);
    navigate('/merchants');
  };

  const handleDelete = async (merchantId) => {
    try {
      await api.delete(`/merchants/${merchantId}`);
      message.success('商户删除成功');
      fetchMerchants();
      if (selectedMerchant?.id === merchantId) {
        setSelectedMerchant(null);
        navigate('/merchants');
      }
    } catch (error) {
      message.error(error.response?.data?.message || '删除商户失败');
    }
  };

  const openSettlement = async (record) => {
    setSettlementMerchant(record);
    setSettlementLoading(true);
    try {
      const resp = await api.get(`/admin/merchant/${record.id}/settlement`)
      if (resp.code === 20000 && resp.data) {
        settlementForm.setFieldsValue(resp.data)
      } else {
        settlementForm.resetFields()
      }
    } catch { settlementForm.resetFields() }
    setSettlementLoading(false)
  };

  const saveSettlement = async () => {
    try {
      const values = await settlementForm.validateFields()
      await api.put(`/admin/merchant/${settlementMerchant.id}/settlement`, values)
      message.success('分账配置保存成功')
    } catch (err) {
      if (err.errorFields) return // validation error
      message.error('保存失败: ' + (err.response?.data?.message || err.message))
    }
  };

  const handleSubmit = async (values) => {
    setSaving(true);
    try {
      if (formMode === 'edit' && editingMerchant) {
        await api.put(`/merchants/${editingMerchant.id}`, values);
        message.success('商户更新成功');
      } else {
        await api.post('/merchants', values);
        message.success('商户创建成功');
      }
      setViewMode('detail');
      fetchMerchants();
      navigate('/merchants');
    } catch (error) {
      message.error(error.response?.data?.message || '操作失败');
    } finally {
      setSaving(false);
    }
  };

  const columns = [
    {
      title: '名称',
      dataIndex: 'name',
      key: 'name',
      render: (name, record) => (
        <Button type="link" style={{ padding: 0 }} onClick={() => { setSelectedMerchant(record); navigate('/merchants/' + record.id); }}>
          {name}
        </Button>
      ),
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
      title: '类型',
      dataIndex: 'merchant_type',
      key: 'merchant_type',
      render: (type) => (
        <Tag color={type === 'controlled' ? 'orange' : 'blue'}>
          {type === 'controlled' ? '受控商户' : '全权商户'}
        </Tag>
      ),
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status, record) => (
        <Space size={4}>
          <span style={{ color: status === 'active' ? 'green' : 'red' }}>
            {status === 'active' ? '启用' : '停用'}
          </span>
          {record.admin_pending && <Tag color="orange">管理员待确认</Tag>}
        </Space>
      ),
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => new Date(date).toLocaleDateString(),
    },
  ];

  // Detail panel with tabs
  const renderDetail = () => {
    if (!selectedMerchant) {
      return <Empty description="请点击左侧商户查看详情" style={{ marginTop: 80 }} />;
    }
    const m = selectedMerchant;
    return (
      <>
        <Card className="mb-4">
          <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', flexWrap: 'wrap', gap: 12 }}>
            <div style={{ flex: 1, minWidth: 0 }}>
              <Space align="center" wrap>
                <h2 style={{ margin: 0, fontSize: 18, fontWeight: 600 }}>{m.name}</h2>
                <Tag color={m.merchant_type === 'controlled' ? 'orange' : 'blue'}>
                  {m.merchant_type === 'controlled' ? '受控商户' : '全权商户'}
                </Tag>
                <Tag color={m.status === 'active' ? 'green' : 'red'}>{m.status === 'active' ? '启用' : '停用'}</Tag>
              </Space>
            </div>
            <Space wrap>
              <Popconfirm
                title="确定要删除此商户吗？"
                onConfirm={() => handleDelete(m.id)}
                okText="确定"
                cancelText="取消"
              >
                <Button danger icon={<DeleteOutlined />}>删除</Button>
              </Popconfirm>
            </Space>
          </div>
        </Card>

        <Card>
          <Tabs defaultActiveKey="info" onChange={(key) => { if (key === 'settlement') openSettlement(m); }}>
            <Tabs.TabPane tab="编辑信息" key="info">
              <Descriptions column={2} bordered size="small">
                <Descriptions.Item label="商户ID">{m.id}</Descriptions.Item>
                <Descriptions.Item label="商户名">{m.name}</Descriptions.Item>
                <Descriptions.Item label="联系电话">{m.phone || '-'}</Descriptions.Item>
                <Descriptions.Item label="地址" span={2}>{m.address || '-'}</Descriptions.Item>
                <Descriptions.Item label="商户类型">{m.merchant_type === 'controlled' ? '受控商户' : '全权商户'}</Descriptions.Item>
                <Descriptions.Item label="参与返点">{m.rebate_opt_in ? '是' : '否'}</Descriptions.Item>
              </Descriptions>
              <div style={{ marginTop: 16 }}>
                <Button type="primary" onClick={() => handleEdit(m)}>编辑基本信息</Button>
              </div>
            </Tabs.TabPane>

            <Tabs.TabPane tab="分账配置" key="settlement">
              <Form
                form={settlementForm}
                layout="vertical"
                onFinish={saveSettlement}
              >
                <Form.Item name="receiver_type" label="接收方类型" rules={[{ required: true, message: '请选择类型' }]}>
                  <Select>
                    <Select.Option value="merchant">商户号</Select.Option>
                    <Select.Option value="personal_openid">个人 openid</Select.Option>
                  </Select>
                </Form.Item>
                <Form.Item name="receiver_account" label="接收方账号" rules={[{ required: true, message: '请输入账号' }]}>
                  <Input placeholder="商户号或个人 openid" />
                </Form.Item>
                <Form.Item name="profit_share_ratio" label="分账比例（%）" tooltip="平台分给商户的分成比例">
                  <InputNumber min={0} max={100} style={{ width: '100%' }} />
                </Form.Item>
                <Form.Item name="is_enabled" label="启用" valuePropName="checked" initialValue={true}>
                  <Switch />
                </Form.Item>
                <Button type="primary" htmlType="submit" loading={settlementLoading}>保存分账配置</Button>
              </Form>
            </Tabs.TabPane>

            <Tabs.TabPane tab="成员管理" key="members">
              <MerchantMemberManagement merchantId={m.id} onRefresh={() => {}} />
            </Tabs.TabPane>
          </Tabs>
        </Card>
      </>
    );
  };

  // Create/edit form view
  const renderForm = () => (
    <Card
      title={
        <Space>
          {formMode === 'create' ? '创建商户' : `编辑商户：${editingMerchant?.name || ''}`}
          {formMode === 'edit' && editingMerchant && (
            <>
              <Tag color={editingMerchant.merchant_type === 'controlled' ? 'orange' : 'blue'}>
                {editingMerchant.merchant_type === 'controlled' ? '受控商户' : '全权商户'}
              </Tag>
              <Tag color={editingMerchant.status === 'active' ? 'green' : 'red'}>
                {editingMerchant.status === 'active' ? '启用' : '停用'}
              </Tag>
            </>
          )}
        </Space>
      }
    >
      <Form form={form} onFinish={handleSubmit} layout="vertical" style={{ maxWidth: 600 }}>
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

        <Form.Item
          name="merchant_type"
          label="商户类型"
          initialValue="full"
        >
          <Select
            onChange={(value) => setMerchantType(value)}
            options={[
              { value: 'full', label: '全权商户' },
              { value: 'controlled', label: '受控商户' },
            ]}
          />
        </Form.Item>

        {merchantType === 'controlled' && (
          <>
            <Form.Item
              name="transit_address"
              label="中转地址"
              rules={[{ required: true, message: '受控商户必须填写中转地址' }]}
            >
              <Input placeholder="输入中转地址" />
            </Form.Item>

            <Form.Item
              name="transit_phone"
              label="中转电话"
              rules={[{ required: true, message: '受控商户必须填写中转电话' }]}
            >
              <Input placeholder="输入中转电话" />
            </Form.Item>

            <Form.Item name="transit_contact_name" label="中转联系人">
              <Input placeholder="输入中转联系人" />
            </Form.Item>
          </>
        )}

        <Form.Item name="rebate_opt_in" label="参与返点" valuePropName="checked" initialValue={true}>
          <Switch />
        </Form.Item>

        <Space>
          <Button type="primary" htmlType="submit" loading={saving}>
            {formMode === 'create' ? '创建商户' : '保存修改'}
          </Button>
          <Button onClick={() => { setViewMode('detail'); if (editingMerchant) navigate('/merchants/' + editingMerchant.id); else navigate('/merchants'); }}>取消</Button>
        </Space>
      </Form>
    </Card>
  );

  return (
    <div style={{ padding: 24 }}>
      <Card
        title="商户管理"
        extra={
          <Button type="primary" icon={<PlusOutlined />} onClick={handleCreate}>
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
          onRow={(record) => ({
            onClick: () => { setSelectedMerchant(record); navigate('/merchants/' + record.id); },
            style: { cursor: 'pointer' },
          })}
        />
      </Card>

      {viewMode === 'detail' && (
        <div style={{ marginTop: 24 }}>
          {renderDetail()}
        </div>
      )}
      {viewMode === 'form' && (
        <div style={{ marginTop: 24 }}>
          {renderForm()}
        </div>
      )}
    </div>
  );
};

export default MerchantManagement;
