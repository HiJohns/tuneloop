import { useState, useEffect } from 'react';
import { Table, Tag, Button, Card, Typography, Space, Modal, Descriptions, Steps, message, Select } from 'antd';
import { TruckOutlined, CheckCircleOutlined, EyeOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;

export default function WarehouseManagement() {
  const [orders, setOrders] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedOrder, setSelectedOrder] = useState(null);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [statusFilter, setStatusFilter] = useState('');

  useEffect(() => {
    fetchOrders();
  }, [statusFilter]);

  const fetchOrders = async () => {
    setLoading(true);
    try {
      const params = {};
      if (statusFilter) {
        params.status = statusFilter;
      }
      const data = await api.get('/warehouse/orders', { params });
      setOrders(data?.list || []);
    } catch (error) {
      console.error('Failed to fetch orders:', error);
      message.error('获取订单列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleUpdateShipping = async (orderId, values) => {
    try {
      await api.put(`/warehouse/orders/${orderId}/shipping`, values);
      message.success('物流信息已更新');
      fetchOrders();
    } catch (error) {
      message.error('更新失败');
    }
  };

  const handleConfirmDelivery = async (orderId) => {
    try {
      await api.put(`/warehouse/orders/${orderId}/delivery`);
      message.success('收货确认成功');
      fetchOrders();
    } catch (error) {
      message.error('确认失败');
    }
  };

  const handleInspectReturn = async (orderId, passed) => {
    try {
      await api.put(`/warehouse/orders/${orderId}/return-inspect`, { passed });
      message.success(passed ? '验收通过' : '验收失败');
      fetchOrders();
    } catch (error) {
      message.error('操作失败');
    }
  };

  const handleViewDetails = async (order) => {
    try {
      const data = await api.get(`/warehouse/orders/${order.id}`);
      setSelectedOrder(data);
      setDetailModalVisible(true);
    } catch (error) {
      console.error('Failed to fetch order details:', error);
      message.error('获取详情失败');
    }
  };

  const getStatusConfig = (status) => {
    const configMap = {
      'preparing': { text: '待发货', color: 'orange' },
      'shipped': { text: '运输中', color: 'blue' },
      'delivered': { text: '已送达', color: 'green' },
      'return_requested': { text: '归还申请', color: 'orange' },
      'return_inspected': { text: '归还验收', color: 'blue' },
      'return_completed': { text: '归还完成', color: 'green' }
    };
    return configMap[status] || { text: status, color: 'default' };
  };

  const getStepStatus = (status) => {
    const stepMap = {
      'preparing': 0,
      'shipped': 1,
      'delivered': 2,
      'return_requested': 2,
      'return_inspected': 3,
      'return_completed': 4
    };
    return stepMap[status] || 0;
  };

  const columns = [
    {
      title: '订单号',
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
      title: '客户',
      dataIndex: 'user_name',
      key: 'user_name'
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
      title: '物流信息',
      dataIndex: 'tracking_number',
      key: 'tracking',
      render: (tracking, record) => (
        tracking ? (
          <span>{record.courier_company}: {tracking}</span>
        ) : '-'
      )
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
          {record.status === 'preparing' && (
            <Button 
              size="small" 
              icon={<TruckOutlined />}
              onClick={() => {
                const tracking = prompt('请输入物流单号:');
                const company = prompt('请输入快递公司:');
                if (tracking && company) {
                  handleUpdateShipping(record.id, { tracking_number: tracking, company });
                }
              }}
            >
              发货
            </Button>
          )}
          {record.status === 'shipped' && (
            <Button 
              size="small" 
              icon={<CheckCircleOutlined />}
              onClick={() => handleConfirmDelivery(record.id)}
            >
              确认收货
            </Button>
          )}
          {record.status === 'return_requested' && (
            <>
              <Button 
                size="small" 
                type="primary"
                onClick={() => handleInspectReturn(record.id, true)}
              >
                通过
              </Button>
              <Button 
                size="small" 
                danger
                onClick={() => handleInspectReturn(record.id, false)}
              >
                拒绝
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
        <div className="flex justify-between items-center mb-6">
          <Title level={2}>库管工作台</Title>
          <Space>
            <Select
              placeholder="筛选状态"
              value={statusFilter}
              onChange={setStatusFilter}
              allowClear
              style={{ width: 120 }}
              options={[
                { label: '待发货', value: 'preparing' },
                { label: '运输中', value: 'shipped' },
                { label: '已送达', value: 'delivered' },
                { label: '归还申请', value: 'return_requested' }
              ]}
            />
            <Button onClick={fetchOrders}>刷新</Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={orders}
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
        title="订单详情"
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
        {selectedOrder && (
          <div>
            <Steps
              current={getStepStatus(selectedOrder.status)}
              className="mb-6"
              items={[
                { title: '待发货' },
                { title: '运输中' },
                { title: '已送达' },
                { title: '验收' },
                { title: '完成' }
              ]}
            />

            <Descriptions bordered column={2} className="mb-4">
              <Descriptions.Item label="订单号">{selectedOrder.id?.slice(0, 8)}</Descriptions.Item>
              <Descriptions.Item label="状态">
                <Tag color={getStatusConfig(selectedOrder.status).color}>
                  {getStatusConfig(selectedOrder.status).text}
                </Tag>
              </Descriptions.Item>
              <Descriptions.Item label="乐器">{selectedOrder.instrument_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="客户">{selectedOrder.user_name || '-'}</Descriptions.Item>
              <Descriptions.Item label="快递公司">{selectedOrder.courier_company || '-'}</Descriptions.Item>
              <Descriptions.Item label="物流单号">{selectedOrder.tracking_number || '-'}</Descriptions.Item>
              {selectedOrder.shipped_at && (
                <Descriptions.Item label="发货时间">{selectedOrder.shipped_at}</Descriptions.Item>
              )}
              {selectedOrder.delivered_at && (
                <Descriptions.Item label="送达时间">{selectedOrder.delivered_at}</Descriptions.Item>
              )}
            </Descriptions>
          </div>
        )}
      </Modal>
    </div>
  );
}