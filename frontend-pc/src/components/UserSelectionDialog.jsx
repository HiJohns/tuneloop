import { useState } from 'react';
import { Modal, Input, Button, List, message, Card, Form } from 'antd';
import { SearchOutlined, UserAddOutlined, UserOutlined } from '@ant-design/icons';
import api from '../services/api';

const UserSelectionDialog = ({ visible, onClose, onSelect, merchantId }) => {
  const [searchTerm, setSearchTerm] = useState('');
  const [searchResults, setSearchResults] = useState([]);
  const [loading, setLoading] = useState(false);
  const [selectedUser, setSelectedUser] = useState(null);
  const [createModalVisible, setCreateModalVisible] = useState(false);
  const [form] = Form.useForm();

  const handleSearch = async () => {
    if (!searchTerm.trim()) {
      message.warning('Please enter search term');
      return;
    }

    setLoading(true);
    try {
      const response = await api.get('/api/iam/users/lookup', {
        params: { identifier: searchTerm, merchant_id: merchantId },
      });

      if (response.data.code === 20000) {
        // Scenario A: User found and belongs to merchant
        setSearchResults([
          {
            ...response.data.data,
            scenario: 'A',
            display: `✓ Found user: ${response.data.data.name} (${response.data.data.email})`,
          },
        ]);
      } else if (response.data.code === 40400) {
        // Scenario C: User not found - directly open create form
        setSelectedUser({
          scenario: 'C',
          searchTerm: searchTerm,
        });
        setCreateModalVisible(true);
      } else {
        // Scenario B: User exists but not in merchant
        setSearchResults([
          {
            scenario: 'B',
            display: `⚠ User exists in platform but not in this merchant. Invite?`,
            userInfo: response.data.data,
          },
        ]);
      }
    } catch (error) {
      message.error('Search failed');
      console.error('Search error:', error);
    } finally {
      setLoading(false);
    }
  };

  const handleSelect = (user) => {
    setSelectedUser(user);
  };

  const handleConfirm = async () => {
    if (!selectedUser) {
      message.warning('Please select a user');
      return;
    }

    try {
      if (selectedUser.scenario === 'B') {
        // Invite user to merchant
        await api.post(`/api/iam/users/${selectedUser.userInfo.user_id}/invite`, {
          merchant_id: merchantId,
        });
        message.success('Invitation sent successfully');
        onSelect(selectedUser);
        handleClose();
      }
    } catch (error) {
      message.error('Operation failed');
    }
  };

  const handleCreateUser = async (values) => {
    try {
      const response = await api.post('/api/iam/users', {
        name: values.name,
        email: values.email,
        phone: values.phone,
        password: values.password,
      });

      if (response.data.code === 20000) {
        message.success('User created successfully');
        const newUser = {
          scenario: 'A',
          userInfo: {
            user_id: response.data.data.user_id,
            name: values.name,
            email: values.email,
          },
        };
        onSelect(newUser);
        handleClose();
      }
    } catch (error) {
      message.error('Failed to create user');
    } finally {
      setCreateModalVisible(false);
      form.resetFields();
    }
  };

  const handleClose = () => {
    setSearchTerm('');
    setSearchResults([]);
    setSelectedUser(null);
    onClose();
  };

  return (<> 
    <Modal
      title="选择用户"
      visible={visible}
      onOk={handleConfirm}
      onCancel={handleClose}
      width={600}
    >
      <Card size="small" style={{ marginBottom: 16 }}>
        <Input.Search
          placeholder="请输入用户名、邮箱或手机号"
          value={searchTerm}
          onChange={(e) => setSearchTerm(e.target.value)}
          onSearch={handleSearch}
          enterButton={<Button type="primary" icon={<SearchOutlined />} />}
          size="large"
        />
      </Card>

      {loading && (
        <div style={{ textAlign: 'center', padding: '20px' }}>
          <span>Searching...</span>
        </div>
      )}

      {!loading && searchResults.length > 0 && (
        <>
          <div style={{ padding: '8px 0', color: '#666', fontSize: '12px' }}>
            请点击选择用户：
          </div>
          <List
            size="small"
            bordered
            dataSource={searchResults}
            renderItem={(item) => (
              <List.Item
                style={{
                  cursor: 'pointer',
                  backgroundColor: selectedUser === item ? '#e6f7ff' : 'white',
                }}
                onClick={() => handleSelect(item)}
              >
                <List.Item.Meta
                  avatar={
                    <UserOutlined style={{ color: item.scenario === 'A' ? '#52c41a' : '#1890ff' }} />
                  }
                  title={item.display}
                  description={
                    item.scenario === 'B'
                      ? `Email: ${item.userInfo.email}, Phone: ${item.userInfo.phone}`
                      : `Member of current merchant`
                  }
                />
              </List.Item>
            )}
          />
        </>
      )}

      {!loading && searchResults.length === 0 && searchTerm && (
        <div style={{ textAlign: 'center', padding: '20px', color: '#999' }}>
          No results found. Try searching by email, phone, or username.
        </div>
      )}
    </Modal>

    <Modal
      title="创建新用户"
      visible={createModalVisible}
      onOk={() => form.submit()}
      onCancel={() => {
        setCreateModalVisible(false);
        form.resetFields();
      }}
      width={500}
    >
      <Form form={form} layout="vertical" onFinish={handleCreateUser}>
        <Form.Item
          name="name"
          label="Name"
          rules={[{ required: true, message: 'Please enter user name' }]}
        >
          <Input placeholder="请输入姓名" />
        </Form.Item>
        <Form.Item
          name="email"
          label="Email"
          rules={[
            { required: true, message: 'Please enter email' },
            { type: 'email', message: 'Please enter a valid email' },
          ]}
        >
          <Input placeholder="请输入邮箱地址" />
        </Form.Item>
        <Form.Item name="phone" label="Phone">
          <Input placeholder="请输入手机号" />
        </Form.Item>
        <Form.Item
          name="password"
          label="Password"
          rules={[{ required: true, message: 'Please enter password' }]}
        >
          <Input.Password placeholder="请输入初始密码" />
        </Form.Item>
      </Form>
    </Modal>
  </>);
};

export default UserSelectionDialog;
