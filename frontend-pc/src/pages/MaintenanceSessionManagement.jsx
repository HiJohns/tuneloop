import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, Modal, Descriptions, Steps, message, Input, Select } from 'antd';
import { PlayCircleOutlined, CheckCircleOutlined, CameraOutlined, ReloadOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;
const { TextArea } = Input;

export default function MaintenanceSessionManagement() {
  const [sessions, setSessions] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedSession, setSelectedSession] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');

  useEffect(() => {
    fetchSessions();
  }, [statusFilter]);

  const fetchSessions = async () => {
    setLoading(true);
    try {
      const params = {};
      if (statusFilter) {
        params.status = statusFilter;
      }
      const data = await api.get('/maintenance/sessions', { params });
      setSessions(data?.list || []);
    } catch (error) {
      console.error('Failed to fetch sessions:', error);
      message.error('获取维修会话列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleStartWork = async (sessionId) => {
    try {
      await api.post(`/maintenance/${sessionId}/start`);
      message.success('开始工作成功');
      fetchSessions();
    } catch (error) {
      message.error('开始工作失败');
    }
  };

  const handleComplete = async (sessionId, passed) => {
    try {
      await api.post(`/maintenance/${sessionId}/inspect`, { status: passed ? 'passed' : 'failed' });
      message.success(passed ? '验收通过' : '验收失败');
      fetchSessions();
    } catch (error) {
      message.error('操作失败');
    }
  };

  const handleViewDetails = async (session) => {
    try {
      const data = await api.get(`/maintenance/sessions/${session.id}`);
      setSelectedSession(data);
      setDetailModalVisible(true);
    } catch (error) {
      console.error('Failed to fetch session details:', error);
      message.error('获取详情失败');
    }
  };

  const getStatusConfig = (status) => {
    const configMap = {
      'pending': { text: '待处理', color: 'orange' },
      'assigned': { text: '已指派', color: 'blue' },
      'in_progress': { text: '进行中', color: 'processing' },
      'completed': { text: '已完成', color: 'green' },
      'passed': { text: '验收通过', color: 'green' },
      'failed': { text: '验收失败', color: 'red' }
    };
    return configMap[status] || { text: status, color: 'default' };
  };

  const columns = [
    {
      title: '工单号',
      dataIndex: 'id',
      key: 'id',
      render: (id) => id?.slice(0, 8) || '-'
    },
    {
      title: '日期',
      dataIndex: 'created_at',
      key: 'created_at',
      render: (date) => date?.slice(0, 10) || '-'
    },
    {
      title: '类别',
      dataIndex: 'category',
      key: 'category',
      render: (cat) => cat || '维修'
    },
    {
      title: '问题简述',
      dataIndex: 'description',
      key: 'description',
      ellipsis: true,
      render: (desc) => desc || '-'
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const config = getStatusConfig(status);
        return <Tag color={config.color}>{config.text}</Tag>;
      }
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space size="small">
          <Button 
            size="small" 
            type="link" 
            onClick={() => handleViewDetails(record)}
          >
            详情
          </Button>
          {record.status === 'assigned' && (
            <Button 
              size="small" 
              icon={<PlayCircleOutlined />}
              onClick={() => handleStartWork(record.id)}
            >
              开始
            </Button>
          )}
          {record.status === 'in_progress' && (
            <>
              <Button 
                size="small" 
                type="primary"
                icon={<CheckCircleOutlined />}
                onClick={() => handleComplete(record.id, true)}
              >
                完成
              </Button>
              <Button 
                size="small" 
                danger
                onClick={() => handleComplete(record.id, false)}
              >
                失败
              </Button>
            </>
          )}
        </Space>
      )
    }
  ];

  const getStepStatus = (status) => {
    const stepMap = {
      'pending': 0,
      'assigned': 1,
      'in_progress': 2,
      'completed': 2,
      'passed': 3,
      'failed': 3
    };
    return stepMap[status] || 0;
  };

  return (
    <div className="p-6">
      <Card>
        <div className="flex justify-between items-center mb-6">
          <Title level={2}>维修会话管理</Title>
          <Space>
            <Select
              placeholder="筛选状态"
              value={statusFilter}
              onChange={setStatusFilter}
              allowClear
              style={{ width: 120 }}
              options={[
                { label: '待处理', value: 'pending' },
                { label: '已指派', value: 'assigned' },
                { label: '进行中', value: 'in_progress' },
                { label: '已完成', value: 'completed' }
              ]}
            />
            <Button icon={<ReloadOutlined />} onClick={fetchSessions}>
              刷新
            </Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={sessions}
          loading={loading}
          rowKey="id"
          pagination={{
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`
          }}
        />
      </Card>

      {/* 详情弹窗 */}
      <Modal
        title="维修详情"
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setDetailModalVisible(false)}>
            关闭
          </Button>
        ]}
        width={700}
        destroyOnClose
      >
        {selectedSession && (
          <div>
            <Steps
              current={getStepStatus(selectedSession.status)}
              className="mb-6"
              items={[
                { title: '待处理' },
                { title: '已指派' },
                { title: '进行中' },
                { title: '完成' }
              ]}
            />

            <Descriptions bordered column={2} className="mb-4">
              <Descriptions.Item label="工单号">{selectedSession.id?.slice(0, 8)}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={getStatusConfig(selectedSession.status).color}>
                  {getStatusConfig(selectedSession.status).text}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="乐器名称">{selectedSession.instrument_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="用户">{selectedSession.user_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="问题描述" span={2}>
                {selectedSession.description || '-'}
              </Descriptions.Item>
              {selectedSession.images && selectedSession.images.length > 0 && (
                <Descriptions.Item label="问题图片" span={2}>
                  <div className="flex gap-2">
                    {selectedSession.images.map((img, idx) => (
                      <img key={idx} src={img} alt={`问题图片${idx + 1}`} style={{ width: 80, height: 80, objectFit: 'cover' }} />
                    ))}
                  </div>
                </Descriptions.Item>
              )}
            </Descriptions>

            {selectedSession.records && selectedSession.records.length > 0 && (
              <Card title="维修记录" size="small">
                {selectedSession.records.map((record, idx) => (
                  <div key={idx} className="mb-3 pb-3 border-b">
                    <p><strong>{record.created_at?.slice(0, 16)}</strong></p>
                    <p>{record.notes}</p>
                  </div>
                ))}
              </Card>
            )}

            <div className="mt-4">
              <Button 
                icon={<CameraOutlined />}
                onClick={() => message.info('扫码功能需在移动端使用')}
              >
                扫码验收
              </Button>
            </div>
          </div>
        )}
      </Modal>
    </div>
  );
}