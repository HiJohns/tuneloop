import { useState, useEffect } from 'react';
import { Card, Table, Button, Space, Modal, Form, Input, DatePicker, Switch, InputNumber, message, Select, Tag } from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined, SettingOutlined } from '@ant-design/icons';
import { api } from '../../services/api';
import dayjs from 'dayjs';

export default function PromoPlanManagePage({ scope }) {
  const isMerchant = scope === 'merchant';
  const prefix = isMerchant ? '/merchant' : '/admin';
  const [plans, setPlans] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [detailVisible, setDetailVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [currentPlan, setCurrentPlan] = useState(null);
  const [details, setDetails] = useState([]);
  const [form] = Form.useForm();

  const fetchPlans = async () => {
    setLoading(true);
    try {
      const res = await api.get(`${prefix}/promo-plans`);
      if (res.code === 20000) setPlans(res.data || []);
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchPlans(); }, []);

  const handleSave = async () => {
    const values = await form.validateFields();
    const body = {
      ...values,
      plan_type: 'discount_policy',
      scope_type: isMerchant ? 'merchant' : 'system',
      start_date: values.start_date ? values.start_date.format('YYYY-MM-DD') : null,
      end_date: values.end_date ? values.end_date.format('YYYY-MM-DD') : null,
    };
    if (values.is_long_term) {
      body.end_date = null;
      body.start_date = null;
    }
    delete body.is_long_term;
    const res = editing ? await api.put(`${prefix}/promo-plans/${editing.id}`, body) : await api.post(`${prefix}/promo-plans`, body);
    if (res.code === 20000) { message.success(editing ? '已更新' : '已创建'); setModalVisible(false); setEditing(null); form.resetFields(); fetchPlans(); }
    else { message.error(res.message); }
  };

  const handleDelete = async (id) => {
    Modal.confirm({
      title: '确认删除',
      onOk: async () => {
        const res = await api.delete(`${prefix}/promo-plans/${id}`);
        if (res.code === 20000) { message.success('已删除'); fetchPlans(); }
        else { message.error(res.message); }
      },
    });
  };

  const handleEditDetails = async (plan) => {
    setCurrentPlan(plan);
    const res = await api.get(`${prefix}/promo-plans/${plan.id}/details`);
    if (res.code === 20000) {
      setDetails(res.data.length > 0 ? res.data : [{ level_id: 1, rent_discount: null, deposit_discount: null, overdue_discount: null }]);
    } else {
      setDetails([{ level_id: 1, rent_discount: null, deposit_discount: null, overdue_discount: null }]);
    }
    setDetailVisible(true);
  };

  const handleSaveDetails = async () => {
    const res = await api.put(`${prefix}/promo-plans/${currentPlan.id}/details`, { details });
    if (res.code === 20000) { message.success('已更新'); setDetailVisible(false); }
    else { message.error(res.message); }
  };

  const updateDetail = (index, field, value) => {
    const newDetails = [...details];
    newDetails[index] = { ...newDetails[index], [field]: value };
    setDetails(newDetails);
  };

  const columns = [
    { title: '名称', dataIndex: 'name' },
    { title: '类型', dataIndex: 'plan_type', render: v => <Tag>{v === 'discount_policy' ? '折扣政策' : '促销活动'}</Tag> },
    { title: '范围', dataIndex: 'scope_type', render: v => ({ system: '全站', merchant: '本商户' })[v] || v },
    { title: '起止日期', render: (_, r) => r.start_date ? `${r.start_date} ~ ${r.end_date || '长期'}` : '长期有效' },
    { title: '启用', dataIndex: 'is_active', render: v => v ? <Tag color="green">是</Tag> : <Tag color="red">否</Tag> },
    {
      title: '操作', width: 200,
      render: (_, r) => (
        <Space>
          <Button size="small" icon={<SettingOutlined />} onClick={() => handleEditDetails(r)}>折扣设置</Button>
          <Button size="small" icon={<EditOutlined />} onClick={() => { setEditing(r); form.setFieldsValue({ ...r, is_long_term: !r.end_date, start_date: r.start_date ? dayjs(r.start_date) : null, end_date: r.end_date ? dayjs(r.end_date) : null }); setModalVisible(true); }}>编辑</Button>
          <Button size="small" danger icon={<DeleteOutlined />} onClick={() => handleDelete(r.id)}>删除</Button>
        </Space>
      ),
    },
  ];

  return (
    <Card title={isMerchant ? '商户折扣政策' : '系统折扣政策'}>
      <div className="mb-4">
        <Button type="primary" icon={<PlusOutlined />} onClick={() => { setEditing(null); form.resetFields(); form.setFieldsValue({ is_active: true, is_long_term: true }); setModalVisible(true); }}>新增方案</Button>
      </div>
      <Table dataSource={plans} columns={columns} rowKey="id" loading={loading} pagination={false} />

      <Modal title={editing ? '编辑方案' : '新增方案'} open={modalVisible} onOk={handleSave} onCancel={() => { setModalVisible(false); setEditing(null); form.resetFields(); }} width={600} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="name" label="方案名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="is_long_term" label="有效期" valuePropName="checked">
            <Switch checkedChildren="长期有效" unCheckedChildren="限时" />
          </Form.Item>
          <Form.Item noStyle shouldUpdate={(prev, cur) => prev.is_long_term !== cur.is_long_term}>
            {({ getFieldValue }) => !getFieldValue('is_long_term') ? (
              <Space direction="horizontal" className="w-full">
                <Form.Item name="start_date" label="开始日期" rules={[{ required: true }]}><DatePicker /></Form.Item>
                <Form.Item name="end_date" label="结束日期" rules={[{ required: true }]}><DatePicker /></Form.Item>
              </Space>
            ) : null}
          </Form.Item>
          <Form.Item name="is_active" label="启用" valuePropName="checked"><Switch /></Form.Item>
        </Form>
      </Modal>

      <Modal title={`折扣设置 - ${currentPlan?.name || ''}`} open={detailVisible} onOk={handleSaveDetails} onCancel={() => setDetailVisible(false)} width={600} destroyOnClose>
        <Table dataSource={details} rowKey="level_id" pagination={false}
          columns={[
            { title: '会员级别', dataIndex: 'level_id', render: v => ['', '初级', '中级', '高级'][v] || v },
            {
              title: '租金折扣', dataIndex: 'rent_discount', render: (v, _, i) => (
                <InputNumber min={0} max={1} step={0.05} value={v} onChange={val => updateDetail(i, 'rent_discount', val)} style={{ width: 100 }} />
              ),
            },
            {
              title: '押金折扣', dataIndex: 'deposit_discount', render: (v, _, i) => (
                <InputNumber min={0} max={1} step={0.05} value={v} onChange={val => updateDetail(i, 'deposit_discount', val)} style={{ width: 100 }} />
              ),
            },
          ]}
        />
      </Modal>
    </Card>
  );
}
