import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, message, Select } from 'antd';
import { EyeOutlined, WarningOutlined } from '@ant-design/icons';
import { api } from '../services/api';
import { formatBeijingDateTimeShort } from '../utils/date';

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
      width: 220,
      render: (_, record) => {
        const sn = record.instrument_sn || '-'
        const cat = record.category_name || ''
        return <span>{cat ? `${cat}（${sn}）` : sn}</span>
      },
    },
    {
      title: '用户',
      key: 'user',
      width: 180,
      render: (_, record) => (
        <span>{record.user_name || '-'} {record.user_phone ? `（${record.user_phone}）` : ''}</span>
      ),
    },
    { title: '应还日期', dataIndex: 'end_date', key: 'end_date', width: 130, render: v => v || '-' },
    {
      title: '逾期天数', dataIndex: 'overdue_days', key: 'overdue_days', width: 110, align: 'right',
      render: v => (v != null ? <span className="text-red-500 font-medium">{v} 天</span> : '-'),
    },
    {
      title: '订单号', dataIndex: 'order_id', key: 'order_id', width: 180,
      render: v => <span style={{ fontFamily: 'monospace', fontSize: 12 }}>{v ? `${v.slice(0, 8)}…` : '-'}</span>,
    },
    {
      title: '状态', dataIndex: 'status', key: 'status', width: 110,
      render: () => <Tag color="red">已逾期</Tag>,
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
