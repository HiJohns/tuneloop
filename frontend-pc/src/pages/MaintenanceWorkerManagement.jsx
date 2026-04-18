import { useState, useEffect } from 'react';
import { Table, Button, Input, Space, Modal, Form, message, Popconfirm, Card, Typography, Tag } from 'antd';
import { PlusOutlined, SearchOutlined, DeleteOutlined, EyeOutlined } from '@ant-design/icons';
import { api } from '../services/api';

const { Title } = Typography;

export default function MaintenanceWorkerManagement() {
  const [workers, setWorkers] = useState([]);
  const [loading, setLoading] = useState(false);
  const [searchText, setSearchText] = useState('');
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [detailModalVisible, setDetailModalVisible] = useState(false);
  const [selectedWorker, setSelectedWorker] = useState(null);
  const [form] = Form.useForm();

  useEffect(() => {
    fetchWorkers();
  }, [searchText]);

  const fetchWorkers = async () => {
    setLoading(true);
    try {
      const params = {};
      if (searchText) {
        params.search = searchText;
      }
      
      const data = await api.get('/maintenance/workers', { params });
      setWorkers(data?.list || []);
    } catch (error) {
      console.error('Failed to fetch workers:', error);
      message.error('获取师傅列表失败');
    } finally {
      setLoading(false);
    }
  };

  const handleCreateWorker = async (values) => {
    try {
      await api.post('/maintenance/workers', values);
      message.success('创建师傅账户成功');
      form.resetFields();
      setCreateModalVisible(false);
      fetchWorkers();
    } catch (error) {
      console.error('Failed to create worker:', error);
      message.error('创建师傅账户失败');
    }
  };

  const handleDeleteWorker = async (workerId) => {
    try {
      await api.delete(`/maintenance/workers/${workerId}`);
      message.success('删除师傅账户成功');
      fetchWorkers();
    } catch (error) {
      console.error('Failed to delete worker:', error);
      message.error('删除师傅账户失败');
    }
  };

  const handleViewDetails = async (worker) => {
    try {
      const data = await api.get(`/maintenance/workers/${worker.id}`);
      setSelectedWorker(data);
      setDetailModalVisible(true);
    } catch (error) {
      console.error('Failed to fetch worker details:', error);
      message.error('获取师傅详情失败');
    }
  };

  const columns = [
    {
      title: '姓名',
      dataIndex: 'name',
      key: 'name',
      render: (text, record) => (
        <Button 
          type="link" 
          onClick={() => handleViewDetails(record)}
          style={{ padding: 0 }}
        >
          {text}
        </Button>
      ),
    },
    {
      title: '电话',
      dataIndex: 'phone',
      key: 'phone',
    },
    {
      title: '入职日期',
      dataIndex: 'hire_date',
      key: 'hire_date',
    },
    {
      title: '在手订单',
      dataIndex: 'active_orders',
      key: 'active_orders',
      render: (count) => <Tag color="blue">{count} 单</Tag>,
    },
    {
      title: '最近完成',
      dataIndex: 'completed_last_month',
      key: 'completed_last_month',
      render: (count) => <Tag color="green">{count} 单</Tag>,
    },
    {
      title: '状态',
      dataIndex: 'status',
      key: 'status',
      render: (status) => {
        const statusMap = {
          'active': { text: '在职', color: 'green' },
          'inactive': { text: '离职', color: 'red' },
        };
        const config = statusMap[status] || { text: '未知', color: 'default' };
        return <Tag color={config.color}>{config.text}</Tag>;
      },
    },
    {
      title: '操作',
      key: 'action',
      render: (_, record) => (
        <Space>
          <Button 
            type="link" 
            icon={<EyeOutlined />} 
            onClick={() => handleViewDetails(record)}
          >
            详情
          </Button>
          <Popconfirm
            title="确定删除该师傅账户吗？"
            description="删除后师傅将无法再接收新订单"
            onConfirm={() => handleDeleteWorker(record.id)}
            okText="确定"
            cancelText="取消"
          >
            <Button type="link" danger icon={<DeleteOutlined />}>
              删除
            </Button>
          </Popconfirm>
        </Space>
      ),
    },
  ];

  return (
    <div className="p-6">
      <Card>
        <div className="flex justify-between items-center mb-6">
          <Title level={2}>维修师傅管理</Title>
          <Space>
            <Input
              placeholder="搜索姓名或电话"
              prefix={<SearchOutlined />}
              value={searchText}
              onChange={(e) => setSearchText(e.target.value)}
              style={{ width: 250 }}
              allowClear
            />
            <Button 
              type="primary" 
              icon={<PlusOutlined />}
              onClick={() => setCreateModalVisible(true)}
            >
              新建师傅
            </Button>
          </Space>
        </div>

        <Table
          columns={columns}
          dataSource={workers}
          loading={loading}
          rowKey="id"
          pagination={{
            showSizeChanger: true,
            showQuickJumper: true,
            showTotal: (total) => `共 ${total} 条`,
          }}
        />
      </Card>

      {/* 创建师傅弹窗 */}
      <Modal
        title="新建维修师傅"
        open={createModalVisible}
        onCancel={() => {
          setCreateModalVisible(false);
          form.resetFields();
        }}
        footer={null}
        destroyOnClose
      >
        <Form
          form={form}
          layout="vertical"
          onFinish={handleCreateWorker}
          initialValues={{ status: 'active' }}
        >
          <Form.Item
            name="name"
            label="姓名"
            rules={[{ required: true, message: '请输入师傅姓名' }]}
          >
            <Input placeholder="请输入师傅姓名" />
          </Form.Item>

          <Form.Item
            name="phone"
            label="电话"
            rules={[
              { required: true, message: '请输入联系电话' },
              { pattern: /^1[3-9]\d{9}$/, message: '请输入有效的手机号码' }
            ]}
          >
            <Input placeholder="请输入联系电话" />
          </Form.Item>

          <Form.Item>
            <Space>
              <Button onClick={() => setCreateModalVisible(false)}>
                取消
              </Button>
              <Button type="primary" htmlType="submit">
                确定
              </Button>
            </Space>
          </Form.Item>
        </Form>
      </Modal>

      {/* 师傅详情弹窗 */}
      <Modal
        title="师傅详情"
        open={detailModalVisible}
        onCancel={() => setDetailModalVisible(false)}
        footer={null}
        width={800}
        destroyOnClose
      >
        {selectedWorker && (
          <div>
            <Card title="基本信息" className="mb-4">
              <p><strong>姓名：</strong>{selectedWorker.name}</p>
              <p><strong>电话：</strong>{selectedWorker.phone}</p>
              <p><strong>入职日期：</strong>{selectedWorker.hire_date}</p>
              <p><strong>状态：</strong>
                <Tag color={selectedWorker.status === 'active' ? 'green' : 'red'}>
                  {selectedWorker.status === 'active' ? '在职' : '离职'}
                </Tag>
              </p>
            </Card>

            <Card title="工作统计">
              <div className="grid grid-cols-2 gap-4">
                <div>
                  <p className="text-gray-500">在手订单</p>
                  <p className="text-2xl font-bold text-blue-500">
                    {selectedWorker.active_orders || 0}
                  </p>
                </div>
                <div>
                  <p className="text-gray-500">本月完成</p>
                  <p className="text-2xl font-bold text-green-500">
                    {selectedWorker.completed_last_month || 0}
                  </p>
                </div>
              </div>
            </Card>

            {selectedWorker.recent_orders && selectedWorker.recent_orders.length > 0 && (
              <Card title="最近订单" className="mt-4">
                <Table
                  dataSource={selectedWorker.recent_orders}
                  rowKey="id"
                  pagination={false}
                  size="small"
                >
                  <Table.Column title="日期" dataIndex="order_date" key="order_date" />
                  <Table.Column title="类别" dataIndex="category" key="category" />
                  <Table.Column 
                    title="状态" 
                    dataIndex="status" 
                    key="status"
                    render={(status) => {
                      const statusMap = {
                        'PENDING': { text: '待处理', color: 'orange' },
                        'PROCESSING': { text: '处理中', color: 'blue' },
                        'COMPLETED': { text: '已完成', color: 'green' },
                      };
                      const config = statusMap[status] || { text: status, color: 'default' };
                      return <Tag color={config.color}>{config.text}</Tag>;
                    }}
                  />
                </Table>
              </Card>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}