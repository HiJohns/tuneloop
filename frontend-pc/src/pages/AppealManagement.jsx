import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, Modal, Descriptions, message, Input, Select, InputNumber, Radio } from 'antd';
import { EyeOutlined, CheckOutlined, CloseOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;
const { TextArea } = Input;

const statusOptions = [
  { value: '', label: '全部' },
  { value: 'pending', label: '待仲裁' },
  { value: 'resolved', label: '已仲裁' },
];

const statusConfig = {
  pending: { text: '待仲裁', color: 'orange' },
  resolved: { text: '已仲裁', color: 'green' },
};

const decisionOptions = [
  { value: 'no_damage', label: '无损坏' },
  { value: 'adjust', label: '调整金额' },
  { value: 'confirm', label: '确认原判' },
];

export default function AppealManagement() {
  const [appeals, setAppeals] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selected, setSelected] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');

  const [decision, setDecision] = useState('no_damage');
  const [adjustAmount, setAdjustAmount] = useState(null);
  const [comment, setComment] = useState('');
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    fetchAppeals();
  }, [statusFilter]);

  const fetchAppeals = async () => {
    setLoading(true);
    try {
      const params = {};
      if (statusFilter) params.status = statusFilter;
      const resp = await api.get('/appeals', { params });
      setAppeals(resp?.data?.list || []);
    } catch (error) {
      message.error('获取申诉列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleViewDetails = async (appeal) => {
    try {
      const resp = await api.get(`/appeals/${appeal.id}`);
      setSelected(resp?.data);
      setDecision('no_damage');
      setAdjustAmount(null);
      setComment('');
      setDetailVisible(true);
    } catch (error) {
      message.error('获取详情失败');
    }
  };

  const handleResolve = async () => {
    if (decision === 'adjust' && (!adjustAmount || adjustAmount <= 0)) {
      message.warning('请填写调整金额');
      return;
    }
    if (!comment.trim()) {
      message.warning('请填写仲裁说明');
      return;
    }
    setSubmitting(true);
    try {
      await api.put(`/appeals/${selected.appeal.id}/resolve`, {
        decision,
        adjust_amount: decision === 'adjust' ? adjustAmount : 0,
        comment,
      });
      message.success('仲裁完成');
      setDetailVisible(false);
      fetchAppeals();
    } catch (error) {
      message.error('仲裁失败');
    } finally {
      setSubmitting(false);
    }
  };

  const deposit = selected?.order?.deposit || 0;
  const damageAmount = selected?.damage_report?.damage_amount || 0;
  const finalAmount = decision === 'adjust' ? (adjustAmount || 0) : (decision === 'confirm' ? damageAmount : 0);

  const getResultPreview = () => {
    if (decision === 'no_damage') return '订单关闭，押金全额退还';
    if (finalAmount < deposit) return `押金退还 ¥${(deposit - finalAmount).toFixed(2)}`;
    if (finalAmount === deposit) return '押金全额扣除，订单关闭';
    return `押金全额扣除 + 需补缴 ¥${(finalAmount - deposit).toFixed(2)}`;
  };

  const columns = [
    {
      title: '申诉编号',
      dataIndex: 'id',
      key: 'id',
      width: 100,
      render: (id) => <span className="font-mono text-xs">{id?.slice(0, 8) || '-'}</span>,
    },
    {
      title: '乐器',
      key: 'instrument',
      width: 150,
      render: (_, record) => {
        const sn = record.instrument_sn || record.damage_report?.order?.instrument_sn || '-';
        const cat = record.category_name || '';
        return <span>{cat ? `${cat} (${sn})` : sn}</span>;
      },
    },
    {
      title: '用户',
      dataIndex: 'user_name',
      key: 'user_name',
      width: 100,
      render: (name) => name || '-',
    },
    {
      title: '定损金额',
      key: 'damage_amount',
      width: 100,
      render: (_, record) => {
        const amount = record.damage_report?.damage_amount;
        return amount ? `¥${amount.toFixed(2)}` : '-';
      },
    },
    {
      title: '申诉原因',
      dataIndex: 'appeal_reason',
      key: 'appeal_reason',
      ellipsis: true,
      width: 200,
    },
    {
      title: '申诉时间',
      dataIndex: 'submitted_at',
      key: 'submitted_at',
      width: 160,
      render: (t) => t ? new Date(t).toLocaleString() : '-',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 100,
      render: (status) => {
        const cfg = statusConfig[status] || { text: status, color: 'default' };
        return <Tag color={cfg.color}>{cfg.text}</Tag>;
      },
    },
    {
      title: '操作',
      key: 'action',
      width: 140,
      render: (_, record) => (
        <Space>
          <Button size="small" icon={<EyeOutlined />} onClick={() => handleViewDetails(record)}>
            详情
          </Button>
          {record.status === 'pending' && (
            <Button size="small" type="primary" icon={<CheckOutlined />} onClick={() => handleViewDetails(record)}>
              仲裁
            </Button>
          )}
        </Space>
      ),
    },
  ];

  return (
    <div className="p-6">
      <Card>
        <div className="flex justify-between items-center mb-6">
          <Title level={2} style={{ margin: 0 }}>申诉处理</Title>
          <Space>
            <Select
              value={statusFilter}
              onChange={setStatusFilter}
              options={statusOptions}
              style={{ width: 120 }}
            />
            <Button icon={<EyeOutlined />} onClick={fetchAppeals}>
              刷新
            </Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={appeals}
          loading={loading}
          rowKey="id"
          pagination={{
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`,
          }}
          scroll={{ x: 1000 }}
        />
      </Card>

      <Modal
        title="申诉详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={720}
        destroyOnClose
      >
        {selected && (
          <div className="space-y-4">
            {/* 基本信息 */}
            <Card size="small" title="基本信息" type="inner">
              <Descriptions column={2} size="small">
                <Descriptions.Item label="申诉编号">
                  <span className="font-mono">{selected.appeal?.id?.slice(0, 8)}</span>
                </Descriptions.Item>
                <Descriptions.Item label="状态">
                  <Tag color={statusConfig[selected.appeal?.status]?.color || 'default'}>
                    {statusConfig[selected.appeal?.status]?.text || selected.appeal?.status}
                  </Tag>
                </Descriptions.Item>
                <Descriptions.Item label="申诉时间" span={2}>
                  {selected.appeal?.submitted_at ? new Date(selected.appeal.submitted_at).toLocaleString() : '-'}
                </Descriptions.Item>
                <Descriptions.Item label="申诉原因" span={2}>
                  {selected.appeal?.appeal_reason || '-'}
                </Descriptions.Item>
                {selected.appeal?.status === 'resolved' && (
                  <>
                    <Descriptions.Item label="仲裁决定">
                      {decisionOptions.find(d => d.value === selected.appeal.resolution)?.label || selected.appeal.resolution}
                    </Descriptions.Item>
                    <Descriptions.Item label="最终金额">
                      {selected.appeal.final_amount != null ? `¥${selected.appeal.final_amount.toFixed(2)}` : '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="仲裁说明" span={2}>
                      {selected.appeal.manager_comment || '-'}
                    </Descriptions.Item>
                    <Descriptions.Item label="处理时间" span={2}>
                      {selected.appeal.resolved_at ? new Date(selected.appeal.resolved_at).toLocaleString() : '-'}
                    </Descriptions.Item>
                  </>
                )}
              </Descriptions>
            </Card>

            {/* 定损信息 */}
            {selected.damage_report?.id && (
              <Card size="small" title="定损信息" type="inner">
                <Descriptions column={2} size="small">
                  <Descriptions.Item label="乐器">
                    {[selected.category_name, selected.instrument_sn].filter(Boolean).join(' (') || '-'}
                    {selected.instrument_sn && selected.category_name ? ')' : ''}
                  </Descriptions.Item>
                  <Descriptions.Item label="用户">{selected.user_name || '-'}</Descriptions.Item>
                  <Descriptions.Item label="定损金额">
                    {selected.damage_report.damage_amount != null
                      ? `¥${selected.damage_report.damage_amount.toFixed(2)}`
                      : '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="定损描述" span={2}>
                    {selected.damage_report.damage_description || '-'}
                  </Descriptions.Item>
                  <Descriptions.Item label="定损状态">
                    <Tag>{selected.damage_report.status}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="已扣押金">
                    {selected.damage_report.deposit_deducted > 0
                      ? `¥${selected.damage_report.deposit_deducted.toFixed(2)}`
                      : '-'}
                  </Descriptions.Item>
                </Descriptions>
              </Card>
            )}

            {/* 订单信息 */}
            {selected.order?.id && (
              <Card size="small" title="订单信息" type="inner">
                <Descriptions column={2} size="small">
                  <Descriptions.Item label="订单号">
                    <span className="font-mono">{selected.order.id?.slice(0, 8)}</span>
                  </Descriptions.Item>
                  <Descriptions.Item label="订单状态">
                    <Tag>{selected.order.status}</Tag>
                  </Descriptions.Item>
                  <Descriptions.Item label="押金">¥{selected.order.deposit?.toFixed(2)}</Descriptions.Item>
                  <Descriptions.Item label="月租">¥{selected.order.monthly_rent?.toFixed(2)}</Descriptions.Item>
                </Descriptions>
              </Card>
            )}

            {/* 仲裁操作 */}
            {selected.appeal?.status === 'pending' && (
              <Card size="small" title="仲裁操作" type="inner">
                <div className="space-y-3">
                  <div>
                    <Radio.Group value={decision} onChange={e => setDecision(e.target.value)}>
                      {decisionOptions.map(opt => (
                        <Radio key={opt.value} value={opt.value}>{opt.label}</Radio>
                      ))}
                    </Radio.Group>
                  </div>

                  {decision === 'adjust' && (
                    <div>
                      <div className="flex items-center gap-3 mb-2 text-sm text-gray-500">
                        <span>当前定损: ¥{damageAmount?.toFixed(2)}</span>
                        <span>押金: ¥{deposit?.toFixed(2)}</span>
                      </div>
                      <InputNumber
                        min={0}
                        precision={2}
                        value={adjustAmount}
                        onChange={setAdjustAmount}
                        prefix="¥"
                        style={{ width: '100%' }}
                        placeholder="输入调整后金额"
                      />
                    </div>
                  )}

                  {(decision === 'adjust' && adjustAmount > 0) || decision === 'confirm' ? (
                    <div className="text-sm px-3 py-2 bg-blue-50 text-blue-700 rounded">
                      预计结果: {getResultPreview()}
                    </div>
                  ) : null}

                  <div>
                    <TextArea
                      rows={3}
                      value={comment}
                      onChange={e => setComment(e.target.value)}
                      placeholder="仲裁说明（必填）"
                    />
                  </div>

                  <Button
                    type="primary"
                    icon={<CheckOutlined />}
                    onClick={handleResolve}
                    loading={submitting}
                  >
                    提交仲裁
                  </Button>
                </div>
              </Card>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}
