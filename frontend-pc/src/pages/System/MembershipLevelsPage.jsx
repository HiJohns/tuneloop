import { useState, useEffect } from 'react';
import { Card, Table, Button, Space, Modal, Form, Input, InputNumber, message } from 'antd';
import { PlusOutlined, EditOutlined, DeleteOutlined } from '@ant-design/icons';
import { api } from '../../services/api';

export default function MembershipLevelsPage() {
  const [levels, setLevels] = useState([]);
  const [loading, setLoading] = useState(false);
  const [modalVisible, setModalVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [form] = Form.useForm();

  const fetchLevels = async () => {
    setLoading(true);
    try {
      const res = await api.get('/admin/membership-levels');
      if (res.code === 20000) setLevels(res.data || []);
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchLevels(); }, []);

  const handleSave = async () => {
    const values = await form.validateFields();
    const method = editing ? api.put : api.post;
    const url = editing ? `/admin/membership-levels/${editing.id}` : '/admin/membership-levels';
    const res = await method(url, values);
    if (res.code === 20000) {
      message.success(editing ? '已更新' : '已创建');
      setModalVisible(false);
      setEditing(null);
      form.resetFields();
      fetchLevels();
    } else { message.error(res.message); }
  };

  const handleDelete = async (id) => {
    Modal.confirm({
      title: '确认删除',
      content: '确定要删除该会员级别吗？',
      onOk: async () => {
        const res = await api.delete(`/admin/membership-levels/${id}`);
        if (res.code === 20000) { message.success('已删除'); fetchLevels(); }
        else { message.error(res.message); }
      },
    });
  };

  const columns = [
    { title: '级别 ID', dataIndex: 'id', width: 100 },
    { title: '名称', dataIndex: 'name' },
    { title: '最低消费金额', dataIndex: 'min_amount', render: v => `¥${v}` },
    {
      title: '操作', width: 180,
      render: (_, r) => (
        <Space>
          <Button size="small" icon={<EditOutlined />} onClick={() => { setEditing(r); form.setFieldsValue(r); setModalVisible(true); }}>编辑</Button>
          <Button size="small" danger icon={<DeleteOutlined />} onClick={() => handleDelete(r.id)}>删除</Button>
        </Space>
      ),
    },
  ];

  return (
    <Card title="会员级别管理">
      <div className="mb-4">
        <Button type="primary" icon={<PlusOutlined />} onClick={() => { setEditing(null); form.resetFields(); setModalVisible(true); }}>新增级别</Button>
      </div>
      <Table dataSource={levels} columns={columns} rowKey="id" loading={loading} pagination={false} />
      <Modal title={editing ? '编辑级别' : '新增级别'} open={modalVisible} onOk={handleSave} onCancel={() => { setModalVisible(false); setEditing(null); form.resetFields(); }} destroyOnClose>
        <Form form={form} layout="vertical">
          <Form.Item name="id" label="级别 ID" rules={[{ required: true }]}><InputNumber min={1} disabled={!!editing} style={{ width: '100%' }} /></Form.Item>
          <Form.Item name="name" label="名称" rules={[{ required: true }]}><Input /></Form.Item>
          <Form.Item name="min_amount" label="最低消费金额" rules={[{ required: true }]}><InputNumber min={0} style={{ width: '100%' }} /></Form.Item>
        </Form>
      </Modal>
    </Card>
  );
}
