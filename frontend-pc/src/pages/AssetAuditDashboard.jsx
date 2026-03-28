import { useState, useEffect } from 'react';
import { Card, Row, Col, Statistic, Table, Tag, Alert, Button } from 'antd';
import { ArrowUpOutlined, ArrowDownOutlined } from '@ant-design/icons';
import { api } from '../services/api';

export default function AssetAuditDashboard() {
  const [stats, setStats] = useState({
    totalAssets: 0,
    rentalRate: 0,
    transferRate: 0,
    totalRevenue: 0
  });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState(null);
  const [nearTransfer, setNearTransfer] = useState([]);

  useEffect(() => {
    fetchDashboardStats();
  }, []);

  const fetchDashboardStats = async () => {
    setLoading(true);
    setError(null);
    try {
      const result = await api.get('/admin/dashboard/stats');
      
      if (result.code === 20000 && result.data) {
        const data = result.data;
        setStats({
          totalAssets: data.total_assets || 0,
          rentalRate: data.rental_rate || 0,
          transferRate: data.transfer_rate || 0,
          totalRevenue: data.total_revenue || 0
        });
      } else {
        throw new Error(result.message || 'Failed to fetch dashboard stats');
      }
    } catch (err) {
      setError(err.message);
    } finally {
      setLoading(false);
    }
  };

  const columns = [
    { title: 'SN码', dataIndex: 'sn', key: 'sn' },
    { title: '乐器名称', dataIndex: 'name', key: 'name' },
    { title: '当前用户', dataIndex: 'user', key: 'user' },
    {
      title: '累计租期',
      dataIndex: 'months',
      key: 'months',
      render: (months) => `${months} / 12 个月`
    },
    {
      title: '距转售',
      dataIndex: 'remaining',
      key: 'remaining',
      render: (rem) => (
        <Tag color={rem <= 1 ? 'red' : rem <= 3 ? 'orange' : 'green'}>
          {rem} 个月
        </Tag>
      )
    }
  ];

  if (loading) {
    return (
      <div className="p-6">
        <div className="text-center py-16">
          <div className="text-2xl mb-4">数据正在同步中...</div>
          <div className="text-gray-500">请稍候，正在加载资产数据</div>
        </div>
      </div>
    );
  }

  if (error) {
    return (
      <div className="p-6">
        <Alert
          message="数据加载失败"
          description={error}
          type="error"
          showIcon
          className="mb-4"
          action={
            <Button onClick={fetchDashboard} type="primary">
              重试
            </Button>
          }
        />
      </div>
    );
  }

  return (
    <div className="p-6">
      <h2 className="text-xl font-bold mb-6">资产审计大屏</h2>

      <Row gutter={16} className="mb-6">
        <Col span={6}>
          <Card>
            <Statistic
              title="全网资产总数"
              value={stats.totalAssets}
              suffix="台"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="租赁率"
              value={stats.rentalRate}
              precision={1}
              suffix="%"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="转售率"
              value={stats.transferRate}
              precision={1}
              suffix="%"
            />
          </Card>
        </Col>
        <Col span={6}>
          <Card>
            <Statistic
              title="累计收入"
              value={stats.totalRevenue}
              precision={2}
              prefix="¥"
            />
          </Card>
        </Col>
      </Row>

      <Card>
        <div className="text-lg font-bold mb-4">即将转售资产</div>
        <Table
          columns={columns}
          dataSource={nearTransfer || [] || []}
          rowKey="sn"
          pagination={{ pageSize: 10 }}
          locale={{ emptyText: '暂无即将转售的资产' }}
        />
      </Card>
    </div>
  );
}
