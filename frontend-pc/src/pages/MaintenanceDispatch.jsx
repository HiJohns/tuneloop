import { useState, useEffect } from 'react';
import { Table, Tag, Button, Select, Modal, Card, Typography, Space } from 'antd';
import { ExclamationCircleOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;

export default function MaintenanceDispatch() {
  const [tickets, setTickets] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedTicket, setSelectedTicket] = useState(null);
  const [technicians, setTechnicians] = useState([]);
  const [selectedTech, setSelectedTech] = useState(null);

  useEffect(() => {
    fetchTickets();
  }, []);

  const fetchTickets = async () => {
    setLoading(true);
    try {
      const data = await api.get('/merchant/maintenance')
      setTickets(data?.list || [])
    } catch (error) {
      console.error('Failed to fetch tickets:', error)
    } finally {
      setLoading(false)
    }
  };

  const handleAssign = async () => {
    if (!selectedTicket || !selectedTech) return;
    
    try {
      await api.put(`/merchant/maintenance/${selectedTicket}/assign`, { technician_id: selectedTech })
      Modal.success({ content: '指派成功' });
      setSelectedTicket(null);
      setSelectedTech(null);
      fetchTickets();
    } catch (error) {
      Modal.error({ content: '指派失败' });
    }
  };

  const handleUpdateStatus = async (ticketId, newStatus) => {
    try {
      await api.put(`/merchant/maintenance/${ticketId}/update`, { status: newStatus })
      Modal.success({ content: '状态更新成功' });
      fetchTickets();
    } catch (error) {
      Modal.error({ content: '更新失败' });
    }
  };

  const getStatusColor = (status) => {
    const colors = {
      pending: 'orange',
      processing: 'blue',
      completed: 'green',
      cancelled: 'red'
    };
    return colors[status] || 'default';
  };

  const getStatusText = (status) => {
    const texts = {
      pending: '待处理',
      processing: '处理中',
      completed: '已完成',
      cancelled: '已取消'
    };
    return texts[status] || status;
  };

  const columns = [
    {
      title: '工单号',
      dataIndex: 'id',
      key: 'id',
      render: (id) => id?.slice(0, 8) || '-'
    },
    {
      title: '乐器名称',
      dataIndex: 'instrument_name',
      key: 'instrument_name'
    },
    {
      title: '用户',
      dataIndex: 'user_name',
      key: 'user_name'
    },
    {
      title: '问题',
      dataIndex: 'problem_description',
      key: 'problem',
      ellipsis: true
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => (
        <Tag color={getStatusColor(status)}>{getStatusText(status)}</Tag>
      )
    },
    {
      title: '师傅',
      dataIndex: 'technician_id',
      key: 'technician',
      render: (tech) => tech || '-'
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space>
          {record.status === 'pending' && (
            <Button size="small" onClick={() => setSelectedTicket(record.id)}>
              接单
            </Button>
          )}
          {record.status === 'processing' && (
            <>
              <Button size="small" onClick={() => setSelectedTicket(record.id)}>
                指派
              </Button>
              <Button size="small" onClick={() => handleUpdateStatus(record.id, 'completed')}>
                完成
              </Button>
            </>
          )}
        </Space>
      )
    }
  ];

  return (
    <div className="p-6">
      <Card>
        <Title level={4}>维保调度中心</Title>
        
        <div className="mb-4 flex gap-2">
          <Button onClick={() => fetchTickets()}>刷新</Button>
        </div>

        <Table
          columns={columns}
          dataSource={tickets || [] || []}
          loading={loading}
          rowKey="id"
          pagination={{ pageSize: 10 }}
        />
      </Card>

      <Modal
        title="指派师傅"
        open={!!selectedTicket}
        onOk={handleAssign}
        onCancel={() => { setSelectedTicket(null); setSelectedTech(null); }}
      >
        <div className="py-4">
          <p className="mb-2">选择维修师傅：</p>
          <Select
            className="w-full"
            placeholder="请选择师傅"
            value={selectedTech}
            onChange={setSelectedTech}
            options={[
              { label: '李师傅 - 钢琴调音', value: 'tech-001' },
              { label: '王师傅 - 弦乐维修', value: 'tech-002' },
              { label: '张师傅 - 管乐维修', value: 'tech-003' }
            ]}
          />
        </div>
      </Modal>
    </div>
  );
}
