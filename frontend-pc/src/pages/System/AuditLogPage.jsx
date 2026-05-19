import { useState, useEffect } from 'react';
import { Table, Card, Button, Space, DatePicker, Select, Input, Tag, message, Modal, Descriptions } from 'antd';
import { DownloadOutlined, SearchOutlined } from '@ant-design/icons';
import { api, auditLogApi } from '../../services/api';

const { RangePicker } = DatePicker;

const actionColorMap = {
  CREATE: 'green',
  UPDATE: 'blue',
  DELETE: 'red',
  PAY: 'orange',
  PICKUP: 'purple',
  RETURN: 'cyan',
  CANCEL: 'volcano',
  TRANSFER: 'geekblue',
  LOGIN: 'lime',
  SYNC: 'gold',
  IMPORT: 'magenta',
};

export default function AuditLogPage() {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filters, setFilters] = useState({
    action: '',
    resource_type: '',
    user_id: '',
    keyword: '',
    date_from: '',
    date_to: '',
  });
  const [detailLog, setDetailLog] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);

  useEffect(() => {
    fetchLogs();
  }, [page, pageSize]);

  const fetchLogs = async () => {
    setLoading(true);
    try {
      const params = { page, pageSize, ...filters };
      Object.keys(params).forEach(k => { if (!params[k]) delete params[k]; });
      const resp = await api.get('/admin/audit-logs', { params });
      setLogs(resp?.data?.list || []);
      setTotal(resp?.data?.total || 0);
    } catch (e) {
      message.error('获取日志失败: ' + (e.message || e));
    } finally {
      setLoading(false);
    }
  };

  const handleSearch = () => {
    setPage(1);
    fetchLogs();
  };

  const handleExport = async () => {
    try {
      const blob = await auditLogApi.export(filters);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url;
      a.download = 'audit_logs.csv';
      a.click();
      window.URL.revokeObjectURL(url);
      message.success('导出成功');
    } catch (e) {
      message.error('导出失败: ' + (e.message || e));
    }
  };

  const showDetail = (record) => {
    setDetailLog(record);
    setDetailVisible(true);
  };

  const columns = [
    { title: '时间', dataIndex: 'created_at', key: 'created_at', render: (v) => v ? new Date(v).toLocaleString() : '-', width: 180 },
    { title: '操作用户', dataIndex: 'user_id', key: 'user_id', render: (v) => v ? v.substring(0, 8) + '...' : '-', width: 120 },
    { title: '角色', dataIndex: 'actor_role', key: 'actor_role', width: 100 },
    { title: '操作', dataIndex: 'action', key: 'action', render: (v) => <Tag color={actionColorMap[v] || 'default'}>{v}</Tag>, width: 160 },
    { title: '资源类型', dataIndex: 'resource_type', key: 'resource_type', width: 140 },
    { title: '资源ID', dataIndex: 'resource_id', key: 'resource_id', render: (v) => v ? v.substring(0, 12) + '...' : '-', width: 140 },
    { title: 'IP', dataIndex: 'ip_address', key: 'ip_address', width: 130 },
    {
      title: '操作', key: 'action_col', render: (_, record) => (
        <Button type="link" onClick={() => showDetail(record)}>详情</Button>
      ), width: 80,
    },
  ];

  return (
    <Card title="操作日志" extra={<Button icon={<DownloadOutlined />} onClick={handleExport}>导出CSV</Button>}>
      <Space wrap style={{ marginBottom: 16 }}>
        <Select
          placeholder="操作类型" allowClear style={{ width: 140 }}
          value={filters.action} onChange={(v) => setFilters(f => ({ ...f, action: v || '' }))}
        >
          {['CREATE', 'UPDATE', 'DELETE', 'PAY', 'PICKUP', 'RETURN', 'CANCEL', 'TRANSFER', 'LOGIN', 'SYNC', 'IMPORT'].map(a =>
            <Select.Option key={a} value={a}>{a}</Select.Option>
          )}
        </Select>
        <Select
          placeholder="资源类型" allowClear style={{ width: 140 }}
          value={filters.resource_type} onChange={(v) => setFilters(f => ({ ...f, resource_type: v || '' }))}
        >
          {['user', 'merchant', 'site', 'site_member', 'role', 'order', 'lease', 'deposit', 'instrument', 'maintenance_ticket', 'appeal', 'inventory'].map(t =>
            <Select.Option key={t} value={t}>{t}</Select.Option>
          )}
        </Select>
        <Input
          placeholder="用户ID" style={{ width: 160 }}
          value={filters.user_id} onChange={(e) => setFilters(f => ({ ...f, user_id: e.target.value }))}
        />
        <Input
          placeholder="关键词" style={{ width: 160 }}
          value={filters.keyword} onChange={(e) => setFilters(f => ({ ...f, keyword: e.target.value }))}
        />
        <RangePicker
          onChange={(dates) => setFilters(f => ({
            ...f,
            date_from: dates ? dates[0].format('YYYY-MM-DD') : '',
            date_to: dates ? dates[1].format('YYYY-MM-DD') : '',
          }))}
        />
        <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch}>查询</Button>
      </Space>

      <Table
        columns={columns}
        dataSource={logs}
        rowKey="id"
        loading={loading}
        pagination={{
          current: page,
          pageSize,
          total,
          showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); },
        }}
        scroll={{ x: 1100 }}
      />

      <Modal
        title="日志详情"
        open={detailVisible}
        onCancel={() => setDetailVisible(false)}
        footer={null}
        width={720}
      >
        {detailLog && (
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="时间" span={2}>{new Date(detailLog.created_at).toLocaleString()}</Descriptions.Item>
            <Descriptions.Item label="操作用户">{detailLog.user_id}</Descriptions.Item>
            <Descriptions.Item label="角色">{detailLog.actor_role}</Descriptions.Item>
            <Descriptions.Item label="操作"><Tag color={actionColorMap[detailLog.action] || 'default'}>{detailLog.action}</Tag></Descriptions.Item>
            <Descriptions.Item label="资源类型">{detailLog.resource_type}</Descriptions.Item>
            <Descriptions.Item label="资源ID">{detailLog.resource_id || '-'}</Descriptions.Item>
            <Descriptions.Item label="IP地址">{detailLog.ip_address || '-'}</Descriptions.Item>
            <Descriptions.Item label="User-Agent" span={2}>{detailLog.user_agent || '-'}</Descriptions.Item>
            {detailLog.details && (
              <Descriptions.Item label="变更详情" span={2}>
                <pre style={{ maxHeight: 200, overflow: 'auto', background: '#f5f5f5', padding: 8, fontSize: 12 }}>{JSON.stringify(JSON.parse(detailLog.details || '{}'), null, 2)}</pre>
              </Descriptions.Item>
            )}
          </Descriptions>
        )}
      </Modal>
    </Card>
  );
}
