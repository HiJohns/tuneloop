import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, Modal, Descriptions, message } from 'antd';
import { FileTextOutlined, EyeOutlined, UndoOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;

export default function UserRental() {
  const [rentals, setRentals] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedRental, setSelectedRental] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);

  useEffect(() => {
    fetchRentals();
  }, []);

  const fetchRentals = async () => {
    setLoading(true);
    try {
      const data = await api.get('/user/rentals');
      setRentals(data?.list || []);
    } catch (error) {
      console.error('Failed to fetch rentals:', error);
      message.error('获取租赁列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleViewContract = async (rentalId) => {
    try {
      const data = await api.get(`/user/contracts/${rentalId}`);
      setSelectedRental(data);
      setDetailModalVisible(true);
    } catch (error) {
      console.error('Failed to fetch contract:', error);
      message.error('获取合同失败');
    }
  };

  const handleReturn = async (rentalId) => {
    try {
      await api.post(`/user/rentals/${rentalId}/return`, {
        return_method: 'courier'
      });
      message.success('归还申请已提交');
      fetchRentals();
    } catch (error) {
      message.error('提交失败');
    }
  };

  const getStatusConfig = (status) => {
    const configMap = {
      'active': { text: '租赁中', color: 'green' },
      'return_requested': { text: '归还申请中', color: 'orange' },
      'return_processing': { text: '归还处理中', color: 'blue' },
      'completed': { text: '已完成', color: 'default' }
    };
    return configMap[status] || { text: status, color: 'default' };
  };

  const columns = [
    {
      title: '租赁单号',
      dataIndex: 'id',
      key: 'id',
      render: (id) => id?.slice(0, 8) || '-'
    },
    {
      title: '乐器',
      dataIndex: 'instrument_name',
      key: 'instrument_name'
    },
    {
      title: '租赁开始',
      dataIndex: 'start_date',
      key: 'start_date',
      render: (date) => date?.slice(0, 10) || '-'
    },
    {
      title: '租赁结束',
      dataIndex: 'end_date',
      key: 'end_date',
      render: (date) => date?.slice(0, 10) || '-'
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
            icon={<FileTextOutlined />}
            onClick={() => handleViewContract(record.id)}
          >
            合同
          </Button>
          {record.status === 'active' && (
            <Button 
              size="small" 
              icon={<UndoOutlined />}
              onClick={() => handleReturn(record.id)}
            >
              归还
            </Button>
          )}
        </Space>
      )
    }
  ];

  return (
    <div className="p-6">
      <Card>
        <Title level={2}>我的租赁</Title>

        <Table
          columns={columns}
          dataSource={rentals}
          loading={loading}
          rowKey="id"
          pagination={{
            showSizeChanger: true,
            showTotal: (total) => `共 ${total} 条`
          }}
        />
      </Card>

      <Modal
        title="电子合同"
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={[
          <Button key="close" onClick={() => setDetailModalVisible(false)}>
            关闭
          </Button>
        ]}
        width={600}
        destroyOnClose
      >
        {selectedRental && (
          <Descriptions bordered column={1}>
            <Descriptions.Item label="合同编号">{selectedRental.contract_number || '-'}</Descriptions.Item>
            <Descriptions.Item label="乐器">{selectedRental.instrument_name || '-'}</Descriptions.Item>
            <Descriptions.Item label="租赁开始">{selectedRental.start_date?.slice(0, 10) || '-'}</Descriptions.Item>
            <Descriptions.Item label="租赁结束">{selectedRental.end_date?.slice(0, 10) || '-'}</Descriptions.Item>
            <Descriptions.Item label="月租金">¥{selectedRental.monthly_rent || '-'}</Descriptions.Item>
            <Descriptions.Item label="押金">¥{selectedRental.deposit || '-'}</Descriptions.Item>
            <Descriptions.Item label="状态">
              <Tag color={getStatusConfig(selectedRental.status).color}>
                {getStatusConfig(selectedRental.status).text}
              </Tag>
            </Descriptions.Item>
          </Descriptions>
        )}
      </Modal>
    </div>
  );
}