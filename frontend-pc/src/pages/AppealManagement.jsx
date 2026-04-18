import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, Modal, Descriptions, message, Input, Tabs } from 'antd';
import { EyeOutlined, CheckOutlined, CloseOutlined, FileTextOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;
const { TextArea } = Input;

export default function AppealManagement() {
  const [appeals, setAppeals] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedAppeal, setSelectedAppeal] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');
  const [activeTab, setActiveTab] = useState('manager');

  useEffect(() => {
    fetchAppeals();
  }, [statusFilter, activeTab]);

  const fetchAppeals = async () => {
    setLoading(true);
    try {
      const endpoint = activeTab === 'manager' ? '/appeals' : '/user/appeals';
      const params = {};
      if (statusFilter) {
        params.status = statusFilter;
      }
      const data = await api.get(endpoint, { params });
      setAppeals(data?.list || []);
    } catch (error) {
      console.error('Failed to fetch appeals:', error);
      message.error('获取申诉列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleResolve = async (appealId, resolution) => {
    try {
      await api.put(`/appeals/${appealId}/resolve`, { 
        resolution,
        notes: resolution.notes 
      });
      message.success('仲裁完成');
      setDetailModalVisible(false);
      fetchAppeals();
    } catch (error) {
      message.error('仲裁失败');
    }
  };

  const handleAgreeDamage = async (appealId) => {
    try {
      await api.post(`/appeals/${appealId}/agree`);
      message.success('已同意定损');
      fetchAppeals();
    } catch (error) {
      message.error('操作失败');
    }
  };

  const handleViewDetails = async (appeal) => {
    try {
      const data = await api.get(`/appeals/${appeal.id}`);
      setSelectedAppeal(data);
      setDetailModalVisible(true);
    } catch (error) {
      console.error('Failed to fetch appeal details:', error);
      message.error('获取详情失败');
    }
  };

  const getStatusConfig = (status) => {
    const configMap = {
      'pending': { text: '待仲裁', color: 'orange' },
      'resolved': { text: '已仲裁', color: 'green' },
      'user_agreed': { text: '用户已同意', color: 'blue' },
      'user_rejected': { text: '用户已拒绝', color: 'red' }
    };
    return configMap[status] || { text: status, color: 'default' };
  };

  const managerColumns = [
    {
      title: '工单号',
      dataIndex: 'id',
      key: 'id',
      render: (id) => id?.slice(0, 8) || '-'
    },
    {
      title: '乐器信息',
      dataIndex: 'instrument_name',
      key: 'instrument_name'
    },
    {
      title: '定损金额',
      dataIndex: 'assessment_amount',
      key: 'assessment_amount',
      render: (amount) => amount ? `¥${amount}` : '-'
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
        <Space>
          <Button 
            size="small" 
            icon={<EyeOutlined />} 
            onClick={() => handleViewDetails(record)}
          >
            详情
          </Button>
          {record.status === 'pending' && (
            <>
              <Button 
                size="small" 
                type="primary"
                icon={<CheckOutlined />}
                onClick={() => handleViewDetails(record)}
              >
                仲裁
              </Button>
            </>
          )}
        </Space>
      )
    }
  ];

  const userColumns = [
    {
      title: '工单号',
      dataIndex: 'id',
      key: 'id',
      render: (id) => id?.slice(0, 8) || '-'
    },
    {
      title: '乐器信息',
      dataIndex: 'instrument_name',
      key: 'instrument_name'
    },
    {
      title: '定损照片',
      dataIndex: 'damage_photos',
      key: 'damage_photos',
      render: (photos) => photos?.length > 0 ? (
        <img src={photos[0]} alt="定损照片" style={{ width: 50, height: 50, objectFit: 'cover' }} />
      ) : '-'
    },
    {
      title: '定损金额',
      dataIndex: 'assessment_amount',
      key: 'assessment_amount',
      render: (amount) => amount ? `¥${amount}` : '-'
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
        <Space>
          <Button 
            size="small" 
            icon={<EyeOutlined />} 
            onClick={() => handleViewDetails(record)}
          >
            查看
          </Button>
          {record.status === 'pending' && (
            <>
              <Button 
                size="small" 
                type="primary"
                onClick={() => handleAgreeDamage(record.id)}
              >
                同意定损
              </Button>
              <Button 
                size="small" 
                danger
                onClick={() => handleViewDetails(record)}
              >
                申诉
              </Button>
            </>
          )}
        </Space>
      )
    }
  ];

  const ResolvePanel = ({ appeal, onResolve }) => {
    const [resolution, setResolution] = useState('no_damage');
    const [notes, setNotes] = useState('');

    return (
      <Card title="仲裁操作面板" className="mt-4">
        <Space direction="vertical" style={{ width: '100%' }}>
          <div>
            <span style={{ marginRight: 8 }}>仲裁结果:</span>
            <Button.Group>
              <Button 
                type={resolution === 'no_damage' ? 'primary' : 'default'}
                onClick={() => setResolution('no_damage')}
              >
                无损坏
              </Button>
              <Button 
                type={resolution === 'adjusted' ? 'primary' : 'default'}
                onClick={() => setResolution('adjusted')}
              >
                调整金额
              </Button>
              <Button 
                type={resolution === 'confirmed' ? 'primary' : 'default'}
                onClick={() => setResolution('confirmed')}
              >
                确定
              </Button>
            </Button.Group>
          </div>
          <div>
            <span style={{ marginRight: 8 }}>说明:</span>
            <TextArea 
              rows={3} 
              value={notes}
              onChange={(e) => setNotes(e.target.value)}
              placeholder="请输入仲裁说明..."
            />
          </div>
          <Button 
            type="primary" 
            icon={<CheckOutlined />}
            onClick={() => onResolve(appeal.id, { resolution, notes })}
          >
            提交仲裁
          </Button>
        </Space>
      </Card>
    );
  };

  const UserAppealForm = ({ appeal, onSubmit }) => {
    const [formData, setFormData] = useState({
      reason: '',
      evidence: ''
    });

    return (
      <Card title="提交申诉" className="mt-4">
        <Space direction="vertical" style={{ width: '100%' }}>
          <div>
            <span style={{ marginRight: 8 }}>申诉原因:</span>
            <TextArea 
              rows={3}
              value={formData.reason}
              onChange={(e) => setFormData({...formData, reason: e.target.value})}
              placeholder="请描述申诉原因..."
            />
          </div>
          <div>
            <span style={{ marginRight: 8 }}>证据材料:</span>
            <TextArea 
              rows={2}
              value={formData.evidence}
              onChange={(e) => setFormData({...formData, evidence: e.target.value})}
              placeholder="请提供相关证据..."
            />
          </div>
          <Button 
            type="primary"
            icon={<FileTextOutlined />}
            onClick={() => onSubmit(appeal.id, formData)}
          >
            提交申诉
          </Button>
        </Space>
      </Card>
    );
  };

  return (
    <div className="p-6">
      <Card>
        <div className="flex justify-between items-center mb-6">
          <Title level={2}>申诉处理</Title>
          <Space>
            <Tabs 
              activeKey={activeTab} 
              onChange={setActiveTab}
              items={[
                { key: 'manager', label: '经理端' },
                { key: 'user', label: '用户端' }
              ]}
            />
            <Button icon={<EyeOutlined />} onClick={fetchAppeals}>
              刷新
            </Button>
          </Space>
        </div>

        <Table
          columns={activeTab === 'manager' ? managerColumns : userColumns}
          dataSource={appeals}
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
        title="申诉详情"
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
        {selectedAppeal && (
          <div>
            <Descriptions bordered column={2} className="mb-4">
              <Descriptions.Item label="工单号">{selectedAppeal.id?.slice(0, 8)}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={getStatusConfig(selectedAppeal.status).color}>
                  {getStatusConfig(selectedAppeal.status).text}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="乐器名称">{selectedAppeal.instrument_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="定损金额">{selectedAppeal.assessment_amount ? `¥${selectedAppeal.assessment_amount}` : '-'}</Descriptions.Item>
              <Descriptions.Item label="定损描述" span={2}>{selectedAppeal.assessment_notes || '-'}</Descriptions.Item>
              {selectedAppeal.damage_photos && selectedAppeal.damage_photos.length > 0 && (
                <Descriptions.Item label="定损照片" span={2}>
                  <div className="flex gap-2">
                    {selectedAppeal.damage_photos.map((photo, idx) => (
                      <img key={idx} src={photo} alt={`定损照片${idx + 1}`} style={{ width: 80, height: 80, objectFit: 'cover' }} />
                    ))}
                  </div>
                </Descriptions.Item>
              )}
              {selectedAppeal.user_appeal_reason && (
                <Descriptions.Item label="用户申诉原因" span={2}>{selectedAppeal.user_appeal_reason}</Descriptions.Item>
              )}
            </Descriptions>

            {activeTab === 'manager' && selectedAppeal.status === 'pending' && (
              <ResolvePanel 
                appeal={selectedAppeal}
                onResolve={handleResolve}
              />
            )}

            {activeTab === 'user' && selectedAppeal.status === 'pending' && (
              <UserAppealForm
                appeal={selectedAppeal}
                onSubmit={async (id, data) => {
                  try {
                    await api.post('/appeals', { ...data, order_id: id });
                    message.success('申诉已提交');
                    setDetailModalVisible(false);
                    fetchAppeals();
                  } catch (error) {
                    message.error('提交失败');
                  }
                }}
              />
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}