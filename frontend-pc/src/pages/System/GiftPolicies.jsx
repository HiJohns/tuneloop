import { useState, useEffect } from 'react';
import { Card, Table, Button, Modal, InputNumber, Switch, message, Space } from 'antd';
import { EditOutlined } from '@ant-design/icons';
import { api } from '../../services/api';

// GiftPolicies — 乐币规则管理（#1945 Sub-B，原「赠点策略」#1605 L-05）
// 分会员级别配置：pay_ratio（抵扣比例）+ referral_ratio（裂变比例）+ referral_reg_points（邀请奖乐币）
// 全局参数：pay_ratio_max（抵扣上限）/ point_batch_validity_months（有效期）/ point_expiry_reminder_days（到期提醒提前量）
export default function GiftPolicies() {
  const [policies, setPolicies] = useState([]);
  const [loading, setLoading] = useState(false);
  const [editVisible, setEditVisible] = useState(false);
  const [editing, setEditing] = useState(null);
  const [settings, setSettings] = useState(null);
  const [savingSettings, setSavingSettings] = useState(false);

  const fetchPolicies = async () => {
    setLoading(true);
    try {
      const res = await api.get('/admin/gift-policies');
      if (res.code === 20000) setPolicies(res.data || []);
    } finally { setLoading(false); }
  };

  const fetchSettings = async () => {
    try {
      const res = await api.get('/admin/point-settings');
      if (res.code === 20000) setSettings(res.data || {});
    } catch (e) {
      message.error(e.message || '读取全局参数失败');
    }
  };

  useEffect(() => { fetchPolicies(); fetchSettings(); }, []);

  const handleSave = async () => {
    if (!editing) return;
    // #1944 Sub-A：网络/服务异常必须透出（原实现在异常时静默无反应）
    try {
      const res = await api.put('/admin/gift-policies', {
        level_id: editing.level_id,
        pay_ratio: editing.pay_ratio,
        referral_ratio: editing.referral_ratio,
        referral_reg_points: editing.referral_reg_points,
        is_active: editing.is_active,
      });
      if (res.code === 20000) { message.success('已更新'); setEditVisible(false); fetchPolicies(); }
      else { message.error(res.message || '保存失败'); }
    } catch (e) {
      message.error(e.message || '保存失败，请稍后重试');
    }
  };

  const handleSaveSettings = async () => {
    if (!settings) return;
    setSavingSettings(true);
    try {
      const res = await api.put('/admin/point-settings', {
        pay_ratio_max: settings.pay_ratio_max,
        point_batch_validity_months: settings.point_batch_validity_months,
        point_expiry_reminder_days: settings.point_expiry_reminder_days,
      });
      if (res.code === 20000) { message.success('全局参数已保存'); fetchSettings(); }
      else { message.error(res.message || '保存失败'); }
    } catch (e) {
      message.error(e.message || '保存失败，请稍后重试');
    } finally { setSavingSettings(false); }
  };

  const pct = (v) => `${((v || 0) * 100).toFixed(1)}%`;

  const columns = [
    { title: '会员级别', dataIndex: 'name', render: (v, r) => v ? `${v}${r.is_fallback ? '（默认）' : ''}` : '-' },
    { title: '抵扣比例', dataIndex: 'pay_ratio', render: pct },
    { title: '裂变比例', dataIndex: 'referral_ratio', render: pct },
    { title: '邀请奖（乐币）', dataIndex: 'referral_reg_points', render: v => (v != null ? v : 0) },
    { title: '状态', dataIndex: 'is_active', render: v => v ? '启用' : '停用' },
    {
      title: '操作', width: 100,
      render: (_, r) => (
        <Button size="small" icon={<EditOutlined />} onClick={() => { setEditing({ ...r }); setEditVisible(true); }}>编辑</Button>
      ),
    },
  ];

  return (
    <div className="space-y-4">
      <Card title="全局参数">
        {settings && (
          <Space direction="vertical" style={{ width: '100%' }} size="middle">
            <div>
              <label className="block text-sm font-medium mb-1">抵扣上限（会员级别抵扣比例不得超过此值，最高 100%）</label>
              <InputNumber min={0} max={1} step={0.01} value={settings.pay_ratio_max}
                onChange={v => setSettings(p => ({ ...p, pay_ratio_max: v }))} style={{ width: 240 }} />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">乐币有效期（月，发放批次到期失效）</label>
              <InputNumber min={1} max={120} step={1} value={settings.point_batch_validity_months}
                onChange={v => setSettings(p => ({ ...p, point_batch_validity_months: v }))} style={{ width: 240 }} />
            </div>
            <div>
              <label className="block text-sm font-medium mb-1">到期提醒提前量（天）</label>
              <InputNumber min={0} max={365} step={1} value={settings.point_expiry_reminder_days}
                onChange={v => setSettings(p => ({ ...p, point_expiry_reminder_days: v }))} style={{ width: 240 }} />
            </div>
            <Button type="primary" loading={savingSettings} onClick={handleSaveSettings}>保存全局参数</Button>
          </Space>
        )}
      </Card>

      <Card title="乐币规则">
        <Table dataSource={policies} columns={columns} rowKey="level_id" loading={loading} pagination={false} />
        <Modal title="编辑乐币规则" open={editVisible} onOk={handleSave} onCancel={() => { setEditVisible(false); setEditing(null); }} destroyOnClose>
          {editing && (
            <div className="space-y-4">
              <div>
                <label className="block text-sm font-medium mb-1">抵扣比例（付款抵扣上限 = 应付总额 × 比例）</label>
                <InputNumber min={0} max={1} step={0.01} value={editing.pay_ratio} onChange={v => setEditing(p => ({ ...p, pay_ratio: v }))} style={{ width: '100%' }} />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">裂变比例（推荐人按被推荐人租金获得返佣 = 租金 × 比例）</label>
                <InputNumber min={0} max={1} step={0.01} value={editing.referral_ratio} onChange={v => setEditing(p => ({ ...p, referral_ratio: v }))} style={{ width: '100%' }} />
              </div>
              <div>
                <label className="block text-sm font-medium mb-1">邀请奖（被推荐人注册后发放给推荐人的乐币数）</label>
                <InputNumber min={0} max={100000} precision={0} value={editing.referral_reg_points} onChange={v => setEditing(p => ({ ...p, referral_reg_points: v }))} style={{ width: '100%' }} />
              </div>
              <div className="flex items-center gap-2"><Switch checked={editing.is_active} onChange={v => setEditing(p => ({ ...p, is_active: v }))} /><span>{editing.is_active ? '启用' : '停用'}</span></div>
            </div>
          )}
        </Modal>
      </Card>
    </div>
  );
}
