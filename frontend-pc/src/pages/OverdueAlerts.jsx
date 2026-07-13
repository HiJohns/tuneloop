import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, message, Select } from 'antd';
import { EyeOutlined, WarningOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;

const statusOptions = [
  { value: '', label: '全部' },
  { value: 'failed', label: '扣款失败' },
  { value: 'partial', label: '部分扣款' },
];

const statusConfig = {
  failed: { text: '扣款失败', color: 'red' },
  partial: { text: '部分扣款', color: 'orange' },
};

export default function OverdueAlerts() {
  const [data, setData] = useState([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const [statusFilter, setStatusFilter] = useState('');

  useEffect(() => {
    fetchData();
  }, [page, pageSize, statusFilter]);

  const fetchData = async () => {
    setLoading(true);
    try {
      const params = { page, page_size: pageSize };
      if (statusFilter) params.status = statusFilter;
      const resp = await api.get('/overdue-leases', { params });
      setData(resp?.data?.list || []);
      setTotal(resp?.data?.total || 0);
    } catch (error) {
      message.error('获取逾期告警列表失败');
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    {
      title: '乐器',
      key: 'instrument',
      width: 200,
      render: (_, record) => {
        const sn = record.instrument_sn || '-';
        const cat = record.category_name || '';
        return <span>{cat ? `${cat} (${sn})` : sn}</span>;
      },
    },
    {
      title: '用户',
      key: 'user',
      width: 160,
      render: (_, record) => (
        <span>{record.user_name || '-'} {record.user_phone ? `(${record.user_phone})` : ''}</span>
      ),
    },
    {
      title: '扣款日期',
      dataIndex: 'charge_date',
      key: 'charge_date',
      width: 120,
    },
    {
      title: '逾期金额',
      dataIndex: 'amount',
      key: 'amount',
      width: 120,
      render: (v) => v != null ? `¥${Number(v).toFixed(2)}` : '-',
    },
    {
      title: '已扣预付',
      dataIndex: 'deducted_from_prepaid',
      key: 'deducted_from_prepaid',
      width: 120,
      render: (v) => v != null ? `¥${Number(v).toFixed(2)}` : '-',
    },
    {
      title: '欠款余额',
      dataIndex: 'remaining_balance',
      key: 'remaining_balance',
      width: 120,
      render: (v) => v != null ? (
        <span className="text-red-500 font-medium">¥{Number(v).toFixed(2)}</span>
      ) : '-',
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      width: 110,
      render: (status) => {
        const cfg = statusConfig[status] || { text: status, color: 'default' };
        return <Tag color={cfg.color}>{cfg.text}</Tag>;
      },
    },
    {
      title: '失败原因',
      dataIndex: 'failure_reason',
      key: 'failure_reason',
      ellipsis: true,
      width: 200,
    },
    {
      title: '创建时间',
      dataIndex: 'created_at',
      key: 'created_at',
      width: 170,
      render: (t) => t ? new Date(t).toLocaleString() : '-',
    },
  ];

  return (
    <div className="p-6">
      <Card>
        <div className="flex justify-between items-center mb-6">
          <Space>
            <WarningOutlined className="text-red-500 text-xl" />
            <Title level={2} style={{ margin: 0 }}>逾期告警</Title>
          </Space>
          <Space>
            <Select
              value={statusFilter}
              onChange={(v) => { setStatusFilter(v); setPage(1); }}
              options={statusOptions}
              style={{ width: 130 }}
            />
            <Button icon={<EyeOutlined />} onClick={fetchData}>
              刷新
            </Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={data}
          loading={loading}
          rowKey="id"
          rowClassName={(record) => {
            if (record.status === 'failed') return 'bg-red-50 hover:bg-red-100';
            if (record.status === 'partial') return 'bg-orange-50 hover:bg-orange-100';
            return '';
          }}
          pagination={{
            current: page,
            pageSize,
            total,
            showSizeChanger: true,
            showTotal: (t) => `共 ${t} 条`,
            onChange: (p, ps) => { setPage(p); setPageSize(ps); },
          }}
          scroll={{ x: 1300 }}
        />
      </Card>
    </div>
  );
}
