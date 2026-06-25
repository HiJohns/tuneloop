import { useState, useEffect } from 'react';
import { Table, Card, Button, Space, DatePicker, Select, Input, Tag, message, Modal, Descriptions } from 'antd';
import { DownloadOutlined, SearchOutlined, CheckCircleOutlined, CloseCircleOutlined } from '@ant-design/icons';
import { api, auditLogApi } from '../../services/api';
import dayjs from 'dayjs';

const { RangePicker } = DatePicker;

const actionDisplayMap = {
  CREATE: '创建', UPDATE: '更新', DELETE: '删除',
  PAY: '支付', PICKUP: '取件', RETURN: '归还', CANCEL: '取消',
  TRANSFER: '转移', LOGIN: '登录', SYNC: '同步', IMPORT: '导入',
  INVITE: '邀请', BATCH_UPDATE: '批量更新', UPDATE_STATUS: '更新状态',
  MERGE: '合并', ASSIGN: '分配', QUOTE: '报价', START: '开始',
  SUBMIT: '提交', CONFIRM: '确认', BATCH_IMPORT: '批量导入',
  TRANSFER_OWNERSHIP: '转移所有权', TERMINATE: '终止', DELIVERY: '发货',
  RETURN_INSPECT: '归还检查', DAMAGE: '损坏', SHIPPING: '运输',
  RESOLVE: '解决', INIT: '初始化',
};

const resourceDisplayMap = {
  user: '用户', iam_user: 'IAM用户', merchant: '商户', site: '网点',
  site_member: '网点成员', role: '角色', role_permission: '角色权限',
  order: '订单', lease: '租约', deposit: '押金', instrument: '乐器',
  maintenance_ticket: '维修工单', maintenance_worker: '维修师傅',
  appeal: '申诉', inventory: '库存', rent_setting: '租金设定',
  property: '属性', label: '标签', organization: '组织', account: '账户',
  confirmation: '确认', user_order: '用户订单', user_rental: '用户租约',
  outbound: '出库', assessment: '评估', system: '系统',
};

const filterActions = Object.keys(actionDisplayMap)
const filterResources = Object.keys(resourceDisplayMap)

function formatLogMessage(record) {
  const action = actionDisplayMap[record.action] || record.action
  const resource = resourceDisplayMap[record.resource_type] || record.resource_type
  let detail = ''
  if (record.request_body) {
    try {
      const body = JSON.parse(record.request_body)
      detail = body.name || body.username || ''
    } catch {}
  }
  const namePart = detail ? `「${detail}」` : ''
  const result = record.status === 'failure' && record.error_message
    ? `失败：${record.error_message}`
    : record.status === 'failure'
      ? '失败'
      : '成功'
  return `${action}${resource}${namePart} → ${result}`
}

function getStatusColor(status) {
  return status === 'failure' ? 'red' : 'green'
}

function getRowClassName(record) {
  return record.status === 'failure' ? 'ant-table-row-failure' : ''
}

function getStatusIcon(status) {
  return status === 'failure'
    ? <CloseCircleOutlined style={{ color: '#ff4d4f' }} />
    : <CheckCircleOutlined style={{ color: '#52c41a' }} />
}

export default function AuditLogPage() {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);
  const [total, setTotal] = useState(0);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [filters, setFilters] = useState({
    action: '', resource_type: '', user_id: '', keyword: '',
    date_from: '', date_to: '',
  });
  const [detailLog, setDetailLog] = useState(null);
  const [detailVisible, setDetailVisible] = useState(false);

  useEffect(() => { fetchLogs(); }, [page, pageSize]);

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
    } finally { setLoading(false); }
  };

  const handleSearch = () => { setPage(1); fetchLogs(); };

  const handleExport = async () => {
    try {
      const blob = await auditLogApi.export(filters);
      const url = window.URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url; a.download = 'audit_logs.csv'; a.click();
      window.URL.revokeObjectURL(url);
      message.success('导出成功');
    } catch (e) { message.error('导出失败: ' + (e.message || e)); }
  };

  const showDetail = (record) => { setDetailLog(record); setDetailVisible(true); };

  const columns = [
    { title: '时间', dataIndex: 'created_at', key: 'created_at',
      render: (v) => v ? new Date(v).toLocaleString() : '-', width: 180 },
    { title: '操作描述', key: 'description', width: 500,
      render: (_, record) => formatLogMessage(record) },
    { title: '结果', dataIndex: 'status', key: 'status', width: 100,
      render: (v) => (
        <Tag color={getStatusColor(v)} icon={getStatusIcon(v)}>
          {v === 'failure' ? '失败' : '成功'}
        </Tag>
      )},
    { title: '操作', key: 'action_col', width: 80,
      render: (_, record) => <Button type="link" onClick={() => showDetail(record)}>详情</Button> },
  ];

  return (
    <Card title="操作日志" extra={<Button icon={<DownloadOutlined />} onClick={handleExport}>导出CSV</Button>}>
      <Space wrap style={{ marginBottom: 16 }}>
        <Select placeholder="操作类型" allowClear style={{ width: 140 }}
          value={filters.action} onChange={(v) => setFilters(f => ({ ...f, action: v || '' }))}>
          {filterActions.map(a =>
            <Select.Option key={a} value={a}>{actionDisplayMap[a] || a}</Select.Option>)}
        </Select>
        <Select placeholder="资源类型" allowClear style={{ width: 140 }}
          value={filters.resource_type} onChange={(v) => setFilters(f => ({ ...f, resource_type: v || '' }))}>
          {filterResources.map(t =>
            <Select.Option key={t} value={t}>{resourceDisplayMap[t] || t}</Select.Option>)}
        </Select>
        <RangePicker onChange={(dates) => setFilters(f => ({
          ...f, date_from: dates ? dates[0].format('YYYY-MM-DD') : '',
          date_to: dates ? dates[1].format('YYYY-MM-DD') : '',
        }))} />
        <Button.Group size="small">
          <Button onClick={() => {
            const today = dayjs();
            setFilters(f => ({ ...f, date_from: today.format('YYYY-MM-DD'), date_to: today.format('YYYY-MM-DD') }));
          }}>今天</Button>
          <Button onClick={() => {
            const today = dayjs();
            setFilters(f => ({ ...f, date_from: today.startOf('week').format('YYYY-MM-DD'), date_to: today.format('YYYY-MM-DD') }));
          }}>本周</Button>
          <Button onClick={() => {
            const today = dayjs();
            setFilters(f => ({ ...f, date_from: today.startOf('month').format('YYYY-MM-DD'), date_to: today.format('YYYY-MM-DD') }));
          }}>本月</Button>
          <Button onClick={() => {
            const today = dayjs();
            setFilters(f => ({ ...f, date_from: today.subtract(60, 'day').format('YYYY-MM-DD'), date_to: today.format('YYYY-MM-DD') }));
          }}>最近60天</Button>
        </Button.Group>
        <Button type="primary" icon={<SearchOutlined />} onClick={handleSearch}>查询</Button>
      </Space>

      <style>{`
        .ant-table-row-failure { background-color: #fff1f0 !important; }
        .ant-table-row-failure:hover > td { background-color: #ffccc7 !important; }
      `}</style>

      <Table
        columns={columns}
        dataSource={logs}
        rowKey="id"
        loading={loading}
        rowClassName={getRowClassName}
        pagination={{
          current: page, pageSize, total, showSizeChanger: true,
          showTotal: (t) => `共 ${t} 条`,
          onChange: (p, ps) => { setPage(p); setPageSize(ps); },
        }}
        scroll={{ x: 900 }}
      />

      <Modal title="日志详情" open={detailVisible} onCancel={() => setDetailVisible(false)}
        footer={null} width={720}>
        {detailLog && (
          <Descriptions column={2} bordered size="small">
            <Descriptions.Item label="时间" span={2}>{new Date(detailLog.created_at).toLocaleString()}</Descriptions.Item>
            <Descriptions.Item label="结果" span={2}>
              <Tag color={getStatusColor(detailLog.status)} icon={getStatusIcon(detailLog.status)}>
                {detailLog.status === 'failure' ? '失败' : '成功'}
              </Tag>
              {detailLog.status === 'failure' && detailLog.error_message &&
                <span style={{ marginLeft: 8, color: '#ff4d4f' }}>{detailLog.error_message}</span>}
            </Descriptions.Item>
            <Descriptions.Item label="操作用户">{detailLog.user_id}</Descriptions.Item>
            <Descriptions.Item label="角色">{detailLog.actor_role}</Descriptions.Item>
            <Descriptions.Item label="操作">{actionDisplayMap[detailLog.action] || detailLog.action}</Descriptions.Item>
            <Descriptions.Item label="资源类型">{resourceDisplayMap[detailLog.resource_type] || detailLog.resource_type}</Descriptions.Item>
            <Descriptions.Item label="资源ID">{detailLog.resource_id || '-'}</Descriptions.Item>
            <Descriptions.Item label="IP地址">{detailLog.ip_address || '-'}</Descriptions.Item>
            <Descriptions.Item label="User-Agent" span={2}>{detailLog.user_agent || '-'}</Descriptions.Item>
            {detailLog.error_message && detailLog.status === 'failure' && (
              <Descriptions.Item label="错误信息" span={2}>
                <span style={{ color: '#ff4d4f' }}>{detailLog.error_message}</span>
              </Descriptions.Item>
            )}
            {detailLog.details && (
              <Descriptions.Item label="变更详情" span={2}>
                <pre style={{ maxHeight: 200, overflow: 'auto', background: '#f5f5f5', padding: 8, fontSize: 12 }}>
                  {JSON.stringify(JSON.parse(detailLog.details || '{}'), null, 2)}
                </pre>
              </Descriptions.Item>
            )}
          </Descriptions>
        )}
      </Modal>
    </Card>
  );
}
