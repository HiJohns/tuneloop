import { useState, useEffect } from 'react';
import { Card, Table, Button, Modal, InputNumber, Switch, message } from 'antd';
import { EditOutlined } from '@ant-design/icons';
import { api } from '../../services/api';

// GiftPolicies — 赠点策略管理（#1605, L-05）
// 分会员级别配置：pay_ratio（赠点使用比例）+ refund_ratio（退款返点比例）
export default function GiftPolicies() {
  const [policies, setPolicies] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editing, setEditing] = useState(null);

  const fetchPolicies = async () => {
    setLoading(true);
    try {
      const res = await api.get('/admin/gift-policies');
      if (res.code === 20000) setPolicies(res.data || []);
    } finally { setLoading(false); }
  };

  useEffect(() => { fetchPolicies(); }, []);

  const handleSave = async () => {
    if (!editing) return;
    const res = await api.put('/admin/gift-policies', {
      level_id: editing.level_id,
      pay_ratio: editing.pay_ratio,
      refund_ratio: editing.refund_ratio,
      is_active: editing.is_active,
    });
    if (res.code === 20000) { message.success('已更新'); setEditVisible(false); fetchPolicies(); }
    else { message.error(res.message); }
  };

  const columns = [
    { title: '会员级别', dataIndex: 'name', render: v => v || '-' },
    { title: '赠点使用比例', dataIndex: 'pay_ratio', render: v => `${((v || 0) * 100).toFixed(1)}%` },
    { title: '退款返点比例', dataIndex: 'refund_ratio', render: v => `${((v || 0) * 100).toFixed(1)}%` },
    { title: '状态', dataIndex: 'is_active', render: v => v ? '启用' : '停用' },
    {
      title: '操作', width: 100,
      render: (_, r) => (
        <Button size="small" icon={<EditOutlined />} onClick={() => { setEditing({ ...r }); setEditVisible(true); }}>编辑</Button>
      ),
    },
  ];

  return (
    <Card title="赠点策略">
      <Table dataSource={policies} columns={columns} rowKey="level_id" loading={loading} pagination={false} />
      <Modal title="编辑赠点策略" open={editVisible} onOk={handleSave} onCancel={() => { setEditVisible(false); setEditing(null); }} destroyOnClose>
        {editing && (
          <div className="space-y-4">
            <div>
              <label className="block text-sm font-medium mb-1">赠点使用比例（付款抵扣上限 = 应付总额 × 比例）</label>
              <InputNumber min={0} max={1} step={0.01} value={editing.pay_ratio} onChange={v => setEditing(p => ({ ...p, pay_ratio: v }))} style={{ width: '100%' }} />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">退款返点比例（退款完成后 = 实付现金 × 比例）</label>
              <InputNumber min={0} max={1} step={0.01} value={editing.refund_ratio} onChange={v => setEditing(p => ({ ...p, refund_ratio: v }))} style={{ width: '100%' }} />
            </div>
            <div className="flex items-center gap-2"><Switch checked={editing.is_active} onChange={v => setEditing(p => ({ ...p, is_active: v }))} /><span>{editing.is_active ? '启用' : '停用'}</span></div>
          </div>
        )}
      </Modal>
    </Card>
  );
}
