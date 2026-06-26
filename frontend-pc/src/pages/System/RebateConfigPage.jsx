import { useState, useEffect } from 'react';
import { Card, Table, Button, Space, Modal, InputNumber, Switch, message } from 'antd';
import { EditOutlined } from '@ant-design/icons';
import { api } from '../../services/api';

export default function RebateConfigPage() {
  const [configs, setConfigs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editing, setEditing] = useState(null);

  const fetchConfig = async () => {
    setLoading(true);
    try {
      const res = await api.get('/admin/rebate-config');
      if (res.code === 20000) setConfigs(res.data || []);
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchConfig(); }, []);

  const handleSave = async () => {
    if (!editing) return;
    const res = await api.put('/admin/rebate-config', {
      level_id: editing.level_id,
      rent_ratio: editing.rent_ratio,
      is_active: editing.is_active,
    });
    if (res.code === 20000) { message.success('已更新'); setEditVisible(false); fetchConfig(); }
    else { message.error(res.message); }
  };

  const columns = [
    { title: '会员级别', dataIndex: 'level_id', render: v => ['', '初级', '中级', '高级'][v] || v },
    { title: '返还比例', dataIndex: 'rent_ratio', render: v => `${(v * 100).toFixed(1)}%` },
    { title: '状态', dataIndex: 'is_active', render: v => v ? '启用' : '停用' },
    {
      title: '操作', width: 100,
      render: (_, r) => (
        <Button size="small" icon={<EditOutlined />} onClick={() => { setEditing({ ...r }); setEditVisible(true); }}>编辑</Button>
      ),
    },
  ];

  return (
    <Card title="返点配置">
      <Table dataSource={configs} columns={columns} rowKey="level_id" loading={loading} pagination={false} />
      <Modal title="编辑返点比例" open={editVisible} onOk={handleSave} onCancel={() => { setEditVisible(false); setEditing(null); }} destroyOnClose>
        {editing && (
          <div className="space-y-4">
            <div><label className="block text-sm font-medium mb-1">返还比例</label><InputNumber min={0} max={1} step={0.001} value={editing.rent_ratio} onChange={v => setEditing(p => ({ ...p, rent_ratio: v }))} style={{ width: '100%' }} /></div>
            <div className="flex items-center gap-2"><Switch checked={editing.is_active} onChange={v => setEditing(p => ({ ...p, is_active: v }))} /><span>{editing.is_active ? '启用' : '停用'}</span></div>
          </div>
        )}
      </Modal>
    </Card>
  );
}
